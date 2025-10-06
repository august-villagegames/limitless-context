package asr

import (
	"os/exec"
	"strings"

	"github.com/offlinefirst/limitless-context/pkg/permissions"
)

// Provider identifiers for ASR backends.
const (
	ProviderWhisperStub = "whisper_stub"
)

// Environment captures ASR dependency availability and permissions.
type Environment struct {
	Provider         string
	Available        bool
	WhisperAvailable bool
	Permission       string
	Message          string
	Guidance         []string
}

// DetectorOptions controls ASR environment probing.
type DetectorOptions struct {
	WhisperBinary string
	LookPath      func(string) (string, error)
}

// DetectEnvironment reports whether Whisper and microphone permissions are satisfied.
func DetectEnvironment(opts DetectorOptions) Environment {
	binary := strings.TrimSpace(opts.WhisperBinary)
	if binary == "" {
		binary = "whisper"
	}
	lookPath := opts.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}

	microphone := permissions.ProbeMicrophone(nil)
	whisperAvailable := false
	if lookPath != nil {
		if _, err := lookPath(binary); err == nil {
			whisperAvailable = true
		}
	}

	env := Environment{
		Provider:         ProviderWhisperStub,
		WhisperAvailable: whisperAvailable,
		Permission:       microphone.StatusString(),
		Message:          microphone.Message,
		Available:        microphone.Status != permissions.StatusDenied,
	}
	if microphone.Guidance != "" {
		env.Guidance = append(env.Guidance, microphone.Guidance)
	}
	if !whisperAvailable {
		env.Message = strings.TrimSpace(env.Message + "; whisper binary missing")
		env.Guidance = append(env.Guidance, "Install whisper.cpp binary and expose it on PATH")
	}
	if microphone.Status == permissions.StatusDenied {
		env.Available = false
	}
	return env
}
