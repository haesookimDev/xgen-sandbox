package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	v1 "github.com/xgen-sandbox/agent/api/v1"
	"github.com/xgen-sandbox/agent/internal/auth"
	"github.com/xgen-sandbox/agent/internal/config"
	k8spkg "github.com/xgen-sandbox/agent/internal/k8s"
	"github.com/xgen-sandbox/agent/internal/proxy"
	"github.com/xgen-sandbox/agent/internal/sandbox"
	"github.com/xgen-sandbox/agent/internal/server"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("xgen-sandbox agent starting...")

	cfg := config.Load()

	authenticator := auth.NewAuthenticator(cfg.APIKey, cfg.JWTSecret)
	sandboxMgr := sandbox.NewManager()
	wsProxy := proxy.NewWSProxy(sandboxMgr)
	previewRouter := proxy.NewRouter(cfg.PreviewDomain, sandboxMgr)

	// Initialize K8s pod manager
	// Use a pointer so the callback closure can reference it after initialization
	var podMgr *k8spkg.PodManager
	var initErr error
	podMgr, initErr = k8spkg.NewPodManager(
		cfg.SandboxNamespace,
		cfg.SidecarImage,
		cfg.RuntimeBaseImage,
		func(sandboxID string) {
			// Called when pod becomes ready
			log.Printf("sandbox %s is ready", sandboxID)
			sandboxMgr.SetStatus(sandboxID, v1.StatusRunning)

			// Update pod IP and connect WebSocket
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	podMgr.StartWatcher(ctx)

	// Start sandbox expiry checker
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				for _, id := range sandboxMgr.GetExpired() {
					log.Printf("sandbox %s expired, cleaning up", id)
					sandboxMgr.SetStatus(id, v1.StatusStopping)
					if err := podMgr.DeletePod(ctx, id); err != nil {
						log.Printf("delete expired pod %s: %v", id, err)
					}
					wsProxy.DisconnectSidecar(id)
					sandboxMgr.Remove(id)
				}
			}
		}
	}()

	srv := server.NewServer(cfg, authenticator, sandboxMgr, podMgr, wsProxy, previewRouter)

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
