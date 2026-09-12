package extract

import (
	"crypto/sha256"
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/4ugane/kubectl-env-diff/internal/model"
	"github.com/4ugane/kubectl-env-diff/internal/redact"
)

// ConfigMap projects a ConfigMap onto the comparable model. Binary entries are
// summarized by size and hash rather than included, and sensitive keys are
// masked — a credential parked in a ConfigMap is not covered by the
// never-read-Secrets guarantee.
func ConfigMap(cm *corev1.ConfigMap) model.ConfigMap {
	out := model.ConfigMap{
		Kind:      model.KindConfigMap,
		Name:      cm.Name,
		Namespace: cm.Namespace,
		Data:      make(map[string]string, len(cm.Data)+len(cm.BinaryData)),
		Immutable: cm.Immutable != nil && *cm.Immutable,
	}

	for k, v := range cm.Data {
		out.Data[k] = redact.Value(k, v)
		if redact.IsSensitive(k) {
			if out.Fingerprints == nil {
				out.Fingerprints = make(map[string]string, len(cm.Data))
			}
			out.Fingerprints[k] = redact.Fingerprint(v)
		}
	}
	for k, v := range cm.BinaryData {
		sum := sha256.Sum256(v)
		out.Data[k] = fmt.Sprintf("<binary, %d bytes, sha256:%x>", len(v), sum[:8])
	}
	return out
}
