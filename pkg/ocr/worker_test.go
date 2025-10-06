package ocr

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/offlinefirst/limitless-context/pkg/events"
	"github.com/offlinefirst/limitless-context/pkg/screenshots"
)

func TestWorkerProcessesScreenshots(t *testing.T) {
	dir := t.TempDir()
	screenshotsDir := t.TempDir()

	shotPath := filepath.Join(screenshotsDir, "screenshot_001.png")
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 10, B: 10, A: 255})
		}
	}
	buf := &bytes.Buffer{}
	if err := png.Encode(buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	if err := os.WriteFile(shotPath, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write screenshot: %v", err)
	}
	metaPath := filepath.Join(screenshotsDir, "screenshot_001.json")
	meta := screenshots.Metadata{
		CapturedAt: time.Date(2024, 5, 12, 9, 30, 0, 0, time.UTC),
		Backend:    "synthetic",
		Width:      2,
		Height:     2,
		ImagePath:  "screenshot_001.png",
		Notes:      []string{"Contact owner@example.com with updates"},
	}
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	if err := os.WriteFile(metaPath, metaBytes, 0o644); err != nil {
		t.Fatalf("write metadata: %v", err)
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

	result, err := worker.Process(context.Background(), []string{"missing.png"}, dir)
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
