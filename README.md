# Agoraform

Marketing Infrastructure as Code.

Agoraform is an open-source Go CLI for declaring marketing-platform
configuration in YAML, reviewing changes with a non-mutating plan, and
reconciling those changes through provider APIs.

The CLI stays provider-neutral:

```text
validate -> plan -> apply -> plan
```

`import` brings an existing remote object under management. Destructive
`destroy` behavior is not implemented yet.

## Releases

### v0.1.0 — current stable release

The first public release manages Matomo analytics goals only.

| Area | v0.1.0 |
| --- | --- |
| Commands | `validate`, `plan`, `apply`, `import` |
| Provider | Matomo |
| Resource | `matomo.goal` |
| State | local `agoraform.state.json` |
| Mutations | create and update |

The `v0.1.0` binaries do **not** contain Matomo Tag Manager resource
management or container publication.

### v0.2.0 — unreleased work on `main`

Current unreleased development adds:

- `matomo.variable` for Data Layer variables;
- `matomo.trigger` for Custom Event triggers;
- `matomo.tag` for Matomo Analytics event tags;
- explicit `$ref` dependencies between managed resources;
- Tag Manager import with logical-reference reconstruction;
- declarative Tag Manager container publication through `plan` and `apply`.

Publication does **not** add an `agoraform publish` command. Provider-specific
behavior is desired state in the manifest while the command surface remains
portable across providers.

```yaml
apiVersion: agoraform.io/v1alpha1
providers:
  matomo:
    publish: true
    environment: live
resources:
  - address: matomo.variable.user_id
    attributes:
      type: dataLayer
      key: userId
```

When publication is enabled, `plan` makes the action visible before any
mutation:

```text
> matomo.container.main: publish -> live
```

`apply` performs that provider action only after all planned Tag Manager draft
resource mutations succeed. Repeated unchanged applies do not create duplicate
container versions.

See [Matomo Tag Manager publication](docs/matomo-publishing.md).

## Install v0.1.0

Official release artifacts require no runtime besides the binary. They are
built with Go 1.26.7 and `CGO_ENABLED=0`.

### GitHub Releases

1. Download the archive for your OS and architecture from GitHub Releases
   (tag `v0.1.0`).
2. Download `checksums.txt` from the same release and verify the archive.
3. Extract `agoraform` (`agoraform.exe` on Windows) and place it on `PATH`.
4. Confirm the version:

```bash
agoraform --version
```

A v0.1.0 binary prints `0.1.0`. Git tags use the Go convention with a `v`
prefix. Untagged local builds print `0.0.0-dev`.

Release archives:

| File | Platform |
| --- | --- |
| `agoraform_0.1.0_linux_amd64.tar.gz` | Linux amd64 |
| `agoraform_0.1.0_linux_arm64.tar.gz` | Linux arm64 |
| `agoraform_0.1.0_darwin_amd64.tar.gz` | macOS amd64 |
| `agoraform_0.1.0_darwin_arm64.tar.gz` | macOS arm64 |
| `agoraform_0.1.0_windows_amd64.zip` | Windows amd64 |

Release archives also contain the README, changelog, license files, and the
example manifest.

### go install

Requires Go 1.26.7 or newer:

```bash
go install github.com/dziblo-music/agoraform/cmd/agoraform@v0.1.0
```

### Build current source

```bash
git clone https://github.com/dziblo-music/agoraform.git
cd agoraform
go build -o agoraform ./cmd/agoraform
./agoraform --version
```

On Windows:

```powershell
go build -o agoraform.exe ./cmd/agoraform
```

## Runtime configuration

Matomo credentials and connection details stay outside the manifest:

```text
MATOMO_URL            Matomo base URL, for example https://matomo.example.com
MATOMO_TOKEN_AUTH     API token
MATOMO_SITE_ID        numeric site id
MATOMO_CONTAINER_ID   Tag Manager container id for v0.2 Tag Manager resources
```

Credentials never belong in YAML, plan output, logs, or local state.

Non-secret provider desired state belongs in the manifest. For v0.2 Matomo
publication:

```yaml
providers:
  matomo:
    publish: true
    environment: live
```

`publish` defaults to `false`; `environment` defaults to `live` when
publication is enabled.

## Quickstart

Copy the included example manifest:

```bash
cp examples/agoraform.yaml agoraform.yaml
```

Then run:

```bash
agoraform validate
agoraform plan
agoraform apply
agoraform plan
```

The last plan should report no changes when desired configuration, local state,
and remote state are unchanged.

A v0.1.0 goal looks like:

```yaml
apiVersion: agoraform.io/v1alpha1
resources:
  - address: matomo.goal.trial_started
    attributes:
      name: Trial Started
      matchAttribute: event_action
      pattern: trialStarted
```

Successful creates write provider-native identities to
`agoraform.state.json` beside the manifest. Do not put credentials in that
file.

## Commands

Default manifest path: `agoraform.yaml`.

```bash
agoraform validate [-f path/to/manifest.yaml]
agoraform plan [-f path/to/manifest.yaml]
agoraform apply [-f path/to/manifest.yaml]
agoraform import [-f path/to/manifest.yaml] ADDRESS REMOTE-ID
```

| Command | Purpose |
| --- | --- |
| `validate` | Validate manifest, provider configuration, dependencies, connectivity, and resource schemas |
| `plan` | Read remote state and show all resource/provider actions; never mutate |
| `apply` | Execute the reviewed resource/provider convergence actions |
| `import` | Bind an existing remote identity and print configurable YAML; no remote mutation |

Provider-specific lifecycle verbs are intentionally not top-level commands.

### Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Success; for `plan`, no changes |
| `1` | Runtime/validation/provider failure |
| `2` | `plan` succeeded and changes are present |
| `3` | Invalid CLI usage |

## Documentation

- [Manifest format](docs/manifest.md)
- [Plan engine](docs/plan.md)
- [Apply execution](docs/apply.md)
- [Import](docs/import.md)
- [Matomo Tag Manager publication](docs/matomo-publishing.md)
- [Local state](docs/state.md)
- [Matomo provider](providers/matomo/README.md)
- [Release process](docs/release.md)
- [Changelog](CHANGELOG.md)

## Development

See [CONTRIBUTING.md](CONTRIBUTING.md) for the build, test, branch, and pull
request workflow.

Agoraform is licensed under the [Apache License 2.0](LICENSE).
