// Package config loads .envdiff.yaml, the single user-facing mechanism for
// silencing differences that are acceptable in a given environment pair.
package config

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"sigs.k8s.io/yaml"
)

// DefaultPath is looked up when the user does not pass --config.
const DefaultPath = ".envdiff.yaml"

// Config is the parsed contents of .envdiff.yaml.
type Config struct {
	Normalize NormalizeRules `json:"normalize"`
	Ignore    []IgnoreRule   `json:"ignore"`
}

// NormalizeRules describe how workload names are made comparable across
// clusters, for teams that bake the environment into the name.
type NormalizeRules struct {
	StripSuffixes []string `json:"stripSuffixes"`
	StripPrefixes []string `json:"stripPrefixes"`
}

// IgnoreRule hides matching differences entirely. An empty rule matches
// everything, so it is rejected at validation.
type IgnoreRule struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// knownKinds are the resource kinds v1 understands.
var knownKinds = map[string]bool{
	"Deployment": true, "StatefulSet": true, "ConfigMap": true,
}

// Load reads a config file. When explicit is false a missing file yields empty
// defaults, because running with no config is the normal case. When the user
// named the file, a missing file is an error — silently ignoring it would apply
// none of their rules while appearing to work.
func Load(path string, explicit bool) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && !explicit {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}

	cfg := &Config{}
	if err := yaml.UnmarshalStrict(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config %s: %w", path, err)
	}
	return cfg, nil
}

// Validate rejects rules that would silently misbehave.
func (c *Config) Validate() error {
	for i, s := range c.Normalize.StripSuffixes {
		if s == "" {
			return fmt.Errorf("normalize.stripSuffixes[%d] is empty; an empty suffix matches every name", i)
		}
	}
	for i, s := range c.Normalize.StripPrefixes {
		if s == "" {
			return fmt.Errorf("normalize.stripPrefixes[%d] is empty; an empty prefix matches every name", i)
		}
	}

	for i, r := range c.Ignore {
		if r.Path == "" && r.Kind == "" && r.Name == "" {
			return fmt.Errorf("ignore[%d] sets no fields; it would hide every difference", i)
		}
		if r.Kind != "" && !knownKinds[r.Kind] {
			return fmt.Errorf("ignore[%d]: unknown kind %q (want Deployment, StatefulSet, or ConfigMap)", i, r.Kind)
		}
	}
	return nil
}

// MatchPath reports whether a difference path matches an ignore pattern.
//
// Only "*" is a wildcard; every other character is literal. This matters
// because paths contain square brackets ("container[api].env.FOO") and
// filepath.Match would interpret "[*]" as a character class rather than the
// literal text the user wrote. Any pattern string is therefore valid, so there
// is nothing to reject at load time.
func MatchPath(pattern, path string) bool {
	parts := strings.Split(pattern, "*")
	for i, part := range parts {
		parts[i] = regexp.QuoteMeta(part)
	}
	re, err := regexp.Compile("^" + strings.Join(parts, ".*") + "$")
	if err != nil {
		// Unreachable: every segment is quoted, so the expression always
		// compiles. Fail closed rather than silently ignoring differences.
		return false
	}
	return re.MatchString(path)
}
