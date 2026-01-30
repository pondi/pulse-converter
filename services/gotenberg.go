package services

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
)

type GotenbergService struct {
	baseURL string
	client  *http.Client
}

const pdfaConformance = "PDF/A-2b"

func NewGotenbergService(baseURL string) *GotenbergService {
	return &GotenbergService{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 0, // Use context timeout instead
		},
	}
}

func (g *GotenbergService) ConvertToPDFA(ctx context.Context, inputPath string, extension string) (string, error) {
	// Open input file
	file, err := os.Open(inputPath)
	if err != nil {
		return "", fmt.Errorf("failed to open input file: %w", err)
	}
	defer file.Close()

	// Create multipart form
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add file
	part, err := writer.CreateFormFile("files", filepath.Base(inputPath))
	if err != nil {
		return "", fmt.Errorf("failed to create form file: %w", err)
	}

	if _, err := io.Copy(part, file); err != nil {
		return "", fmt.Errorf("failed to copy file: %w", err)
	}

	// Add PDF/A-2b option (modern archival standard with better compression)
	writer.WriteField("pdfa", pdfaConformance)

	// Close writer
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("failed to close writer: %w", err)
	}

	// Create request
	url := fmt.Sprintf("%s/forms/libreoffice/convert", g.baseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, body)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Send request
	resp, err := g.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("gotenberg request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("gotenberg returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Save response to temporary file
	outputPath := inputPath + ".converted.pdf"
	outFile, err := os.Create(outputPath)
	if err != nil {
		return "", fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	if _, err := io.Copy(outFile, resp.Body); err != nil {
		return "", fmt.Errorf("failed to save converted file: %w", err)
	}

	return outputPath, nil
}
