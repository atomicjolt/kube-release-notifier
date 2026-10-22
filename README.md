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

## Issue statuses

When a Deployment with these annotations rolls out a new tag, the notifier
dispatches the `mark-deployed.yml` workflow in `atomicjolt/aj-app-releases`.
That workflow moves the issues closed by PRs between the previous and new
tags to a Status on the given GitHub Projects, and records the new tag and
the environment in their Release and Deployed fields on boards that have
them. Set the first three to enable it:

| Annotation | Example |
| --- | --- |
| `atomicjolt.com/release-notifier-issues-repo` | `atomicjolt/atomic-assessments` |
| `atomicjolt.com/release-notifier-issues-projects` | `25,40` |
| `atomicjolt.com/release-notifier-issues-status` | `On Dev` |
| `atomicjolt.com/release-notifier-issues-environment` (optional) | `Dev` |

The GitHub App's installation must also cover `aj-app-releases`, with
**Actions: read & write**.
