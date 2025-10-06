package screenshots

import (
	"runtime"

	"github.com/offlinefirst/limitless-context/pkg/permissions"
)

// Environment describes screenshot capture availability.
type Environment struct {
	Provider   string
	Available  bool
	Permission string
	Message    string
	Guidance   string
}

const (
	providerScreenCaptureKit = "screencapturekit"
	providerCoreGraphics     = "coregraphics"
	providerStub             = "stub"
)

// DetectEnvironment reports screenshot backend support and permissions.
func DetectEnvironment() Environment {
	screenRecording := permissions.ProbeScreenRecording(nil)
	env := Environment{
		Provider:   providerStub,
		Permission: screenRecording.StatusString(),
		Message:    screenRecording.Message,
		Guidance:   screenRecording.Guidance,
		Available:  true,
	}

	switch runtime.GOOS {
	case "darwin":
		env.Provider = providerScreenCaptureKit
		env.Available = screenRecording.Status != permissions.StatusDenied
		if !env.Available && env.Message == "" {
			env.Message = "screen recording permission missing"
		}
	default:
		env.Permission = "not_applicable"
		if env.Message == "" {
			env.Message = "synthetic screenshot stub"
		}
	}

	if !env.Available {
		env.Provider = providerStub
	}
	return env
}
