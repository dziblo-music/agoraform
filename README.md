# Agoraform

Marketing Infrastructure as Code.

Agoraform **0.1.0** is a CLI that declares Matomo analytics goals in YAML,
shows a non-mutating plan, applies creates and updates, and imports existing
goals into local management.

```text
validate -> plan -> apply -> plan
```

The last `plan` in that sequence reports no changes when the remote goal and
local state are unchanged.

## 0.1.0 status

| Included | Not included |
| --- | --- |
| `validate`, `plan`, `apply`, `import` | Google Ads, Meta Ads |
| `matomo.goal`, `matomo.variable`, `matomo.trigger`, and `matomo.tag` | Matomo Tag Manager versions or publish |
| Local `agoraform.state.json` | Remote state, workspaces, locking, encryption |
| Create and update | Destroy / delete of remote objects |
| Environment-variable Matomo auth | Credentials in manifests |
| Reviewable `plan` before `apply` | Budget safeguards, approval prompts, automatic rollback |

Later work may add other providers and remaining Tag Manager publishing.
Container versions are not implemented yet.

Agoraform is licensed under the [Apache License 2.0](LICENSE).

## Install

Requires no runtime besides the binary. Official release artifacts are built
with Go 1.26.7 and `CGO_ENABLED=0`.

### GitHub Releases

1. Download the archive for your OS and architecture from
   [GitHub Releases](https://github.com/dziblo-music/agoraform/releases)
   (tag `v0.1.0` for this version).
2. Download `checksums.txt` from the same release and verify the archive.
3. Extract `agoraform` (Windows: `agoraform.exe`) and place it on `PATH`.
4. Confirm the SemVer 2.0 version string:

```bash
agoraform --version
```

A 0.1.0 release binary prints `0.1.0`. Git tags use the Go convention with a
`v` prefix (`v0.1.0`); the CLI version string does not. Untagged local builds
print `0.0.0-dev`.

Archives:

| File | Platform |
| --- | --- |
| `agoraform_0.1.0_linux_amd64.tar.gz` | Linux amd64 |
| `agoraform_0.1.0_linux_arm64.tar.gz` | Linux arm64 |
| `agoraform_0.1.0_darwin_amd64.tar.gz` | macOS amd64 |
| `agoraform_0.1.0_darwin_arm64.tar.gz` | macOS arm64 |
| `agoraform_0.1.0_windows_amd64.zip` | Windows amd64 |

Release archives also contain `README.md`, `CHANGELOG.md`, license files, and
the `examples/agoraform.yaml` quickstart manifest.

#### Verify checksums

Linux example:

```bash
sha256sum -c checksums.txt --ignore-missing
```

macOS example (replace the archive name for Intel Macs):

```bash
shasum -a 256 agoraform_0.1.0_darwin_arm64.tar.gz
grep 'agoraform_0.1.0_darwin_arm64.tar.gz' checksums.txt
```

The two SHA-256 values must match.

Windows PowerShell:

```powershell
Get-FileHash .\agoraform_0.1.0_windows_amd64.zip -Algorithm SHA256
Select-String 'agoraform_0.1.0_windows_amd64.zip' .\checksums.txt
```

The two SHA-256 values must match before you run the binary.

### go install

Requires Go 1.26.7 or newer.

```bash
go install github.com/dziblo-music/agoraform/cmd/agoraform@v0.1.0
```

The module version in the tag is `v0.1.0`. The CLI still prints `0.1.0`.

### Build from source

Requires Go 1.26.7 or newer.

```bash
git clone https://github.com/dziblo-music/agoraform.git
cd agoraform
go build -o agoraform ./cmd/agoraform
./agoraform --version
```

On Windows:

```bash
go build -o agoraform.exe ./cmd/agoraform
```

A plain untagged `go build` reports `0.0.0-dev`. Release builds inject the
version rather than hard-coding it in source. To reproduce a 0.1.0 version
stamp locally:

```bash
go build -ldflags "-X github.com/dziblo-music/agoraform/internal/cli.Version=0.1.0" -o agoraform ./cmd/agoraform
```

## Quickstart

You need a Matomo site you are allowed to change, plus:

```text
MATOMO_URL            Matomo base URL, for example https://matomo.example.com
MATOMO_TOKEN_AUTH     API token
MATOMO_SITE_ID        numeric site id
MATOMO_CONTAINER_ID   Tag Manager container id (required for matomo.variable, matomo.trigger, and matomo.tag)
```

`validate`, `plan`, `apply`, and `import` call Matomo. They fail without
those variables and a reachable instance. Credentials never belong in YAML.

From an extracted release archive or a source checkout, copy the included
example manifest:

```bash
cp examples/agoraform.yaml agoraform.yaml
```

On Windows PowerShell:

```powershell
Copy-Item .\examples\agoraform.yaml .\agoraform.yaml
```

Then:

```bash
agoraform validate
agoraform plan      # expect a create; exit code 2 when changes exist
agoraform apply
agoraform plan      # no changes; exit code 0
```

The example declares one `matomo.goal`:

```yaml
apiVersion: agoraform.io/v1alpha1
resources:
  - address: matomo.goal.trial_started
    attributes:
      name: Trial Started
      matchAttribute: event_action
      pattern: trialStarted
```

Successful apply writes the Matomo goal id to `agoraform.state.json` next to
the manifest. That file is local management metadata; do not put tokens in it,
and do not commit it unless you have an explicit sharing workflow.

### Import an existing goal

```bash
agoraform import matomo.goal.trial_started 12
```

Import prints YAML for configurable fields and records the remote id in local
state. It does not rewrite `agoraform.yaml` and does not change Matomo.
Paste the printed resource into the manifest, then `agoraform plan` should
report no changes if the live goal still matches.

## Commands

Default manifest path: `agoraform.yaml`.

```bash
agoraform validate
agoraform validate -f path/to/manifest.yaml

agoraform plan
agoraform plan -f path/to/manifest.yaml

agoraform apply
agoraform apply -f path/to/manifest.yaml

agoraform import ADDRESS REMOTE-ID
agoraform import -f path/to/manifest.yaml ADDRESS REMOTE-ID
```

| Command | 0.1.0 behavior |
| --- | --- |
| `validate` | Schema, addresses, duplicates, provider type, Matomo connectivity, goal fields |
| `plan` | Diff vs live Matomo; never mutates; reads `agoraform.state.json` |
| `apply` | Executes planned creates and updates; persists identities; no destroy |
| `import` | Binds one existing remote id; prints YAML; no remote mutation |

Details: [docs/manifest.md](docs/manifest.md), [docs/plan.md](docs/plan.md),
[docs/apply.md](docs/apply.md), [docs/import.md](docs/import.md),
[docs/state.md](docs/state.md), [providers/matomo/README.md](providers/matomo/README.md).

### Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Success. For `plan`, no changes. For `apply`, planned mutations finished |
| `1` | Command failure (plan, apply, or import error) |
| `2` | `plan` succeeded and changes are present |
| `3` | Invalid usage |

## Development

See [CONTRIBUTING.md](CONTRIBUTING.md). Release tagging and artifact
verification are in [docs/release.md](docs/release.md). Changes by version
are in [CHANGELOG.md](CHANGELOG.md).
