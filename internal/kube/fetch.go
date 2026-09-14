package kube

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/4ugane/kubectl-env-diff/internal/extract"
	"github.com/4ugane/kubectl-env-diff/internal/model"
)

// pageSize bounds each List call. A namespace with thousands of objects must
// not arrive in one response, and every List below follows continue tokens.
const pageSize = 500

// AllKinds is the default kind selection.
var AllKinds = []model.Kind{model.KindDeployment, model.KindStatefulSet, model.KindConfigMap}

// systemConfigMaps are cluster-managed, not application config: every
// namespace in a conformant Kubernetes cluster gets its own kube-root-ca.crt
// (the CA bundle for bound service account tokens, injected by
// kube-controller-manager since v1.20). Comparing two distinct physical
// clusters would otherwise always report it as drift - the CA differs by
// cluster, never by intent - with no action a reader could ever take on it.
var systemConfigMaps = map[string]bool{
	"kube-root-ca.crt": true,
}

// Fetch reads the requested kinds from one namespace.
//
// A permission failure on one kind is recorded in Snapshot.Skipped rather than
// aborting: the caller renders what succeeded plus a loud warning and exits 1.
// Any other failure is returned as an error, because the result would then be
// wrong rather than merely partial.
func Fetch(ctx context.Context, cs kubernetes.Interface,
	contextName, namespace string, kinds []model.Kind) (model.Snapshot, error) {

	snap := model.Snapshot{Context: contextName, Namespace: namespace}
	want := map[model.Kind]bool{}
	for _, k := range kinds {
		want[k] = true
	}

	// each pairs a kind with its paginated read, so the skip-or-fail decision
	// is written exactly once.
	each := []struct {
		kind model.Kind
		read func(metav1.ListOptions) (string, error)
	}{
		{model.KindDeployment, func(opts metav1.ListOptions) (string, error) {
			list, err := cs.AppsV1().Deployments(namespace).List(ctx, opts)
			if err != nil {
				return "", err
			}
			for i := range list.Items {
				snap.Workloads = append(snap.Workloads, extract.Deployment(&list.Items[i]))
			}
			return list.Continue, nil
		}},
		{model.KindStatefulSet, func(opts metav1.ListOptions) (string, error) {
			list, err := cs.AppsV1().StatefulSets(namespace).List(ctx, opts)
			if err != nil {
				return "", err
			}
			for i := range list.Items {
				snap.Workloads = append(snap.Workloads, extract.StatefulSet(&list.Items[i]))
			}
			return list.Continue, nil
		}},
		{model.KindConfigMap, func(opts metav1.ListOptions) (string, error) {
			list, err := cs.CoreV1().ConfigMaps(namespace).List(ctx, opts)
			if err != nil {
				return "", err
			}
			for i := range list.Items {
				if systemConfigMaps[list.Items[i].Name] {
					continue
				}
				snap.ConfigMaps = append(snap.ConfigMaps, extract.ConfigMap(&list.Items[i]))
			}
			return list.Continue, nil
		}},
	}

	for _, e := range each {
		if !want[e.kind] {
			continue
		}
		if err := forEachPage(e.read); err != nil {
			note, recoverable := skipNote(e.kind, contextName, err)
			if !recoverable {
				return snap, fmt.Errorf("listing %ss in %s/%s: %w",
					e.kind, contextName, namespace, err)
			}
			snap.Skipped = append(snap.Skipped, note)
		}
	}

	return snap, nil
}

// forEachPage drives a paginated List to completion.
func forEachPage(read func(metav1.ListOptions) (string, error)) error {
	opts := metav1.ListOptions{Limit: pageSize}
	for {
		next, err := read(opts)
		if err != nil {
			return err
		}
		if next == "" {
			return nil
		}
		opts.Continue = next
	}
}
