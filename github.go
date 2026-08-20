package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v88/github"
)

const (
	targetOwner    = "atomicjolt"
	targetRepo     = "atomic-e2e-testing"
	targetWorkflow = "manual-run.yml"
)

// The authenticated client is built once and reused. ghinstallation's
// transport caches the installation token and refreshes it shortly before it
// expires, so sharing one client avoids re-minting a token per dispatch.
var (
	clientMu     sync.Mutex
	cachedClient *http.Client
)

func notifyGithub(env, label, tags, ref, lmsDomain, slackThreadTs string) {
	client, err := githubClient()
	if err != nil {
		fmt.Println("Failed to build GitHub client:", err)
		return
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/actions/workflows/%s/dispatches", targetOwner, targetRepo, targetWorkflow)

	payload := map[string]any{
		"ref": ref,
		"inputs": map[string]string{
			"canvasDomain":  lmsDomain,
			"tags":          tags,
			"appEnv":        env,
			"label":         label,
			"slackThreadTs": slackThreadTs,
		},
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		fmt.Println("Failed to marshal JSON:", err)
		return
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		fmt.Println("Failed to create request:", err)
		return
	}

	// The installation transport sets Authorization for us.
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Request failed:", err)
		return
	}
	defer resp.Body.Close()

	fmt.Printf("Response status: %s\n", resp.Status)
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

// githubInstallationID looks up the installation of this app on the target
// repo, so the installation id does not have to be configured by hand.
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

	installation, _, err := client.Apps.GetRepositoryInstallation(context.Background(), targetOwner, targetRepo)
	if err != nil {
		return 0, fmt.Errorf("unable to find app installation on %s/%s: %w", targetOwner, targetRepo, err)
	}
	return installation.GetID(), nil
}
