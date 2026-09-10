package extract

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/4ugane/kubectl-env-diff/internal/model"
)

func TestContainerImageAndResources(t *testing.T) {
	in := corev1.Container{
		Name:  "api",
		Image: "ghcr.io/acme/api:v1.2.3",
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{"cpu": resource.MustParse("500m")},
			Limits:   corev1.ResourceList{"memory": resource.MustParse("1Gi")},
		},
	}
	got := Container(in)

	if got.Image.Repo != "ghcr.io/acme/api" || got.Image.Tag != "v1.2.3" {
		t.Errorf("image = %+v", got.Image)
	}
	if v, ok := got.Requests.Get("cpu"); !ok || v != "500m" {
		t.Errorf("cpu request = %q %v", v, ok)
	}
	if _, ok := got.Limits.Get("cpu"); ok {
		t.Error("absent cpu limit must stay absent, not become zero")
	}
}

// Kubernetes takes the last value when an env name repeats. So must we.
func TestContainerDuplicateEnvLastWins(t *testing.T) {
	in := corev1.Container{
		Name: "api",
		Env: []corev1.EnvVar{
			{Name: "LOG_LEVEL", Value: "debug"},
			{Name: "LOG_LEVEL", Value: "info"},
		},
	}
	got := Container(in)
	if got.Env["LOG_LEVEL"].Value != "info" {
		t.Errorf("LOG_LEVEL = %q, want info (last wins)", got.Env["LOG_LEVEL"].Value)
	}
}

func TestContainerEnvValueFromShapes(t *testing.T) {
	in := corev1.Container{
		Name: "api",
		Env: []corev1.EnvVar{
			{Name: "PLAIN", Value: "hello"},
			{Name: "FROM_CM", ValueFrom: &corev1.EnvVarSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "app-config"},
					Key:                  "LOG_LEVEL",
				}}},
			{Name: "FROM_SECRET", ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "db-creds"},
					Key:                  "PASSWORD",
				}}},
			{Name: "POD_IP", ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{FieldPath: "status.podIP"}}},
			{Name: "MEM_LIMIT", ValueFrom: &corev1.EnvVarSource{
				ResourceFieldRef: &corev1.ResourceFieldSelector{
					ContainerName: "api", Resource: "limits.memory"}}},
		},
	}
	got := Container(in)

	checks := []struct {
		key    string
		kind   model.EnvKind
		source string
	}{
		{"PLAIN", model.EnvInline, ""},
		{"FROM_CM", model.EnvConfigMapKey, "configmap/app-config:LOG_LEVEL"},
		{"FROM_SECRET", model.EnvSecretKey, "secret/db-creds:PASSWORD"},
		{"POD_IP", model.EnvFieldRef, "field:status.podIP"},
		{"MEM_LIMIT", model.EnvResourceField, "resource:api:limits.memory"},
	}
	for _, c := range checks {
		ev, ok := got.Env[c.key]
		if !ok {
			t.Fatalf("%s missing", c.key)
		}
		if ev.Kind != c.kind {
			t.Errorf("%s kind = %q, want %q", c.key, ev.Kind, c.kind)
		}
		if c.source != "" && ev.Source != c.source {
			t.Errorf("%s source = %q, want %q", c.key, ev.Source, c.source)
		}
	}
}

// The raw credential must never enter the model.
func TestContainerRedactsInlineSecrets(t *testing.T) {
	in := corev1.Container{
		Name: "api",
		Env: []corev1.EnvVar{
			{Name: "DB_PASSWORD", Value: "hunter2"},
			{Name: "KEYCLOAK_URL", Value: "https://sso.example.com"},
		},
	}
	got := Container(in)

	pw := got.Env["DB_PASSWORD"]
	if !pw.Redacted {
		t.Error("DB_PASSWORD should be flagged redacted")
	}
	if pw.Value == "hunter2" {
		t.Fatal("raw credential leaked into the model")
	}
	if got.Env["KEYCLOAK_URL"].Value != "https://sso.example.com" {
		t.Error("KEYCLOAK_URL must not be redacted")
	}
}

func TestContainerEnvFrom(t *testing.T) {
	in := corev1.Container{
		Name: "api",
		EnvFrom: []corev1.EnvFromSource{
			{ConfigMapRef: &corev1.ConfigMapEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: "app-config"}}},
			{SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: "db-creds"}}},
		},
	}
	got := Container(in)
	want := []string{"configmap/app-config", "secret/db-creds"}
	if len(got.EnvFrom) != 2 || got.EnvFrom[0] != want[0] || got.EnvFrom[1] != want[1] {
		t.Errorf("EnvFrom = %v, want %v", got.EnvFrom, want)
	}
}

func TestContainerProbeTypes(t *testing.T) {
	tests := []struct {
		name   string
		probe  *corev1.Probe
		typ    string
		target string
	}{
		{"httpGet", &corev1.Probe{ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{Path: "/healthz", Port: intstrFromInt(8080)}}},
			"httpGet", "/healthz:8080"},
		{"tcpSocket", &corev1.Probe{ProbeHandler: corev1.ProbeHandler{
			TCPSocket: &corev1.TCPSocketAction{Port: intstrFromInt(5432)}}},
			"tcpSocket", "5432"},
		{"exec", &corev1.Probe{ProbeHandler: corev1.ProbeHandler{
			Exec: &corev1.ExecAction{Command: []string{"sh", "-c", "pg_isready"}}}},
			"exec", "sh -c pg_isready"},
		{"grpc", &corev1.Probe{ProbeHandler: corev1.ProbeHandler{
			GRPC: &corev1.GRPCAction{Port: 9000}}},
			"grpc", "9000"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Probe(tc.probe)
			if !got.Present {
				t.Fatal("probe should be present")
			}
			if got.Type != tc.typ || got.Target != tc.target {
				t.Errorf("got {%q %q}, want {%q %q}", got.Type, got.Target, tc.typ, tc.target)
			}
		})
	}
}

// Absent must be distinguishable from empty, or "probe removed in prod" reads
// as no change at all.
func TestProbeAbsent(t *testing.T) {
	got := Probe(nil)
	if got.Present {
		t.Error("nil probe must report Present=false")
	}
}

func intstrFromInt(i int32) intstr.IntOrString {
	return intstr.FromInt32(i)
}
