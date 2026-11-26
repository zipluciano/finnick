package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"filippo.io/age"
)

// testKeys are age public keys for the placeholders {A}…{D} in the configs below.
var testKeys = func() map[string]string {
	keys := map[string]string{}
	for _, p := range []string{"A", "B", "C", "D"} {
		id, _ := age.GenerateX25519Identity()
		keys[p] = id.Recipient().String()
	}
	return keys
}()

func withKeys(s string) string {
	for p, k := range testKeys {
		s = strings.ReplaceAll(s, "{"+p+"}", k)
	}
	return s
}

// editConfig writes src as .sops.yaml, applies ops and returns the new file.
func editConfig(t *testing.T, src string, ops ...ConfigOp) (string, error) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, ".sops.yaml")
	if err := os.WriteFile(path, []byte(withKeys(src)), 0o644); err != nil {
		t.Fatal(err)
	}
	ws := &Workspace{Root: dir}
	if err := ws.Reload(); err != nil {
		t.Fatal(err)
	}
	for i := range ops {
		ops[i].Key = withKeys(ops[i].Key)
	}
	_, err := ws.EditConfig(ws.Config().Version, ops)
	out, _ := os.ReadFile(path)
	if err != nil && string(out) != withKeys(src) {
		t.Errorf("a failed edit changed the file:\n%s", out)
	}
	return string(out), err
}

const peopleConfig = `# Who can read what.
keys:
  - &alice {A} # laptop
  - &bob {B}

  # CI
  - &ci {C}

creation_rules:
  # production: alice and CI only
  - path_regex: prod/.*
    key_groups:
      - age:
          - *alice
          - *ci

  - path_regex: staging/.*
    age: [*alice, *bob]
  - path_regex: dev/.*
    age: *bob
  - age: >-
      {A},
      {B}
`

func TestEditConfigPeople(t *testing.T) {
	tests := []struct {
		name string
		ops  []ConfigOp
		want string // the lines that differ from peopleConfig, as a unified list of replacements
		repl [][2]string
	}{
		{
			name: "add a person to the keys list",
			ops:  []ConfigOp{{Op: "addPerson", Name: "dave", Key: "{D}"}},
			repl: [][2]string{{"  - &ci {C}\n", "  - &ci {C}\n  - &dave {D}\n"}},
		},
		{
			name: "grant in a key group, a flow list, an alias and a string",
			ops: []ConfigOp{
				{Op: "addPerson", Name: "dave", Key: "{D}"},
				{Op: "grant", Rule: 0, Key: "{D}"},
				{Op: "grant", Rule: 1, Key: "{D}"},
				{Op: "grant", Rule: 2, Key: "{D}"},
				{Op: "grant", Rule: 3, Key: "{D}"},
			},
			repl: [][2]string{
				{"  - &ci {C}\n", "  - &ci {C}\n  - &dave {D}\n"},
				{"          - *ci\n", "          - *ci\n          - *dave\n"},
				{"age: [*alice, *bob]", "age: [*alice, *bob, *dave]"},
				{"age: *bob\n", "age: [*bob, *dave]\n"},
				{"  - age: >-\n      {A},\n      {B}\n", "  - age: {A}, {B}, {D}\n"},
			},
		},
		{
			name: "granting twice changes nothing",
			ops:  []ConfigOp{{Op: "grant", Rule: 1, Key: "{B}"}},
		},
		{
			name: "revoke from a key group, a flow list and a string",
			ops: []ConfigOp{
				{Op: "revoke", Rule: 0, Key: "{C}"},
				{Op: "revoke", Rule: 1, Key: "{A}"},
				{Op: "revoke", Rule: 3, Key: "{A}"},
			},
			repl: [][2]string{
				{"          - *ci\n", ""},
				{"age: [*alice, *bob]", "age: [*bob]"},
				{"  - age: >-\n      {A},\n      {B}\n", "  - age: {B}\n"},
			},
		},
		{
			name: "remove a person everywhere, with the comment about them",
			ops:  []ConfigOp{{Op: "removePerson", Key: "{C}"}},
			repl: [][2]string{
				{"\n  # CI\n  - &ci {C}\n", ""},
				{"          - *ci\n", ""},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := editConfig(t, peopleConfig, tt.ops...)
			if err != nil {
				t.Fatal(err)
			}
			want := peopleConfig
			for _, r := range tt.repl {
				if !strings.Contains(want, r[0]) {
					t.Fatalf("bad test: %q not in config", r[0])
				}
				want = strings.Replace(want, r[0], r[1], 1)
			}
			if want = withKeys(want); got != want {
				t.Errorf("got:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

func TestEditConfigStartsAKeysList(t *testing.T) {
	got, err := editConfig(t, "# Secrets.\ncreation_rules:\n  - age: {A}\n",
		ConfigOp{Op: "addPerson", Name: "bob", Key: "{B}"}, ConfigOp{Op: "grant", Rule: 0, Key: "{B}"})
	if err != nil {
		t.Fatal(err)
	}
	want := withKeys("# Secrets.\n\nkeys:\n  - &bob {B}\n\ncreation_rules:\n  - age: {A}, {B}\n")
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}

	got, err = editConfig(t, "creation_rules:\n- path_regex: x\n  pgp: ABC\n",
		ConfigOp{Op: "addPerson", Name: "bob", Key: "{B}"}, ConfigOp{Op: "grant", Rule: 0, Key: "{B}"})
	if err != nil {
		t.Fatal(err)
	}
	want = withKeys("keys:\n- &bob {B}\n\ncreation_rules:\n- path_regex: x\n  pgp: ABC\n  age:\n    - *bob\n")
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestEditConfigRefusals(t *testing.T) {
	tests := []struct {
		name string
		src  string
		ops  []ConfigOp
		want error
	}{
		{"invalid name", peopleConfig, []ConfigOp{{Op: "addPerson", Name: "da ve", Key: "{D}"}}, ErrInvalidName},
		{"name taken", peopleConfig, []ConfigOp{{Op: "addPerson", Name: "bob", Key: "{D}"}}, ErrNameTaken},
		{"key named already", peopleConfig, []ConfigOp{{Op: "addPerson", Name: "bobby", Key: "{B}"}}, ErrPersonExists},
		{"not an age key", peopleConfig, []ConfigOp{{Op: "grant", Rule: 0, Key: "age1nope"}}, ErrInvalidRecipient},
		{"no such rule", peopleConfig, []ConfigOp{{Op: "grant", Rule: 9, Key: "{D}"}}, ErrRuleNotFound},
		{"not in the config", peopleConfig, []ConfigOp{{Op: "removePerson", Key: "{D}"}}, ErrPersonNotFound},
		{"last recipient", peopleConfig, []ConfigOp{{Op: "revoke", Rule: 2, Key: "{B}"}}, ErrLastRecipient},
		{"last of a group", peopleConfig, []ConfigOp{{Op: "revoke", Rule: 0, Key: "{A}"}, {Op: "revoke", Rule: 0, Key: "{C}"}}, ErrLastRecipient},
		{"all or nothing", peopleConfig, []ConfigOp{{Op: "addPerson", Name: "dave", Key: "{D}"}, {Op: "grant", Rule: 9, Key: "{D}"}}, ErrRuleNotFound},
		{"shamir", "creation_rules:\n  - key_groups:\n      - age: [{A}]\n      - age: [{B}]\n", []ConfigOp{{Op: "grant", Rule: 0, Key: "{C}"}}, ErrKeyGroups},
		{"flow style", "{creation_rules: [{age: {A}}]}\n", []ConfigOp{{Op: "grant", Rule: 0, Key: "{C}"}}, ErrConfigLayout},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := editConfig(t, tt.src, tt.ops...); !errors.Is(err, tt.want) {
				t.Errorf("got %v, want %v", err, tt.want)
			}
		})
	}
}

func TestEditConfigRefusesStaleVersion(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, ".sops.yaml", withKeys("creation_rules:\n  - age: {A}\n"))
	ws := &Workspace{Root: dir}
	if err := ws.Reload(); err != nil {
		t.Fatal(err)
	}
	version := ws.Config().Version
	write(t, dir, ".sops.yaml", withKeys("creation_rules:\n  - age: {A},{B}\n"))
	if _, err := ws.EditConfig(version, []ConfigOp{{Op: "addPerson", Name: "c", Key: testKeys["C"]}}); !errors.Is(err, ErrStale) {
		t.Fatalf("got %v, want ErrStale", err)
	}
}

// A person added and granted in the UI can decrypt once someone with access updates the keys.
func TestAddPersonThenUpdateKeys(t *testing.T) {
	dir, _, bob := newRepo(t)
	ws, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	carol, _ := age.GenerateX25519Identity()
	if _, err := ws.EditConfig(ws.Config().Version, []ConfigOp{
		{Op: "addPerson", Name: "carol", Key: carol.Recipient().String()},
		{Op: "grant", Rule: 0, Key: carol.Recipient().String()},
		{Op: "removePerson", Key: bob.Recipient().String()},
	}); err != nil {
		t.Fatal(err)
	}
	cfg, _ := os.ReadFile(filepath.Join(dir, ".sops.yaml"))
	if strings.Contains(string(cfg), "bob") || !strings.Contains(string(cfg), "*carol") {
		t.Fatalf("config:\n%s", cfg)
	}
	if changed, err := ws.UpdateKeys("secrets/prod.env"); err != nil || !changed {
		t.Fatalf("updatekeys: %v %v", changed, err)
	}
	t.Setenv("SOPS_AGE_KEY", carol.String())
	if got := sopsDecrypt(t, dir, "secrets/prod.env"); !strings.Contains(got, "API_KEY=prod-123") {
		t.Errorf("carol cannot decrypt: %q", got)
	}
}

func TestEditConfigKeepsShape(t *testing.T) {
	// Removing the key that holds a rule's dash hands the dash to the next key.
	got, err := editConfig(t, "creation_rules:\n  - age: {A}\n    pgp: ABC\n", ConfigOp{Op: "revoke", Rule: 0, Key: "{A}"})
	if want := "creation_rules:\n  - pgp: ABC\n"; err != nil || got != want {
		t.Errorf("got %v:\n%s\nwant:\n%s", err, got, want)
	}
	// Windows line endings stay.
	got, err = editConfig(t, "creation_rules:\r\n  - age: [{A}] # team\r\n", ConfigOp{Op: "grant", Rule: 0, Key: "{B}"})
	if want := withKeys("creation_rules:\r\n  - age: [{A}, {B}] # team\r\n"); err != nil || got != want {
		t.Errorf("got %v: %q\nwant: %q", err, got, want)
	}
}
