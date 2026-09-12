// Package kube resolves kubeconfig contexts and reads resources.
//
// It never calls the Secrets API. Secret references are read from pod specs by
// the extract package; no Secret object is ever fetched.
package kube

import (
	"fmt"
	"sort"
	"strings"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// Target is one side of the comparison.
type Target struct {
	Context   string
	Namespace string
}

// ParseTarget splits a "context[/namespace]" argument. An omitted namespace is
// filled in later from the context's own kubeconfig setting, falling back to
// "default", which is what kubectl would do.
func ParseTarget(arg string) (Target, error) {
	if arg == "" {
		return Target{}, fmt.Errorf("empty target; expected context[/namespace]")
	}
	if i := strings.LastIndex(arg, "/"); i != -1 {
		if i == 0 || i == len(arg)-1 {
			return Target{}, fmt.Errorf("malformed target %q; expected context/namespace", arg)
		}
		return Target{Context: arg[:i], Namespace: arg[i+1:]}, nil
	}
	return Target{Context: arg}, nil
}

// ClientFor builds a clientset for one context and resolves its namespace.
func ClientFor(t Target) (kubernetes.Interface, Target, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	raw, err := rules.Load()
	if err != nil {
		return nil, t, fmt.Errorf("loading kubeconfig: %w", err)
	}

	if _, ok := raw.Contexts[t.Context]; !ok {
		names := make([]string, 0, len(raw.Contexts))
		for n := range raw.Contexts {
			names = append(names, n)
		}
		sort.Strings(names)
		return nil, t, fmt.Errorf(
			"context %q not found in kubeconfig\navailable contexts:\n  %s",
			t.Context, strings.Join(names, "\n  "))
	}

	override := &clientcmd.ConfigOverrides{CurrentContext: t.Context}
	cfg := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, override)

	if t.Namespace == "" {
		ns, _, nsErr := cfg.Namespace()
		if nsErr != nil || ns == "" {
			ns = "default"
		}
		t.Namespace = ns
	}

	rest, err := cfg.ClientConfig()
	if err != nil {
		return nil, t, fmt.Errorf("building client for context %q: %w", t.Context, err)
	}
	cs, err := kubernetes.NewForConfig(rest)
	if err != nil {
		return nil, t, fmt.Errorf("creating clientset for context %q: %w", t.Context, err)
	}
	return cs, t, nil
}
