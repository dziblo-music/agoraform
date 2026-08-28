# Google Ads setup

Agoraform v0.4.0 uses the Google Ads API through OAuth 2.0 **single-user
authentication**. Before using the `googleads` provider, configure Google Ads
API access and provide the required runtime values through `.agoraform.env` or
the process environment.

Google also supports service-account and multi-user OAuth workflows. Agoraform
v0.4.0 does not implement those authentication modes; supply a client ID,
client secret, and previously issued refresh token for one authorized Google
user.

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

Google requires a Google Cloud/API Console project for OAuth credentials and
Google Ads API enablement. Billing is optional for Google Ads API use.

1. Open Google Cloud Console and select or create a project for Agoraform.
2. Open **APIs & Services -> Library**.
3. Find **Google Ads API** and click **Enable**.

Google documentation:
https://developers.google.com/google-ads/api/docs/oauth/cloud-project

Use a project that has not already been configured for Google Ads API access
with a different developer token. Google permits one developer token per Cloud
project.

## 3. Configure the Google Auth Platform app

Google Cloud now exposes OAuth application configuration under **Google Auth
Platform**. Older Google documentation and screenshots may call this the
**OAuth consent screen**.

For a new project:

1. Open **Google Auth Platform -> Overview** and click **Get started**.
2. Enter an app name, user-support email, audience, and developer contact
   information.
3. For a personal/single-user CLI setup, choose an audience that includes the
   Google account that will authorize Agoraform. An external app may remain in
   **Testing** while you are developing it; add the authorizing account as a
   test user under **Audience**.
4. Open **Data Access** and add this scope:

   ```text
   https://www.googleapis.com/auth/adwords
   ```

5. Review **Branding**, **Audience**, and **Data Access** before generating the
   OAuth client.

Google Auth Platform documentation:
https://support.google.com/cloud/answer/15544987

The authorizing Google account must have access to the Google Ads advertiser
account that Agoraform will manage.

### Testing versus production OAuth status

The Google Ads scope is restricted. Google permits development/testing before
verification, but OAuth apps left in **Testing** have tester limits and a
limited refresh-token lifetime. For long-lived production automation, follow
Google's OAuth production-readiness and verification requirements rather than
relying indefinitely on a Testing refresh token.

Google documentation:
https://developers.google.com/identity/protocols/oauth2/production-readiness/sensitive-scope-verification
https://developers.google.com/google-ads/api/docs/productionize/secure-credentials

## 4. Create the OAuth client ID and secret

For Agoraform's single-user CLI flow, use a Desktop app client because that is
the flow Google documents for generating credentials with `gcloud`.

1. Open **Google Auth Platform -> Clients**.
2. Click **Create client**.
3. Select **Desktop app** as the application type.
4. Create the client and download its JSON file as `credentials.json`.

The downloaded file contains the values used for:

```text
GOOGLE_ADS_CLIENT_ID
GOOGLE_ADS_CLIENT_SECRET
```

Google documentation:
https://developers.google.com/google-ads/api/docs/oauth/single-user-authentication

## 5. Generate a refresh token

Google's documented single-user flow uses `gcloud` to authorize the Google Ads
user and generate a reusable refresh token.

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

Do not commit either credentials JSON file. Treat the refresh token and client
secret like passwords.

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

## 8. Verify authentication before applying changes

With a manifest containing `providers.googleads: {}`, run:

```bash
agoraform validate
agoraform plan
```

These commands may contact Google Ads but do not mutate the account. Confirm
that authentication succeeds and that diagnostics do not print any configured
secret value.

Review the plan before running:

```bash
agoraform apply
agoraform plan
```

An unchanged post-apply plan should report `No changes.`

When testing against a production Google Ads account, review conversion-action,
conversion-goal, budget, campaign, and serving-state settings carefully before
`apply`: goal biddability, primary conversion settings, and campaign status
can affect optimization and spend. The v0.4.0 release checklist requires
mutation verification against a non-production Google Ads account before
publishing the release. New Search campaigns, ad groups, positive keywords,
and Responsive Search Ads default to `PAUSED`.

For a complete v0.4.0 Search campaign example, see
[`examples/googleads-search/`](../examples/googleads-search/README.md).
For the conversion-measurement-only v0.3.0 example, see
[`examples/googleads-conversion/`](../examples/googleads-conversion/README.md).
