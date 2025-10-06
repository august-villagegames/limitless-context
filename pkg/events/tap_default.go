//go:build !darwin

package events

import (
	"context"
	"time"
)

type syntheticSource struct {
	fine  time.Duration
	clock func() time.Time
}

func defaultEventSource(opts Options, clock func() time.Time) EventSource {
	return syntheticSource{fine: opts.FineInterval, clock: clock}
}

func (s syntheticSource) Stream(ctx context.Context, emit func(Event) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	start := s.clock().UTC()
	fine := s.fine
	timeline := []Event{
		{
			Timestamp: start,
			Category:  "keyboard",
			Action:    "type",
			Target:    "compose",
			Metadata: map[string]string{
				"text": "Drafting email to support@example.com about rollout",
				"app":  "mail",
				"url":  "mailto:support@example.com",
			},
		},
		{
			Timestamp: start.Add(fine),
			Category:  "mouse",
			Action:    "click",
			Target:    "submit-button",
			Metadata: map[string]string{
				"label": "Submit order",
				"app":   "checkout",
				"url":   "https://orders.example.com/checkout",
			},
		},
		{
			Timestamp: start.Add(2 * fine),
			Category:  "window",
			Action:    "focus",
			Target:    "docs-app",
			Metadata: map[string]string{
				"title": "Roadmap token=abcd1234",
				"app":   "docs",
				"url":   "https://docs.example.com/roadmap",
			},
		},
		{
			Timestamp: start.Add(3 * fine),
			Category:  "clipboard",
			Action:    "copy",
			Target:    "",
			Metadata: map[string]string{
				"preview": "Quarterly plan summary",
				"app":     "notes",
				"url":     "https://notes.example.com/q1",
			},
		},
	}

	for _, event := range timeline {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := emit(event); err != nil {
			return err
		}
	}
	return nil
}
