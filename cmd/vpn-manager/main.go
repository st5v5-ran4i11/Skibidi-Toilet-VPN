package main

import (
	"fmt"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/config"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/core/engine"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/gui"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/logger"
	"os"
	"path/filepath"
)

func main() {
	root, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	for _, d := range []string{"data/cache", "data/history", "output/subscriptions", "logs"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0755); err != nil {
			panic(err)
		}
	}
	cfg, err := config.Load(filepath.Join(root, "configs", "settings.yaml"))
	if err != nil {
		panic(err)
	}
	if err := logger.Init(filepath.Join(root, "logs", "application.log")); err != nil {
		panic(err)
	}
	defer logger.Close()
	eng := engine.New(cfg, root)
	if err := eng.ValidateRuntime(); err != nil {
		panic(fmt.Sprintf("runtime validation failed: %v", err))
	}
	app := gui.New(cfg, eng)
	app.ShowAndRun()
}
