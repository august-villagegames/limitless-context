package screenshots

import (
	"context"
	"time"
)

// CaptureProvider produces screenshot frames for the scheduler.
type CaptureProvider interface {
	Grab(context.Context) (FrameCapture, error)
}

// FrameCapture bundles the encoded PNG bytes with metadata.
type FrameCapture struct {
	PNG      []byte
	Metadata Metadata
}

// Metadata captures details about a screenshot frame written to disk.
type Metadata struct {
	CapturedAt  time.Time `json:"captured_at"`
	Backend     string    `json:"backend"`
	Width       int       `json:"width"`
	Height      int       `json:"height"`
	PixelFormat string    `json:"pixel_format,omitempty"`
	Scale       float64   `json:"scale,omitempty"`
	ImagePath   string    `json:"image_path"`
	Notes       []string  `json:"notes,omitempty"`
}
