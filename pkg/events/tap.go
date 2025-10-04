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
}

// Tap synthesises interaction events at multiple granularities.
type Tap struct {
	fineInterval   time.Duration
	coarseInterval time.Duration
	redactor       Redactor
	clock          func() time.Time
}

// Result reports the files produced by a tap capture session.
type Result struct {
	FinePath     string
	CoarsePath   string
	EventCount   int
	BucketCount  int
	CaptureStart time.Time
	CaptureEnd   time.Time
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
	return &Tap{
		fineInterval:   opts.FineInterval,
		coarseInterval: opts.CoarseInterval,
		redactor:       opts.Redactor,
		clock:          clock,
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

	start := t.clock().UTC()
	events := t.syntheticTimeline(start)

	finePath := filepath.Join(destDir, "events_fine.jsonl")
	coarsePath := filepath.Join(destDir, "events_coarse.json")

	fineFile, err := os.OpenFile(finePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return Result{}, fmt.Errorf("create fine events file: %w", err)
	}
	defer fineFile.Close()

	encoder := json.NewEncoder(fineFile)
	encoder.SetEscapeHTML(false)

	buckets := make(map[time.Time]*CoarseBucket)
	for _, event := range events {
		if ctx != nil && ctx.Err() != nil {
			return Result{}, ctx.Err()
		}
		redacted := event
		redacted.Metadata = t.redactor.ApplyMetadata(event.Metadata)

		if err := encoder.Encode(redacted); err != nil {
			return Result{}, fmt.Errorf("write fine event: %w", err)
		}

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
	}

	if err := fineFile.Close(); err != nil {
		return Result{}, fmt.Errorf("close fine events file: %w", err)
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

	end := events[len(events)-1].Timestamp
	return Result{
		FinePath:     finePath,
		CoarsePath:   coarsePath,
		EventCount:   len(events),
		BucketCount:  len(summary),
		CaptureStart: start,
		CaptureEnd:   end,
	}, nil
}

func (t *Tap) syntheticTimeline(start time.Time) []Event {
	fine := t.fineInterval
	events := []Event{
		{
			Timestamp: start,
			Category:  "keyboard",
			Action:    "type",
			Target:    "compose",
			Metadata: map[string]string{
				"text": "Drafting email to support@example.com about rollout",
			},
		},
		{
			Timestamp: start.Add(fine),
			Category:  "mouse",
			Action:    "click",
			Target:    "submit-button",
			Metadata: map[string]string{
				"label": "Submit order",
			},
		},
		{
			Timestamp: start.Add(2 * fine),
			Category:  "window",
			Action:    "focus",
			Target:    "docs-app",
			Metadata: map[string]string{
				"title": "Roadmap token=abcd1234",
			},
		},
		{
			Timestamp: start.Add(3 * fine),
			Category:  "clipboard",
			Action:    "copy",
			Target:    "",
			Metadata: map[string]string{
				"preview": "Quarterly plan summary",
			},
		},
	}
	return events
}
