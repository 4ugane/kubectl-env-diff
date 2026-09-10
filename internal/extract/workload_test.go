package extract

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/4ugane/kubectl-env-diff/internal/model"
)

func int32p(i int32) *int32 { return &i }

func TestDeploymentReplicasDefault(t *testing.T) {
	// A nil replicas field means 1 at runtime, so nil and explicit 1 are the
	// same configuration and must not report as drift.
	nilRep := Deployment(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "web"},
		Spec:       appsv1.DeploymentSpec{Replicas: nil},
	})
	oneRep := Deployment(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "web"},
		Spec:       appsv1.DeploymentSpec{Replicas: int32p(1)},
	})
	if nilRep.Replicas != 1 {
		t.Errorf("nil replicas = %d, want 1", nilRep.Replicas)
	}
	if nilRep.Replicas != oneRep.Replicas {
		t.Error("nil and explicit 1 must normalize to the same value")
	}
}

func TestDeploymentInitContainers(t *testing.T) {
	d := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "web"},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32p(3),
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				ServiceAccountName: "api-sa",
				InitContainers: []corev1.Container{
					{Name: "migrate", Image: "acme/migrate:v1"},
				},
				Containers: []corev1.Container{
					{Name: "api", Image: "acme/api:v2"},
				},
			}},
		},
	}
	got := Deployment(d)

	if got.Kind != model.KindDeployment {
		t.Errorf("kind = %q", got.Kind)
	}
	if got.ServiceAccount != "api-sa" {
		t.Errorf("serviceAccount = %q", got.ServiceAccount)
	}
	if len(got.Containers) != 1 || got.Containers[0].Name != "api" {
		t.Errorf("containers = %+v", got.Containers)
	}
	if len(got.InitContainers) != 1 || got.InitContainers[0].Name != "migrate" {
		t.Errorf("initContainers = %+v", got.InitContainers)
	}
}

// References are collected from every place they can appear, sorted and
// deduplicated so output is stable.
func TestDeploymentCollectsReferences(t *testing.T) {
	d := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "web"},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				Volumes: []corev1.Volume{
					{Name: "cfg", VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{Name: "vol-config"}}}},
					{Name: "sec", VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{SecretName: "vol-secret"}}},
				},
				Containers: []corev1.Container{{
					Name: "api",
					EnvFrom: []corev1.EnvFromSource{
						{ConfigMapRef: &corev1.ConfigMapEnvSource{
							LocalObjectReference: corev1.LocalObjectReference{Name: "envfrom-config"}}},
					},
					Env: []corev1.EnvVar{
						{Name: "P", ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{Name: "env-secret"},
								Key:                  "PASSWORD"}}},
						// duplicate reference must be deduplicated
						{Name: "Q", ValueFrom: &corev1.EnvVarSource{
							ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{Name: "vol-config"},
								Key:                  "K"}}},
					},
				}},
			}},
		},
	}
	got := Deployment(d)

	wantCM := []string{"envfrom-config", "vol-config"}
	if len(got.ConfigMapRefs) != len(wantCM) {
		t.Fatalf("configMapRefs = %v, want %v", got.ConfigMapRefs, wantCM)
	}
	for i := range wantCM {
		if got.ConfigMapRefs[i] != wantCM[i] {
			t.Fatalf("configMapRefs = %v, want %v (sorted, deduplicated)", got.ConfigMapRefs, wantCM)
		}
	}
	wantSec := []string{"env-secret", "vol-secret"}
	for i := range wantSec {
		if got.SecretRefs[i] != wantSec[i] {
			t.Fatalf("secretRefs = %v, want %v", got.SecretRefs, wantSec)
		}
	}
}

func TestStatefulSet(t *testing.T) {
	got := StatefulSet(&appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "web"},
		Spec: appsv1.StatefulSetSpec{
			Replicas: int32p(3),
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "db", Image: "postgres:16"}},
			}},
		},
	})
	if got.Kind != model.KindStatefulSet || got.Replicas != 3 {
		t.Errorf("got %+v", got)
	}
}
