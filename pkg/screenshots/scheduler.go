package screenshots

import (
	"context"
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
}

// Scheduler manages screenshot capture cadence with throttling.
type Scheduler struct {
	interval     time.Duration
	maxPerMinute int
	clock        func() time.Time
}

// Result summarises screenshot capture outcomes.
type Result struct {
	Files        []string
	Count        int
	FirstCapture time.Time
	LastCapture  time.Time
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
	return &Scheduler{interval: opts.Interval, maxPerMinute: opts.MaxPerMinute, clock: clock}, nil
}

// Capture writes placeholder screenshots to disk while respecting throttling.
func (s *Scheduler) Capture(ctx context.Context, destDir string) (Result, error) {
	if destDir == "" {
		return Result{}, errors.New("destination directory must not be empty")
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return Result{}, fmt.Errorf("ensure destination: %w", err)
	}

	base := s.clock().UTC()
	limit := s.maxPerMinute
	files := make([]string, 0, limit)

	for i := 0; i < limit; i++ {
		if ctx != nil && ctx.Err() != nil {
			return Result{}, ctx.Err()
		}
		timestamp := base.Add(time.Duration(i) * s.interval)
		name := fmt.Sprintf("screenshot_%03d.txt", i+1)
		path := filepath.Join(destDir, name)
		contents := fmt.Sprintf("synthetic screenshot captured at %s\n", timestamp.Format(time.RFC3339))
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			return Result{}, fmt.Errorf("write screenshot %q: %w", name, err)
		}
		files = append(files, path)
	}

	if len(files) == 0 {
		return Result{}, nil
	}

	return Result{
		Files:        files,
		Count:        len(files),
		FirstCapture: base,
		LastCapture:  base.Add(time.Duration(len(files)-1) * s.interval),
	}, nil
}
