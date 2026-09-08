package model

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
)

func q(s string) resource.Quantity { return resource.MustParse(s) }

func TestResourceListEqual(t *testing.T) {
	tests := []struct {
		name  string
		a, b  ResourceList
		equal bool
	}{
		{"identical", ResourceList{"memory": q("1Gi")}, ResourceList{"memory": q("1Gi")}, true},
		{"same amount different unit", ResourceList{"memory": q("1Gi")}, ResourceList{"memory": q("1024Mi")}, true},
		{"cpu millis vs decimal", ResourceList{"cpu": q("500m")}, ResourceList{"cpu": q("0.5")}, true},
		{"different amount", ResourceList{"memory": q("1Gi")}, ResourceList{"memory": q("512Mi")}, false},
		{"both empty", ResourceList{}, ResourceList{}, true},
		{"nil vs empty", nil, ResourceList{}, true},
		{"absent is not zero", nil, ResourceList{"cpu": q("0")}, false},
		{"extra key", ResourceList{"cpu": q("1")}, ResourceList{"cpu": q("1"), "memory": q("1Gi")}, false},
		{"gpu resource", ResourceList{"nvidia.com/gpu": q("1")}, ResourceList{"nvidia.com/gpu": q("1")}, true},
		{"ephemeral storage differs", ResourceList{"ephemeral-storage": q("1Gi")}, ResourceList{"ephemeral-storage": q("2Gi")}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.a.Equal(tc.b); got != tc.equal {
				t.Errorf("Equal() = %v, want %v", got, tc.equal)
			}
		})
	}
}

// Keys must be sorted or output is nondeterministic across runs.
func TestResourceListKeysSorted(t *testing.T) {
	rl := ResourceList{"memory": q("1Gi"), "cpu": q("1"), "nvidia.com/gpu": q("1")}
	got := rl.Keys()
	want := []string{"cpu", "memory", "nvidia.com/gpu"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestResourceListGet(t *testing.T) {
	rl := ResourceList{"cpu": q("500m")}
	if v, ok := rl.Get("cpu"); !ok || v != "500m" {
		t.Errorf("Get(cpu) = %q, %v", v, ok)
	}
	if _, ok := rl.Get("memory"); ok {
		t.Error("Get(memory) should report absent")
	}
}
