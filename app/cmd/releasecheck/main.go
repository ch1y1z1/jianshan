// releasecheck validates version metadata and signed artifacts before a
// release becomes visible to installed applications.
package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type config struct {
	Version string `json:"version"`
	Updates struct {
		PublicKey string `json:"publicKey"`
		GitHub    string `json:"github"`
	} `json:"updates"`
}
type archive struct {
	URL       string `json:"url"`
	Size      int64  `json:"size"`
	Signature string `json:"signature"`
}
type manifest struct {
	Version   string    `json:"version"`
	Notes     string    `json:"notes"`
	URL       string    `json:"url"`
	Size      int64     `json:"size"`
	Signature string    `json:"signature"`
	Deltas    []archive `json:"deltas"`
}
type options struct{ configPath, changelog, tag, repository, artifacts, targets, stage, notes string }

var semver = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?$`)

func releaseNotes(text, version string) (string, error) {
	active := false
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "## ") {
			if active {
				break
			}
			heading := strings.TrimSpace(strings.TrimPrefix(line, "## "))
			active = heading == version || strings.HasPrefix(heading, "["+version+"]")
			continue
		}
		if active {
			lines = append(lines, line)
		}
	}
	notes := strings.TrimSpace(strings.Join(lines, "\n"))
	if notes == "" {
		return "", fmt.Errorf("changelog has no nonempty section for %s", version)
	}
	return notes + "\n", nil
}
func digest(path string) ([32]byte, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return [32]byte{}, 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum, n, err
}
func collect(root string) (map[string]string, error) {
	files := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if strings.HasSuffix(d.Name(), ".app") {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("unexpected symlink: %s", path)
		}
		name := d.Name()
		if name == "SHA256SUMS" {
			return nil
		}
		allowed := strings.HasSuffix(name, ".dmg") || strings.HasSuffix(name, ".exe") || strings.HasSuffix(name, ".deb") || strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".delta") || strings.HasPrefix(name, "update-") && strings.HasSuffix(name, ".json") || name == "install.sh" || name == "install.ps1"
		if !allowed {
			return nil
		}
		if previous := files[name]; previous != "" {
			x, _, err := digest(previous)
			if err != nil {
				return err
			}
			y, _, err := digest(path)
			if err != nil {
				return err
			}
			if x != y {
				return fmt.Errorf("conflicting artifact %s", name)
			}
		}
		files[name] = path
		return nil
	})
	return files, err
}
func verifyArchive(a archive, files map[string]string, key ed25519.PublicKey, c config) error {
	u, err := url.Parse(a.URL)
	if err != nil {
		return err
	}
	prefix := "/" + c.Updates.GitHub + "/releases/download/v" + c.Version + "/"
	if u.Scheme != "https" || u.Host != "github.com" || !strings.HasPrefix(u.Path, prefix) {
		return fmt.Errorf("unexpected update URL: %s", a.URL)
	}
	name := strings.TrimPrefix(u.Path, prefix)
	if name == "" || filepath.Base(name) != name || files[name] == "" {
		return fmt.Errorf("missing update archive: %s", name)
	}
	sum, size, err := digest(files[name])
	if err != nil {
		return err
	}
	if size != a.Size {
		return fmt.Errorf("size mismatch for %s", name)
	}
	signature, err := base64.StdEncoding.DecodeString(a.Signature)
	if err != nil || !ed25519.Verify(key, sum[:], signature) {
		return fmt.Errorf("invalid update signature for %s", name)
	}
	return nil
}
func validateArtifacts(c config, root, targets string) (map[string]string, error) {
	files, err := collect(root)
	if err != nil {
		return nil, err
	}
	key, err := base64.StdEncoding.DecodeString(c.Updates.PublicKey)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return nil, errors.New("invalid public key")
	}
	expected := map[string]bool{}
	for _, target := range strings.Split(targets, ",") {
		expected["update-"+strings.TrimSpace(target)+".json"] = true
	}
	seen := 0
	for name, path := range files {
		if !strings.HasPrefix(name, "update-") || !strings.HasSuffix(name, ".json") {
			continue
		}
		if !expected[name] {
			return nil, fmt.Errorf("unexpected target manifest: %s", name)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var m manifest
		if err := json.Unmarshal(b, &m); err != nil {
			return nil, err
		}
		if m.Version != c.Version || strings.TrimSpace(m.Notes) == "" {
			return nil, fmt.Errorf("invalid release metadata: %s", name)
		}
		if err := verifyArchive(archive{URL: m.URL, Size: m.Size, Signature: m.Signature}, files, key, c); err != nil {
			return nil, err
		}
		for _, d := range m.Deltas {
			if err := verifyArchive(d, files, key, c); err != nil {
				return nil, err
			}
		}
		seen++
	}
	if seen != len(expected) {
		return nil, fmt.Errorf("have %d update manifests; need %d (%s)", seen, len(expected), targets)
	}
	return files, nil
}
func stageArtifacts(files map[string]string, stage string) error {
	if err := os.MkdirAll(stage, 0755); err != nil {
		return err
	}
	existing, err := os.ReadDir(stage)
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return errors.New("staging directory must be empty")
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	var checksums strings.Builder
	for _, name := range names {
		source, err := os.Open(files[name])
		if err != nil {
			return err
		}
		mode := os.FileMode(0644)
		if name == "install.sh" {
			mode = 0755
		}
		dest, err := os.OpenFile(filepath.Join(stage, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
		if err != nil {
			source.Close()
			return err
		}
		h := sha256.New()
		_, err = io.Copy(io.MultiWriter(dest, h), source)
		source.Close()
		closeErr := dest.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		fmt.Fprintf(&checksums, "%s  %s\n", hex.EncodeToString(h.Sum(nil)), name)
	}
	return os.WriteFile(filepath.Join(stage, "SHA256SUMS"), []byte(checksums.String()), 0644)
}
func run(o options) error {
	data, err := os.ReadFile(o.configPath)
	if err != nil {
		return err
	}
	var c config
	if err := json.Unmarshal(data, &c); err != nil {
		return err
	}
	if !semver.MatchString(c.Version) {
		return errors.New("invalid release version")
	}
	if o.tag != "" && o.tag != "v"+c.Version {
		return fmt.Errorf("tag %s does not match v%s", o.tag, c.Version)
	}
	if c.Updates.GitHub == "" || o.repository != "" && o.repository != c.Updates.GitHub {
		return errors.New("update repository does not match release repository")
	}
	key, err := base64.StdEncoding.DecodeString(c.Updates.PublicKey)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return errors.New("invalid update public key")
	}
	changelog, err := os.ReadFile(o.changelog)
	if err != nil {
		return err
	}
	notes, err := releaseNotes(string(changelog), c.Version)
	if err != nil {
		return err
	}
	if o.notes != "" {
		if err := os.WriteFile(o.notes, []byte(notes), 0644); err != nil {
			return err
		}
	}
	if o.artifacts != "" {
		files, err := validateArtifacts(c, o.artifacts, o.targets)
		if err != nil {
			return err
		}
		if o.stage != "" {
			if err := stageArtifacts(files, o.stage); err != nil {
				return err
			}
		}
	}
	fmt.Printf("Validated release v%s\n", c.Version)
	return nil
}
func main() {
	var o options
	flag.StringVar(&o.configPath, "config", "mygo.json", "application config")
	flag.StringVar(&o.changelog, "changelog", "CHANGELOG.md", "release notes source")
	flag.StringVar(&o.tag, "tag", "", "expected release tag")
	flag.StringVar(&o.repository, "repository", "", "expected GitHub repository")
	flag.StringVar(&o.artifacts, "artifacts", "", "built artifacts to verify")
	flag.StringVar(&o.targets, "targets", "darwin-arm64,darwin-amd64,windows-amd64,linux-amd64", "required target manifests")
	flag.StringVar(&o.stage, "stage", "", "empty directory to assemble release assets")
	flag.StringVar(&o.notes, "notes", "", "write the version's release notes")
	flag.Parse()
	if err := run(o); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
