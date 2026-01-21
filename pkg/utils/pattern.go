// pkg/utils/pattern.go
package utils

import (
	"regexp"
	"strings"
)

// WildCardToRegexp converts a wildcard pattern to regex
func WildCardToRegexp(pattern string) string {
	pattern = strings.ReplaceAll(pattern, ".", "\\.")
	components := strings.Split(pattern, "*")
	if len(components) == 1 {
		return "^" + pattern + "$"
	}
	var result strings.Builder
	for i, literal := range components {
		if i > 0 {
			result.WriteString(".*")
		}
		result.WriteString(regexp.QuoteMeta(literal))
	}
	return "^" + result.String() + "$"
}

// MatchesPattern checks if text matches a wildcard pattern
func MatchesPattern(text, pattern string, caseSensitive bool) bool {
	if pattern == "" {
		return true
	}

	regexPattern := WildCardToRegexp(pattern)

	if !caseSensitive {
		regexPattern = "(?i)" + regexPattern
	}

	matched, err := regexp.MatchString(regexPattern, text)
	if err != nil {
		// Fallback to simple contains
		if !caseSensitive {
			return strings.Contains(strings.ToLower(text), strings.ToLower(pattern))
		}
		return strings.Contains(text, pattern)
	}
	return matched
}
