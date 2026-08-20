package main

import (
	"os"
	"testing"
)

func TestGithubAppID(t *testing.T) {
	t.Setenv("GITHUB_APP_ID", "123456")
	got, err := githubAppID()
	if err != nil {
		t.Fatal(err)
	}
	if got != 123456 {
		t.Fatalf("got %d, want 123456", got)
	}

	t.Setenv("GITHUB_APP_ID", "")
	if _, err := githubAppID(); err == nil {
		t.Fatal("expected an error when GITHUB_APP_ID is unset")
	}

	t.Setenv("GITHUB_APP_ID", "not-a-number")
	if _, err := githubAppID(); err == nil {
		t.Fatal("expected an error for a non-numeric GITHUB_APP_ID")
	}
}

func TestGithubAppPrivateKey(t *testing.T) {
	const pem = "-----BEGIN RSA PRIVATE KEY-----\nabc\n-----END RSA PRIVATE KEY-----\n"

	// Inline, as an env var on the deployment.
	t.Setenv("GITHUB_APP_PRIVATE_KEY", pem)
	got, err := githubAppPrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != pem {
		t.Fatalf("got %q, want the inline key", got)
	}

	// From a file, as a mounted secret would be.
	path := t.TempDir() + "/key.pem"
	if err := os.WriteFile(path, []byte(pem), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITHUB_APP_PRIVATE_KEY", "")
	t.Setenv("GITHUB_APP_PRIVATE_KEY_PATH", path)
	got, err = githubAppPrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != pem {
		t.Fatalf("got %q, want the key from %s", got, path)
	}

	// Neither configured.
	t.Setenv("GITHUB_APP_PRIVATE_KEY_PATH", "")
	if _, err := githubAppPrivateKey(); err == nil {
		t.Fatal("expected an error when no key is configured")
	}

	// Configured, but the file is missing.
	t.Setenv("GITHUB_APP_PRIVATE_KEY_PATH", path+".missing")
	if _, err := githubAppPrivateKey(); err == nil {
		t.Fatal("expected an error when the key file does not exist")
	}
}

func TestGithubInstallationIDFromEnv(t *testing.T) {
	t.Setenv("GITHUB_APP_INSTALLATION_ID", "987654")
	got, err := githubInstallationID(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != 987654 {
		t.Fatalf("got %d, want 987654", got)
	}

	t.Setenv("GITHUB_APP_INSTALLATION_ID", "not-a-number")
	if _, err := githubInstallationID(nil); err == nil {
		t.Fatal("expected an error for a non-numeric GITHUB_APP_INSTALLATION_ID")
	}
}
