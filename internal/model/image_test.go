package model

import "testing"

func TestParseImage(t *testing.T) {
	tests := []struct {
		name, in          string
		repo, tag, digest string
	}{
		{"tag only", "nginx:1.25", "nginx", "1.25", ""},
		{"implicit latest", "nginx", "nginx", "latest", ""},
		{"registry with port", "registry:5000/acme/api:v1", "registry:5000/acme/api", "v1", ""},
		{"registry with port, no tag", "registry:5000/acme/api", "registry:5000/acme/api", "latest", ""},
		{"digest", "acme/api@sha256:abc123", "acme/api", "", "sha256:abc123"},
		{"tag and digest", "acme/api:v1@sha256:abc123", "acme/api", "v1", "sha256:abc123"},
		{"fully qualified", "ghcr.io/acme/api:v1.2.3", "ghcr.io/acme/api", "v1.2.3", ""},
		{"empty", "", "", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseImage(tc.in)
			if got.Repo != tc.repo || got.Tag != tc.tag || got.Digest != tc.digest {
				t.Errorf("ParseImage(%q) = {%q %q %q}, want {%q %q %q}",
					tc.in, got.Repo, got.Tag, got.Digest, tc.repo, tc.tag, tc.digest)
			}
		})
	}
}

func TestImageString(t *testing.T) {
	if got := (Image{Repo: "nginx", Tag: "1.25"}).String(); got != "nginx:1.25" {
		t.Errorf("got %q", got)
	}
	if got := (Image{Repo: "api", Digest: "sha256:ab"}).String(); got != "api@sha256:ab" {
		t.Errorf("got %q", got)
	}
	if got := (Image{}).String(); got != "" {
		t.Errorf("got %q", got)
	}
}
