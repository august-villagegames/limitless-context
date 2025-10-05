package ocr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/offlinefirst/limitless-context/pkg/events"
)

// Options configure the OCR worker stub.
type Options struct {
	Languages       []string
	TesseractBinary string
	Redactor        events.Redactor
	LookPath        func(string) (string, error)
	Clock           func() time.Time
}

// Worker processes screenshot placeholders into recognised text fixtures.
type Worker struct {
	languages []string
	binary    string
	redactor  events.Redactor
	lookPath  func(string) (string, error)
	clock     func() time.Time
}

// Result reports OCR processing details.
type Result struct {
	ProcessedCount     int
	SkippedCount       int
	IndexPath          string
	StatusPath         string
	TesseractAvailable bool
}

type ocrStatus struct {
	GeneratedAt        time.Time `json:"generated_at"`
	Processed          int       `json:"processed"`
	Skipped            int       `json:"skipped"`
	Languages          []string  `json:"languages"`
	Index              string    `json:"index"`
	TesseractAvailable bool      `json:"tesseract_available"`
	Notes              []string  `json:"notes,omitempty"`
}

type indexEntry struct {
	Screenshot string `json:"screenshot"`
	Text       string `json:"text"`
	Language   string `json:"language"`
}

// NewWorker constructs a worker instance.
func NewWorker(opts Options) (*Worker, error) {
	if len(opts.Languages) == 0 {
		return nil, errors.New("languages must not be empty")
	}
	binary := strings.TrimSpace(opts.TesseractBinary)
	if binary == "" {
		binary = "tesseract"
	}
	lookPath := opts.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}

	languages := make([]string, 0, len(opts.Languages))
	for _, lang := range opts.Languages {
		trimmed := strings.TrimSpace(lang)
		if trimmed == "" {
			continue
		}
		languages = append(languages, trimmed)
	}
	if len(languages) == 0 {
		return nil, errors.New("no usable languages provided")
	}

	return &Worker{
		languages: languages,
		binary:    binary,
		redactor:  opts.Redactor,
		lookPath:  lookPath,
		clock:     clock,
	}, nil
}

// Process reads screenshot fixtures and writes OCR outputs.
func (w *Worker) Process(ctx context.Context, screenshots []string, destDir string) (Result, error) {
	if strings.TrimSpace(destDir) == "" {
		return Result{}, errors.New("destination directory must not be empty")
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return Result{}, fmt.Errorf("ensure ocr directory: %w", err)
	}

	available := w.tesseractAvailable()
	processed := 0
	skipped := 0
	entries := make([]indexEntry, 0, len(screenshots))

	for _, shot := range screenshots {
		if ctx != nil && ctx.Err() != nil {
			return Result{}, ctx.Err()
		}
		data, err := os.ReadFile(shot)
		if err != nil {
			skipped++
			continue
		}
		recognised := w.redactor.ApplyString(strings.TrimSpace(string(data)))
		entries = append(entries, indexEntry{
			Screenshot: filepath.Base(shot),
			Text:       recognised,
			Language:   w.languages[0],
		})
		processed++
	}

	indexPath := filepath.Join(destDir, "index.json")
	if len(entries) > 0 {
		indexDoc := struct {
			GeneratedAt time.Time    `json:"generated_at"`
			Entries     []indexEntry `json:"entries"`
		}{
			GeneratedAt: w.clock().UTC(),
			Entries:     entries,
		}
		payload, err := json.MarshalIndent(indexDoc, "", "  ")
		if err != nil {
			return Result{}, fmt.Errorf("marshal ocr index: %w", err)
		}
		if err := os.WriteFile(indexPath, payload, 0o644); err != nil {
			return Result{}, fmt.Errorf("write ocr index: %w", err)
		}
	} else {
		indexPath = ""
	}

	status := ocrStatus{
		GeneratedAt:        w.clock().UTC(),
		Processed:          processed,
		Skipped:            skipped,
		Languages:          w.languages,
		Index:              indexPath,
		TesseractAvailable: available,
	}
	if !available {
		status.Notes = append(status.Notes, fmt.Sprintf("tesseract binary %q not detected", w.binary))
	}

	statusPath := filepath.Join(destDir, "status.json")
	statusPayload, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return Result{}, fmt.Errorf("marshal ocr status: %w", err)
	}
	if err := os.WriteFile(statusPath, statusPayload, 0o644); err != nil {
		return Result{}, fmt.Errorf("write ocr status: %w", err)
	}

	return Result{
		ProcessedCount:     processed,
		SkippedCount:       skipped,
		IndexPath:          indexPath,
		StatusPath:         statusPath,
		TesseractAvailable: available,
	}, nil
}

func (w *Worker) tesseractAvailable() bool {
	if w.lookPath == nil {
		return false
	}
	_, err := w.lookPath(w.binary)
	return err == nil
}
