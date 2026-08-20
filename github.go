package main

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	githubAPI      = "https://api.github.com"
	targetOwner    = "atomicjolt"
	targetRepo     = "atomic-e2e-testing"
	targetWorkflow = "manual-run.yml"
)

// The installation token is the credential the app actually acts as. GitHub
// issues them with about an hour of life, so we hold on to one until it is
// close to expiring rather than minting a fresh one per dispatch.
var (
	tokenMu      sync.Mutex
	cachedToken  string
	cachedExpiry time.Time
)

func notifyGithub(env, label, tags, ref, lmsDomain, slackThreadTs string) {
	token, err := githubInstallationToken()
	if err != nil {
		fmt.Println("Failed to get GitHub installation token:", err)
		return
	}

	url := fmt.Sprintf("%s/repos/%s/%s/actions/workflows/%s/dispatches", githubAPI, targetOwner, targetRepo, targetWorkflow)

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

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Request failed:", err)
		return
	}
	defer resp.Body.Close()

	fmt.Printf("Response status: %s\n", resp.Status)
}

// githubInstallationToken returns a token the GitHub App can use to act as the
// bot, minting a new one when the cached token is missing or nearly expired.
func githubInstallationToken() (string, error) {
	tokenMu.Lock()
	defer tokenMu.Unlock()

	if cachedToken != "" && time.Now().Before(cachedExpiry.Add(-time.Minute)) {
		return cachedToken, nil
	}

	appID := os.Getenv("GITHUB_APP_ID")
	if appID == "" {
		return "", fmt.Errorf("GITHUB_APP_ID is not set")
	}

	key, err := githubAppPrivateKey()
	if err != nil {
		return "", err
	}

	jwt, err := githubAppJWT(appID, key)
	if err != nil {
		return "", fmt.Errorf("unable to sign app JWT: %w", err)
	}

	installationID := os.Getenv("GITHUB_APP_INSTALLATION_ID")
	if installationID == "" {
		installationID, err = githubInstallationID(jwt)
		if err != nil {
			return "", err
		}
	}

	url := fmt.Sprintf("%s/app/installations/%s/access_tokens", githubAPI, installationID)
	body, err := githubAppRequest("POST", url, jwt)
	if err != nil {
		return "", err
	}

	var result struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("unable to parse installation token response: %w", err)
	}
	if result.Token == "" {
		return "", fmt.Errorf("installation token response contained no token")
	}

	cachedToken = result.Token
	cachedExpiry = result.ExpiresAt
	return cachedToken, nil
}

// githubInstallationID looks up the installation of this app on the target
// repo, so the installation id does not have to be configured by hand.
func githubInstallationID(jwt string) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/installation", githubAPI, targetOwner, targetRepo)
	body, err := githubAppRequest("GET", url, jwt)
	if err != nil {
		return "", err
	}

	var result struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("unable to parse installation response: %w", err)
	}
	if result.ID == 0 {
		return "", fmt.Errorf("app is not installed on %s/%s", targetOwner, targetRepo)
	}

	return fmt.Sprintf("%d", result.ID), nil
}

func githubAppRequest(method, url, jwt string) ([]byte, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+jwt)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("%s %s returned %s: %s", method, url, resp.Status, strings.TrimSpace(string(body)))
	}

	return body, nil
}

// githubAppPrivateKey loads the app's RSA key, either inline from
// GITHUB_APP_PRIVATE_KEY or from the file at GITHUB_APP_PRIVATE_KEY_PATH.
func githubAppPrivateKey() (*rsa.PrivateKey, error) {
	keyPEM := []byte(os.Getenv("GITHUB_APP_PRIVATE_KEY"))
	if len(keyPEM) == 0 {
		path := os.Getenv("GITHUB_APP_PRIVATE_KEY_PATH")
		if path == "" {
			return nil, fmt.Errorf("neither GITHUB_APP_PRIVATE_KEY nor GITHUB_APP_PRIVATE_KEY_PATH is set")
		}
		var err error
		keyPEM, err = os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("unable to read private key: %w", err)
		}
	}

	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return nil, fmt.Errorf("private key is not valid PEM")
	}

	// GitHub hands out PKCS#1 keys, but accept PKCS#8 in case the key has
	// been converted along the way.
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("unable to parse private key: %w", err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is not an RSA key")
	}
	return key, nil
}

// githubAppJWT builds the short-lived RS256 token GitHub requires to
// authenticate as the app itself.
func githubAppJWT(appID string, key *rsa.PrivateKey) (string, error) {
	now := time.Now()
	header := map[string]string{"alg": "RS256", "typ": "JWT"}
	claims := map[string]any{
		// Backdate iat to tolerate clock drift against GitHub.
		"iat": now.Add(-60 * time.Second).Unix(),
		"exp": now.Add(8 * time.Minute).Unix(),
		"iss": appID,
	}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	enc := base64.RawURLEncoding
	signingInput := enc.EncodeToString(headerJSON) + "." + enc.EncodeToString(claimsJSON)

	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}

	return signingInput + "." + enc.EncodeToString(signature), nil
}
