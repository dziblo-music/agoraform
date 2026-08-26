# Google Ads setup

Agoraform uses the Google Ads API through OAuth 2.0 user credentials. Before
using the `googleads` provider, configure Google Ads API access and provide the
required runtime values through `.agoraform.env` or the process environment.

This guide covers the current Agoraform authentication flow. Service-account
authentication is not currently supported by Agoraform.

## Required values

Agoraform expects:

```text
GOOGLE_ADS_DEVELOPER_TOKEN
GOOGLE_ADS_CLIENT_ID
GOOGLE_ADS_CLIENT_SECRET
GOOGLE_ADS_REFRESH_TOKEN
GOOGLE_ADS_CUSTOMER_ID
GOOGLE_ADS_LOGIN_CUSTOMER_ID   optional
```

Keep tokens and client secrets out of manifests, source control, logs, shell
history, and `agoraform.state.json`.

## 1. Get a Google Ads developer token

A Google Ads **Manager Account** is required to obtain a developer token.

1. Create or sign in to a Google Ads Manager Account.
2. Open **Admin -> API Center**.
3. Apply for Google Ads API access and copy the developer token.
4. Check the token access level:
   - **Test Account Access** can call test accounts only.
   - **Explorer, Basic, or Standard Access** can call production accounts,
     subject to Google's limits and restrictions for that access level.

Google documentation:
https://developers.google.com/google-ads/api/docs/api-policy/developer-token

The manager account that owns the developer token does not have to be the
advertiser account that Agoraform manages.

## 2. Create or select a Google Cloud project

Google requires a Google Cloud/API Console project for OAuth credentials.
Billing is optional for Google Ads API use.

1. Open Google Cloud Console.
2. Create or select a project for Agoraform.
3. Open **APIs & Services -> Library**.
4. Find **Google Ads API** and click **Enable**.

Google documentation:
https://developers.google.com/google-ads/api/docs/oauth/cloud-project

## 3. Configure OAuth consent

In the same Google Cloud project:

1. Open **APIs & Services -> OAuth consent screen**.
2. Configure the application name and required contact information.
3. Add this OAuth scope:

   ```text
   https://www.googleapis.com/auth/adwords
   ```

4. If the OAuth app is in **Testing** status, add the Google account that will
   authorize Agoraform as a test user.

The authorizing Google account must have access to the Google Ads advertiser
account that Agoraform will manage.

## 4. Create the OAuth client ID and secret

For the simplest single-user CLI setup:

1. Open **APIs & Services -> Credentials**.
2. Choose **Create credentials -> OAuth client ID**.
3. Select **Desktop app**.
4. Create the client and download its JSON file as `credentials.json`.

The downloaded file contains the values used for:

```text
GOOGLE_ADS_CLIENT_ID
GOOGLE_ADS_CLIENT_SECRET
```

Google documentation:
https://developers.google.com/google-ads/api/docs/oauth/single-user-authentication

## 5. Generate a refresh token

Google's single-user flow uses `gcloud` to authorize the Google Ads user and
generate a reusable refresh token.

Install the Google Cloud CLI, then run:

```bash
gcloud auth application-default login \
  --scopes=https://www.googleapis.com/auth/adwords,https://www.googleapis.com/auth/cloud-platform \
  --client-id-file=./credentials.json
```

Sign in with the Google account that has access to the target Google Ads
account and approve the requested scopes.

The command prints the path to an
`application_default_credentials.json` file. Open that file and copy:

```json
{
  "client_id": "...",
  "client_secret": "...",
  "refresh_token": "..."
}
```

Use `refresh_token` as `GOOGLE_ADS_REFRESH_TOKEN`.

Do not commit either credentials JSON file.

## 6. Get the Google Ads customer IDs

Open the Google Ads account that Agoraform should manage and copy its customer
ID. Agoraform accepts the ID with or without hyphens.

```text
123-456-7890 -> GOOGLE_ADS_CUSTOMER_ID=1234567890
```

If the OAuth user accesses that advertiser **through a Manager Account**, also
set that manager's customer ID:

```text
GOOGLE_ADS_LOGIN_CUSTOMER_ID=0987654321
```

If the OAuth user has direct access to the advertiser, the login customer ID
can normally be omitted.

## 7. Configure Agoraform

For local use, create `.agoraform.env` next to `agoraform.yaml`:

```dotenv
GOOGLE_ADS_DEVELOPER_TOKEN=replace-with-developer-token
GOOGLE_ADS_CLIENT_ID=replace-with-client-id
GOOGLE_ADS_CLIENT_SECRET=replace-with-client-secret
GOOGLE_ADS_REFRESH_TOKEN=replace-with-refresh-token
GOOGLE_ADS_CUSTOMER_ID=1234567890
# GOOGLE_ADS_LOGIN_CUSTOMER_ID=0987654321
```

Agoraform loads this file automatically before initializing providers. You can
copy the repository `.agoraform.env.example` template and fill in the values
you need. Add `.agoraform.env` to the `.gitignore` of the repository containing
your campaign; the Agoraform source repository already ignores it.

If a process environment variable is already set, it overrides the value in
`.agoraform.env`. Direct PowerShell/Bash environment variables therefore remain
fully supported and are preferred for CI/CD or secret-manager injection.

See [Local provider configuration](local-configuration.md) for file format,
precedence, and security guidance.

## 8. Verify before applying changes

With a manifest containing `providers.googleads: {}`, run:

```bash
agoraform validate
agoraform plan
```

Review the plan before running:

```bash
agoraform apply
agoraform plan
```

An unchanged post-apply plan should report `No changes.`

When testing against a production Google Ads account, review conversion-action
and conversion-goal settings carefully before `apply`: goal biddability and
primary conversion settings can affect campaign optimization.

For a complete v0.3 conversion example, see
[`examples/googleads-conversion/`](../examples/googleads-conversion/README.md).
