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

Agoraform is currently in early development. The CLI scaffold exists; provider integrations and reconciliation logic are still in progress.

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

Current subcommands (`validate`, `plan`, `apply`, `import`) are registered shells and return a clear not-implemented error until their behavior is added.

### Exit codes

| Code | Meaning |
|------|---------|
| `0` | Success |
| `1` | Command failure (including not-implemented commands) |
| `2` | Invalid usage (unknown command/flag or bad arguments) |

## Development

See [CONTRIBUTING.md](CONTRIBUTING.md) for local development, testing, and pull request expectations.

Contributions and ideas are welcome.
