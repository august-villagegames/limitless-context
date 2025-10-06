package ocr

import (
	"errors"
	"testing"
)

type fakePath struct {
	err error
}

func (f fakePath) lookup(string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return "/usr/local/bin/tesseract", nil
}

func TestDetectEnvironmentReportsAvailability(t *testing.T) {
	env := DetectEnvironment(DetectorOptions{TesseractBinary: "tesseract", LookPath: fakePath{}.lookup})
	if !env.Available || !env.TesseractAvailable {
		t.Fatalf("expected tesseract to be available")
	}
}

func TestDetectEnvironmentMissingBinary(t *testing.T) {
	env := DetectEnvironment(DetectorOptions{TesseractBinary: "tesseract", LookPath: fakePath{err: errors.New("missing")}.lookup})
	if env.TesseractAvailable {
		t.Fatalf("expected tesseract to be unavailable")
	}
	if len(env.Guidance) == 0 {
		t.Fatalf("expected guidance when missing")
	}
}
