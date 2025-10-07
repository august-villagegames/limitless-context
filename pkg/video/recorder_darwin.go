//go:build darwin

package video

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -fmodules
#cgo LDFLAGS: -framework Foundation -framework AVFoundation -framework ScreenCaptureKit -framework CoreMedia -framework CoreVideo -framework VideoToolbox
#include <stdlib.h>
#include "recorder_darwin.h"
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"
)

const permissionErrorPrefix = "SCREEN_RECORDING_PERMISSION_REQUIRED:"

type macRecorder struct {
	format string
	mu     sync.Mutex
}

func newNativeRecorder(format string) (NativeRecorder, error) {
	if format != "mp4" {
		return nil, fmt.Errorf("format %q is not supported on macOS recorder", format)
	}
	if C.recorder_initialize() != 0 {
		return nil, errors.New("screen recording frameworks are not available")
	}
	return &macRecorder{format: format}, nil
}

func (m *macRecorder) Record(ctx context.Context, dest string, filename string, started time.Time, duration time.Duration) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	absDir, err := filepath.Abs(dest)
	if err != nil {
		return "", fmt.Errorf("resolve destination: %w", err)
	}
	absFile := filepath.Join(absDir, filename)

	durationSeconds := duration.Seconds()
	if durationSeconds <= 0 {
		return "", errors.New("duration must be positive")
	}

	cPath := C.CString(absFile)
	defer C.free(unsafe.Pointer(cPath))

	errCh := make(chan error, 1)
	go func() {
		var cerr *C.char
		rc := C.recorder_record_screen(cPath, C.double(durationSeconds), &cerr)
		if cerr != nil {
			errMsg := C.GoString(cerr)
			C.recorder_free_string(cerr)
			if strings.HasPrefix(errMsg, permissionErrorPrefix) {
				errCh <- newPermissionError(strings.TrimPrefix(errMsg, permissionErrorPrefix))
				return
			}
			errCh <- errors.New(errMsg)
			return
		}
		if rc != 0 {
			errCh <- fmt.Errorf("recording failed with status %d", int(rc))
			return
		}
		errCh <- nil
	}()

	if ctx != nil {
		select {
		case <-ctx.Done():
			C.recorder_cancel_active()
			err := <-errCh
			if err != nil {
				return "", err
			}
			return "", ctx.Err()
		case err := <-errCh:
			if err != nil {
				return "", err
			}
		}
	} else {
		if err := <-errCh; err != nil {
			return "", err
		}
	}

	return absFile, nil
}
