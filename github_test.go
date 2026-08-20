package main

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"os"
	"strings"
	"testing"
	"time"
)

func TestGithubAppJWT(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	tok, err := githubAppJWT("123456", key)
	if err != nil {
		t.Fatal(err)
	}

	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 parts, got %d", len(parts))
	}

	// Signature must verify over header.claims with RS256.
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatal(err)
	}
	if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, digest[:], sig); err != nil {
		t.Fatalf("signature did not verify: %v", err)
	}

	var header map[string]string
	raw, _ := base64.RawURLEncoding.DecodeString(parts[0])
	if err := json.Unmarshal(raw, &header); err != nil {
		t.Fatal(err)
	}
	if header["alg"] != "RS256" || header["typ"] != "JWT" {
		t.Fatalf("bad header: %v", header)
	}

	var claims map[string]any
	raw, _ = base64.RawURLEncoding.DecodeString(parts[1])
	if err := json.Unmarshal(raw, &claims); err != nil {
		t.Fatal(err)
	}
	if claims["iss"] != "123456" {
		t.Fatalf("bad iss: %v", claims["iss"])
	}
	iat := int64(claims["iat"].(float64))
	exp := int64(claims["exp"].(float64))
	now := time.Now().Unix()
	if iat > now {
		t.Fatalf("iat %d is in the future (now %d)", iat, now)
	}
	if exp-iat > 600 {
		t.Fatalf("lifetime %ds exceeds GitHub's 10 minute max", exp-iat)
	}
	t.Logf("iat=%d exp=%d lifetime=%ds", iat, exp, exp-iat)
}

func TestGithubAppPrivateKey(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pkcs1 := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	pkcs8Bytes, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	pkcs8 := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8Bytes})

	// Inline PKCS#1 (what GitHub hands out).
	t.Setenv("GITHUB_APP_PRIVATE_KEY", string(pkcs1))
	got, err := githubAppPrivateKey()
	if err != nil {
		t.Fatalf("pkcs1 inline: %v", err)
	}
	if !got.Equal(key) {
		t.Fatal("pkcs1 inline: wrong key")
	}

	// Inline PKCS#8.
	t.Setenv("GITHUB_APP_PRIVATE_KEY", string(pkcs8))
	got, err = githubAppPrivateKey()
	if err != nil {
		t.Fatalf("pkcs8 inline: %v", err)
	}
	if !got.Equal(key) {
		t.Fatal("pkcs8 inline: wrong key")
	}

	// From a file, as a mounted secret would be.
	path := t.TempDir() + "/key.pem"
	if err := os.WriteFile(path, pkcs1, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITHUB_APP_PRIVATE_KEY", "")
	t.Setenv("GITHUB_APP_PRIVATE_KEY_PATH", path)
	got, err = githubAppPrivateKey()
	if err != nil {
		t.Fatalf("file: %v", err)
	}
	if !got.Equal(key) {
		t.Fatal("file: wrong key")
	}

	// Neither set.
	t.Setenv("GITHUB_APP_PRIVATE_KEY_PATH", "")
	if _, err := githubAppPrivateKey(); err == nil {
		t.Fatal("expected an error when no key is configured")
	}

	// Garbage PEM.
	t.Setenv("GITHUB_APP_PRIVATE_KEY", "not a pem")
	if _, err := githubAppPrivateKey(); err == nil {
		t.Fatal("expected an error for invalid PEM")
	}
}
