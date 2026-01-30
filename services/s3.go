package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"converter/config"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

type S3Service struct {
	session    *session.Session
	bucket     string
	downloader *s3manager.Downloader
	uploader   *s3manager.Uploader
}

func NewS3Service(cfg *config.Config) *S3Service {
	awsCfg := &aws.Config{
		Region: aws.String(cfg.S3Region),
		Credentials: credentials.NewStaticCredentials(
			cfg.AWSS3AccessKey,
			cfg.AWSS3SecretKey,
			"",
		),
	}

	if cfg.S3Endpoint != "" {
		awsCfg.Endpoint = aws.String(cfg.S3Endpoint)
	}

	if cfg.S3UsePathStyle {
		awsCfg.S3ForcePathStyle = aws.Bool(true)
	}

	sess := session.Must(session.NewSession(awsCfg))

	return &S3Service{
		session:    sess,
		bucket:     cfg.S3Bucket,
		downloader: s3manager.NewDownloader(sess),
		uploader:   s3manager.NewUploader(sess),
	}
}

func (s *S3Service) Download(ctx context.Context, s3Path string, fileGUID string, extension string) (string, error) {
	// Create temp directory
	tempDir := "/tmp/conversions"
	os.MkdirAll(tempDir, 0755)

	localPath := filepath.Join(tempDir, fmt.Sprintf("%s.%s", fileGUID, extension))

	// Create file
	file, err := os.Create(localPath)
	if err != nil {
		return "", fmt.Errorf("failed to create local file: %w", err)
	}
	defer file.Close()

	// Download from S3
	_, err = s.downloader.DownloadWithContext(ctx, file, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s3Path),
	})

	if err != nil {
		return "", fmt.Errorf("failed to download from S3: %w", err)
	}

	return localPath, nil
}

func (s *S3Service) Upload(ctx context.Context, localPath string, s3Path string) error {
	// Open file
	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Upload to S3
	_, err = s.uploader.UploadWithContext(ctx, &s3manager.UploadInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(s3Path),
		Body:        file,
		ContentType: aws.String("application/pdf"),
	})

	if err != nil {
		return fmt.Errorf("failed to upload to S3: %w", err)
	}

	return nil
}

func (s *S3Service) Cleanup(path string) error {
	if path == "" {
		return nil
	}
	return os.Remove(path)
}
