package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/vhysxl/relay/bootstrap/app"
	"github.com/vhysxl/relay/bootstrap/server"
	"golang.org/x/sync/errgroup"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	if err := godotenv.Load(); err != nil {
		log.Println("Note: .env file not found, using env or defaults")
	}

	application, err := app.InitApp()
	if err != nil {
		return fmt.Errorf("init app: %w", err)
	}
	defer application.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		log.Println("Step reaper started")
		application.Reaper.Start(ctx)
		return nil
	})

	g.Go(func() error {
		return server.Listen(ctx, application)
	})

	return g.Wait()
}
