package events

import (
	"regexp"
	"strings"
)

// Redactor applies sensitive-data filters to event metadata.
//
// The zero value is a no-op redactor.
type Redactor struct {
	patterns []*regexp.Regexp
}

// NewRedactor constructs a redaction pipeline using the supplied options.
// When redactEmails is true a built-in expression masks common email formats.
// Additional custom expressions are appended to the pipeline.
func NewRedactor(redactEmails bool, custom []string) (Redactor, error) {
	patterns := make([]*regexp.Regexp, 0, len(custom)+1)

	if redactEmails {
		rx, err := regexp.Compile(`(?i)[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}`)
		if err != nil {
			return Redactor{}, err
		}
		patterns = append(patterns, rx)
	}

	named := map[string]string{
		"email": `(?i)[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}`,
		"cc16":  `\b(?:\d[ -]?){16}\b`,
		"jwt":   `eyJ[A-Za-z0-9_-]*\.[A-Za-z0-9._-]+\.[A-Za-z0-9._-]+`,
	}

	for _, expr := range custom {
		trimmed := strings.TrimSpace(expr)
		if trimmed == "" {
			continue
		}

		candidate := trimmed
		if mapped, ok := named[strings.ToLower(trimmed)]; ok {
			candidate = mapped
		}

		rx, err := regexp.Compile(candidate)
		if err != nil {
			return Redactor{}, err
		}
		patterns = append(patterns, rx)
	}

	return Redactor{patterns: patterns}, nil
}

// ApplyString redacts sensitive content from a string.
func (r Redactor) ApplyString(input string) string {
	if len(r.patterns) == 0 {
		return input
	}

	redacted := input
	for _, rx := range r.patterns {
		redacted = rx.ReplaceAllString(redacted, "[REDACTED]")
	}
	return redacted
}

// ApplyMetadata clones the provided metadata map and redacts each value.
func (r Redactor) ApplyMetadata(in map[string]string) map[string]string {
	if len(r.patterns) == 0 || len(in) == 0 {
		if len(in) == 0 {
			return nil
		}
		clone := make(map[string]string, len(in))
		for k, v := range in {
			clone[k] = v
		}
		return clone
	}

	clone := make(map[string]string, len(in))
	for k, v := range in {
		clone[k] = r.ApplyString(v)
	}
	return clone
}
