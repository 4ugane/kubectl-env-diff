package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/4ugane/kubectl-env-diff/internal/classify"
	"github.com/4ugane/kubectl-env-diff/internal/compare"
	"github.com/4ugane/kubectl-env-diff/internal/config"
	"github.com/4ugane/kubectl-env-diff/internal/kube"
	"github.com/4ugane/kubectl-env-diff/internal/model"
	"github.com/4ugane/kubectl-env-diff/internal/report"
)

func i32(i int32) *int32 { return &i }

// deploy builds a Deployment with one container.
func deploy(name, ns, image string, replicas int32, env map[string]string, mem string) *appsv1.Deployment {
	var vars []corev1.EnvVar
	for k, v := range env {
		vars = append(vars, corev1.EnvVar{Name: k, Value: v})
	}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: appsv1.DeploymentSpec{
			Replicas: i32(replicas),
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				Containers: []corev1.Container{{
					Name:  "app",
					Image: image,
					Env:   vars,
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{"memory": resource.MustParse(mem)},
					},
				}},
			}},
		},
	}
}

// runComparison exercises the full pipeline against two fake clusters.
func runComparison(t *testing.T, from, to []runtime.Object,
	opts compare.Options, cfg *config.Config) report.Report {
	t.Helper()

	fromCS := fake.NewSimpleClientset(from...)
	toCS := fake.NewSimpleClientset(to...)

	ctx := context.Background()
	fromSnap, err := kube.Fetch(ctx, fromCS, "staging", "web", kube.AllKinds)
	if err != nil {
		t.Fatalf("fetch from: %v", err)
	}
	toSnap, err := kube.Fetch(ctx, toCS, "prod", "web", kube.AllKinds)
	if err != nil {
		t.Fatalf("fetch to: %v", err)
	}

	pairs, err := compare.Pairs(fromSnap, toSnap, opts)
	if err != nil {
		t.Fatalf("pair: %v", err)
	}
	diffs := classify.Apply(compare.DiffAll(pairs), cfg.Ignore)
	skipped := append(append([]model.SkipNote{}, fromSnap.Skipped...), toSnap.Skipped...)

	return report.Build(report.Meta{
		FromContext: "staging", FromNamespace: "web",
		ToContext: "prod", ToNamespace: "web",
	}, pairs, diffs, skipped)
}

func TestEndToEndDriftDetected(t *testing.T) {
	from := []runtime.Object{
		deploy("checkout", "web", "acme/checkout:v1", 3,
			map[string]string{"LOG_LEVEL": "debug"}, "4Gi"),
		deploy("notifier", "web", "acme/hook:v1", 1, nil, "256Mi"),
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "app-config", Namespace: "web"},
			Data:       map[string]string{"REGION": "us-east-1"},
		},
	}
	to := []runtime.Object{
		deploy("checkout", "web", "acme/checkout:v2", 1,
			map[string]string{"LOG_LEVEL": "info"}, "512Mi"),
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "app-config", Namespace: "web"},
			Data:       map[string]string{"REGION": "us-east-1", "KAFKA_BROKERS": "b-1:9092"},
		},
	}

	rep := runComparison(t, from, to, compare.Options{}, &config.Config{})

	if !rep.HasDrift() {
		t.Fatal("expected drift")
	}
	if rep.Incomplete() {
		t.Error("comparison should be complete")
	}
	if got := ExitCode(rep.HasDrift(), rep.Incomplete()); got != 2 {
		t.Errorf("exit code = %d, want 2", got)
	}

	paths := map[string]model.Difference{}
	for _, d := range rep.Differences {
		paths[d.Path] = d
	}

	// Memory limit drift is the headline finding.
	if d, ok := paths["container[app].resources.limits.memory"]; !ok {
		t.Error("memory limit drift missed")
	} else if d.From != "4Gi" || d.To != "512Mi" {
		t.Errorf("memory drift = %q -> %q", d.From, d.To)
	}

	// New ConfigMap key present only in prod.
	if d, ok := paths["data.KAFKA_BROKERS"]; !ok {
		t.Error("new ConfigMap key missed")
	} else if d.Type != model.MissingInFrom {
		t.Errorf("KAFKA_BROKERS type = %s", d.Type)
	}

	// Image tag drift is expected, not drift.
	if d, ok := paths["container[app].image.tag"]; !ok {
		t.Error("image tag difference missed")
	} else if d.Severity != model.SeverityExpected {
		t.Errorf("image tag severity = %s, want Expected", d.Severity)
	}

	// Whole workload absent in prod.
	if len(rep.Missing(model.MissingInTo)) != 1 {
		t.Errorf("expected notifier reported missing, got %+v", rep.Missing(model.MissingInTo))
	}
}

// The end-to-end guarantee: no credential value reaches any output format.
func TestEndToEndNeverLeaksCredentials(t *testing.T) {
	from := []runtime.Object{
		deploy("api", "web", "acme/api:v1", 1,
			map[string]string{"DB_PASSWORD": "staging-p4ssw0rd", "API_TOKEN": "tok-staging"}, "1Gi"),
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "app-config", Namespace: "web"},
			Data:       map[string]string{"CLIENT_SECRET": "cs-staging"},
		},
	}
	to := []runtime.Object{
		deploy("api", "web", "acme/api:v1", 1,
			map[string]string{"DB_PASSWORD": "prod-p4ssw0rd", "API_TOKEN": "tok-prod"}, "1Gi"),
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "app-config", Namespace: "web"},
			Data:       map[string]string{"CLIENT_SECRET": "cs-prod"},
		},
	}

	rep := runComparison(t, from, to, compare.Options{}, &config.Config{})

	secrets := []string{
		"staging-p4ssw0rd", "prod-p4ssw0rd",
		"tok-staging", "tok-prod", "cs-staging", "cs-prod",
	}

	renderers := map[string]func(*bytes.Buffer) error{
		"text":     func(b *bytes.Buffer) error { return report.Text(b, rep, report.TextOptions{Width: 120}) },
		"json":     func(b *bytes.Buffer) error { return report.JSON(b, rep) },
		"markdown": func(b *bytes.Buffer) error { return report.Markdown(b, rep, true) },
		"html":     func(b *bytes.Buffer) error { return report.HTML(b, rep, true) },
	}
	for name, render := range renderers {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := render(&buf); err != nil {
				t.Fatalf("render: %v", err)
			}
			out := buf.String()
			for _, s := range secrets {
				if strings.Contains(out, s) {
					t.Fatalf("%s output leaked credential %q", name, s)
				}
			}
			if !strings.Contains(out, "redacted") {
				t.Errorf("%s output should mark redacted values", name)
			}
		})
	}
}

func TestEndToEndIdenticalNamespaces(t *testing.T) {
	build := func() []runtime.Object {
		return []runtime.Object{
			deploy("api", "web", "acme/api:v1", 2, map[string]string{"LOG_LEVEL": "info"}, "1Gi"),
		}
	}
	rep := runComparison(t, build(), build(), compare.Options{}, &config.Config{})

	if rep.HasDrift() {
		t.Errorf("identical namespaces should report no drift, got %+v", rep.DriftOnly())
	}
	if got := ExitCode(rep.HasDrift(), rep.Incomplete()); got != 0 {
		t.Errorf("exit code = %d, want 0", got)
	}
}

// 1Gi vs 1024Mi is the same amount and must not report as drift end to end.
func TestEndToEndEquivalentQuantities(t *testing.T) {
	from := []runtime.Object{deploy("api", "web", "acme/api:v1", 1, nil, "1Gi")}
	to := []runtime.Object{deploy("api", "web", "acme/api:v1", 1, nil, "1024Mi")}

	rep := runComparison(t, from, to, compare.Options{}, &config.Config{})
	if rep.HasDrift() {
		t.Errorf("1Gi vs 1024Mi should be equal, got %+v", rep.DriftOnly())
	}
}

// Names differ per environment: the explicit-pair case.
func TestEndToEndExplicitNamePair(t *testing.T) {
	from := []runtime.Object{
		deploy("test-staging", "web", "acme/test:v1", 1,
			map[string]string{"LOG_LEVEL": "debug"}, "1Gi"),
	}
	to := []runtime.Object{
		deploy("test-prod", "web", "acme/test:v1", 1,
			map[string]string{"LOG_LEVEL": "info"}, "1Gi"),
	}

	rep := runComparison(t, from, to,
		compare.Options{FromName: "test-staging", ToName: "test-prod"}, &config.Config{})

	if !rep.HasDrift() {
		t.Fatal("expected LOG_LEVEL drift")
	}
	found := false
	for _, d := range rep.Differences {
		if d.Path == "container[app].env.LOG_LEVEL" {
			found = true
			if d.From != "debug" || d.To != "info" {
				t.Errorf("LOG_LEVEL = %q -> %q", d.From, d.To)
			}
		}
	}
	if !found {
		t.Errorf("LOG_LEVEL drift missed: %+v", rep.Differences)
	}
}

func TestEndToEndIgnoreRuleSuppresses(t *testing.T) {
	from := []runtime.Object{
		deploy("api", "web", "acme/api:v1", 1,
			map[string]string{"DATADOG_ENV": "staging", "LOG_LEVEL": "debug"}, "1Gi"),
	}
	to := []runtime.Object{
		deploy("api", "web", "acme/api:v1", 1,
			map[string]string{"DATADOG_ENV": "prod", "LOG_LEVEL": "debug"}, "1Gi"),
	}

	cfg := &config.Config{Ignore: []config.IgnoreRule{{Path: "container[*].env.DATADOG_ENV"}}}
	rep := runComparison(t, from, to, compare.Options{}, cfg)

	if rep.HasDrift() {
		t.Errorf("the only difference was ignored, so no drift should remain: %+v", rep.DriftOnly())
	}
	if got := ExitCode(rep.HasDrift(), rep.Incomplete()); got != 0 {
		t.Errorf("exit code = %d, want 0", got)
	}
}

// Normalization pairs api-staging with api-prod.
func TestEndToEndNormalization(t *testing.T) {
	from := []runtime.Object{
		deploy("api-staging", "web", "acme/api:v1", 1,
			map[string]string{"LOG_LEVEL": "debug"}, "1Gi"),
	}
	to := []runtime.Object{
		deploy("api-prod", "web", "acme/api:v1", 1,
			map[string]string{"LOG_LEVEL": "info"}, "1Gi"),
	}

	opts := compare.Options{Normalize: config.NormalizeRules{
		StripSuffixes: []string{"-staging", "-prod"}}}
	rep := runComparison(t, from, to, opts, &config.Config{})

	if len(rep.Missing(model.MissingInTo)) != 0 || len(rep.Missing(model.MissingInFrom)) != 0 {
		t.Error("normalized names should pair, not report as missing")
	}
	if !rep.HasDrift() {
		t.Error("expected LOG_LEVEL drift on the paired resource")
	}
}
