package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"converter/config"
	"converter/services"
	"converter/worker"

	"github.com/redis/go-redis/v9"
)

func main() {
	log.Println("Starting PaperPulse Conversion Service...")

	// Load configuration
	cfg := config.Load()

	// Initialize Redis client
	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})

	// Test Redis connection
	ctx := context.Background()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	log.Println("Connected to Redis successfully")

	// Initialize database service
	dbSvc, err := services.NewDatabaseService(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer dbSvc.Close()
	log.Println("Connected to database successfully")

	// Create worker pool
	pool := worker.NewPool(cfg, redisClient, dbSvc)

	// Start workers
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())

	for i := 0; i < cfg.WorkerCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			pool.StartWorker(ctx, workerID)
		}(i)
		log.Printf("Started worker %d", i)
	}

	// Start stale job recovery goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		pool.RecoveryLoop(ctx)
	}()

	log.Printf("Started %d conversion workers", cfg.WorkerCount)
	log.Printf("Listening on Redis queue: %s", cfg.PendingQueue)
	log.Printf("Gotenberg URL: %s", cfg.GotenbergURL)
	log.Println("Service is ready to process conversions")

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutdown signal received, stopping workers...")
	cancel()

	// Wait for workers to finish with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("All workers stopped gracefully")
	case <-time.After(30 * time.Second):
		log.Println("Shutdown timeout, forcing exit")
	}

	redisClient.Close()
	log.Println("Conversion service stopped")
}
