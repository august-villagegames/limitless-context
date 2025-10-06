package ocr

import (
	"os/exec"
	"strings"
)

// Provider identifiers for OCR backends.
const (
	ProviderTesseractStub = "tesseract_stub"
)

// Environment captures OCR dependency availability.
type Environment struct {
	Provider           string
	Available          bool
	TesseractAvailable bool
	Message            string
	Guidance           []string
}

// DetectorOptions controls OCR environment probing.
type DetectorOptions struct {
	TesseractBinary string
	LookPath        func(string) (string, error)
}

// DetectEnvironment reports whether Tesseract is available.
func DetectEnvironment(opts DetectorOptions) Environment {
	binary := strings.TrimSpace(opts.TesseractBinary)
	if binary == "" {
		binary = "tesseract"
	}
	lookPath := opts.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}

	available := false
	if lookPath != nil {
		if _, err := lookPath(binary); err == nil {
			available = true
		}
	}

	env := Environment{
		Provider:           ProviderTesseractStub,
		TesseractAvailable: available,
		Available:          true,
	}
	if !available {
		env.Message = "tesseract binary missing"
		env.Guidance = append(env.Guidance, "Install Tesseract OCR and expose it on PATH")
	} else {
		env.Message = "tesseract binary detected"
	}
	return env
}
