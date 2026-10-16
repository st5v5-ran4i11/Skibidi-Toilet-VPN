package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/config"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/core/engine"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/ipc"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/logger"
	"go.uber.org/zap"
)

func main() {
	rootFlag := flag.String("root", "", "project root (defaults to executable directory or parent directory when launched from cmd/vpn-backend)")
	flag.Parse()
	root := *rootFlag
	if root == "" {
		exe, err := os.Executable()
		if err != nil {
			fatal(err)
		}
		root = filepath.Dir(exe)
		if _, err = os.Stat(filepath.Join(root, "configs", "settings.yaml")); err != nil {
			root = filepath.Clean(filepath.Join(root, "..", ".."))
		}
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		fatal(err)
	}
	root = abs
	for _, d := range []string{"data/cache", "data/history", "output/subscriptions", "logs"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0755); err != nil {
			fatal(err)
		}
	}
	if err := logger.Init(filepath.Join(root, "logs", "application.log")); err != nil {
		fatal(err)
	}
	defer logger.Close()
	cfg, err := config.Load(filepath.Join(root, "configs", "settings.yaml"))
	if err != nil {
		fatal(err)
	}
	if err := cfg.LoadRuntimeFiles(root); err != nil {
		fatal(err)
	}
	eng := engine.New(cfg, root)
	if err := eng.ValidateRuntime(); err != nil {
		fatal(err)
	}
	server := ipc.New(root, cfg, eng)
	if err := server.Start(); err != nil {
		fatal(err)
	}
	endpoint := server.Endpoint()
	fmt.Printf("IPC_READY %s\n", filepath.Join(root, "data", "ipc.json"))
	_ = endpoint
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	eng.Stop()
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Close(shutdown); err != nil && logger.Log != nil {
		logger.Log.Warn("IPC shutdown failed", zap.Error(err))
	}
}
func fatal(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
