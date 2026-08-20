# kube-release-notifier

## GitHub App configuration

Workflow dispatches to `atomicjolt/atomic-e2e-testing` authenticate as a GitHub
App (bot) rather than with a personal access token. The app needs the
**Actions: read & write** repository permission and must be installed on that
repo.

| Variable | Required | Notes |
| --- | --- | --- |
| `GITHUB_APP_ID` | yes | The app's numeric ID, from its settings page. |
| `GITHUB_APP_PRIVATE_KEY` | one of these | PEM contents of a private key generated for the app. |
| `GITHUB_APP_PRIVATE_KEY_PATH` | one of these | Path to the PEM file, e.g. a mounted Kubernetes secret. |
| `GITHUB_APP_INSTALLATION_ID` | no | Looked up from the target repo when unset. |

The notifier exchanges the key for an installation access token on demand and
reuses it until shortly before it expires.
