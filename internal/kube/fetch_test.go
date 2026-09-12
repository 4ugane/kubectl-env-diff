package kube

import (
	"context"
	"errors"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/4ugane/kubectl-env-diff/internal/model"
)

func deployment(name, ns string) *appsv1.Deployment {
	return &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}
}

func TestFetchAllKinds(t *testing.T) {
	cs := fake.NewSimpleClientset(
		deployment("api", "web"),
		&appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "web"}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "app-config", Namespace: "web"},
			Data: map[string]string{"K": "v"}},
	)

	snap, err := Fetch(context.Background(), cs, "staging", "web", AllKinds)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if len(snap.Workloads) != 2 {
		t.Errorf("got %d workloads, want 2", len(snap.Workloads))
	}
	if len(snap.ConfigMaps) != 1 {
		t.Errorf("got %d configmaps, want 1", len(snap.ConfigMaps))
	}
	if snap.Context != "staging" || snap.Namespace != "web" {
		t.Errorf("snapshot identity = %q/%q", snap.Context, snap.Namespace)
	}
	if len(snap.Skipped) != 0 {
		t.Errorf("unexpected skips: %+v", snap.Skipped)
	}
}

// Wrong-namespace objects must not leak into the snapshot.
func TestFetchFiltersByNamespace(t *testing.T) {
	cs := fake.NewSimpleClientset(
		deployment("api", "web"),
		deployment("other", "kube-system"),
	)
	snap, err := Fetch(context.Background(), cs, "staging", "web", AllKinds)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Workloads) != 1 || snap.Workloads[0].Name != "api" {
		t.Errorf("workloads = %+v", snap.Workloads)
	}
}

// An empty namespace is a valid result, not an error.
func TestFetchEmptyNamespace(t *testing.T) {
	snap, err := Fetch(context.Background(), fake.NewSimpleClientset(), "staging", "web", AllKinds)
	if err != nil {
		t.Fatalf("empty namespace should not error, got %v", err)
	}
	if len(snap.Workloads) != 0 || len(snap.ConfigMaps) != 0 {
		t.Error("expected an empty snapshot")
	}
}

func TestFetchSelectedKindsOnly(t *testing.T) {
	cs := fake.NewSimpleClientset(
		deployment("api", "web"),
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "app-config", Namespace: "web"}},
	)
	snap, err := Fetch(context.Background(), cs, "staging", "web",
		[]model.Kind{model.KindDeployment})
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.ConfigMaps) != 0 {
		t.Error("ConfigMaps should not be fetched when not selected")
	}
	if len(snap.Workloads) != 1 {
		t.Errorf("got %d workloads, want 1", len(snap.Workloads))
	}
}

// A forbidden kind is recorded, not swallowed. The caller turns this into a
// loud warning and exit code 1.
func TestFetchRecordsForbiddenKind(t *testing.T) {
	cs := fake.NewSimpleClientset(deployment("api", "web"))
	cs.PrependReactor("list", "statefulsets",
		func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, apierrors.NewForbidden(
				schema.GroupResource{Group: "apps", Resource: "statefulsets"}, "",
				errors.New("no permission"))
		})

	snap, err := Fetch(context.Background(), cs, "staging", "web", AllKinds)
	if err != nil {
		t.Fatalf("a forbidden kind should be recorded, not returned as an error: %v", err)
	}
	if len(snap.Workloads) != 1 {
		t.Errorf("readable kinds should still be returned, got %d", len(snap.Workloads))
	}
	if len(snap.Skipped) != 1 || snap.Skipped[0].Kind != model.KindStatefulSet {
		t.Fatalf("skips = %+v", snap.Skipped)
	}
	if !strings.Contains(snap.Skipped[0].Reason, "forbidden") {
		t.Errorf("reason should explain the failure, got %q", snap.Skipped[0].Reason)
	}
}

// A non-permission failure is a real error: the result would be wrong, not
// merely partial.
func TestFetchReturnsErrorOnUnexpectedFailure(t *testing.T) {
	cs := fake.NewSimpleClientset()
	cs.PrependReactor("list", "deployments",
		func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, apierrors.NewInternalError(errors.New("boom"))
		})

	if _, err := Fetch(context.Background(), cs, "staging", "web", AllKinds); err == nil {
		t.Fatal("an internal server error should fail the run")
	}
}

func TestFetchPaginates(t *testing.T) {
	var objs []runtime.Object
	for i := 0; i < 750; i++ {
		objs = append(objs, deployment("api-"+itoa(i), "web"))
	}
	snap, err := Fetch(context.Background(), fake.NewSimpleClientset(objs...),
		"staging", "web", []model.Kind{model.KindDeployment})
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Workloads) != 750 {
		t.Errorf("got %d workloads, want 750 (pagination dropped items)", len(snap.Workloads))
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func TestParseTarget(t *testing.T) {
	tests := []struct {
		in      string
		ctx, ns string
		wantErr bool
	}{
		{"staging/web", "staging", "web", false},
		{"staging", "staging", "", false},
		{"prod-au/web", "prod-au", "web", false},
		{"", "", "", true},
		{"/platform", "", "", true},
		{"staging/", "", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got, err := ParseTarget(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseTarget(%q) should error", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("error = %v", err)
			}
			if got.Context != tc.ctx || got.Namespace != tc.ns {
				t.Errorf("got %q/%q, want %q/%q", got.Context, got.Namespace, tc.ctx, tc.ns)
			}
		})
	}
}
