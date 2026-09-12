package compare

import (
	"fmt"
	"sort"
	"strings"

	"github.com/4ugane/kubectl-env-diff/internal/model"
)

// Workload diffs one paired workload. An absent side yields a single
// whole-object difference with an empty Path.
func Workload(p Pair) []model.Difference {
	base := func(path string, typ model.DiffType, from, to string) model.Difference {
		return model.Difference{
			Kind: p.Kind, Name: p.Name, FromName: p.FromName, ToName: p.ToName,
			Path: path, Type: typ, From: from, To: to,
		}
	}

	switch {
	case p.From == nil && p.To == nil:
		return nil
	case p.To == nil:
		return []model.Difference{base("", model.MissingInTo, p.FromName, "")}
	case p.From == nil:
		return []model.Difference{base("", model.MissingInFrom, "", p.ToName)}
	}

	var diffs []model.Difference
	add := func(d model.Difference) { diffs = append(diffs, d) }

	if p.From.Replicas != p.To.Replicas {
		add(base("replicas", model.ValueChanged,
			fmt.Sprint(p.From.Replicas), fmt.Sprint(p.To.Replicas)))
	}
	if p.From.ServiceAccount != p.To.ServiceAccount {
		add(base("serviceAccount", model.ValueChanged, p.From.ServiceAccount, p.To.ServiceAccount))
	}
	diffs = append(diffs, sliceDiff(base, "configMapRefs", p.From.ConfigMapRefs, p.To.ConfigMapRefs)...)
	diffs = append(diffs, sliceDiff(base, "secretRefs", p.From.SecretRefs, p.To.SecretRefs)...)
	diffs = append(diffs, containersDiff(base, "container", p.From.Containers, p.To.Containers)...)
	diffs = append(diffs, containersDiff(base, "initContainer", p.From.InitContainers, p.To.InitContainers)...)
	return diffs
}

type baseFn func(path string, typ model.DiffType, from, to string) model.Difference

// containersDiff pairs containers by name so that a missing container is
// reported by name rather than as a bulk list change.
func containersDiff(base baseFn, prefix string, from, to []model.Container) []model.Difference {
	fromByName := indexContainers(from)
	toByName := indexContainers(to)

	names := make([]string, 0, len(fromByName)+len(toByName))
	seen := map[string]bool{}
	for n := range fromByName {
		if !seen[n] {
			names, seen[n] = append(names, n), true
		}
	}
	for n := range toByName {
		if !seen[n] {
			names, seen[n] = append(names, n), true
		}
	}
	sort.Strings(names)

	var diffs []model.Difference
	for _, name := range names {
		path := fmt.Sprintf("%s[%s]", prefix, name)
		f, okF := fromByName[name]
		t, okT := toByName[name]
		switch {
		case !okT:
			diffs = append(diffs, base(path, model.MissingInTo, f.Image.String(), ""))
		case !okF:
			diffs = append(diffs, base(path, model.MissingInFrom, "", t.Image.String()))
		default:
			diffs = append(diffs, containerDiff(base, path, f, t)...)
		}
	}
	return diffs
}

func indexContainers(cs []model.Container) map[string]model.Container {
	out := make(map[string]model.Container, len(cs))
	for _, c := range cs {
		out[c.Name] = c
	}
	return out
}

// containerDiff compares two containers of the same name.
func containerDiff(base baseFn, path string, f, t model.Container) []model.Difference {
	var diffs []model.Difference

	// Repo and tag are reported separately: a tag change is routine, a repo
	// change means an environment pulls from somewhere unexpected.
	if f.Image.Repo != t.Image.Repo {
		diffs = append(diffs, base(path+".image.repo", model.ValueChanged, f.Image.Repo, t.Image.Repo))
	}
	if f.Image.Tag != t.Image.Tag {
		diffs = append(diffs, base(path+".image.tag", model.ValueChanged, f.Image.Tag, t.Image.Tag))
	}
	if f.Image.Digest != t.Image.Digest {
		diffs = append(diffs, base(path+".image.digest", model.ValueChanged, f.Image.Digest, t.Image.Digest))
	}

	diffs = append(diffs, envDiff(base, path+".env", f.Env, t.Env)...)
	diffs = append(diffs, resourceDiff(base, path+".resources.requests", f.Requests, t.Requests)...)
	diffs = append(diffs, resourceDiff(base, path+".resources.limits", f.Limits, t.Limits)...)
	diffs = append(diffs, sliceDiff(base, path+".envFrom", f.EnvFrom, t.EnvFrom)...)

	for _, pr := range []struct {
		name string
		f, t model.Probe
	}{
		{"liveness", f.Liveness, t.Liveness},
		{"readiness", f.Readiness, t.Readiness},
		{"startup", f.Startup, t.Startup},
	} {
		diffs = append(diffs, probeDiff(base, path+".probes."+pr.name, pr.f, pr.t)...)
	}

	if strings.Join(f.Command, " ") != strings.Join(t.Command, " ") {
		diffs = append(diffs, base(path+".command", model.ValueChanged,
			strings.Join(f.Command, " "), strings.Join(t.Command, " ")))
	}
	if strings.Join(f.Args, " ") != strings.Join(t.Args, " ") {
		diffs = append(diffs, base(path+".args", model.ValueChanged,
			strings.Join(f.Args, " "), strings.Join(t.Args, " ")))
	}
	return diffs
}

// envDiff compares env maps key by key. Keys are sorted because map order is
// random and would otherwise make output nondeterministic.
func envDiff(base baseFn, prefix string, f, t map[string]model.EnvValue) []model.Difference {
	var diffs []model.Difference
	for _, k := range unionKeys(f, t) {
		fv, okF := f[k]
		tv, okT := t[k]
		path := prefix + "." + k
		switch {
		case !okT:
			d := base(path, model.MissingInTo, fv.Display(), "")
			d.Redacted = fv.Redacted
			diffs = append(diffs, d)
		case !okF:
			d := base(path, model.MissingInFrom, "", tv.Display())
			d.Redacted = tv.Redacted
			diffs = append(diffs, d)
		case fv.Kind != tv.Kind || !fv.Equal(tv):
			d := base(path, model.ValueChanged, fv.Display(), tv.Display())
			d.Redacted = fv.Redacted || tv.Redacted
			diffs = append(diffs, d)
		}
	}
	return diffs
}

func unionKeys[V any](a, b map[string]V) []string {
	seen := make(map[string]bool, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for k := range a {
		if !seen[k] {
			out, seen[k] = append(out, k), true
		}
	}
	for k := range b {
		if !seen[k] {
			out, seen[k] = append(out, k), true
		}
	}
	sort.Strings(out)
	return out
}

// resourceDiff compares by amount, so 1Gi equals 1024Mi. Absence stays distinct
// from an explicit zero.
func resourceDiff(base baseFn, prefix string, f, t model.ResourceList) []model.Difference {
	var diffs []model.Difference
	for _, name := range unionKeys(f, t) {
		fq, okF := f[name]
		tq, okT := t[name]
		path := prefix + "." + name
		switch {
		case !okT:
			diffs = append(diffs, base(path, model.MissingInTo, fq.String(), ""))
		case !okF:
			diffs = append(diffs, base(path, model.MissingInFrom, "", tq.String()))
		case fq.Cmp(tq) != 0:
			diffs = append(diffs, base(path, model.ValueChanged, fq.String(), tq.String()))
		}
	}
	return diffs
}

// probeDiff treats presence as the first-class difference. A removed probe is
// reported as removed rather than compared field by field against a zero value.
func probeDiff(base baseFn, path string, f, t model.Probe) []model.Difference {
	switch {
	case !f.Present && !t.Present:
		return nil
	case f.Present && !t.Present:
		return []model.Difference{base(path, model.MissingInTo, f.Type+" "+f.Target, "")}
	case !f.Present && t.Present:
		return []model.Difference{base(path, model.MissingInFrom, "", t.Type+" "+t.Target)}
	}

	var diffs []model.Difference
	if f.Type != t.Type {
		diffs = append(diffs, base(path+".type", model.ValueChanged, f.Type, t.Type))
	}
	if f.Target != t.Target {
		diffs = append(diffs, base(path+".target", model.ValueChanged, f.Target, t.Target))
	}
	for _, tm := range []struct {
		name string
		f, t int32
	}{
		{"initialDelaySeconds", f.InitialDelaySeconds, t.InitialDelaySeconds},
		{"periodSeconds", f.PeriodSeconds, t.PeriodSeconds},
		{"timeoutSeconds", f.TimeoutSeconds, t.TimeoutSeconds},
		{"failureThreshold", f.FailureThreshold, t.FailureThreshold},
		{"successThreshold", f.SuccessThreshold, t.SuccessThreshold},
	} {
		if tm.f != tm.t {
			diffs = append(diffs, base(path+"."+tm.name, model.ValueChanged,
				fmt.Sprint(tm.f), fmt.Sprint(tm.t)))
		}
	}
	return diffs
}

// sliceDiff compares reference lists as sets, reporting entries present on only
// one side. Both inputs are already sorted by the extract package.
func sliceDiff(base baseFn, path string, f, t []string) []model.Difference {
	inFrom := map[string]bool{}
	for _, v := range f {
		inFrom[v] = true
	}
	inTo := map[string]bool{}
	for _, v := range t {
		inTo[v] = true
	}

	var diffs []model.Difference
	for _, v := range f {
		if !inTo[v] {
			diffs = append(diffs, base(path+"."+v, model.MissingInTo, v, ""))
		}
	}
	for _, v := range t {
		if !inFrom[v] {
			diffs = append(diffs, base(path+"."+v, model.MissingInFrom, "", v))
		}
	}
	return diffs
}
