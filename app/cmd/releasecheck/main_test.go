package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func signedFixture(t *testing.T) (config, string) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	var c config
	c.Version = "0.1.0"
	c.Updates.GitHub = "example/photos"
	c.Updates.PublicKey = base64.StdEncoding.EncodeToString(public)
	root := t.TempDir()
	data := []byte("signed application archive")
	sum := sha256.Sum256(data)
	m := manifest{
		Version: c.Version, Notes: "Release notes",
		URL:  "https://github.com/example/photos/releases/download/v0.1.0/photos.tar.gz",
		Size: int64(len(data)), Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(private, sum[:])),
	}
	writeJSON(t, filepath.Join(root, "update-darwin-arm64.json"), m)
	if err := os.WriteFile(filepath.Join(root, "photos.tar.gz"), data, 0644); err != nil {
		t.Fatal(err)
	}
	return c, root
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestSignedArtifactsRejectTamperingAndMissingTargets(t *testing.T) {
	for _, test := range []struct {
		name, want string
		mutate     func(*testing.T, *config, string)
	}{
		{"valid", "", func(*testing.T, *config, string) {}},
		{"tamper", "signature", func(t *testing.T, c *config, root string) {
			path := filepath.Join(root, "photos.tar.gz")
			data, _ := os.ReadFile(path)
			data[0] ^= 1
			if err := os.WriteFile(path, data, 0644); err != nil {
				t.Fatal(err)
			}
		}},
		{"wrong key", "signature", func(t *testing.T, c *config, root string) {
			key, _, err := ed25519.GenerateKey(rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			c.Updates.PublicKey = base64.StdEncoding.EncodeToString(key)
		}},
		{"truncated", "size mismatch", func(t *testing.T, c *config, root string) {
			if err := os.Truncate(filepath.Join(root, "photos.tar.gz"), 1); err != nil {
				t.Fatal(err)
			}
		}},
		{"wrong version", "metadata", func(t *testing.T, c *config, root string) { c.Version = "0.2.0" }},
		{"wrong repository", "URL", func(t *testing.T, c *config, root string) { c.Updates.GitHub = "other/photos" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			c, root := signedFixture(t)
			test.mutate(t, &c, root)
			_, err := validateArtifacts(c, root, "darwin-arm64")
			if test.want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v; want %s", err, test.want)
			}
		})
	}
	c, root := signedFixture(t)
	if _, err := validateArtifacts(c, root, "darwin-arm64,windows-amd64"); err == nil {
		t.Fatal("accepted missing Windows manifest")
	}
	// Deltas must pass the same signature and size checks as full archives.
	path := filepath.Join(root, "update-darwin-arm64.json")
	data, _ := os.ReadFile(path)
	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	m.Deltas = []archive{{URL: m.URL, Size: m.Size, Signature: "invalid"}}
	writeJSON(t, path, m)
	if _, err := validateArtifacts(c, root, "darwin-arm64"); err == nil {
		t.Fatal("accepted unsigned delta")
	}
}

func TestAggregateChecksumsAndDuplicateScripts(t *testing.T) {
	c, root := signedFixture(t)
	for _, target := range []string{"darwin", "linux"} {
		dir := filepath.Join(root, target)
		if err := os.Mkdir(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "install.sh"), []byte("#!/bin/sh\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	files, err := validateArtifacts(c, root, "darwin-arm64")
	if err != nil {
		t.Fatal(err)
	}
	stage := filepath.Join(t.TempDir(), "assets")
	if err := stageArtifacts(files, stage); err != nil {
		t.Fatal(err)
	}
	checksums, err := os.ReadFile(filepath.Join(stage, "SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	for name := range files {
		sum, _, err := digest(filepath.Join(stage, name))
		if err != nil {
			t.Fatal(err)
		}
		want := fmt.Sprintf("%x  %s\n", sum, name)
		if !strings.Contains(string(checksums), want) {
			t.Fatalf("missing checksum %s", want)
		}
	}
	if err := stageArtifacts(files, stage); err == nil {
		t.Fatal("accepted nonempty stage")
	}
	if err := os.WriteFile(filepath.Join(root, "linux", "install.sh"), []byte("different"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := collect(root); err == nil {
		t.Fatal("accepted conflicting scripts")
	}
}

func TestReleaseMetadataMatchesTagAndChangelog(t *testing.T) {
	c, root := signedFixture(t)
	configPath := filepath.Join(root, "mygo.json")
	changelog := filepath.Join(root, "CHANGELOG.md")
	writeJSON(t, configPath, c)
	if err := os.WriteFile(changelog, []byte("## 0.2.0\nFuture\n## 0.1.0\nFirst release\n## 0.0.1\nOld\n"), 0644); err != nil {
		t.Fatal(err)
	}
	o := options{configPath: configPath, changelog: changelog, tag: "v0.1.0", repository: "example/photos"}
	if err := run(o); err != nil {
		t.Fatal(err)
	}
	o.tag = "v0.2.0"
	if err := run(o); err == nil {
		t.Fatal("accepted mismatched tag")
	}
	if _, err := releaseNotes("## 0.1.0\n\n## 0.0.1\nold", "0.1.0"); err == nil {
		t.Fatal("accepted empty notes")
	}
	notes, err := releaseNotes("## [0.1.0] - today\nFirst release\n## 0.0.1\nOld", "0.1.0")
	if err != nil || notes != "First release\n" {
		t.Fatalf("notes = %q, %v", notes, err)
	}
}
