// Package model holds the normalized allowlist representation of Kubernetes
// objects. Only the extract package converts Kubernetes API types into these.
package model

import "strings"

// Image is a container image reference split into its parts. Repo and Tag are
// separated because a differing tag is expected between environments while a
// differing repo is not.
type Image struct {
	Repo   string
	Tag    string
	Digest string
}

// ParseImage splits a container image reference. An image with neither tag nor
// digest is reported with the implicit "latest" tag, matching what the runtime
// actually pulls.
func ParseImage(ref string) Image {
	if ref == "" {
		return Image{}
	}
	img := Image{}

	// Digest first: everything after "@" is the digest.
	if at := strings.LastIndex(ref, "@"); at != -1 {
		img.Digest = ref[at+1:]
		ref = ref[:at]
	}

	// A colon is only a tag separator if it appears after the final slash.
	// Otherwise it is a registry port, e.g. "registry:5000/acme/api".
	if colon := strings.LastIndex(ref, ":"); colon != -1 && colon > strings.LastIndex(ref, "/") {
		img.Tag = ref[colon+1:]
		img.Repo = ref[:colon]
	} else {
		img.Repo = ref
	}

	// A digest-pinned image has no implicit tag; the digest is the identity.
	if img.Tag == "" && img.Digest == "" {
		img.Tag = "latest"
	}
	return img
}

// String renders the reference for display.
func (i Image) String() string {
	if i.Repo == "" {
		return ""
	}
	s := i.Repo
	if i.Tag != "" {
		s += ":" + i.Tag
	}
	if i.Digest != "" {
		s += "@" + i.Digest
	}
	return s
}
