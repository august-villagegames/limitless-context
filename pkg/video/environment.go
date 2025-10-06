package video

import (
	"os"
	"runtime"
	"strings"

	"github.com/offlinefirst/limitless-context/pkg/permissions"
)

// Provider identifiers for manifest reporting.
const (
	ProviderScreenCaptureKit = "screencapturekit"
	ProviderAVFoundation     = "avfoundation"
	ProviderStub             = "stub"
)

// Permission states for downstream tooling.
const (
	PermissionNotApplicable = "not_applicable"
)

// Environment describes the current platform support for the recorder.
type Environment struct {
	Provider   string
	Available  bool
	Permission string
	Message    string
	Guidance   string
}

// DetectEnvironment reports the recorder backend and availability for the host platform.
func DetectEnvironment() Environment {
	backend := strings.ToLower(strings.TrimSpace(os.Getenv("LIMITLESS_VIDEO_BACKEND")))
	screenRecording := permissions.ProbeScreenRecording(nil)

	env := Environment{
		Provider:   ProviderStub,
		Permission: screenRecording.StatusString(),
		Message:    screenRecording.Message,
		Guidance:   screenRecording.Guidance,
		Available:  true,
	}

	switch runtime.GOOS {
	case "darwin":
		provider := ProviderScreenCaptureKit
		switch backend {
		case "avfoundation":
			provider = ProviderAVFoundation
		case "stub":
			provider = ProviderStub
		case "":
			// fall back to ScreenCaptureKit
		default:
			provider = backend
		}
		env.Provider = provider
		env.Available = screenRecording.Status != permissions.StatusDenied
		if screenRecording.Status == permissions.StatusUnavailable && provider != ProviderStub {
			env.Available = false
		}
		if env.Provider == ProviderStub {
			env.Available = true
		}
		if !env.Available && env.Message == "" {
			env.Message = "screen recording permission missing"
		}
	default:
		env.Permission = PermissionNotApplicable
		if env.Message == "" {
			env.Message = "non-mac platform; synthetic recorder"
		}
	}

	if env.Provider == ProviderStub && env.Message == "" {
		env.Message = "synthetic recorder stub"
	}
	return env
}
