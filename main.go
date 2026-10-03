package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/joho/godotenv"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/vhysxl/relay/bootstrap/config"
	"github.com/vhysxl/relay/bootstrap/server"
	"github.com/vhysxl/relay/internal/executor"
	"github.com/vhysxl/relay/internal/step"
	"github.com/vhysxl/relay/internal/worker"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Println("Note: .env file not found, using environment variables")
	}

	conf, err := config.Init()
	if err != nil {
		panic(err)
	}
	if conf.DB != nil {
		defer conf.DB.Close()
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// 1. Setup Executor Registry
	reg := executor.NewRegistry()
	reg.Register(step.StepTypeHTTP, executor.NewHTTPExecutor(nil))

	if rmqURL := os.Getenv("RABBITMQ_URL"); rmqURL != "" {
		rmqConn, err := amqp.Dial(rmqURL)
		if err != nil {
			log.Printf("Warning: failed to connect to RabbitMQ at %s: %v", rmqURL, err)
		} else {
			defer rmqConn.Close()
			rmqCh, err := rmqConn.Channel()
			if err != nil {
				log.Printf("Warning: failed to open RabbitMQ channel: %v", err)
			} else {
				defer rmqCh.Close()
				reg.Register(step.StepTypeMessage, executor.NewRabbitMQExecutor(rmqCh))
				log.Println("RabbitMQ message executor registered successfully")
			}
		}
	}

	// 2. Setup Worker Pool (only if database is connected)
	if conf.DB != nil {
		stepRepo := step.NewStepRepository(conf.DB)
		workerLogic := worker.NewWorkerLogic(stepRepo)

		workerCount := worker.DefaultWorkerCount
		if countStr := os.Getenv("WORKER_COUNT"); countStr != "" {
			if c, err := strconv.Atoi(countStr); err == nil && c > 0 {
				workerCount = c
			}
		}

		pool := worker.NewWorkerPool(
			workerLogic,
			reg,
			worker.WithWorkerCount(workerCount),
		)

		// 3. Start Worker Pool in background goroutine
		go func() {
			log.Printf("Worker pool started with %d workers\n", workerCount)
			if err := pool.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("Worker pool exited with error: %v\n", err)
			}
		}()
	} else {
		log.Println("Note: Database not connected. Worker pool paused (frontend dev mode active)")
	}

	// 4. Start HTTP Server (blocks until shutdown or error)
	if err := server.Listen(ctx, conf.Port, conf.DB); err != nil {
		log.Fatal(err)
	}
}
