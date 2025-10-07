package video

import (
	"errors"
	"strings"
)

// ErrPermissionRequired indicates macOS screen recording permission is needed for video capture.
var ErrPermissionRequired = errors.New("macOS screen recording permission required for video capture")

type permissionError struct {
	message string
}

func (e *permissionError) Error() string {
	return e.message
}

func (e *permissionError) Is(target error) bool {
	return target == ErrPermissionRequired
}

func newPermissionError(message string) error {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		trimmed = ErrPermissionRequired.Error()
	}
	return &permissionError{message: trimmed}
}
