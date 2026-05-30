package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"xboard_link_3x-ui/internal/config"
	"xboard_link_3x-ui/internal/state"
	"xboard_link_3x-ui/internal/syncer"
	"xboard_link_3x-ui/internal/xboard"
	"xboard_link_3x-ui/internal/xui"
)

func main() {
	configPath := flag.String("config", "config.json", "path to config json")
	once := flag.Bool("once", false, "run one sync and exit")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	store, err := state.Load(cfg.Sync.StateFile)
	if err != nil {
		log.Fatalf("load state: %v", err)
	}

	xb, err := xboard.NewClient(cfg.Xboard)
	if err != nil {
		log.Fatalf("create xboard client: %v", err)
	}

	xu, err := xui.NewClient(cfg.XUI)
	if err != nil {
		log.Fatalf("create 3x-ui client: %v", err)
	}

	service := syncer.New(cfg, xb, xu, store)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if *once {
		if err := service.RunOnce(ctx, true); err != nil {
			log.Fatalf("sync once: %v", err)
		}
		return
	}

	if err := service.Run(ctx); err != nil {
		log.Fatalf("run: %v", err)
	}
}
