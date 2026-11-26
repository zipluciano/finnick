package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func sh(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// newClone returns a clone of a bare origin whose main branch has .sops.yaml in a subfolder.
func newClone(t *testing.T) (clone, origin string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	empty := filepath.Join(t.TempDir(), "gitconfig")
	os.WriteFile(empty, nil, 0o644)
	t.Setenv("GIT_CONFIG_GLOBAL", empty) // no signing or hooks from the machine running the tests
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, k := range []string{"AUTHOR", "COMMITTER"} {
		t.Setenv("GIT_"+k+"_NAME", "Test")
		t.Setenv("GIT_"+k+"_EMAIL", "test@example.com")
	}
	root := t.TempDir()
	origin = filepath.Join(root, "origin.git")
	sh(t, root, "init", "--quiet", "--bare", "--initial-branch=main", origin)
	clone = filepath.Join(root, "clone")
	sh(t, root, "clone", "--quiet", origin, clone)
	os.MkdirAll(filepath.Join(clone, "app"), 0o755)
	os.WriteFile(filepath.Join(clone, "app", ".sops.yaml"), []byte("creation_rules:\n  - age: age1x\n"), 0o644)
	sh(t, clone, "add", ".")
	sh(t, clone, "commit", "--quiet", "-m", "init")
	sh(t, clone, "push", "--quiet", "origin", "HEAD:main")
	return clone, origin
}

func TestProposeLeavesTheCheckoutAlone(t *testing.T) {
	clone, origin := newClone(t)
	// Local work in progress that must not end up in the proposal or be disturbed by it.
	os.WriteFile(filepath.Join(clone, "app", ".sops.yaml"), []byte("# local edit\n"), 0o644)
	sh(t, clone, "checkout", "--quiet", "-b", "feature")
	head := sh(t, clone, "rev-parse", "HEAD")

	repo, err := Open(filepath.Join(clone, "app"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	base, err := repo.Base(ctx)
	if err != nil || base != "main" {
		t.Fatalf("base = %q, %v", base, err)
	}
	old, err := repo.Show(ctx, base, "app/.sops.yaml")
	if err != nil || string(old) != "creation_rules:\n  - age: age1x\n" {
		t.Fatalf("show: %q %v", old, err)
	}
	if exists, err := repo.BranchExists(ctx, "finnick/access-bob"); err != nil || exists {
		t.Fatalf("branch exists before: %v %v", exists, err)
	}
	content := []byte("creation_rules:\n  - age: age1x, age1y\n")
	if err := repo.Propose(ctx, base, "finnick/access-bob", "app/.sops.yaml", content, "Add bob\n"); err != nil {
		t.Fatal(err)
	}
	if got := sh(t, origin, "show", "finnick/access-bob:app/.sops.yaml"); got != strings.TrimSpace(string(content)) {
		t.Errorf("pushed content = %q", got)
	}
	if parent := sh(t, origin, "rev-parse", "finnick/access-bob^"); parent != sh(t, origin, "rev-parse", "main") {
		t.Errorf("proposal is not on top of main")
	}
	if exists, _ := repo.BranchExists(ctx, "finnick/access-bob"); !exists {
		t.Error("branch not found on origin")
	}
	if sh(t, clone, "rev-parse", "HEAD") != head || sh(t, clone, "branch", "--show-current") != "feature" {
		t.Error("the checkout moved")
	}
	if got, _ := os.ReadFile(filepath.Join(clone, "app", ".sops.yaml")); string(got) != "# local edit\n" {
		t.Errorf("working tree changed: %q", got)
	}
	if status := sh(t, clone, "status", "--porcelain"); status != "M app/.sops.yaml" {
		t.Errorf("index changed: %q", status)
	}
}

func TestOpenNeedsOrigin(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	sh(t, dir, "init", "--quiet")
	if _, err := Open(dir); err != ErrNoRepo {
		t.Errorf("got %v, want ErrNoRepo", err)
	}
	if _, err := Open(t.TempDir()); err != ErrNoRepo {
		t.Errorf("outside a repository: got %v, want ErrNoRepo", err)
	}
}

func TestCompareURL(t *testing.T) {
	tests := []struct{ remote, want string }{
		{"git@github.com:acme/app.git", "https://github.com/acme/app/compare/main...finnick/access-bob?expand=1"},
		{"https://github.com/acme/app", "https://github.com/acme/app/compare/main...finnick/access-bob?expand=1"},
		{"ssh://git@gitlab.com/group/sub/app.git", "https://gitlab.com/group/sub/app/-/merge_requests/new?merge_request%5Bsource_branch%5D=finnick%2Faccess-bob&merge_request%5Btarget_branch%5D=main"},
		{"git@bitbucket.org:acme/app.git", "https://bitbucket.org/acme/app/pull-requests/new?source=finnick%2Faccess-bob"},
		{"/srv/git/app.git", ""},
		{"https://git.example.com/app.git", ""},
	}
	for _, tt := range tests {
		if got := compareURL(tt.remote, "main", "finnick/access-bob"); got != tt.want {
			t.Errorf("%s:\n got %s\nwant %s", tt.remote, got, tt.want)
		}
	}
}
