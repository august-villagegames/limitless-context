package events

import (
	"runtime"

	"github.com/offlinefirst/limitless-context/pkg/permissions"
)

// Environment summarises event tap backend support.
type Environment struct {
	Provider   string
	Available  bool
	Permission string
	Message    string
	Guidance   string
}

const (
	providerQuartz = "quartz_event_tap"
	providerStub   = "stub"
)

// DetectEnvironment reports the availability of a real Quartz event tap.
func DetectEnvironment() Environment {
	accessibility := permissions.ProbeAccessibility(nil)
	env := Environment{
		Provider:   providerStub,
		Permission: accessibility.StatusString(),
		Message:    accessibility.Message,
		Guidance:   accessibility.Guidance,
		Available:  true,
	}

	if runtime.GOOS == "darwin" {
		env.Provider = providerQuartz
		env.Available = accessibility.Status != permissions.StatusDenied
		if !env.Available && env.Message == "" {
			env.Message = "accessibility permission missing"
		}
	} else {
		env.Permission = "not_applicable"
		if env.Message == "" {
			env.Message = "synthetic event tap stub"
		}
	}

	if !env.Available {
		env.Provider = providerStub
	}
	return env
}
