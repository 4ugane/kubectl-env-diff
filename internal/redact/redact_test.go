package redact

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestIsSensitive(t *testing.T) {
	sensitive := []string{
		"PASSWORD", "DB_PASSWORD", "db.passwd", "API_KEY", "api-key",
		"authToken", "AUTH_TOKEN", "CLIENT_SECRET", "AWS_SECRET_ACCESS_KEY",
		"DATABASE_CREDENTIALS", "APIKEY", "privateKey", "GITHUB_PAT",
	}
	for _, k := range sensitive {
		t.Run("sensitive/"+k, func(t *testing.T) {
			if !IsSensitive(k) {
				t.Errorf("IsSensitive(%q) = false, want true", k)
			}
		})
	}

	// Over-redaction hides real drift. These must NOT match.
	safe := []string{
		"KEYCLOAK_URL", "MONKEY_NAME", "KEYSPACE", "TURKEY_REGION",
		"LOG_LEVEL", "REPLICA_COUNT", "KAFKA_BROKERS", "SERVICE_NAME",
		"KEYBOARD_LAYOUT", "PASSTHROUGH_MODE", "TOKENIZER_PATH",
	}
	for _, k := range safe {
		t.Run("safe/"+k, func(t *testing.T) {
			if IsSensitive(k) {
				t.Errorf("IsSensitive(%q) = true, want false (over-redaction)", k)
			}
		})
	}
}

func TestTokenize(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"DB_PASSWORD", []string{"db", "password"}},
		{"authToken", []string{"auth", "token"}},
		{"api-key", []string{"api", "key"}},
		{"db.passwd", []string{"db", "passwd"}},
		{"AWSSecretKey", []string{"aws", "secret", "key"}},
		{"KEYCLOAK_URL", []string{"keycloak", "url"}},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got := tokenize(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("tokenize(%q) = %v, want %v", tc.in, got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("tokenize(%q) = %v, want %v", tc.in, got, tc.want)
				}
			}
		})
	}
}

func TestValue(t *testing.T) {
	if got := Value("LOG_LEVEL", "debug"); got != "debug" {
		t.Errorf("non-sensitive value should pass through, got %q", got)
	}
	if got := Value("DB_PASSWORD", "hunter2"); got != Placeholder {
		t.Errorf("sensitive value = %q, want %q", got, Placeholder)
	}
	if got := Value("DB_PASSWORD", ""); got != Placeholder {
		t.Errorf("empty sensitive value must still be masked, got %q", got)
	}
}

func TestFingerprint(t *testing.T) {
	if got := Fingerprint("hunter2"); got != Fingerprint("hunter2") {
		t.Errorf("fingerprint must be stable for equal input, got %q vs %q", got, Fingerprint("hunter2"))
	}
	if Fingerprint("hunter2") == Fingerprint("hunter3") {
		t.Error("fingerprint must differ for different input")
	}
	if got := Fingerprint("hunter2"); strings.Contains(got, "hunter2") {
		t.Errorf("fingerprint must not contain the raw value, got %q", got)
	}
}

// TestFingerprintIsKeyedNotBareHash guards against silently regressing to an
// unsalted hash: a bare SHA-256 is dictionary/rainbow-table crackable for a
// low-entropy secret, which a per-process-keyed HMAC is not.
func TestFingerprintIsKeyedNotBareHash(t *testing.T) {
	bare := sha256.Sum256([]byte("hunter2"))
	if got := Fingerprint("hunter2"); got == hex.EncodeToString(bare[:]) {
		t.Error("fingerprint must not equal a bare unsalted SHA-256 of the input")
	}
}
