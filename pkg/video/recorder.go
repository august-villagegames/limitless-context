package video

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Options configure the recorder stub.
type Options struct {
	ChunkSeconds int
	Format       string
	Clock        func() time.Time
}

// Recorder simulates writing a single video segment to disk.
type Recorder struct {
	chunkDuration time.Duration
	format        string
	clock         func() time.Time
}

// Result summarises recorder output.
type Result struct {
	File    string
	Started time.Time
	Ended   time.Time
}

// NewRecorder validates options and constructs a recorder instance.
func NewRecorder(opts Options) (*Recorder, error) {
	if opts.ChunkSeconds <= 0 {
		return nil, errors.New("chunk seconds must be positive")
	}
	format := strings.TrimSpace(opts.Format)
	if format == "" {
		return nil, errors.New("format must not be empty")
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	return &Recorder{
		chunkDuration: time.Duration(opts.ChunkSeconds) * time.Second,
		format:        format,
		clock:         clock,
	}, nil
}

// Record writes a placeholder segment to the destination directory.
func (r *Recorder) Record(ctx context.Context, destDir string) (Result, error) {
	if destDir == "" {
		return Result{}, errors.New("destination directory must not be empty")
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return Result{}, fmt.Errorf("ensure destination: %w", err)
	}

	started := r.clock().UTC()
	ended := started.Add(r.chunkDuration)
	name := fmt.Sprintf("segment_0001.%s", r.format)
	path := filepath.Join(destDir, name)
	payload := fmt.Sprintf("synthetic video segment from %s to %s\n", started.Format(time.RFC3339), ended.Format(time.RFC3339))

	if ctx != nil && ctx.Err() != nil {
		return Result{}, ctx.Err()
	}

	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		return Result{}, fmt.Errorf("write segment: %w", err)
	}

	return Result{File: path, Started: started, Ended: ended}, nil
}
