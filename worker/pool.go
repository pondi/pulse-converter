package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"time"

	"converter/config"
	"converter/models"
	"converter/services"

	"github.com/redis/go-redis/v9"
)

type Pool struct {
	config       *config.Config
	redisClient  *redis.Client
	gotenbergSvc *services.GotenbergService
	s3Svc        *services.S3Service
	dbSvc        *services.DatabaseService
}

func NewPool(cfg *config.Config, redisClient *redis.Client, dbSvc *services.DatabaseService) *Pool {
	return &Pool{
		config:       cfg,
		redisClient:  redisClient,
		gotenbergSvc: services.NewGotenbergService(cfg.GotenbergURL),
		s3Svc:        services.NewS3Service(cfg),
		dbSvc:        dbSvc,
	}
}

func (p *Pool) StartWorker(ctx context.Context, workerID int) {
	log.Printf("[Worker %d] Starting", workerID)

	for {
		select {
		case <-ctx.Done():
			log.Printf("[Worker %d] Shutting down", workerID)
			return
		default:
			// Atomic pop from pending and push to processing
			result, err := p.redisClient.BRPopLPush(
				ctx,
				p.config.PendingQueue,
				p.config.ProcessingQueue,
				30*time.Second,
			).Result()

			if err == redis.Nil {
				// Timeout, no jobs available
				continue
			}

			if err != nil {
				log.Printf("[Worker %d] Redis error: %v", workerID, err)
				time.Sleep(5 * time.Second)
				continue
			}

			// Parse job
			var job models.ConversionJob
			if err := json.Unmarshal([]byte(result), &job); err != nil {
				log.Printf("[Worker %d] Failed to parse job: %v", workerID, err)
				// Remove malformed job from processing queue
				p.redisClient.LRem(ctx, p.config.ProcessingQueue, 1, result)
				continue
			}

			// Process job
			p.processJob(ctx, workerID, &job, result)
		}
	}
}

func (p *Pool) processJob(ctx context.Context, workerID int, job *models.ConversionJob, jobJSON string) {
	log.Printf("[Worker %d] Processing conversion %d (file: %s)", workerID, job.ConversionID, job.FileGUID)

	// Update DB status to processing
	if err := p.dbSvc.UpdateConversionStatus(ctx, job.ConversionID, "processing", "", nil); err != nil {
		log.Printf("[Worker %d] Failed to update DB status: %v", workerID, err)
	}

	// Create timeout context
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(job.Timeout)*time.Second)
	defer cancel()

	// Track start time
	startTime := time.Now()

	// Download from S3
	localInputPath, err := p.s3Svc.Download(timeoutCtx, job.InputS3Path, job.FileGUID, job.InputExtension)
	if err != nil {
		p.handleJobFailure(ctx, workerID, job, jobJSON, fmt.Sprintf("S3 download failed: %v", err))
		return
	}
	defer p.s3Svc.Cleanup(localInputPath)

	// Convert to PDF/A using LibreOffice endpoint (office files only)
	localOutputPath, err := p.gotenbergSvc.ConvertToPDFA(timeoutCtx, localInputPath, job.InputExtension)
	if err != nil {
		p.handleJobFailure(ctx, workerID, job, jobJSON, fmt.Sprintf("Office conversion failed: %v", err))
		return
	}
	defer p.s3Svc.Cleanup(localOutputPath)

	// Upload PDF to S3
	if err := p.s3Svc.Upload(timeoutCtx, localOutputPath, job.OutputS3Path); err != nil {
		p.handleJobFailure(ctx, workerID, job, jobJSON, fmt.Sprintf("S3 upload failed: %v", err))
		return
	}

	// Success - update DB and remove from processing queue
	duration := time.Since(startTime)
	metadata := map[string]interface{}{
		"worker_id":   workerID,
		"duration_ms": duration.Milliseconds(),
	}

	if err := p.dbSvc.UpdateConversionStatus(ctx, job.ConversionID, "completed", job.OutputS3Path, metadata); err != nil {
		log.Printf("[Worker %d] Failed to update DB to completed: %v", workerID, err)
	}

	// Update Redis status hash
	p.redisClient.HSet(ctx, fmt.Sprintf("conversion:status:%d", job.ConversionID), map[string]interface{}{
		"status":     "completed",
		"updated_at": time.Now().Format(time.RFC3339),
	})

	// Remove from processing queue
	p.redisClient.LRem(ctx, p.config.ProcessingQueue, 1, jobJSON)

	log.Printf("[Worker %d] Conversion %d completed successfully (%.2fs)", workerID, job.ConversionID, duration.Seconds())
}

func (p *Pool) handleJobFailure(ctx context.Context, workerID int, job *models.ConversionJob, jobJSON string, errorMsg string) {
	log.Printf("[Worker %d] Conversion %d failed: %s", workerID, job.ConversionID, errorMsg)

	// Remove from processing queue
	p.redisClient.LRem(ctx, p.config.ProcessingQueue, 1, jobJSON)

	// Increment retry count in DB
	p.dbSvc.IncrementRetryCount(ctx, job.ConversionID)

	// Check if we should retry
	if job.RetryCount < job.MaxRetries {
		job.RetryCount++
		newJobJSON, _ := json.Marshal(job)

		// Calculate exponential backoff delay
		delay := time.Duration(math.Pow(2, float64(job.RetryCount))) * time.Second
		if delay > 30*time.Second {
			delay = 30 * time.Second
		}

		// Schedule retry with delay
		time.AfterFunc(delay, func() {
			p.redisClient.LPush(context.Background(), p.config.PendingQueue, newJobJSON)
			log.Printf("[Worker %d] Scheduled retry %d/%d for conversion %d in %v",
				workerID, job.RetryCount, job.MaxRetries, job.ConversionID, delay)
		})
	} else {
		// Max retries reached - move to failed queue
		p.redisClient.LPush(ctx, p.config.FailedQueue, jobJSON)

		// Update DB status
		p.dbSvc.UpdateConversionStatus(ctx, job.ConversionID, "failed", "", nil)
		p.dbSvc.UpdateConversionError(ctx, job.ConversionID, errorMsg)

		// Update Redis status
		p.redisClient.HSet(ctx, fmt.Sprintf("conversion:status:%d", job.ConversionID), map[string]interface{}{
			"status":     "failed",
			"error":      errorMsg,
			"updated_at": time.Now().Format(time.RFC3339),
		})

		log.Printf("[Worker %d] Conversion %d moved to failed queue after %d retries",
			workerID, job.ConversionID, job.MaxRetries)
	}
}

func (p *Pool) RecoveryLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	log.Println("[Recovery] Starting stale job recovery loop")

	for {
		select {
		case <-ctx.Done():
			log.Println("[Recovery] Shutting down")
			return
		case <-ticker.C:
			p.recoverStaleJobs(ctx)
		}
	}
}

func (p *Pool) recoverStaleJobs(ctx context.Context) {
	// Get all jobs in processing queue
	jobs, err := p.redisClient.LRange(ctx, p.config.ProcessingQueue, 0, -1).Result()
	if err != nil {
		log.Printf("[Recovery] Failed to get processing queue: %v", err)
		return
	}

	recovered := 0
	for _, jobJSON := range jobs {
		var job models.ConversionJob
		if err := json.Unmarshal([]byte(jobJSON), &job); err != nil {
			continue
		}

		// Check if job is stale (> 5 minutes in processing)
		if time.Since(job.CreatedAt) > 5*time.Minute {
			// Remove from processing
			p.redisClient.LRem(ctx, p.config.ProcessingQueue, 1, jobJSON)

			// Retry or fail
			if job.RetryCount < job.MaxRetries {
				job.RetryCount++
				newJobJSON, _ := json.Marshal(job)
				p.redisClient.LPush(ctx, p.config.PendingQueue, newJobJSON)
				p.dbSvc.IncrementRetryCount(ctx, job.ConversionID)
				recovered++
			} else {
				p.redisClient.LPush(ctx, p.config.FailedQueue, jobJSON)
				p.dbSvc.UpdateConversionStatus(ctx, job.ConversionID, "failed", "", nil)
				p.dbSvc.UpdateConversionError(ctx, job.ConversionID, "Job timeout - exceeded 5 minutes")
			}
		}
	}

	if recovered > 0 {
		log.Printf("[Recovery] Recovered %d stale jobs", recovered)
	}
}
