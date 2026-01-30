package models

import "time"

type ConversionJob struct {
	ConversionID    int       `json:"conversionId"`
	FileID          int       `json:"fileId"`
	FileGUID        string    `json:"fileGuid"`
	UserID          int       `json:"userId"`
	InputS3Path     string    `json:"inputS3Path"`
	OutputS3Path    string    `json:"outputS3Path"`
	InputExtension  string    `json:"inputExtension"`
	RetryCount      int       `json:"retryCount"`
	MaxRetries      int       `json:"maxRetries"`
	CreatedAt       time.Time `json:"createdAt"`
	Timeout         int       `json:"timeout"`
}
