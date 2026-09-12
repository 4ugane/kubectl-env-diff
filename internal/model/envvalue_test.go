package model

import "testing"

// Two redacted values with the same fingerprint are equal; different
// fingerprints mean the underlying secret differs even though both display
// the same placeholder text.
func TestEnvValueEqualRedactedComparesByFingerprint(t *testing.T) {
	a := EnvValue{Kind: EnvInline, Value: "<redacted>", Redacted: true, Fingerprint: "aaa"}
	b := EnvValue{Kind: EnvInline, Value: "<redacted>", Redacted: true, Fingerprint: "aaa"}
	c := EnvValue{Kind: EnvInline, Value: "<redacted>", Redacted: true, Fingerprint: "bbb"}

	if !a.Equal(b) {
		t.Error("same fingerprint should be equal")
	}
	if a.Equal(c) {
		t.Error("different fingerprints should not be equal, even with identical display text")
	}
}

func TestEnvValueEqualNonRedactedComparesByDisplay(t *testing.T) {
	a := EnvValue{Kind: EnvInline, Value: "debug"}
	b := EnvValue{Kind: EnvInline, Value: "debug"}
	c := EnvValue{Kind: EnvInline, Value: "info"}

	if !a.Equal(b) {
		t.Error("identical inline values should be equal")
	}
	if a.Equal(c) {
		t.Error("differing inline values should not be equal")
	}
}

func TestEnvValueEqualMismatchedRedactionNeverEqual(t *testing.T) {
	redacted := EnvValue{Kind: EnvInline, Value: "<redacted>", Redacted: true, Fingerprint: "aaa"}
	plain := EnvValue{Kind: EnvInline, Value: "<redacted>"}

	if redacted.Equal(plain) {
		t.Error("a redacted value must never be considered equal to a non-redacted one")
	}
}
