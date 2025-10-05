package ocr

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/offlinefirst/limitless-context/pkg/events"
)

func TestWorkerProcessesScreenshots(t *testing.T) {
	dir := t.TempDir()
	screenshotsDir := t.TempDir()

	shotPath := filepath.Join(screenshotsDir, "screenshot_001.txt")
	if err := os.WriteFile(shotPath, []byte("Contact owner@example.com with updates"), 0o644); err != nil {
		t.Fatalf("write screenshot: %v", err)
	}

	redactor, err := events.NewRedactor(true, nil)
	if err != nil {
		t.Fatalf("new redactor: %v", err)
	}

	worker, err := NewWorker(Options{
		Languages:       []string{"eng"},
		TesseractBinary: "tesseract",
		Redactor:        redactor,
		LookPath:        func(string) (string, error) { return "/usr/local/bin/tesseract", nil },
		Clock:           func() time.Time { return time.Date(2024, 5, 12, 9, 30, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}

	result, err := worker.Process(context.Background(), []string{shotPath}, dir)
	if err != nil {
		t.Fatalf("process: %v", err)
	}

	if result.ProcessedCount != 1 {
		t.Fatalf("expected 1 processed screenshot, got %d", result.ProcessedCount)
	}
	if result.IndexPath == "" {
		t.Fatalf("expected index path")
	}

	data, err := os.ReadFile(result.IndexPath)
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	if strings.Contains(string(data), "owner@example.com") {
		t.Fatalf("expected redacted index content: %s", string(data))
	}

	statusData, err := os.ReadFile(result.StatusPath)
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	var status ocrStatus
	if err := json.Unmarshal(statusData, &status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if !status.TesseractAvailable {
		t.Fatalf("expected tesseract to be available")
	}
}

func TestWorkerHandlesMissingBinary(t *testing.T) {
	dir := t.TempDir()
	redactor, err := events.NewRedactor(false, nil)
	if err != nil {
		t.Fatalf("new redactor: %v", err)
	}

	worker, err := NewWorker(Options{
		Languages:       []string{"eng"},
		TesseractBinary: "tesseract",
		Redactor:        redactor,
		LookPath:        func(string) (string, error) { return "", os.ErrNotExist },
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}

	result, err := worker.Process(context.Background(), []string{"missing.txt"}, dir)
	if err != nil {
		t.Fatalf("process: %v", err)
	}

	if result.TesseractAvailable {
		t.Fatalf("expected tesseract to be unavailable")
	}
	statusData, err := os.ReadFile(result.StatusPath)
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	if !strings.Contains(string(statusData), "tesseract binary") {
		t.Fatalf("expected status to mention missing binary: %s", string(statusData))
	}
}
