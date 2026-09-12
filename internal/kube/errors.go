package kube

import (
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/4ugane/kubectl-env-diff/internal/model"
)

// skipNote converts a permission error into a recorded skip. Only permission
// and not-found errors are recoverable; anything else means the result would be
// wrong, not partial.
func skipNote(kind model.Kind, contextName string, err error) (model.SkipNote, bool) {
	switch {
	case apierrors.IsForbidden(err), apierrors.IsUnauthorized(err):
		return model.SkipNote{
			Kind:   kind,
			Reason: fmt.Sprintf("forbidden: cannot list %ss in %s", kind, contextName),
		}, true
	case apierrors.IsNotFound(err):
		return model.SkipNote{
			Kind:   kind,
			Reason: fmt.Sprintf("not found: the %s API is unavailable in %s", kind, contextName),
		}, true
	}
	return model.SkipNote{}, false
}

// MinimalRole is printed alongside a permission error so the operator can fix
// it without guessing. It grants exactly what the tool needs, and deliberately
// no access to secrets.
const MinimalRole = `apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: env-diff-reader
  namespace: <NAMESPACE>
rules:
  - apiGroups: ["apps"]
    resources: ["deployments", "statefulsets"]
    verbs: ["get", "list"]
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["get", "list"]
# No secrets rule. kubectl-env-diff never reads Secret values.`
