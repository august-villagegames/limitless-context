package events

import "errors"

// ErrAccessibilityPermission indicates the host must grant Accessibility trust.
var ErrAccessibilityPermission = errors.New("macOS accessibility permission required for event capture")
