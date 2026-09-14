package model

import "sort"

// Kind is a supported resource kind.
type Kind string

const (
	KindDeployment  Kind = "Deployment"
	KindStatefulSet Kind = "StatefulSet"
	KindConfigMap   Kind = "ConfigMap"
)

// EnvKind records how an environment variable gets its value. Reference kinds
// are never value-compared, because a fieldRef resolves differently per pod by
// design and is not drift.
type EnvKind string

const (
	EnvInline        EnvKind = "inline"
	EnvConfigMapKey  EnvKind = "configMapKeyRef"
	EnvSecretKey     EnvKind = "secretKeyRef"
	EnvFieldRef      EnvKind = "fieldRef"
	EnvResourceField EnvKind = "resourceFieldRef"
)

// EnvValue is one environment variable. For inline values, Value is already
// redacted when the key looks sensitive — the raw value is never stored.
//
// Fingerprint is a one-way marker of a redacted value's original content,
// present only when Redacted is true. It exists so that two masked values can
// be compared for equality without either raw value ever being stored or
// displayed — without it, every redacted value would look identical and a
// rotated credential would silently vanish from the report.
type EnvValue struct {
	Kind        EnvKind
	Value       string
	Source      string
	Optional    bool
	Redacted    bool
	Fingerprint string `json:"-"`
}

// Equal reports whether two EnvValues represent the same underlying value.
// Redacted values are compared by fingerprint, never by their shared
// placeholder text, so a differing credential is still detected as different.
func (e EnvValue) Equal(o EnvValue) bool {
	if e.Redacted || o.Redacted {
		return e.Redacted == o.Redacted && e.Fingerprint == o.Fingerprint
	}
	return e.Display() == o.Display()
}

// Display renders the value for output.
func (e EnvValue) Display() string {
	if e.Kind == EnvInline {
		return e.Value
	}
	return e.Source
}

// Probe is a normalized health check. Present distinguishes "no probe
// configured" from "a probe with zero-valued fields".
type Probe struct {
	Present             bool
	Type                string
	Target              string
	InitialDelaySeconds int32
	PeriodSeconds       int32
	TimeoutSeconds      int32
	FailureThreshold    int32
	SuccessThreshold    int32
}

// Container is the allowlist of container fields worth comparing.
type Container struct {
	Name      string
	Image     Image
	Env       map[string]EnvValue
	EnvFrom   []string
	Requests  ResourceList
	Limits    ResourceList
	Liveness  Probe
	Readiness Probe
	Startup   Probe
	Command   []string
	Args      []string
}

// EnvKeys returns env var names in sorted order. Never range over Env directly
// when producing output; map order is random.
func (c Container) EnvKeys() []string {
	keys := make([]string, 0, len(c.Env))
	for k := range c.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Workload is a Deployment or StatefulSet.
type Workload struct {
	Kind           Kind
	Name           string
	Namespace      string
	Replicas       int32
	ServiceAccount string
	Containers     []Container
	InitContainers []Container
	ConfigMapRefs  []string
	SecretRefs     []string
}

// ConfigMap holds comparable ConfigMap contents. Binary entries are summarized,
// never dumped.
//
// Fingerprints holds a one-way marker for each key masked in Data because its
// name looked sensitive, keyed the same as Data. It lets two masked entries be
// compared for equality without the raw value ever being stored or displayed;
// a key absent from Fingerprints was not masked.
type ConfigMap struct {
	Kind         Kind
	Name         string
	Namespace    string
	Data         map[string]string
	Fingerprints map[string]string `json:"-"`
	Immutable    bool
}

// DataKeys returns ConfigMap keys in sorted order.
func (c ConfigMap) DataKeys() []string {
	keys := make([]string, 0, len(c.Data))
	for k := range c.Data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// SkipNote records a kind that could not be read, so the report can say so
// loudly rather than appear complete.
type SkipNote struct {
	Kind   Kind
	Reason string
}

// Snapshot is everything read from one side of the comparison.
type Snapshot struct {
	Context    string
	Namespace  string
	Workloads  []Workload
	ConfigMaps []ConfigMap
	Skipped    []SkipNote
}

// WithoutKinds returns a copy of the snapshot with every resource of the
// given kinds removed. A kind skipped (RBAC-forbidden, say) on either side of
// a comparison has no trustworthy data on that side: diffing it against the
// other side's real data would report a resource as "missing" when the truth
// is simply "unreadable", which is worse than not reporting it at all. Call
// this on BOTH sides with the union of both sides' skipped kinds, before
// pairing, so a skipped kind is never diffed - only ever reported as skipped.
func (s Snapshot) WithoutKinds(kinds map[Kind]bool) Snapshot {
	out := s
	out.Workloads = nil
	for _, w := range s.Workloads {
		if !kinds[w.Kind] {
			out.Workloads = append(out.Workloads, w)
		}
	}
	out.ConfigMaps = nil
	for _, c := range s.ConfigMaps {
		if !kinds[c.Kind] {
			out.ConfigMaps = append(out.ConfigMaps, c)
		}
	}
	return out
}
