package extract

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestConfigMapTextData(t *testing.T) {
	got := ConfigMap(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "app-config", Namespace: "web"},
		Data:       map[string]string{"LOG_LEVEL": "debug", "REGION": "us-east-1"},
	})
	if got.Data["LOG_LEVEL"] != "debug" {
		t.Errorf("LOG_LEVEL = %q", got.Data["LOG_LEVEL"])
	}
	keys := got.DataKeys()
	if len(keys) != 2 || keys[0] != "LOG_LEVEL" || keys[1] != "REGION" {
		t.Errorf("DataKeys() = %v, want sorted", keys)
	}
}

// Binary blobs are summarized. Dumping them would flood the report and could
// spill key material.
func TestConfigMapBinaryDataSummarized(t *testing.T) {
	got := ConfigMap(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "certs", Namespace: "web"},
		BinaryData: map[string][]byte{"truststore.jks": []byte{0x00, 0x01, 0x02, 0x03}},
	})
	v := got.Data["truststore.jks"]
	if !strings.HasPrefix(v, "<binary, 4 bytes, sha256:") {
		t.Errorf("binary summary = %q", v)
	}
	if strings.Contains(v, "\x00") {
		t.Error("raw binary content must not appear in the model")
	}
}

// Sensitive keys are masked in ConfigMaps too — a credential parked in a
// ConfigMap never touches the Secrets API, so the guarantee does not cover it.
func TestConfigMapRedactsSensitiveKeys(t *testing.T) {
	got := ConfigMap(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "app-config", Namespace: "web"},
		Data:       map[string]string{"API_TOKEN": "abc123", "KEYCLOAK_URL": "https://sso"},
	})
	if got.Data["API_TOKEN"] == "abc123" {
		t.Fatal("credential in a ConfigMap leaked into the model")
	}
	if got.Data["KEYCLOAK_URL"] != "https://sso" {
		t.Error("KEYCLOAK_URL must not be redacted")
	}
}
