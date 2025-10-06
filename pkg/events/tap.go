package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Options controls tap behaviour.
type Options struct {
	FineInterval   time.Duration
	CoarseInterval time.Duration
	Redactor       Redactor
	Clock          func() time.Time
	Privacy        PrivacyPolicy
	Source         EventSource
}

// Tap synthesises interaction events at multiple granularities.
type Tap struct {
	fineInterval   time.Duration
	coarseInterval time.Duration
	redactor       Redactor
	clock          func() time.Time
	privacy        PrivacyPolicy
	source         EventSource
}

// EventSource emits interaction events that should be recorded by the tap.
type EventSource interface {
	Stream(ctx context.Context, emit func(Event) error) error
}

// EventSourceFunc adapts a function literal to the EventSource interface.
type EventSourceFunc func(ctx context.Context, emit func(Event) error) error

// Stream calls the underlying function.
func (f EventSourceFunc) Stream(ctx context.Context, emit func(Event) error) error {
	return f(ctx, emit)
}

// Result reports the files produced by a tap capture session.
type Result struct {
	FinePath      string
	CoarsePath    string
	EventCount    int
	BucketCount   int
	FilteredCount int
	CaptureStart  time.Time
	CaptureEnd    time.Time
}

// Event describes a single interaction sample.
type Event struct {
	Timestamp time.Time         `json:"timestamp"`
	Category  string            `json:"category"`
	Action    string            `json:"action"`
	Target    string            `json:"target"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// CoarseBucket aggregates events within a coarse interval window.
type CoarseBucket struct {
	Start      time.Time      `json:"start"`
	End        time.Time      `json:"end"`
	Count      int            `json:"count"`
	Categories map[string]int `json:"categories"`
}

// NewTap validates options and constructs a tap instance.
func NewTap(opts Options) (*Tap, error) {
	if opts.FineInterval <= 0 {
		return nil, errors.New("fine interval must be positive")
	}
	if opts.CoarseInterval <= 0 {
		return nil, errors.New("coarse interval must be positive")
	}
	if opts.CoarseInterval < opts.FineInterval {
		return nil, errors.New("coarse interval must be greater than or equal to fine interval")
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	source := opts.Source
	if source == nil {
		source = defaultEventSource(opts, clock)
	}
	return &Tap{
		fineInterval:   opts.FineInterval,
		coarseInterval: opts.CoarseInterval,
		redactor:       opts.Redactor,
		clock:          clock,
		privacy:        opts.Privacy,
		source:         source,
	}, nil
}

// Capture generates synthetic events, persists the datasets, and returns metadata.
func (t *Tap) Capture(ctx context.Context, destDir string) (Result, error) {
	if destDir == "" {
		return Result{}, errors.New("destination directory must not be empty")
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return Result{}, fmt.Errorf("ensure destination: %w", err)
	}

	if ctx == nil {
		ctx = context.Background()
	}

	start := t.clock().UTC()

	finePath := filepath.Join(destDir, "events_fine.jsonl")
	coarsePath := filepath.Join(destDir, "events_coarse.json")

	fineFile, err := os.OpenFile(finePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return Result{}, fmt.Errorf("create fine events file: %w", err)
	}
	defer fineFile.Close()

	encoder := json.NewEncoder(fineFile)
	encoder.SetEscapeHTML(false)

	allowed := 0
	filtered := 0
	var lastAllowed time.Time
	var firstEvent time.Time
	buckets := make(map[time.Time]*CoarseBucket)
	streamErr := t.source.Stream(ctx, func(event Event) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if firstEvent.IsZero() {
			firstEvent = event.Timestamp
		}

		if !t.privacy.Allows(event) {
			filtered++
			return nil
		}

		redacted := event
		redacted.Metadata = t.redactor.ApplyMetadata(event.Metadata)

		if err := encoder.Encode(redacted); err != nil {
			return fmt.Errorf("write fine event: %w", err)
		}

		allowed++
		lastAllowed = event.Timestamp

		bucketStart := event.Timestamp.Truncate(t.coarseInterval)
		bucket := buckets[bucketStart]
		if bucket == nil {
			bucket = &CoarseBucket{
				Start:      bucketStart,
				End:        bucketStart.Add(t.coarseInterval),
				Categories: make(map[string]int),
			}
			buckets[bucketStart] = bucket
		}
		bucket.Count++
		bucket.Categories[event.Category]++
		return nil
	})

	if err := fineFile.Close(); err != nil {
		return Result{}, fmt.Errorf("close fine events file: %w", err)
	}

	if streamErr != nil {
		if errors.Is(streamErr, context.Canceled) || errors.Is(streamErr, context.DeadlineExceeded) {
			return Result{}, streamErr
		}
		return Result{}, fmt.Errorf("stream events: %w", streamErr)
	}

	summary := make([]CoarseBucket, 0, len(buckets))
	for _, bucket := range buckets {
		summary = append(summary, *bucket)
	}
	sort.Slice(summary, func(i, j int) bool {
		return summary[i].Start.Before(summary[j].Start)
	})

	coarseData, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return Result{}, fmt.Errorf("marshal coarse summary: %w", err)
	}
	if err := os.WriteFile(coarsePath, coarseData, 0o644); err != nil {
		return Result{}, fmt.Errorf("write coarse summary: %w", err)
	}

	end := start
	if allowed > 0 {
		end = lastAllowed
	}
	captureStart := start
	if !firstEvent.IsZero() {
		captureStart = firstEvent
	}
	return Result{
		FinePath:      finePath,
		CoarsePath:    coarsePath,
		EventCount:    allowed,
		BucketCount:   len(summary),
		FilteredCount: filtered,
		CaptureStart:  captureStart,
		CaptureEnd:    end,
	}, nil
}
