package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

func notifyGithub(env, label, tags, ref string) {
	url := "https://api.github.com/repos/atomicjolt/atomic-e2e-testing/actions/workflows/manual-run.yml/dispatches"
	githubToken := os.Getenv("GITHUB_TOKEN")

	payload := map[string]any{
		"ref": ref,
		"inputs": map[string]string{
			"canvasDomain": "atomicjolt.instructure.com",
			"tags":         tags,
			"appEnv":       env,
			"label":        label,
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
	req.Header.Set("Authorization", "token "+githubToken)
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
