package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"filippo.io/age"
)

// newRepo creates a single-app layout (one .sops.yaml at the root, secrets next to the code),
// encrypting fixtures with the real sops binary.
func newRepo(t *testing.T) (dir string, alice, bob *age.X25519Identity) {
	t.Helper()
	if _, err := exec.LookPath("sops"); err != nil {
		t.Skip("sops binary not installed")
	}
	alice, _ = age.GenerateX25519Identity()
	bob, _ = age.GenerateX25519Identity()
	dir = t.TempDir()
	t.Setenv("SOPS_AGE_KEY", alice.String())
	t.Setenv("SOPS_AGE_KEY_FILE", "")

	write(t, dir, ".sops.yaml", `
keys:
  - &alice `+alice.Recipient().String()+`
  - &bob `+bob.Recipient().String()+`
creation_rules:
  - path_regex: secrets/prod\..*
    age: *alice
  - age: [*alice, *bob]
`)
	write(t, dir, "secrets/dev.yaml", "db:\n  user: app\n  password: hunter2\nports: [80, 443]\n")
	write(t, dir, "secrets/prod.env", "API_KEY=prod-123\n")
	write(t, dir, "README.md", "not a secret\n")
	write(t, dir, "node_modules/x/secrets.yaml", "sops: nope\n")
	for _, f := range []string{"secrets/dev.yaml", "secrets/prod.env"} {
		runSops(t, dir, "encrypt", "--in-place", f)
	}
	return dir, alice, bob
}

func TestScanSingleRepo(t *testing.T) {
	dir, alice, _ := newRepo(t)
	ws, err := Open(filepath.Join(dir, "secrets")) // opening a subdirectory finds the root config
	if err != nil {
		t.Fatal(err)
	}
	if ws.Root != dir && !strings.HasSuffix(ws.Root, filepath.Base(dir)) {
		t.Fatalf("root = %s, want %s", ws.Root, dir)
	}
	files, err := ws.Scan(Identity{Recipients: []string{alice.Recipient().String()}})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("got %d files, want 2: %+v", len(files), files)
	}
	dev, prod := files[0], files[1]
	if dev.Path != "secrets/dev.yaml" || dev.Format != "yaml" || dev.Rule != 1 {
		t.Errorf("dev = %+v", dev)
	}
	if got := strings.Join(dev.Keys, ","); got != "db.user,db.password,ports[0],ports[1]" {
		t.Errorf("dev keys = %s", got)
	}
	if prod.Path != "secrets/prod.env" || prod.Format != "dotenv" || prod.Rule != 0 {
		t.Errorf("prod = %+v", prod)
	}
	if !dev.CanDecrypt || dev.Drift != nil {
		t.Errorf("dev should be decryptable and in sync: %+v", dev)
	}
	names := map[string]bool{}
	for _, r := range dev.KeyGroups[0] {
		names[r.Name] = true
	}
	if !names["alice"] || !names["bob"] {
		t.Errorf("recipient names not resolved from anchors: %+v", dev.KeyGroups)
	}
}

func TestDriftAfterConfigChange(t *testing.T) {
	dir, alice, bob := newRepo(t)
	// Give bob access to prod in .sops.yaml without running updatekeys.
	write(t, dir, ".sops.yaml", `
keys:
  - &alice `+alice.Recipient().String()+`
  - &bob `+bob.Recipient().String()+`
creation_rules:
  - age: [*alice, *bob]
`)
	ws, _ := Open(dir)
	files, _ := ws.Scan(Identity{})
	var prod File
	for _, f := range files {
		if f.Path == "secrets/prod.env" {
			prod = f
		}
	}
	if prod.Drift == nil || len(prod.Drift.Missing) != 1 || prod.Drift.Missing[0].Name != "bob" {
		t.Fatalf("expected bob missing from prod, got %+v", prod.Drift)
	}
	if prod.CanDecrypt {
		t.Error("CanDecrypt should be false without a local identity")
	}
}

func TestReveal(t *testing.T) {
	dir, _, _ := newRepo(t)
	ws, _ := Open(dir)
	entries, err := ws.Reveal("secrets/dev.yaml")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, e := range entries {
		got[e.Key] = e.Value
	}
	if got["db.password"] != "hunter2" || got["ports[1]"] != "443" {
		t.Errorf("reveal = %v", got)
	}
	if _, err := ws.Reveal("../outside.env"); err == nil {
		t.Error("path traversal must be rejected")
	}
}

func TestRevealWithoutAccess(t *testing.T) {
	dir, _, bob := newRepo(t)
	t.Setenv("SOPS_AGE_KEY", bob.String())
	ws, _ := Open(dir)
	if _, err := ws.Reveal("secrets/prod.env"); err == nil {
		t.Error("bob is not a recipient of prod and must not decrypt it")
	}
	if _, err := ws.Reveal("secrets/dev.yaml"); err != nil {
		t.Errorf("bob should decrypt dev: %v", err)
	}
}

func TestRuleForFirstMatchWins(t *testing.T) {
	cfg, err := parseConfig(".sops.yaml", []byte(`
creation_rules:
  - path_regex: ^projects/app/prod\.env$
    age: age1a
  - path_regex: ^projects/[^/]+/[^/]+\.env$
    key_groups:
      - age: [age1a, age1b]
        gcp_kms:
          - resource_id: projects/p/locations/global/keyRings/r/cryptoKeys/k
`))
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]int{"projects/app/prod.env": 0, "projects/app/dev.env": 1, "other/x.env": -1}
	for path, want := range cases {
		if got := cfg.RuleFor(path); got != want {
			t.Errorf("RuleFor(%s) = %d, want %d", path, got, want)
		}
	}
	if g := cfg.Rules[1].KeyGroups[0]; len(g) != 3 || g[2].Type != "gcp_kms" {
		t.Errorf("key_groups parsed wrong: %+v", g)
	}
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runSops(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("sops", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sops %v: %v\n%s", args, err, out)
	}
}

func TestOpenWithoutConfig(t *testing.T) {
	dir := t.TempDir()
	if _, err := Open(dir); err == nil {
		t.Fatal("a directory with no .sops.yaml and no git repo must be refused")
	}
	os.MkdirAll(filepath.Join(dir, ".git"), 0o755)
	os.MkdirAll(filepath.Join(dir, "sub"), 0o755)
	ws, err := Open(filepath.Join(dir, "sub"))
	if err != nil || ws.Config != nil || !strings.HasSuffix(ws.Root, filepath.Base(dir)) {
		t.Fatalf("git root fallback: %+v %v", ws, err)
	}
}
