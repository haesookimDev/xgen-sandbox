package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	v1 "github.com/xgen-sandbox/agent/api/v1"
	"github.com/xgen-sandbox/agent/internal/audit"
	"github.com/xgen-sandbox/agent/internal/auth"
	"github.com/xgen-sandbox/agent/internal/config"
	k8spkg "github.com/xgen-sandbox/agent/internal/k8s"
	"github.com/xgen-sandbox/agent/internal/proxy"
	"github.com/xgen-sandbox/agent/internal/sandbox"
	"github.com/xgen-sandbox/agent/internal/server"
)

func main() {
	// Initialize structured logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("xgen-sandbox agent starting...")

	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid configuration: %v", err)
	}

	authenticator := auth.NewAuthenticator(cfg.APIKey, cfg.JWTSecret)
	sandboxMgr := sandbox.NewManager()
	wsProxy := proxy.NewWSProxy(sandboxMgr)
	previewRouter := proxy.NewRouter(cfg.PreviewDomain, sandboxMgr)

	// Initialize K8s pod manager
	var podMgr *k8spkg.PodManager
	var warmPool *k8spkg.WarmPool
	var initErr error
	podMgr, initErr = k8spkg.NewPodManager(
		cfg.SandboxNamespace,
		cfg.SidecarImage,
		cfg.RuntimeBaseImage,
		cfg.ImagePullPolicy,
		func(sandboxID string) {
			// Check if this is a warm pool pod
			if warmPool != nil && warmPool.IsWarm(sandboxID) {
				// Don't set sandbox status; just mark ready in warm pool
				return
			}
			if warmPool != nil && strings.HasPrefix(sandboxID, "warm-") {
				// Warm pod became ready, add to pool
				if info, ok := podMgr.GetPodInfo(sandboxID); ok {
					// Connect sidecar WS for warm pod
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					if err := wsProxy.ConnectToSidecar(ctx, sandboxID, info.PodIP); err != nil {
						log.Printf("warm pool: connect to sidecar %s: %v", sandboxID, err)
						return
					}
					// Extract template from pod labels via pod info
					warmPool.MarkReady(sandboxID, "base")
					log.Printf("warm pool: pod %s is ready", sandboxID)
				}
				return
			}

			// Normal sandbox pod ready
			log.Printf("sandbox %s is ready", sandboxID)
			sandboxMgr.SetStatus(sandboxID, v1.StatusRunning)

			if info, ok := podMgr.GetPodInfo(sandboxID); ok {
				sandboxMgr.SetPodIP(sandboxID, info.PodIP)
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if err := wsProxy.ConnectToSidecar(ctx, sandboxID, info.PodIP); err != nil {
					log.Printf("connect to sidecar %s: %v", sandboxID, err)
				}
			}
		},
	)
	if initErr != nil {
		log.Fatalf("init pod manager: %v", initErr)
	}

	warmPool = k8spkg.NewWarmPool(podMgr, cfg.WarmPoolSize, cfg.WarmPoolSizes)
	auditStore := audit.NewStore(1000)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Recover sandboxes from existing K8s pods (survives agent restart)
	recovered, recoverErr := podMgr.RecoverExistingPods(ctx)
	if recoverErr != nil {
		log.Printf("warning: sandbox recovery failed: %v", recoverErr)
	} else if len(recovered) > 0 {
		for _, rs := range recovered {
			// expiresAt falls back to now + DefaultTimeout only when the
			// pod predates the annotation scheme (or the annotation was
			// malformed). With annotations present the original expiry is
			// honored across restarts.
			expiresAt := rs.ExpiresAt
			if expiresAt.IsZero() {
				expiresAt = time.Now().Add(cfg.DefaultTimeout)
			}
			sandboxMgr.Recover(
				rs.SandboxID, rs.Template, rs.PodIP,
				rs.Ports, rs.GUI, rs.Env, rs.Metadata,
				rs.Capabilities, rs.CreatedAt, expiresAt,
				rs.Ready,
			)
			if rs.Ready && rs.PodIP != "" {
				rctx, rcancel := context.WithTimeout(ctx, 10*time.Second)
				if err := wsProxy.ConnectToSidecar(rctx, rs.SandboxID, rs.PodIP); err != nil {
					log.Printf("recover: connect to sidecar %s: %v", rs.SandboxID, err)
				}
				rcancel()
			}
		}
		log.Printf("recovered %d sandbox(es) from existing pods", len(recovered))
	}

	podMgr.StartWatcher(ctx)

	// Start sandbox expiry checker
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		pendingDeletes := make(map[string]time.Time)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Retry force-delete for pods stuck past grace period
				for id, deletedAt := range pendingDeletes {
					if time.Since(deletedAt) > 30*time.Second {
						log.Printf("sandbox %s still pending after 30s, force deleting", id)
						if err := podMgr.ForceDeletePod(ctx, id); err != nil {
							log.Printf("force delete pod %s: %v", id, err)
						}
						delete(pendingDeletes, id)
					}
				}

				for _, id := range sandboxMgr.GetExpired() {
					log.Printf("sandbox %s expired, cleaning up", id)
					sandboxMgr.SetStatus(id, v1.StatusStopping)
					if err := podMgr.DeletePod(ctx, id); err != nil {
						log.Printf("delete expired pod %s: %v", id, err)
						pendingDeletes[id] = time.Now()
					}
					wsProxy.DisconnectSidecar(id)
					sandboxMgr.Remove(id)
				}
			}
		}
	}()

	// Start warm pool
	warmPool.Start(ctx)

	srv := server.NewServer(cfg, logger, authenticator, sandboxMgr, podMgr, warmPool, wsProxy, previewRouter, auditStore)

	httpServer := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: srv.Handler(),
	}

	go func() {
		log.Printf("agent listening on %s", cfg.ListenAddr)
		if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("http server error: %v", err)
		}
	}()

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	<-sigCh

	log.Println("shutting down agent...")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	httpServer.Shutdown(shutdownCtx)

	log.Println("agent stopped")
}
