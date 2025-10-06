package video

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// Options configure the recorder implementation.
type Options struct {
	ChunkSeconds int
	Format       string
	Clock        func() time.Time
}

// Result summarises recorder output.
type Result struct {
	File    string
	Started time.Time
	Ended   time.Time
}

// Recorder coordinates platform specific capture implementations.
type Recorder struct {
	chunkDuration time.Duration
	format        string
	clock         func() time.Time

	native NativeRecorder
}

// NativeRecorder abstracts the OS specific capture backend.
type NativeRecorder interface {
	Record(ctx context.Context, dest string, filename string, started time.Time, duration time.Duration) (string, error)
}

var (
	nativeFactoryMu sync.Mutex
	nativeFactory   = defaultNativeFactory
)

// SetNativeFactory overrides the native recorder factory. It is intended for
// tests that need to stub the platform specific implementation.
func SetNativeFactory(factory func(format string) (NativeRecorder, error)) {
	nativeFactoryMu.Lock()
	defer nativeFactoryMu.Unlock()
	if factory == nil {
		nativeFactory = defaultNativeFactory
		return
	}
	nativeFactory = factory
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

	nativeFactoryMu.Lock()
	factory := nativeFactory
	nativeFactoryMu.Unlock()

	native, err := factory(format)
	if err != nil {
		return nil, err
	}

	return &Recorder{
		chunkDuration: time.Duration(opts.ChunkSeconds) * time.Second,
		format:        format,
		clock:         clock,
		native:        native,
	}, nil
}

// Record captures a single segment to the destination directory.
func (r *Recorder) Record(ctx context.Context, destDir string) (Result, error) {
	if destDir == "" {
		return Result{}, errors.New("destination directory must not be empty")
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return Result{}, fmt.Errorf("ensure destination: %w", err)
	}

	started := r.clock().UTC()
	filename := fmt.Sprintf("segment_%s.%s", started.Format("20060102T150405"), r.format)

	if ctx != nil && ctx.Err() != nil {
		return Result{}, ctx.Err()
	}

	file, err := r.native.Record(ctx, destDir, filename, started, r.chunkDuration)
	if err != nil {
		return Result{}, err
	}

	ended := started.Add(r.chunkDuration)
	return Result{File: file, Started: started, Ended: ended}, nil
}

func defaultNativeFactory(format string) (NativeRecorder, error) {
	return newNativeRecorder(format)
}
