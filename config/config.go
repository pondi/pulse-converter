package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	RedisAddr         string
	RedisPassword     string
	RedisDB           int
	RedisPrefix       string
	PendingQueue      string
	ProcessingQueue   string
	FailedQueue       string
	WorkerCount       int
	GotenbergURL      string
	S3Bucket          string
	S3Region          string
	AWSS3AccessKey    string
	AWSS3SecretKey    string
	S3Endpoint        string
	S3UsePathStyle    bool
	DatabaseURL       string
	ConversionTimeout int
	MaxRetries        int
}

func Load() *Config {
	redisPrefix := getEnv("REDIS_PREFIX", "")
	dbHost := getEnv("DB_HOST", "localhost")
	dbPort := getEnv("DB_PORT", "5432")
	dbName := getEnv("DB_DATABASE", "paperpulse")
	dbUser := getEnv("DB_USERNAME", "paperpulse")
	dbPassword := getEnv("DB_PASSWORD", "")
	dbSSLMode := getEnv("DB_SSLMODE", "disable")
	dbSSLCert := getEnv("DB_SSLCERT", "")
	dbSSLKey := getEnv("DB_SSLKEY", "")
	dbSSLRootCert := getEnv("DB_SSLROOTCERT", "")

	// lib/pq supports "key=value" connection strings and this avoids
	// URI escaping issues for special characters in passwords.
	// Build connection string with optional SSL certificate parameters
	var dbURL string
	if dbPassword != "" {
		dbURL = fmt.Sprintf(
			"host=%s port=%s dbname=%s user=%s password=%s sslmode=%s",
			dbHost, dbPort, dbName, dbUser, dbPassword, dbSSLMode,
		)
	} else {
		dbURL = fmt.Sprintf(
			"host=%s port=%s dbname=%s user=%s sslmode=%s",
			dbHost, dbPort, dbName, dbUser, dbSSLMode,
		)
	}

	// Append SSL certificate paths if provided
	if dbSSLCert != "" {
		dbURL += fmt.Sprintf(" sslcert=%s", dbSSLCert)
	}
	if dbSSLKey != "" {
		dbURL += fmt.Sprintf(" sslkey=%s", dbSSLKey)
	}
	if dbSSLRootCert != "" {
		dbURL += fmt.Sprintf(" sslrootcert=%s", dbSSLRootCert)
	}

	return &Config{
		RedisAddr:     getEnv("REDIS_ADDR", "redis:6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),
		RedisDB:       getEnvInt("REDIS_CONVERSION_DB", 3),
		RedisPrefix:   redisPrefix,
		PendingQueue:  applyPrefix(getEnv("CONVERSION_PENDING_QUEUE", "conversion:pending"), redisPrefix),
		ProcessingQueue: applyPrefix(
			getEnv("CONVERSION_PROCESSING_QUEUE", "conversion:processing"),
			redisPrefix,
		),
		FailedQueue: applyPrefix(
			getEnv("CONVERSION_FAILED_QUEUE", "conversion:failed"),
			redisPrefix,
		),
		WorkerCount:       getEnvInt("CONVERSION_WORKER_COUNT", 3),
		GotenbergURL:      getEnv("GOTENBERG_URL", "http://gotenberg:3000"),
		S3Bucket:          getEnv("AWS_BUCKET", "paperpulse"),
		// Prefer unified S3_* vars, fall back to legacy AWS_* vars for compatibility
		S3Region:          getEnvWithFallback("S3_REGION", "AWS_DEFAULT_REGION", "us-east-1"),
		AWSS3AccessKey:    getEnvWithFallback("S3_KEY", "AWS_ACCESS_KEY_ID", ""),
		AWSS3SecretKey:    getEnvWithFallback("S3_SECRET", "AWS_SECRET_ACCESS_KEY", ""),
		S3Endpoint:        getEnv("S3_ENDPOINT", ""),
		S3UsePathStyle:    getEnvBool("S3_USE_PATH_STYLE_ENDPOINT", false),
		DatabaseURL:       dbURL,
		ConversionTimeout: getEnvInt("CONVERSION_TIMEOUT", 120),
		MaxRetries:        getEnvInt("CONVERSION_MAX_RETRIES", 3),
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getEnvWithFallback(primaryKey, secondaryKey, fallback string) string {
	if value := os.Getenv(primaryKey); value != "" {
		return value
	}
	if value := os.Getenv(secondaryKey); value != "" {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return fallback
}

func applyPrefix(key string, prefix string) string {
	if prefix == "" {
		return key
	}
	return prefix + key
}

func getEnvBool(key string, fallback bool) bool {
	if value := os.Getenv(key); value != "" {
		switch strings.ToLower(value) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	return fallback
}
