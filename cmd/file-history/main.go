package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/unok/local-text-history/internal/config"
	"github.com/unok/local-text-history/internal/db"
	"github.com/unok/local-text-history/internal/server"
	"github.com/unok/local-text-history/internal/watcher"
	"github.com/unok/local-text-history/web"
)

func main() {
	configPath := flag.String("config", "", "path to config file")
	flag.Parse()

	if *configPath == "" {
		fmt.Fprintln(os.Stderr, "error: --config flag is required")
		flag.Usage()
		os.Exit(1)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// Ensure DB directory exists
	dbDir := filepath.Dir(cfg.DBPath)
	if err := os.MkdirAll(dbDir, 0o700); err != nil {
		log.Fatalf("failed to create db directory: %v", err)
	}

	database, err := db.New(cfg.DBPath, cfg.MaxSnapshots)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// Set up static file system
	var staticFS fs.FS
	sub, err := fs.Sub(web.DistFS, "dist")
	if err != nil {
		log.Printf("warning: static files not available: %v", err)
	} else {
		staticFS = sub
	}

	// Set up watcher
	watchCfg := watcher.Config{
		WatchDirs:       cfg.WatchDirs,
		Extensions:      cfg.Extensions,
		ExcludePatterns: cfg.ExcludePatterns,
		DebounceSec:     cfg.DebounceSec,
		MaxFileSize:     cfg.MaxFileSize,
	}
	w, err := watcher.New(watchCfg, database.SaveSnapshot)
	if err != nil {
		log.Fatalf("failed to create watcher: %v", err)
	}

	// Wire rename detection
	w.SetRenameSaver(database.SaveRename)

	// Set up HTTP server
	srv := server.New(database, staticFS, cfg.WatchDirs)

	// Wire watcher snapshot notifications to SSE
	w.OnSnapshot = func(filePath string) {
		srv.Notify(filePath)
	}

	// Wire rename notifications to SSE
	w.OnRename = func(oldPath, newPath string) {
		srv.Notify(newPath)
	}

	httpServer := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", cfg.BindAddress, cfg.Port),
		Handler: srv.Handler(),
	}

	// Graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	done := make(chan struct{})
	go w.Run(done)

	go func() {
		log.Printf("server starting on http://%s:%d", cfg.BindAddress, cfg.Port)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down...")

	close(done)
	if err := w.Close(); err != nil {
		log.Printf("error closing watcher: %v", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("error shutting down server: %v", err)
	}

	log.Println("shutdown complete")
}
