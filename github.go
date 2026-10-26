package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v88/github"
)

const (
	targetOwner = "atomicjolt"

	e2eRepo               = "atomic-e2e-testing"
	e2eCypressWorkflow    = "manual-run.yml"
	e2ePlaywrightWorkflow = "manual-run-playwright.yml"

	issuesRepo     = "aj-app-releases"
	issuesWorkflow = "mark-deployed.yml"
)

// The authenticated client is built once and reused. ghinstallation's
// transport caches the installation token and refreshes it shortly before it
// expires, so sharing one client avoids re-minting a token per dispatch.
var (
	clientMu     sync.Mutex
	cachedClient *http.Client
)

// notifyGithub runs the E2E tests for a deploy with the given runner
// ("cypress" when empty, or "playwright").
func notifyGithub(runner, env, label, tags, ref, lmsDomain, slackThreadTs string) error {
	workflow, inputs, err := e2eDispatch(runner, env, label, tags, lmsDomain, slackThreadTs)
	if err != nil {
		return err
	}
	return dispatchWorkflow(e2eRepo, workflow, ref, inputs)
}

// e2eDispatch picks the workflow for runner and builds its inputs. The two
// workflows take the tag filter under different names, and workflow_dispatch
// rejects inputs a workflow does not declare, so each gets only its own.
func e2eDispatch(runner, env, label, tags, lmsDomain, slackThreadTs string) (string, map[string]string, error) {
	inputs := map[string]string{
		"canvasDomain":  lmsDomain,
		"appEnv":        env,
		"label":         label,
		"slackThreadTs": slackThreadTs,
	}
	switch runner {
	case "", "cypress":
		inputs["tags"] = tags
		return e2eCypressWorkflow, inputs, nil
	case "playwright":
		// grepInvert and workers are left to the workflow's defaults.
		inputs["grep"] = tags
		return e2ePlaywrightWorkflow, inputs, nil
	default:
		return "", nil, fmt.Errorf("unknown e2e runner %q", runner)
	}
}

// markDeployed moves the issues closed by PRs in previousTag...tag of
// sourceRepo to status on the given GitHub Projects (comma-separated
// numbers), and records tag and environment (optional) on them, via the
// mark-deployed workflow in aj-app-releases.
func markDeployed(sourceRepo, projects, status, environment, tag, previousTag string) error {
	return dispatchWorkflow(issuesRepo, issuesWorkflow, "main", map[string]string{
		"source_repo":     sourceRepo,
		"image_tag":       tag,
		"previous_tag":    previousTag,
		"project_numbers": projects,
		"status_name":     status,
		"environment":     environment,
	})
}

// dispatchWorkflow triggers a workflow_dispatch run of workflow in
// targetOwner/repo on ref.
func dispatchWorkflow(repo, workflow, ref string, inputs map[string]string) error {
	client, err := githubClient()
	if err != nil {
		return fmt.Errorf("failed to build GitHub client: %w", err)
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/actions/workflows/%s/dispatches", targetOwner, repo, workflow)

	payload := map[string]any{
		"ref":    ref,
		"inputs": inputs,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// The installation transport sets Authorization for us.
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("dispatch of %s/%s failed: %w", repo, workflow, err)
	}
	defer resp.Body.Close()

	fmt.Printf("Dispatch %s/%s: %s\n", repo, workflow, resp.Status)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("dispatch of %s/%s failed: %s %s", repo, workflow, resp.Status, bytes.TrimSpace(body))
	}
	return nil
}

// githubClient returns an http.Client that authenticates as the GitHub App's
// installation, building it on first use.
func githubClient() (*http.Client, error) {
	clientMu.Lock()
	defer clientMu.Unlock()

	if cachedClient != nil {
		return cachedClient, nil
	}

	appID, err := githubAppID()
	if err != nil {
		return nil, err
	}

	keyPEM, err := githubAppPrivateKey()
	if err != nil {
		return nil, err
	}

	appTransport, err := ghinstallation.NewAppsTransport(http.DefaultTransport, appID, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("unable to authenticate as app %d: %w", appID, err)
	}

	installationID, err := githubInstallationID(appTransport)
	if err != nil {
		return nil, err
	}

	cachedClient = &http.Client{
		Transport: ghinstallation.NewFromAppsTransport(appTransport, installationID),
	}
	return cachedClient, nil
}

func githubAppID() (int64, error) {
	value := os.Getenv("GITHUB_APP_ID")
	if value == "" {
		return 0, fmt.Errorf("GITHUB_APP_ID is not set")
	}
	appID, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("GITHUB_APP_ID %q is not a number", value)
	}
	return appID, nil
}

// githubAppPrivateKey loads the app's PEM key, either inline from
// GITHUB_APP_PRIVATE_KEY or from the file at GITHUB_APP_PRIVATE_KEY_PATH.
func githubAppPrivateKey() ([]byte, error) {
	if key := os.Getenv("GITHUB_APP_PRIVATE_KEY"); key != "" {
		return []byte(key), nil
	}

	path := os.Getenv("GITHUB_APP_PRIVATE_KEY_PATH")
	if path == "" {
		return nil, fmt.Errorf("neither GITHUB_APP_PRIVATE_KEY nor GITHUB_APP_PRIVATE_KEY_PATH is set")
	}

	key, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("unable to read private key: %w", err)
	}
	return key, nil
}

// githubInstallationID looks up the installation of this app on the E2E
// repo, so the installation id does not have to be configured by hand. The
// same installation must also cover aj-app-releases.
func githubInstallationID(appTransport *ghinstallation.AppsTransport) (int64, error) {
	if value := os.Getenv("GITHUB_APP_INSTALLATION_ID"); value != "" {
		installationID, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("GITHUB_APP_INSTALLATION_ID %q is not a number", value)
		}
		return installationID, nil
	}

	client, err := github.NewClient(github.WithHTTPClient(&http.Client{Transport: appTransport}))
	if err != nil {
		return 0, fmt.Errorf("unable to build GitHub client: %w", err)
	}

	installation, _, err := client.Apps.GetRepositoryInstallation(context.Background(), targetOwner, e2eRepo)
	if err != nil {
		return 0, fmt.Errorf("unable to find app installation on %s/%s: %w", targetOwner, e2eRepo, err)
	}
	return installation.GetID(), nil
}
