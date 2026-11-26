package workspace

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// sopsDecrypt reads a file back with the real sops binary: whatever Finnick writes must stay a
// file sops accepts (ADR 0003).
func sopsDecrypt(t *testing.T, dir, file string) string {
	t.Helper()
	cmd := exec.Command("sops", "decrypt", file)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("sops decrypt %s: %v\n%s", file, err, out)
	}
	return string(out)
}

func reveal(t *testing.T, ws *Workspace, rel string) *Revealed {
	t.Helper()
	res, err := ws.Reveal(rel)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestEditYAML(t *testing.T) {
	dir, _, _ := newRepo(t)
	ws, _ := Open(dir)
	before := reveal(t, ws, "secrets/dev.yaml")

	version, entries, err := ws.Edit("secrets/dev.yaml", before.Version, []Edit{
		{Op: "set", Path: []any{"db", "password"}, Value: "correct horse"},
		{Op: "set", Path: []any{"ports", float64(1)}, Value: "8443"}, // numbers arrive from JSON as float64
		{Op: "add", Path: []any{"db", "host"}, Value: "localhost"},
		{Op: "add", Path: []any{"cache", "url"}, Value: "redis://x"},
		{Op: "rename", Path: []any{"db", "user"}, To: "username"},
		{Op: "delete", Path: []any{"ports", float64(0)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if version == before.Version || len(entries) != 5 {
		t.Fatalf("version %s, entries %+v", version, entries)
	}
	want := "db:\n    username: app\n    password: correct horse\n    host: localhost\nports:\n    - 8443\ncache:\n    url: redis://x\n"
	if got := sopsDecrypt(t, dir, "secrets/dev.yaml"); got != want {
		t.Errorf("sops decrypt after edit:\n%s\nwant:\n%s", got, want)
	}
	if after := reveal(t, ws, "secrets/dev.yaml"); after.Version != version {
		t.Error("returned version does not match the file on disk")
	}

	// Deleting the last keys of a map removes the map too.
	if _, _, err := ws.Edit("secrets/dev.yaml", version, []Edit{{Op: "delete", Path: []any{"cache", "url"}}}); err != nil {
		t.Fatal(err)
	}
	if got := sopsDecrypt(t, dir, "secrets/dev.yaml"); strings.Contains(got, "cache") {
		t.Errorf("empty map left behind:\n%s", got)
	}
}

func TestEditDotenv(t *testing.T) {
	dir, _, _ := newRepo(t)
	ws, _ := Open(dir)
	v := reveal(t, ws, "secrets/prod.env").Version
	v, _, err := ws.Edit("secrets/prod.env", v, []Edit{
		{Op: "add", Path: []any{"DATABASE_URL"}, Value: "postgres://db"},
		{Op: "rename", Path: []any{"API_KEY"}, To: "API_TOKEN"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := sopsDecrypt(t, dir, "secrets/prod.env"); got != "API_TOKEN=prod-123\nDATABASE_URL=postgres://db\n" {
		t.Errorf("sops decrypt after edit:\n%s", got)
	}

	cases := []struct {
		edit Edit
		want error
	}{
		{Edit{Op: "add", Path: []any{"API_TOKEN"}, Value: "x"}, ErrKeyExists},
		{Edit{Op: "add", Path: []any{"BAD NAME"}, Value: "x"}, ErrInvalidKey},
		{Edit{Op: "add", Path: []any{"sops_mac"}, Value: "x"}, ErrInvalidKey},
		{Edit{Op: "add", Path: []any{"A", "B"}, Value: "x"}, ErrInvalidKey},
		{Edit{Op: "set", Path: []any{"MISSING"}, Value: "x"}, ErrKeyNotFound},
		{Edit{Op: "rename", Path: []any{"API_TOKEN"}, To: "DATABASE_URL"}, ErrKeyExists},
	}
	for _, c := range cases {
		if _, _, err := ws.Edit("secrets/prod.env", v, []Edit{c.edit}); !errors.Is(err, c.want) {
			t.Errorf("%+v: got %v, want %v", c.edit, err, c.want)
		}
	}
}

func TestEditIsAllOrNothing(t *testing.T) {
	dir, _, _ := newRepo(t)
	ws, _ := Open(dir)
	path := filepath.Join(dir, "secrets/dev.yaml")
	original, _ := os.ReadFile(path)
	v := reveal(t, ws, "secrets/dev.yaml").Version

	_, _, err := ws.Edit("secrets/dev.yaml", v, []Edit{
		{Op: "set", Path: []any{"db", "password"}, Value: "changed"},
		{Op: "set", Path: []any{"db", "password", "deeper"}, Value: "x"}, // a value is not a map
	})
	if !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("got %v, want ErrInvalidKey", err)
	}
	if now, _ := os.ReadFile(path); string(now) != string(original) {
		t.Error("a failed edit changed the file")
	}
}

func TestEditRefusesStaleVersion(t *testing.T) {
	dir, _, _ := newRepo(t)
	ws, _ := Open(dir)
	v := reveal(t, ws, "secrets/dev.yaml").Version
	runSops(t, dir, "set", "secrets/dev.yaml", `["db"]["user"]`, `"someone-else"`)
	_, _, err := ws.Edit("secrets/dev.yaml", v, []Edit{{Op: "set", Path: []any{"db", "password"}, Value: "x"}})
	if !errors.Is(err, ErrStale) {
		t.Fatalf("got %v, want ErrStale", err)
	}
}

func TestEditNeedsAccessAndAValidPath(t *testing.T) {
	dir, _, bob := newRepo(t)
	ws, _ := Open(dir)
	set := []Edit{{Op: "set", Path: []any{"API_KEY"}, Value: "x"}}
	for _, rel := range []string{"../outside.env", ".sops.yaml", ".git/config", "/etc/passwd"} {
		if _, _, err := ws.Edit(rel, "v", set); !errors.Is(err, ErrInvalidPath) {
			t.Errorf("%s: got %v, want ErrInvalidPath", rel, err)
		}
	}
	if err := os.Symlink(filepath.Join(dir, "secrets/prod.env"), filepath.Join(dir, "link.env")); err == nil {
		if _, _, err := ws.Edit("link.env", "v", set); !errors.Is(err, ErrInvalidPath) {
			t.Errorf("symlink: got %v, want ErrInvalidPath", err)
		}
	}
	t.Setenv("SOPS_AGE_KEY", bob.String())
	if _, _, err := ws.Edit("secrets/prod.env", "v", set); !errors.Is(err, ErrNotRecipient) {
		t.Errorf("bob editing prod: got %v, want ErrNotRecipient", err)
	}
}

func TestCreate(t *testing.T) {
	dir, _, bob := newRepo(t)
	ws, _ := Open(dir)
	version, entries, err := ws.Create("secrets/staging.env", []Edit{
		{Op: "add", Path: []any{"API_URL"}, Value: "https://staging"},
		{Op: "add", Path: []any{"TOKEN"}, Value: "s3cret"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || len(version) != 64 {
		t.Fatalf("entries %+v version %q", entries, version)
	}
	if got := sopsDecrypt(t, dir, "secrets/staging.env"); got != "API_URL=https://staging\nTOKEN=s3cret\n" {
		t.Errorf("sops decrypt:\n%s", got)
	}
	// Recipients come from the matching rule (the catch-all one: alice and bob).
	files, _ := ws.Scan(Identity{})
	for _, f := range files {
		if f.Path == "secrets/staging.env" && (f.Rule != 1 || f.Drift != nil || len(f.KeyGroups[0]) != 2) {
			t.Errorf("new file %+v", f)
		}
	}
	t.Setenv("SOPS_AGE_KEY", bob.String())
	if _, err := ws.Reveal("secrets/staging.env"); err != nil {
		t.Errorf("bob is in the rule and should decrypt the new file: %v", err)
	}

	// Nested keys in a new YAML file, in a directory that does not exist yet.
	if _, _, err := ws.Create("config/app.yaml", []Edit{{Op: "add", Path: []any{"db", "url"}, Value: "postgres://x"}}); err != nil {
		t.Fatal(err)
	}
	if got := sopsDecrypt(t, dir, "config/app.yaml"); got != "db:\n    url: postgres://x\n" {
		t.Errorf("sops decrypt:\n%s", got)
	}
}

func TestCreateRefusals(t *testing.T) {
	dir, _, _ := newRepo(t)
	ws, _ := Open(dir)
	add := []Edit{{Op: "add", Path: []any{"A"}, Value: "1"}}
	cases := map[string]error{
		"secrets/dev.yaml": ErrFileExists,
		"secrets/key.pem":  ErrUnsupported,
		"../outside.env":   ErrInvalidPath,
		".sops.yaml":       ErrInvalidPath,
	}
	for rel, want := range cases {
		if _, _, err := ws.Create(rel, add); !errors.Is(err, want) {
			t.Errorf("%s: got %v, want %v", rel, err, want)
		}
	}
	if _, _, err := ws.Create("secrets/empty.env", nil); !errors.Is(err, ErrInvalidKey) {
		t.Errorf("a file without keys: got %v", err)
	}
	write(t, dir, ".sops.yaml", "creation_rules:\n  - path_regex: ^only/.*\n    age: age1qqqq\n")
	ws, _ = Open(dir)
	if _, _, err := ws.Create("secrets/new.env", add); !errors.Is(err, ErrNoRule) {
		t.Errorf("no matching rule: got %v", err)
	}
}

func dataKey(t *testing.T, dir, rel string) string {
	t.Helper()
	data, _ := os.ReadFile(filepath.Join(dir, rel))
	tree, err := loadTree(filepath.Join(dir, rel), data)
	if err != nil {
		t.Fatal(err)
	}
	key, err := tree.Metadata.GetDataKey()
	if err != nil {
		t.Fatal(err)
	}
	return string(key)
}

func TestUpdateKeys(t *testing.T) {
	dir, alice, bob := newRepo(t)
	// Give bob prod and take him out of everything else, without touching the files.
	write(t, dir, ".sops.yaml", `
keys:
  - &alice `+alice.Recipient().String()+`
  - &bob `+bob.Recipient().String()+`
creation_rules:
  - path_regex: secrets/prod\..*
    age: [*alice, *bob]
  - age: *alice
`)
	ws, _ := Open(dir)
	keyBefore := dataKey(t, dir, "secrets/prod.env")
	for _, rel := range []string{"secrets/prod.env", "secrets/dev.yaml"} {
		changed, err := ws.UpdateKeys(rel)
		if err != nil || !changed {
			t.Fatalf("%s: changed=%v err=%v", rel, changed, err)
		}
	}
	if changed, err := ws.UpdateKeys("secrets/prod.env"); err != nil || changed {
		t.Errorf("a file in sync must be left alone: changed=%v err=%v", changed, err)
	}
	files, _ := ws.Scan(Identity{})
	for _, f := range files {
		if f.Drift != nil {
			t.Errorf("%s still drifts: %+v", f.Path, f.Drift)
		}
	}
	if dataKey(t, dir, "secrets/prod.env") != keyBefore {
		t.Error("updatekeys must keep the data key")
	}
	sopsDecrypt(t, dir, "secrets/prod.env")

	t.Setenv("SOPS_AGE_KEY", bob.String())
	if _, err := ws.Reveal("secrets/prod.env"); err != nil {
		t.Errorf("bob was added to prod: %v", err)
	}
	if _, err := ws.Reveal("secrets/dev.yaml"); !errors.Is(err, ErrNotRecipient) {
		t.Errorf("bob was removed from dev: %v", err)
	}
	if _, err := ws.UpdateKeys("secrets/dev.yaml"); err != nil {
		t.Errorf("in sync, so no key is needed: %v", err)
	}
}

func TestUpdateKeysNeedsAccess(t *testing.T) {
	dir, alice, bob := newRepo(t)
	write(t, dir, ".sops.yaml", "creation_rules:\n  - age: "+alice.Recipient().String()+","+bob.Recipient().String()+"\n")
	t.Setenv("SOPS_AGE_KEY", bob.String()) // bob cannot read prod, so he cannot share it
	ws, _ := Open(dir)
	if _, err := ws.UpdateKeys("secrets/prod.env"); !errors.Is(err, ErrNotRecipient) {
		t.Fatalf("got %v, want ErrNotRecipient", err)
	}
}

func TestRotate(t *testing.T) {
	dir, _, _ := newRepo(t)
	ws, _ := Open(dir)
	before := sopsDecrypt(t, dir, "secrets/dev.yaml")
	keyBefore := dataKey(t, dir, "secrets/dev.yaml")
	version, err := ws.Rotate("secrets/dev.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if dataKey(t, dir, "secrets/dev.yaml") == keyBefore {
		t.Error("rotate must change the data key")
	}
	if after := sopsDecrypt(t, dir, "secrets/dev.yaml"); after != before {
		t.Errorf("rotate changed the values:\n%s\nwant:\n%s", after, before)
	}
	if reveal(t, ws, "secrets/dev.yaml").Version != version {
		t.Error("returned version does not match the file on disk")
	}
}
