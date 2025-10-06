//go:build !darwin

package video

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type stubRecorder struct {
	format string
}

func newNativeRecorder(format string) (NativeRecorder, error) {
	if format != "mp4" {
		return nil, fmt.Errorf("format %q is unsupported on this platform", format)
	}
	return &stubRecorder{format: format}, nil
}

func (s *stubRecorder) Record(ctx context.Context, dest string, filename string, started time.Time, duration time.Duration) (string, error) {
	if ctx != nil && ctx.Err() != nil {
		return "", ctx.Err()
	}
	return "", errors.New("screen recording is only supported on macOS")
}
