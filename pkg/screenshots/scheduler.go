package screenshots

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Options configure the screenshot scheduler.
type Options struct {
	Interval     time.Duration
	MaxPerMinute int
	Clock        func() time.Time
	Provider     CaptureProvider
	Sleeper      func(context.Context, time.Duration) error
}

// Scheduler manages screenshot capture cadence with throttling.
type Scheduler struct {
	interval     time.Duration
	maxPerMinute int
	clock        func() time.Time
	provider     CaptureProvider
	sleeper      func(context.Context, time.Duration) error
}

// Result summarises screenshot capture outcomes.
type Result struct {
	Files         []string
	MetadataFiles []string
	Captures      []CaptureSummary
	Count         int
	FirstCapture  time.Time
	LastCapture   time.Time
}

// CaptureSummary describes where a capture was written.
type CaptureSummary struct {
	ImagePath    string
	MetadataPath string
}

// NewScheduler validates options and returns a scheduler instance.
func NewScheduler(opts Options) (*Scheduler, error) {
	if opts.Interval <= 0 {
		return nil, errors.New("interval must be positive")
	}
	if opts.MaxPerMinute <= 0 {
		return nil, errors.New("max per minute must be positive")
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	provider := opts.Provider
	if provider == nil {
		var err error
		provider, err = defaultCaptureProvider()
		if err != nil {
			return nil, err
		}
	}
	sleeper := opts.Sleeper
	if sleeper == nil {
		sleeper = defaultSleeper
	}
	return &Scheduler{
		interval:     opts.Interval,
		maxPerMinute: opts.MaxPerMinute,
		clock:        clock,
		provider:     provider,
		sleeper:      sleeper,
	}, nil
}

// Capture writes screenshots to disk as PNG images with JSON metadata while respecting throttling.
func (s *Scheduler) Capture(ctx context.Context, destDir string) (Result, error) {
	if destDir == "" {
		return Result{}, errors.New("destination directory must not be empty")
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return Result{}, fmt.Errorf("ensure destination: %w", err)
	}

	base := s.clock().UTC()
	limit := s.maxPerMinute
	pngFiles := make([]string, 0, limit)
	metadataFiles := make([]string, 0, limit)
	summaries := make([]CaptureSummary, 0, limit)

	nextCapture := base
	var firstCapture time.Time
	var lastCapture time.Time

	for i := 0; i < limit; i++ {
		if ctx != nil && ctx.Err() != nil {
			return Result{}, ctx.Err()
		}
		if err := s.waitForNext(ctx, nextCapture); err != nil {
			return Result{}, err
		}

		capture, err := s.provider.Grab(ctx)
		if err != nil {
			return Result{}, fmt.Errorf("capture frame: %w", err)
		}
		if len(capture.PNG) == 0 {
			return Result{}, errors.New("capture provider returned empty PNG data")
		}

		timestamp := capture.Metadata.CapturedAt
		if timestamp.IsZero() {
			timestamp = s.clock()
		}
		timestamp = timestamp.UTC()
		capture.Metadata.CapturedAt = timestamp

		name := fmt.Sprintf("screenshot_%03d", i+1)
		imagePath := filepath.Join(destDir, name+".png")
		if err := os.WriteFile(imagePath, capture.PNG, 0o644); err != nil {
			return Result{}, fmt.Errorf("write screenshot %q: %w", name, err)
		}

		capture.Metadata.ImagePath = filepath.Base(imagePath)
		metadataPath := filepath.Join(destDir, name+".json")
		metadataBytes, err := json.MarshalIndent(capture.Metadata, "", "  ")
		if err != nil {
			return Result{}, fmt.Errorf("marshal metadata for %q: %w", name, err)
		}
		if err := os.WriteFile(metadataPath, metadataBytes, 0o644); err != nil {
			return Result{}, fmt.Errorf("write metadata %q: %w", name, err)
		}

		pngFiles = append(pngFiles, imagePath)
		metadataFiles = append(metadataFiles, metadataPath)
		summaries = append(summaries, CaptureSummary{ImagePath: imagePath, MetadataPath: metadataPath})

		if firstCapture.IsZero() {
			firstCapture = timestamp
		}
		lastCapture = timestamp

		nextCapture = nextCapture.Add(s.interval)
	}

	if len(pngFiles) == 0 {
		return Result{}, nil
	}

	return Result{
		Files:         pngFiles,
		MetadataFiles: metadataFiles,
		Captures:      summaries,
		Count:         len(pngFiles),
		FirstCapture:  firstCapture,
		LastCapture:   lastCapture,
	}, nil
}

func (s *Scheduler) waitForNext(ctx context.Context, scheduled time.Time) error {
	if ctx == nil {
		ctx = context.Background()
	}
	now := s.clock()
	if !now.Before(scheduled) {
		return nil
	}
	wait := scheduled.Sub(now)
	if wait <= 0 {
		return nil
	}
	return s.sleeper(ctx, wait)
}

func defaultSleeper(ctx context.Context, wait time.Duration) error {
	if wait <= 0 {
		return nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
