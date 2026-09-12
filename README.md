# kubectl-env-diff

Compare workload configuration between two **live** Kubernetes clusters, and see
what actually differs — without ever reading a Secret value.

Answers the question behind every "works in staging, fails in production":
*what is actually different between these two environments for this service?*

## Why

`kubectl diff` compares local manifests to one cluster. ArgoCD and Flux compare
Git to one cluster. Neither compares **two live clusters**, which is where
manual `kubectl edit`, mutating webhooks, and HPA overrides live — drift that Git
cannot see.

## Install

```
kubectl krew install env-diff
```

Or download a binary from the [releases page](https://github.com/4ugane/kubectl-env-diff/releases)
and put it on your PATH as `kubectl-env_diff` (the underscore is what lets
kubectl expose it as `kubectl env-diff`; kubectl maps underscores in plugin
names to dashes).

## Use

```
# whole namespace
kubectl env-diff --from staging/web --to prod-au/web

# one workload, same name on both sides
kubectl env-diff --from staging/web --to prod/web --name checkout

# names differ between environments
kubectl env-diff --from staging/web --to prod/web \
    --from-name test-staging --to-name test-prod

# shareable report
kubectl env-diff --from staging/web --to prod/web --output html > drift.html
```

`--from` and `--to` take `context[/namespace]`. If the namespace is omitted,
`default` is used.

## Example

```
kubectl env-diff --from staging/web --to prod-au/web

SUMMARY   12 resources - 9 identical - 2 drifted - 1 missing

DRIFT  Deployment/checkout
  container[checkout].env.FEATURE_FLAG_X      (missing)  -> true
  container[checkout].env.LOG_LEVEL           debug      -> info
  container[checkout].env.DB_PASSWORD         <redacted> -> <redacted, differs>
  container[checkout].resources.limits.memory 4Gi        -> 512Mi
  container[checkout].probes.readiness.target /healthz:8080 -> /health:8080

DRIFT  ConfigMap/app-config
  data.KAFKA_BROKERS                        (missing)  -> b-1.msk.internal:9092

MISSING IN prod-au/web
  Deployment/notifier

1 expected difference hidden (image tag, replicas) - --show-expected to display
```

## Secret safety

The tool **never calls the Secrets API**. It needs no permission on secrets, so
a leak is impossible by construction — there is no code path to
`CoreV1().Secrets()`, and a test (`internal/kube/nosecrets_test.go`) fails the
build if one is ever introduced. Secret *references* (`secretKeyRef` names and
keys) are still reported, so "prod references `secret/db-creds:PASSWORD` and
staging does not" stays visible.

Values whose key looks like a credential (`PASSWORD`, `TOKEN`, `SECRET`,
`API_KEY`, and so on, matched as whole tokens including camelCase) are shown as
`<redacted, differs>` when they differ, or `<redacted>` when they're equal —
you learn that a credential drifted, never what to. Matching is by name token,
so `KEYCLOAK_URL` is *not* redacted.

Minimal RBAC is in [docs/rbac.yaml](docs/rbac.yaml).

## Output formats

| Format | Flag value | Use |
|---|---|---|
| Text (default) | `text` | Terminal. Width-aware; `--full` stops truncating; colour auto-disabled when not a TTY or when `NO_COLOR` is set |
| JSON | `json` | Pipe to `jq`, a dashboard, or an agent |
| Markdown | `markdown` | Paste into Slack, Jira, or a PR comment |
| HTML | `html` | Self-contained file with true side-by-side and a drift heatmap |

Select with `-o`/`--output`.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | No drift |
| 2 | Drift found |
| 1 | Tool error, or the comparison was incomplete (e.g. a kind was skipped due to missing RBAC) |

Incompleteness outranks drift: a run that could not read every resource did not
answer the question, so it never reports exit 2 for that run.

## Flags

| Flag | Description |
|---|---|
| `--from` | source `context[/namespace]` |
| `--to` | target `context[/namespace]` |
| `--name` | compare only this resource name on both sides |
| `--from-name` | resource name on the source side (use with `--to-name`) |
| `--to-name` | resource name on the target side (use with `--from-name`) |
| `--kind` | kinds to compare (default `deployment,statefulset,configmap`) |
| `-o`, `--output` | output format: `text`, `json`, `markdown`, `html` |
| `--config` | path to `.envdiff.yaml` (default: look for `.envdiff.yaml` in the working directory) |
| `--show-expected` | show expected differences instead of collapsing them |
| `--full` | do not truncate long values in text output |
| `--no-color` | disable coloured output |

## Configuration

Optional `.envdiff.yaml`, loaded from the working directory unless `--config`
points elsewhere:

```yaml
normalize:
  stripSuffixes: ["-staging", "-prod"]
  stripPrefixes: []
ignore:
  - path: "container[*].env.DATADOG_ENV"
  - path: "replicas"
  - kind: ConfigMap
    name: aws-auth
```

`normalize` strips a matching suffix/prefix so differently-named workloads
(`api-staging` vs `api-prod`) are recognized as the same pair. `ignore` rules
hide matching differences from the report entirely; each rule needs at least
one of `path`, `kind`, or `name` set. Only `*` is a wildcard in `path` — every
other character, including `[` and `]`, is literal.

## RBAC

The tool needs `get`/`list` on Deployments, StatefulSets, and ConfigMaps in
both namespaces — nothing else. See [docs/rbac.yaml](docs/rbac.yaml) for a
ready-to-apply Role and RoleBinding. The same minimal role definition is
printed automatically whenever a run hits a permissions error, so you never
have to guess it by hand.

## What this is not

- Not continuous monitoring or alerting
- Not Git-to-cluster drift detection (ArgoCD and Flux own that)
- Never reads Secret values, and no flag will be added to
- Read-only: no `--fix`, no writes, ever
- No hosted UI and no server; the HTML output is a static file

## License

MIT
