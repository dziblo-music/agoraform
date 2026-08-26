# Local provider configuration

Agoraform keeps marketing desired state and provider credentials separate.

- `agoraform.yaml` is the version-controlled declaration of marketing resources.
- `.agoraform.env` is optional local configuration for credentials and connection details and must not be committed.

When Agoraform starts, it looks for `.agoraform.env` next to the selected manifest. With the default `agoraform.yaml`, that is the current directory. With `-f path/to/agoraform.yaml` or a positional manifest path, Agoraform loads `path/to/.agoraform.env`. If the file is absent, existing environment-variable behavior is unchanged.

## File format

Use dotenv-style `KEY=VALUE` entries:

```dotenv
# Matomo
MATOMO_URL=https://matomo.example.com
MATOMO_TOKEN_AUTH=replace-with-token
MATOMO_SITE_ID=1
MATOMO_CONTAINER_ID=replace-with-container-id

# Google Ads
GOOGLE_ADS_DEVELOPER_TOKEN=replace-with-developer-token
GOOGLE_ADS_CLIENT_ID=replace-with-client-id
GOOGLE_ADS_CLIENT_SECRET=replace-with-client-secret
GOOGLE_ADS_REFRESH_TOKEN=replace-with-refresh-token
GOOGLE_ADS_CUSTOMER_ID=1234567890
GOOGLE_ADS_LOGIN_CUSTOMER_ID=0987654321
```

Blank lines and lines beginning with `#` are ignored. Single- and double-quoted values are supported:

```dotenv
EXAMPLE_VALUE="value with spaces"
```

`export KEY=VALUE` is also accepted.

## Precedence

Existing process environment variables always win over `.agoraform.env`.

For example, given:

```dotenv
GOOGLE_ADS_CUSTOMER_ID=1111111111
```

this command uses `2222222222` instead:

```bash
GOOGLE_ADS_CUSTOMER_ID=2222222222 agoraform plan
```

This makes `.agoraform.env` useful for normal local defaults while still allowing CI, secret managers, or one-off shell overrides.

## Security

`.agoraform.env` is included in the repository `.gitignore`. Do not force-add it to Git.

Treat the file like any other local secrets file:

- do not commit or share it;
- restrict filesystem access where appropriate;
- use a secret manager for CI/CD and production automation;
- do not copy credentials into `agoraform.yaml`.

Agoraform does not intentionally write loaded values to the manifest, plan output, logs, or local state.

## Project layout

A typical project looks like:

```text
campaign/
├── agoraform.yaml       # committed
└── .agoraform.env       # local only, ignored by Git
```

Run Agoraform from that directory:

```bash
agoraform validate
agoraform plan
agoraform apply
agoraform plan
```

Or select a manifest from elsewhere:

```bash
agoraform plan -f campaign/agoraform.yaml
```

Agoraform will still use `campaign/.agoraform.env`.

Directly exported environment variables remain fully supported; `.agoraform.env` is optional convenience for local use.
