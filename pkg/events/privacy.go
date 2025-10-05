package events

import "strings"

// PrivacyPolicy enforces allow-list rules for event capture.
// The zero value permits all events.
type PrivacyPolicy struct {
	allowApps   map[string]struct{}
	allowURLs   []string
	dropUnknown bool
}

// NewPrivacyPolicy constructs an allow-list filter.
func NewPrivacyPolicy(allowApps, allowURLs []string, dropUnknown bool) PrivacyPolicy {
	policy := PrivacyPolicy{
		allowApps:   make(map[string]struct{}, len(allowApps)),
		allowURLs:   make([]string, 0, len(allowURLs)),
		dropUnknown: dropUnknown,
	}

	for _, app := range allowApps {
		trimmed := strings.TrimSpace(app)
		if trimmed == "" {
			continue
		}
		policy.allowApps[strings.ToLower(trimmed)] = struct{}{}
	}

	for _, url := range allowURLs {
		trimmed := strings.TrimSpace(url)
		if trimmed == "" {
			continue
		}
		policy.allowURLs = append(policy.allowURLs, strings.ToLower(trimmed))
	}

	return policy
}

// Allows reports whether the event passes allow-list checks.
func (p PrivacyPolicy) Allows(event Event) bool {
	if len(p.allowApps) == 0 && len(p.allowURLs) == 0 {
		return true
	}

	metadata := event.Metadata
	var app string
	if metadata != nil {
		app = strings.ToLower(strings.TrimSpace(metadata["app"]))
	}

	if len(p.allowApps) > 0 {
		if app == "" {
			if p.dropUnknown {
				return false
			}
		} else {
			if _, ok := p.allowApps[app]; !ok {
				return false
			}
		}
	}

	if len(p.allowURLs) > 0 {
		var url string
		if metadata != nil {
			url = strings.ToLower(strings.TrimSpace(metadata["url"]))
		}
		if url == "" {
			if p.dropUnknown {
				return false
			}
		} else {
			matched := false
			for _, prefix := range p.allowURLs {
				if strings.HasPrefix(url, prefix) {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		}
	}

	return true
}
