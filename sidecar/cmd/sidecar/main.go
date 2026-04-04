package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	execpkg "github.com/xgen-sandbox/sidecar/internal/exec"
	fspkg "github.com/xgen-sandbox/sidecar/internal/fs"
	ws "github.com/xgen-sandbox/sidecar/internal/ws"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("xgen-sandbox sidecar starting...")

	workspaceRoot := os.Getenv("WORKSPACE_ROOT")
	if workspaceRoot == "" {
		workspaceRoot = "/home/sandbox/workspace"
	}

	listenAddr := os.Getenv("SIDECAR_LISTEN_ADDR")
	if listenAddr == "" {
		listenAddr = ":9000"
	}

	healthAddr := os.Getenv("SIDECAR_HEALTH_ADDR")
	if healthAddr == "" {
		healthAddr = ":9001"
	}

	execMgr := execpkg.NewManager()
	fsHandler := fspkg.NewHandler(workspaceRoot)
	server := ws.NewServer(execMgr, fsHandler)

	// Main WebSocket server
	mux := http.NewServeMux()
	mux.Handle("/ws", server.Handler())

	httpServer := &http.Server{
		Addr:    listenAddr,
		Handler: mux,
	}

	// Health check server
	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	healthMux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	healthServer := &http.Server{
		Addr:    healthAddr,
		Handler: healthMux,
	}

	// Start servers
	go func() {
		log.Printf("health server listening on %s", healthAddr)
		if err := healthServer.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("health server error: %v", err)
		}
	}()

	go func() {
		log.Printf("websocket server listening on %s", listenAddr)
		if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("http server error: %v", err)
		}
	}()

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	<-sigCh

	log.Println("shutting down...")
	execMgr.KillAll()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	httpServer.Shutdown(ctx)
	healthServer.Shutdown(ctx)
	log.Println("sidecar stopped")
}
