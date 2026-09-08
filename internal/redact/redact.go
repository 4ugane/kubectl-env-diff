// Package redact masks values whose key name suggests a credential.
//
// It exists because the tool's "never call the Secrets API" guarantee does not
// cover a plaintext credential sitting inline in a Deployment's env block or in
// a ConfigMap. Matching is by token rather than substring: substring matching on
// "key" would mask KEYCLOAK_URL, and hiding real drift is itself a bug.
package redact

import (
	"strings"
	"unicode"
)

// Placeholder replaces a sensitive value. Differences use PlaceholderDiffers so
// the reader learns that a credential drifted without learning its value.
const (
	Placeholder        = "<redacted>"
	PlaceholderDiffers = "<redacted, differs>"
)

// sensitiveTokens are matched against whole tokens of a key name.
var sensitiveTokens = map[string]bool{
	"password": true, "passwd": true, "pass": true,
	"secret": true, "secrets": true,
	"token": true, "tokens": true,
	"credential": true, "credentials": true, "creds": true,
	"key": true, "keys": true, "apikey": true,
	"pat": true, "privatekey": true, "cert": true,
	"signature": true, "salt": true,
}

// tokenize splits a key on separators and camelCase boundaries, lowercasing the
// result. "AWSSecretKey" becomes [aws secret key].
func tokenize(key string) []string {
	fields := strings.FieldsFunc(key, func(r rune) bool {
		return r == '_' || r == '-' || r == '.' || r == '/' || r == ' '
	})

	var out []string
	for _, f := range fields {
		for _, part := range splitCamel(f) {
			if part != "" {
				out = append(out, strings.ToLower(part))
			}
		}
	}
	return out
}

// splitCamel breaks camelCase and consecutive-capital runs into words.
func splitCamel(s string) []string {
	runes := []rune(s)
	var parts []string
	start := 0
	for i := 1; i < len(runes); i++ {
		prev, cur := runes[i-1], runes[i]
		// lower→upper is a boundary: authToken
		lowerToUpper := unicode.IsLower(prev) && unicode.IsUpper(cur)
		// UPPER→Upper+lower is a boundary: AWSSecret
		upperRunEnd := unicode.IsUpper(prev) && unicode.IsUpper(cur) &&
			i+1 < len(runes) && unicode.IsLower(runes[i+1])
		if lowerToUpper || upperRunEnd {
			parts = append(parts, string(runes[start:i]))
			start = i
		}
	}
	return append(parts, string(runes[start:]))
}

// IsSensitive reports whether a key name indicates a credential.
func IsSensitive(key string) bool {
	for _, tok := range tokenize(key) {
		if sensitiveTokens[tok] {
			return true
		}
	}
	return false
}

// Value returns the value to display for a key, masking it when the key looks
// sensitive. An empty sensitive value is still masked, so that "set here, unset
// there" does not leak by implication.
func Value(key, value string) string {
	if IsSensitive(key) {
		return Placeholder
	}
	return value
}
