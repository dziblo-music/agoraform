# Multi-file configuration

This example is the [Matomo conversion](../matomo-conversion/README.md)
workflow split across YAML files so a campaign can be organized by concern
while Agoraform still evaluates one logical configuration and one local
state file.

```text
multi-file/
├── providers.agoraform.yaml
├── variables.agoraform.yaml
└── tags.agoraform.yaml
```

`tags.agoraform.yaml` references `matomo.variable.user_id` from
`variables.agoraform.yaml`. File names and lexical order do not change
dependency order; the tag still waits for the variable and trigger.

## Usage

Copy the directory and run commands against it:

```bash
cp -r examples/multi-file ./campaign
cd campaign

export MATOMO_URL=https://matomo.example.com
export MATOMO_TOKEN_AUTH=replace-with-your-api-token
export MATOMO_SITE_ID=1
export MATOMO_CONTAINER_ID=replace-with-your-container-id

agoraform validate .
agoraform plan .
agoraform apply .
agoraform plan .
```

On Windows PowerShell, copy with `Copy-Item -Recurse`. Keep
`MATOMO_TOKEN_AUTH` out of shell history, logs, source control, and YAML.

Passing an explicit file still loads only that file:

```bash
agoraform validate -f tags.agoraform.yaml
```

That command fails here because the tag's `$ref` targets live in other
files. Directory mode is required for this layout.

State is `agoraform.state.json` in the configuration directory, not one
file per YAML document. See [Manifest format](../../docs/manifest.md#multi-file-configuration)
and the [Matomo conversion example](../matomo-conversion/README.md) for
prerequisites, application events, import, and publication behavior.
