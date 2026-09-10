package extract

import (
	"sort"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/4ugane/kubectl-env-diff/internal/model"
)

// Deployment projects a Deployment onto the comparable model.
func Deployment(d *appsv1.Deployment) model.Workload {
	w := podSpec(d.Spec.Template.Spec)
	w.Kind = model.KindDeployment
	w.Name = d.Name
	w.Namespace = d.Namespace
	w.Replicas = replicas(d.Spec.Replicas)
	return w
}

// StatefulSet projects a StatefulSet onto the comparable model.
func StatefulSet(s *appsv1.StatefulSet) model.Workload {
	w := podSpec(s.Spec.Template.Spec)
	w.Kind = model.KindStatefulSet
	w.Name = s.Name
	w.Namespace = s.Namespace
	w.Replicas = replicas(s.Spec.Replicas)
	return w
}

// replicas resolves the runtime default. A nil field means one replica, so nil
// and an explicit 1 are the same configuration.
func replicas(r *int32) int32 {
	if r == nil {
		return 1
	}
	return *r
}

// podSpec extracts the container-level fields shared by both workload kinds.
func podSpec(spec corev1.PodSpec) model.Workload {
	w := model.Workload{ServiceAccount: spec.ServiceAccountName}

	cmRefs := map[string]bool{}
	secRefs := map[string]bool{}

	collect := func(c corev1.Container) model.Container {
		out := Container(c)
		for _, e := range c.EnvFrom {
			if e.ConfigMapRef != nil {
				cmRefs[e.ConfigMapRef.Name] = true
			}
			if e.SecretRef != nil {
				secRefs[e.SecretRef.Name] = true
			}
		}
		for _, e := range c.Env {
			if e.ValueFrom == nil {
				continue
			}
			if r := e.ValueFrom.ConfigMapKeyRef; r != nil {
				cmRefs[r.Name] = true
			}
			if r := e.ValueFrom.SecretKeyRef; r != nil {
				secRefs[r.Name] = true
			}
		}
		return out
	}

	for _, c := range spec.InitContainers {
		w.InitContainers = append(w.InitContainers, collect(c))
	}
	for _, c := range spec.Containers {
		w.Containers = append(w.Containers, collect(c))
	}

	// Volume-mounted config counts as a reference too.
	for _, v := range spec.Volumes {
		if v.ConfigMap != nil {
			cmRefs[v.ConfigMap.Name] = true
		}
		if v.Secret != nil {
			secRefs[v.Secret.SecretName] = true
		}
		if v.Projected != nil {
			for _, src := range v.Projected.Sources {
				if src.ConfigMap != nil {
					cmRefs[src.ConfigMap.Name] = true
				}
				if src.Secret != nil {
					secRefs[src.Secret.Name] = true
				}
			}
		}
	}

	w.ConfigMapRefs = sortedKeys(cmRefs)
	w.SecretRefs = sortedKeys(secRefs)
	return w
}

// sortedKeys returns deduplicated, sorted names. Sorting matters: unsorted
// output would differ between runs because Go map order is random.
func sortedKeys(set map[string]bool) []string {
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
