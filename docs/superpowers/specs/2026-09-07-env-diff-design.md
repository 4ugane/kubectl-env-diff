# env-diff — Multi-Environment Kubernetes Configuration Comparator

**Status:** Design approved · **Date:** 2026-09-07

## Problem

Debugging "works in staging, fails in production" means answering one question:
*what is actually different between these two environments for this service?*

Today that answer requires composing three tools per side
(`kubectl get -o yaml` → `kubectl-neat` → `dyff`), manually juggling contexts and
namespaces, and then reading past a flood of intentional differences. Nobody does
this under incident pressure, which is exactly when the answer is needed.

`env-diff` is a single command that answers it: side-by-side comparison of
workload configuration between two **live** clusters, in human terms, without
exposing secret values.

## Why this gap is real

| Existing tool | Axis it covers | Why it does not cover this |
|---|---|---|
| `kubectl diff` | local manifests → one live cluster | Not cross-cluster; structurally cannot be |
| `dyff` | any YAML → any YAML | Generic differ; caller fetches and pre-cleans both sides; no k8s semantics |
| ArgoCD / Flux | Git → one cluster per app | Git-centric; blind to manual edits; never compares two live clusters |
| `kubediff` | Git → cluster | Unmaintained |
| `kubectl-neat` | cleans one object | Complementary, not a differ |
| Komodor / Nova | drift monitoring | Paid SaaS, agent install, org buy-in |

Cluster-to-cluster comparison also catches drift that Git *cannot* see: manual
`kubectl edit`, mutating webhooks, and HPA/VPA overrides.

### Accepted risks

1. **Narrow trigger moment.** People reach for this during one specific class of
   incident; stars will not equal usage. Mitigated by a CI-usable exit code, so
   it also runs when nobody is watching.
2. **Scope-creep pressure.** Requests for more kinds, a UI, and continuous
   monitoring are certain. Mitigated by the Non-Goals section, published in the
   README as a prewritten answer.

## Scope

**v1 compares:** Deployments, StatefulSets, ConfigMaps, and non-sensitive
environment variables, between two live kubeconfig contexts.

**v1 does not compare:** anything else. Additional kinds (NetworkPolicy,
ServiceAccount/IRSA annotations, HPA, PDB, Ingress) are added one extractor at a
time, post-v1.

## CLI

```
kubectl env-diff --from <context>[/<namespace>] --to <context>[/<namespace>]
                 [--name NAME | --from-name A --to-name B]
                 [--kind deployment,statefulset,configmap]
                 [--show-expected] [--full] [--config .envdiff.yaml]
                 [--output text|json|markdown|html]
```

The binary is named `kubectl-env_diff`; krew maps the underscore to a dash, so it
works both standalone and as `kubectl env-diff`. `--kind` defaults to all three
v1 kinds; when narrowed, a name filter applies within each selected kind.

```bash
# whole namespace
kubectl env-diff --from staging/web --to prod-au/web

# one workload, same name both sides
kubectl env-diff --from staging/web --to prod-au/web --name checkout

# names diverge — explicit pair
kubectl env-diff --from staging/web --to prod/web \
                 --from-name test-staging --to-name test-prod

# ConfigMaps with divergent names
kubectl env-diff --from staging/web --to prod/web --kind configmap \
                 --from-name app-config-staging --to-name app-config-prod
```

### Exit codes

| Code | Meaning |
|---|---|
| 0 | No drift |
| 2 | Drift found |
| 1 | Tool error (bad context, RBAC, unreachable cluster, skipped resources) |

Separating 1 from 2 lets CI distinguish "the tool broke" from "production
actually drifted". A single non-zero code cannot, and that distinction matters
the first time it fails in a pipeline.

## Architecture

### Approach: allowlist extraction, then typed diff

Objects are fetched and projected into a small normalized model — image,
replicas, resources, probes, env map, config references — which is then diffed
field by field with typed comparators.

The noise problem largely disappears by construction: `status`, `managedFields`,
`uid`, `resourceVersion`, `creationTimestamp`, and revision annotations are never
extracted, so they can never be compared. Output is human-shaped
(`resources.limits.memory: 4Gi → 512Mi`) rather than path-shaped
(`spec.template.spec.containers[0].resources.limits.memory`), and readability is
the entire product.

Rejected alternatives:

- **Full-object diff with denylist pruning** (the `dyff` model). Free support for
  any kind, but noise control becomes a permanent arms race and the path-shaped
  output reads worse — which is precisely why the existing compose-three-tools
  workflow already fails in practice.
- **Hybrid** (allowlist for known kinds, generic fallback for the rest). The
  right answer eventually; unnecessary complexity for v1.

Cost of the chosen approach: adding a kind means writing an extractor
(~30 lines), not editing a config file. This is acceptable and arguably correct,
since what matters genuinely differs per kind — ingress rules for a
NetworkPolicy, the IRSA annotation for a ServiceAccount.

### Packages

```
cmd/kubectl-env_diff/    cobra flags, wiring, exit codes
internal/kube/           context resolution, typed clients, fetch
internal/model/          normalized structs (the allowlist, as types)
internal/extract/        k8s object → model; one file per kind
internal/compare/        name normalization, pairing, field diff
internal/classify/       Difference → severity; applies ignore rules
internal/redact/         sensitive-key detection
internal/config/         .envdiff.yaml loading + built-in defaults
internal/report/         text, json, markdown, html renderers + embedded template
```

**The load-bearing seam: `extract` is the only package that imports
`k8s.io/api`.** Everything downstream operates on `model` types, so `compare`,
`classify`, `redact`, and `report` are pure functions over plain structs —
testable with literals, no fake clientset, no cluster. Adding a kind touches
`extract` and `model` only.

Each package stays under ~300 lines with a single responsibility.

### Data model

```go
type Workload struct {
    Kind, Name, Namespace string
    Replicas              *int32
    ServiceAccount        string
    Containers            []Container
    ConfigMapRefs         []string     // referenced names
    SecretRefs            []SecretRef  // name + key only — never fetched
}

type Container struct {
    Name                string
    ImageRepo, ImageTag string          // split: tag drift and repo drift differ in meaning
    Env                 map[string]EnvValue
    Resources           ResourceSpec    // requests/limits as resource.Quantity
    Probes              ProbeSpec       // liveness/readiness/startup: path, port, timings
}

type EnvValue struct {
    Kind   EnvKind  // Inline | ConfigMapRef | SecretRef | FieldRef
    Value  string   // Inline only; redacted when the key looks sensitive
    Source string   // "secret/db-creds:PASSWORD" — a reference, never content
}

type ConfigMapData struct {
    Name, Namespace string
    Keys            map[string]string
}
```

Two deliberate decisions:

- **Image split into repo and tag.** A differing tag is normal; a differing repo
  means an environment is pulling from somewhere unexpected. Same field, opposite
  meanings — they cannot share a code path.
- **Resources as `resource.Quantity`, not string.** `1Gi` and `1024Mi` are equal
  and must not report as drift. String comparison gets this wrong, and it is the
  field people check most often.

## Pairing

First match wins, so the explicit case always beats the inferred case:

| Invocation | Behavior |
|---|---|
| `--from-name A --to-name B` | Exact pair; normalization **not** applied |
| `--name X` | `X` on both sides; normalization applied |
| neither | Whole namespace pair; normalization applied |

- `--name` and `--from-name`/`--to-name` are mutually exclusive, rejected at
  flag-parse time with a message naming which to use.
- `--from-name` and `--to-name` must be supplied together; one without the other
  is a flag-parse error.
- Explicit pairing is same-kind only. `Deployment` vs `StatefulSet` is rejected
  rather than half-supported.
- **Normalization collisions are a hard error.** If `api-staging` and `api` both
  normalize to `api`, the tool refuses and names both offenders. Silently
  choosing one would produce a confident wrong diff during an incident — worse
  than having no tool.

## Comparison and classification

```go
type Difference struct {
    Kind, Name, Path string   // "container[checkout].resources.limits.memory"
    From, To         string   // empty with a presence flag when absent
    Type             DiffType // ValueChanged | MissingInTo | MissingInFrom
    Severity         Severity // Drift | Expected
}
```

`MissingInFrom` and `MissingInTo` render before value changes. A missing env key
or an absent workload is the classic cause of environment-specific failure, so it
leads the report.

### Built-in Expected set: image tag and replica count only

Nothing else. Auto-detecting namespace strings, ARN account IDs, or hostnames
inside env values was considered and rejected: each heuristic is an opportunity
to silently hide a real problem, and a wrong "expected" label is far more
damaging than one extra line of output. Everything else defaults to `Drift`; the
ignore file is how a user declares a specific difference acceptable.

Expected differences are counted, collapsed to a single summary line, and shown
in full only with `--show-expected`.

### Configuration

```yaml
# .envdiff.yaml
normalize:
  stripSuffixes: ["-staging", "-prod", "-dev"]
ignore:
  - path: "container[*].env.DATADOG_ENV"
  - path: "replicas"
  - kind: ConfigMap
    name: aws-auth
```

`ignore` hides a difference entirely. There is deliberately no second
"show but do not fail" tier — `Expected` already covers that case, and a third
severity is a knob nobody would tune correctly.

## Secret safety

**The tool never calls the Secrets API.** It requires no secret read permission,
so a leak is impossible by construction and the minimal Role can be documented
exactly. Secret *references* are still reported from the pod spec, so
"prod references `secret/db-creds:PASSWORD` and staging does not" remains a
visible finding.

**Redaction covers what that guarantee does not.** A plaintext credential sitting
inline in a Deployment's `env: value:` or in a ConfigMap never touches the
Secrets API. Values whose *key* matches
`(?i)password|passwd|token|secret|key|credential|apikey` render as `<redacted>`
or `<redacted, differs>` — you learn that it drifted, never what to.

Redaction is always on. No flag disables it.

## Output

Four formats, selected with `--output`. All write to stdout; there is deliberately
no `--output-file` flag, since `--output html > drift.html` already covers it.

**The renderer invariant:** redaction, classification, and ignore rules all run
*before* `report` is reached. Renderers are pure formatters over the same
`[]Difference` slice and summary counts — they cannot reach a raw value, so they
cannot leak one. This is what makes four formats cheap instead of four times the
work, and it means a new renderer can never regress secret safety.

### `--output text` (default)

```
kubectl env-diff   staging/web → prod-au/web

SUMMARY   12 workloads · 9 identical · 2 drifted · 1 missing

DRIFT  Deployment/checkout
  container[checkout]
    env.FEATURE_FLAG_X            (missing)   →  true
    env.LOG_LEVEL                 debug       →  info
    env.DB_PASSWORD               <redacted>  →  <redacted, differs>
    resources.limits.memory       4Gi         →  512Mi
    readinessProbe.path           /healthz    →  /health

DRIFT  ConfigMap/app-config
    KAFKA_BROKERS                 (missing)   →  b-1.msk.internal:9092

MISSING IN prod-au
  Deployment/notifier

2 expected differences hidden (image tag, replicas) — --show-expected
```

Summary first so the report is scannable under pressure; details only for what
drifted.

Output is `from → to` single-column, not true side-by-side: at 80 columns, side-by-side
with long values (ARNs, broker lists, JVM flag strings) is unreadable. The renderer is
terminal-width aware and truncates long values with an ellipsis; `--full` disables
truncation. True side-by-side is what the HTML renderer is for.

### `--output json`

The `[]Difference` slice plus summary counts. No bespoke schema, so `jq`, a
dashboard, or an agent can all consume it.

### `--output markdown`

A table that pastes directly into Slack, Jira, GitHub issues, and PR comments —
the sharing moment that recurs on every real investigation. Values must have `|`
and backticks escaped, or a single env value containing a pipe silently corrupts
the table.

### `--output html`

One self-contained file: `embed` plus `html/template`, inline CSS, no external
assets and no JS framework. Native `<details>` elements handle collapse; a small
filter box is the only hand-written JavaScript. Light and dark via
`prefers-color-scheme`.

This is the only renderer that does true side-by-side, and the only one that adds
an analytical view rather than a formatting one: a **drift heatmap** of workloads
× field category (env, resources, probes, image, replicas), as a plain CSS grid
of shaded cells. It surfaces systemic findings — "resource limits drift in 8 of 12
services" is one problem, not eight — which no line-by-line format conveys.

The heatmap renders only when a run covers three or more paired workloads. On a
single-workload `--name` run it would be one row, which is noise.

`html/template` auto-escapes, which matters here: env values and ConfigMap
contents are untrusted input and can contain markup.

## Error handling

The governing rule: **never produce a partial report that looks complete.**

| Failure | Behavior |
|---|---|
| Context not found in kubeconfig | Exit 1; list available contexts |
| Namespace not found | Exit 1; name which side |
| RBAC 403 | Exit 1; print the failing verb/resource/context and the minimal Role YAML needed |
| Either cluster unreachable | Exit 1; fail fast, no half diff |
| One kind listable, another not | Render what succeeded, plus a loud `WARNING: skipped StatefulSets in <ctx> (403)` block; exit 1 |

The last row matters most. Rendering a clean "no drift" report while silently
having skipped half the resources is the worst possible failure for this tool —
it would be trusted, and wrong.

## Testing

Target: 80% coverage minimum, TDD throughout.

| Package | Strategy |
|---|---|
| `compare`, `classify`, `redact` | Table-driven unit tests over struct literals |
| `extract` | Fixture YAML → expected model |
| `report` | Golden files for all four renderers; HTML asserted to contain no unescaped input and no redacted value |
| `kube` | Two fake clientsets standing in for two clusters |

**The entire suite runs offline via `go test ./...`** — no kind cluster, no
envtest. For an open-source project this is not a nicety: a contributor who
cannot run the tests in thirty seconds does not send a second PR.

## Non-goals

Published in the README so every feature request has a prewritten answer.

- Not continuous monitoring or alerting
- Not Git-to-cluster drift detection (ArgoCD and Flux own that axis)
- Never reads Secret values, and no flag will be added to
- Read-only — no `--fix`, no writes, ever
- No hosted UI and no server — the HTML output is a static file, not an app

## v1 milestone

- Deployment, StatefulSet, ConfigMap extractors
- Four output formats: text, JSON, markdown, HTML (with drift heatmap)
- krew plugin manifest
- README with a real before/after example and the minimal RBAC Role

Everything else is v2.
