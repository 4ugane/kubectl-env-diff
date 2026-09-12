// Package extract converts Kubernetes API objects into the normalized model.
//
// This is the ONLY package permitted to import k8s.io/api. Everything
// downstream works on plain structs, which is what makes the rest of the
// codebase testable without a cluster.
//
// Redaction happens here rather than at render time, so a plaintext credential
// never enters the model and no renderer can leak one.
package extract

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/4ugane/kubectl-env-diff/internal/model"
	"github.com/4ugane/kubectl-env-diff/internal/redact"
)

// Container projects a PodSpec container onto the comparable allowlist.
func Container(c corev1.Container) model.Container {
	out := model.Container{
		Name:     c.Name,
		Image:    model.ParseImage(c.Image),
		Env:      make(map[string]model.EnvValue, len(c.Env)),
		Requests: resourceList(c.Resources.Requests),
		Limits:   resourceList(c.Resources.Limits),
		Command:  c.Command,
		Args:     c.Args,
	}

	// Kubernetes resolves duplicate env names to the last occurrence, so a
	// plain map assignment in order reproduces runtime behaviour.
	for _, e := range c.Env {
		out.Env[e.Name] = envValue(e)
	}

	for _, ef := range c.EnvFrom {
		switch {
		case ef.ConfigMapRef != nil:
			out.EnvFrom = append(out.EnvFrom, "configmap/"+ef.ConfigMapRef.Name)
		case ef.SecretRef != nil:
			out.EnvFrom = append(out.EnvFrom, "secret/"+ef.SecretRef.Name)
		}
	}

	out.Liveness = Probe(c.LivenessProbe)
	out.Readiness = Probe(c.ReadinessProbe)
	out.Startup = Probe(c.StartupProbe)
	return out
}

// envValue normalizes one environment variable. Reference kinds record where
// the value comes from; only inline values carry content, and only after
// redaction.
func envValue(e corev1.EnvVar) model.EnvValue {
	if e.ValueFrom != nil {
		vf := e.ValueFrom
		switch {
		case vf.ConfigMapKeyRef != nil:
			return model.EnvValue{
				Kind:     model.EnvConfigMapKey,
				Source:   fmt.Sprintf("configmap/%s:%s", vf.ConfigMapKeyRef.Name, vf.ConfigMapKeyRef.Key),
				Optional: vf.ConfigMapKeyRef.Optional != nil && *vf.ConfigMapKeyRef.Optional,
			}
		case vf.SecretKeyRef != nil:
			// The name and key only. The Secret itself is never fetched.
			return model.EnvValue{
				Kind:     model.EnvSecretKey,
				Source:   fmt.Sprintf("secret/%s:%s", vf.SecretKeyRef.Name, vf.SecretKeyRef.Key),
				Optional: vf.SecretKeyRef.Optional != nil && *vf.SecretKeyRef.Optional,
			}
		case vf.FieldRef != nil:
			return model.EnvValue{Kind: model.EnvFieldRef, Source: "field:" + vf.FieldRef.FieldPath}
		case vf.ResourceFieldRef != nil:
			return model.EnvValue{
				Kind: model.EnvResourceField,
				Source: fmt.Sprintf("resource:%s:%s",
					vf.ResourceFieldRef.ContainerName, vf.ResourceFieldRef.Resource),
			}
		}
	}

	if redact.IsSensitive(e.Name) {
		return model.EnvValue{
			Kind: model.EnvInline, Value: redact.Placeholder, Redacted: true,
			Fingerprint: redact.Fingerprint(e.Value),
		}
	}
	return model.EnvValue{Kind: model.EnvInline, Value: e.Value}
}

// Probe normalizes a health check. A nil probe yields Present=false so that a
// removed probe is reported as removed, not as an all-zero probe.
func Probe(p *corev1.Probe) model.Probe {
	if p == nil {
		return model.Probe{}
	}
	out := model.Probe{
		Present:             true,
		InitialDelaySeconds: p.InitialDelaySeconds,
		PeriodSeconds:       p.PeriodSeconds,
		TimeoutSeconds:      p.TimeoutSeconds,
		FailureThreshold:    p.FailureThreshold,
		SuccessThreshold:    p.SuccessThreshold,
	}
	switch {
	case p.HTTPGet != nil:
		out.Type = "httpGet"
		out.Target = fmt.Sprintf("%s:%s", p.HTTPGet.Path, p.HTTPGet.Port.String())
	case p.TCPSocket != nil:
		out.Type = "tcpSocket"
		out.Target = p.TCPSocket.Port.String()
	case p.Exec != nil:
		out.Type = "exec"
		out.Target = strings.Join(p.Exec.Command, " ")
	case p.GRPC != nil:
		out.Type = "grpc"
		out.Target = fmt.Sprintf("%d", p.GRPC.Port)
	}
	return out
}

// resourceList converts a Kubernetes ResourceList, preserving absence. A nil
// input stays nil so that "no limits set" never becomes "limits of zero".
func resourceList(in corev1.ResourceList) model.ResourceList {
	if len(in) == 0 {
		return nil
	}
	out := make(model.ResourceList, len(in))
	for name, qty := range in {
		out[string(name)] = qty
	}
	return out
}
