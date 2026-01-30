package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/lib/pq"
)

type DatabaseService struct {
	db *sql.DB
}

func NewDatabaseService(databaseURL string) (*DatabaseService, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &DatabaseService{db: db}, nil
}

func (d *DatabaseService) UpdateConversionStatus(ctx context.Context, conversionID int, status string, outputPath string, metadata map[string]interface{}) error {
	query := `UPDATE file_conversions SET status = $1, updated_at = $2`
	args := []interface{}{status, time.Now()}
	argIndex := 3

	if status == "processing" {
		query += fmt.Sprintf(`, started_at = $%d`, argIndex)
		args = append(args, time.Now())
		argIndex++
	}

	if status == "completed" {
		query += fmt.Sprintf(`, completed_at = $%d, output_s3_path = $%d`, argIndex, argIndex+1)
		args = append(args, time.Now(), outputPath)
		argIndex += 2

		if metadata != nil {
			metadataJSON, _ := json.Marshal(metadata)
			query += fmt.Sprintf(`, metadata = $%d`, argIndex)
			args = append(args, metadataJSON)
			argIndex++
		}
	}

	query += fmt.Sprintf(` WHERE id = $%d`, argIndex)
	args = append(args, conversionID)

	_, err := d.db.ExecContext(ctx, query, args...)
	return err
}

func (d *DatabaseService) UpdateConversionError(ctx context.Context, conversionID int, errorMsg string) error {
	query := `UPDATE file_conversions SET error_message = $1, updated_at = $2 WHERE id = $3`
	_, err := d.db.ExecContext(ctx, query, errorMsg, time.Now(), conversionID)
	return err
}

func (d *DatabaseService) IncrementRetryCount(ctx context.Context, conversionID int) error {
	query := `UPDATE file_conversions SET retry_count = retry_count + 1, updated_at = $1 WHERE id = $2`
	_, err := d.db.ExecContext(ctx, query, time.Now(), conversionID)
	return err
}

func (d *DatabaseService) Close() error {
	return d.db.Close()
}
