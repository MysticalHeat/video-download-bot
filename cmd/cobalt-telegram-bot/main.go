package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"cobalt-telegram-bot/internal/app"
	"cobalt-telegram-bot/internal/config"
)

func main() {
	logger := log.New(os.Stdout, "", log.LstdFlags|log.LUTC|log.Lmsgprefix)

	cfg, err := config.Load()
	if err != nil {
		logger.Fatalf("config error: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	application, err := app.New(cfg, logger)
	if err != nil {
		logger.Fatalf("app init error: %v", err)
	}

	if err := application.Run(ctx); err != nil {
		logger.Fatalf("runtime error: %v", err)
	}
	logger.Println("bot stopped")
}
