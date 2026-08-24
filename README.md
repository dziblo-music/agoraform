# Agoraform

**Marketing Infrastructure as Code**

Agoraform is an open-source tool for defining, reviewing, planning, and applying marketing infrastructure from code.

The goal is to bring infrastructure-as-code and GitOps workflows to systems such as advertising platforms, analytics, conversion tracking, and tag management.

Instead of configuring marketing infrastructure manually across multiple dashboards, Agoraform aims to let you describe the desired state declaratively, preview changes, and safely apply them through provider APIs.

## Vision

```bash
agoraform validate
```

Validate configuration files, provider settings, resource references, and schema rules before making any changes.

```bash
agoraform plan
```

Read the current state from configured providers, compare it with the declared configuration, and show the changes required to reconcile them. This also exposes configuration drift caused by changes made outside Agoraform.

```bash
agoraform apply
```

Apply the reviewed changes through provider APIs, bringing the remote infrastructure into the declared desired state.

```bash
agoraform import
```

Import existing resources that were created outside Agoraform and bring them under management. Agoraform reads the current live configuration and updates the local manifest and resource identity/state information to match it, without modifying the remote resource.

This makes it possible to adopt existing Google Ads, Meta Ads, or Matomo configurations without recreating them from scratch.

Initial provider targets include:

- Google Ads
- Meta Ads
- Matomo

## Status

Agoraform is currently in early development. The CLI can validate v0.1 YAML manifests, produce a non-mutating plan, apply planned creates and updates, and import existing remote resources into local management. Local identity state maps logical addresses to opaque provider-native IDs. The Matomo provider implements `matomo.goal` (read, create, update, and import).

The project is being built from scratch as an open-source initiative, with an emphasis on:

- Declarative configuration
- Idempotent operations
- Safe `plan` / `apply` workflows
- Importing existing infrastructure
- Provider extensibility
- Git-based review and auditability
- Strong safeguards around advertising budgets and destructive changes

## Build

Requires Go 1.23 or newer.

```bash
go build -o agoraform ./cmd/agoraform
```

On Windows:

```bash
go build -o agoraform.exe ./cmd/agoraform
```

## CLI

```bash
./agoraform --help
./agoraform --version
```

Development builds report version `0.0.0-dev` by default. Release builds can override the version with:

```bash
go build -ldflags "-X github.com/dziblo-music/agoraform/internal/cli.Version=0.1.0" -o agoraform ./cmd/agoraform
```

### Commands

```bash
./agoraform validate
./agoraform validate -f path/to/manifest.yaml
```

`validate` loads a v0.1 YAML manifest (default `agoraform.yaml`), checks the schema, resource addresses, and duplicate logical names. See [docs/manifest.md](docs/manifest.md) for the document format.

```bash
./agoraform plan
./agoraform plan -f path/to/manifest.yaml
```

`plan` reads live resources through registered providers, diffs them against the manifest, and prints a deterministic execution plan. It never mutates remote resources. Managed identities are read from `agoraform.state.json` next to the manifest. See [docs/plan.md](docs/plan.md) and [docs/state.md](docs/state.md).

```bash
./agoraform apply
./agoraform apply -f path/to/manifest.yaml
```

`apply` builds the same plan, then executes create and update actions through provider APIs. Successful creates persist the returned provider-native identity in local state. Deletion is out of scope. See [docs/apply.md](docs/apply.md).

```bash
./agoraform import ADDRESS REMOTE-ID
./agoraform import -f path/to/manifest.yaml ADDRESS REMOTE-ID
```

`import` reads an existing remote resource, prints deterministic YAML for configurable fields, and persists the provider-native identity in local state. It never mutates the remote resource and does not rewrite the manifest. See [docs/import.md](docs/import.md).

The Matomo provider is registered with the CLI. `matomo.goal` can be declared, validated, planned, applied, and imported. Tests use the in-memory `fake` provider when they need a complete resource lifecycle that does not depend on Matomo.

### Exit codes

| Code | Meaning |
|------|---------|
| `0` | Success. For `plan`, succeeded and no changes are required. For `apply`, planned mutations completed |
| `1` | Command failure (failed planning, apply, or import) |
| `2` | `plan` succeeded and changes are present |
| `3` | Invalid usage (unknown command/flag or bad arguments) |

## Development

See [CONTRIBUTING.md](CONTRIBUTING.md) for local development, testing, and pull request expectations.

Contributions and ideas are welcome.
