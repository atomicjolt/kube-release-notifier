package main

import (
	"os"
	"reflect"
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

func TestE2EDispatch(t *testing.T) {
	common := map[string]string{
		"canvasDomain":  "atomicjolt.instructure.com",
		"appEnv":        "Dev",
		"label":         "Atomic Assessments",
		"slackThreadTs": "1700000000.000100",
	}
	with := func(key, value string) map[string]string {
		inputs := map[string]string{key: value}
		for k, v := range common {
			inputs[k] = v
		}
		return inputs
	}

	tests := []struct {
		runner       string
		tags         string
		wantWorkflow string
		wantInputs   map[string]string
	}{
		{"", "AtomicAssessments --Broken", "manual-run.yml", with("tags", "AtomicAssessments --Broken")},
		{"cypress", "AtomicAssessments --Broken", "manual-run.yml", with("tags", "AtomicAssessments --Broken")},
		{"playwright", "@AtomicAssessments", "manual-run-playwright.yml", with("grep", "@AtomicAssessments")},
	}
	for _, tt := range tests {
		workflow, inputs, err := e2eDispatch(tt.runner, "Dev", "Atomic Assessments", tt.tags, "atomicjolt.instructure.com", "1700000000.000100")
		if err != nil {
			t.Fatalf("runner %q: %v", tt.runner, err)
		}
		if workflow != tt.wantWorkflow {
			t.Errorf("runner %q: got workflow %s, want %s", tt.runner, workflow, tt.wantWorkflow)
		}
		if !reflect.DeepEqual(inputs, tt.wantInputs) {
			t.Errorf("runner %q: got inputs %v, want %v", tt.runner, inputs, tt.wantInputs)
		}
	}

	if _, _, err := e2eDispatch("selenium", "Dev", "Atomic Assessments", "x", "atomicjolt.instructure.com", ""); err == nil {
		t.Fatal("expected an error for an unknown runner")
	}
}
