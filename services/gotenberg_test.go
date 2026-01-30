package services

import (
	"bytes"
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func assertMultipartPDFAField(t *testing.T, r *http.Request, expectedPath string) {
	t.Helper()

	if r.URL.Path != expectedPath {
		t.Fatalf("unexpected path: %s", r.URL.Path)
	}

	mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" {
		t.Fatalf("expected multipart/form-data, got %q (err=%v)", mediaType, err)
	}

	reader := multipart.NewReader(r.Body, params["boundary"])
	defer func() { _ = r.Body.Close() }()

	var pdfaValue string
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("failed to read multipart part: %v", err)
		}

		if part.FormName() == "pdfa" {
			b, _ := io.ReadAll(part)
			pdfaValue = string(b)
		} else {
			_, _ = io.Copy(io.Discard, part)
		}
		_ = part.Close()
	}

	if pdfaValue != pdfaConformance {
		t.Fatalf("expected pdfa=%q, got %q", pdfaConformance, pdfaValue)
	}
}

func TestGotenbergService_ConvertToPDFA_UsesPDFA2b(t *testing.T) {
	t.Parallel()

	svc := NewGotenbergService("http://example.invalid")
	svc.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		assertMultipartPDFAField(t, r, "/forms/libreoffice/convert")
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader([]byte("%PDF-1.4\n%EOF\n"))),
			Header:     make(http.Header),
		}, nil
	})

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "input.docx")
	if err := os.WriteFile(inputPath, []byte("dummy"), 0644); err != nil {
		t.Fatalf("failed to write temp input: %v", err)
	}

	outputPath, err := svc.ConvertToPDFA(context.Background(), inputPath, "docx")
	if err != nil {
		t.Fatalf("ConvertToPDFA failed: %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read output: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty output")
	}
}
