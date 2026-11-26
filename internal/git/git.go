// Package git runs the few git commands Finnick needs to propose a change as a branch, without
// touching the working tree, the index or the current branch of the repository.
package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ErrNoRepo means the directory is not inside a git repository with an origin remote.
var ErrNoRepo = errors.New("not a git repository with an origin remote")

// ErrFailed wraps the output of a git command that failed (no network, no permission to push…).
var ErrFailed = errors.New("git failed")

// Repo is a git repository; Dir is its top-level directory.
type Repo struct {
	Dir string
}

// Open finds the repository that contains dir and checks it has an origin remote.
func Open(dir string) (*Repo, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return nil, fmt.Errorf("%w: git is not installed", ErrNoRepo)
	}
	r := &Repo{Dir: dir}
	top, err := r.run(context.Background(), nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, ErrNoRepo
	}
	r.Dir = strings.TrimSpace(top)
	if _, err := r.run(context.Background(), nil, "remote", "get-url", "origin"); err != nil {
		return nil, ErrNoRepo
	}
	return r, nil
}

func (r *Repo) run(ctx context.Context, env []string, args ...string) (string, error) {
	return r.runInput(ctx, env, nil, args...)
}

func (r *Repo) runInput(ctx context.Context, env []string, input []byte, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = r.Dir
	// Never wait for a password prompt nobody can see: fail instead, and say why.
	cmd.Env = append(os.Environ(), append([]string{"GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never"}, env...)...)
	if input != nil {
		cmd.Stdin = bytes.NewReader(input)
	}
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%w: git %s: %s", ErrFailed, args[0], msg)
	}
	return out.String(), nil
}

// Base fetches origin and returns its default branch (origin/HEAD), the branch a proposal targets.
func (r *Repo) Base(ctx context.Context) (string, error) {
	if _, err := r.run(ctx, nil, "fetch", "--quiet", "origin"); err != nil {
		return "", err
	}
	if ref, err := r.run(ctx, nil, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD"); err == nil {
		return strings.TrimPrefix(strings.TrimSpace(ref), "origin/"), nil
	}
	// origin/HEAD is not set in every clone: ask the remote.
	out, err := r.run(ctx, nil, "ls-remote", "--symref", "origin", "HEAD")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(out, "\n") {
		if rest, ok := strings.CutPrefix(line, "ref: refs/heads/"); ok {
			return strings.Fields(rest)[0], nil
		}
	}
	return "", fmt.Errorf("%w: cannot tell the default branch of origin", ErrFailed)
}

// Show returns the content of path (relative to the repository) at origin/base.
func (r *Repo) Show(ctx context.Context, base, path string) ([]byte, error) {
	out, err := r.run(ctx, nil, "show", "origin/"+base+":"+filepath.ToSlash(path))
	return []byte(out), err
}

// BranchExists reports whether origin has a branch with this name.
func (r *Repo) BranchExists(ctx context.Context, branch string) (bool, error) {
	out, err := r.run(ctx, nil, "ls-remote", "--heads", "origin", "refs/heads/"+branch)
	return strings.TrimSpace(out) != "", err
}

// Propose commits content as path on top of origin/base and pushes it to origin as a new branch. It
// builds the commit in a temporary index, so the working tree, the index and HEAD stay as they are.
func (r *Repo) Propose(ctx context.Context, base, branch, path string, content []byte, message string) error {
	blob, err := r.runInput(ctx, nil, content, "hash-object", "-w", "--stdin")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp("", "finnick-index-*")
	if err != nil {
		return err
	}
	tmp.Close()
	os.Remove(tmp.Name()) // git wants to create the index itself
	defer os.Remove(tmp.Name())
	index := []string{"GIT_INDEX_FILE=" + tmp.Name()}
	if _, err := r.run(ctx, index, "read-tree", "origin/"+base); err != nil {
		return err
	}
	mode := "100644"
	if out, err := r.run(ctx, nil, "ls-tree", "origin/"+base, "--", filepath.ToSlash(path)); err == nil && strings.HasPrefix(out, "100755") {
		mode = "100755"
	}
	cacheinfo := mode + "," + strings.TrimSpace(blob) + "," + filepath.ToSlash(path)
	if _, err := r.run(ctx, index, "update-index", "--add", "--cacheinfo", cacheinfo); err != nil {
		return err
	}
	tree, err := r.run(ctx, index, "write-tree")
	if err != nil {
		return err
	}
	commit, err := r.runInput(ctx, nil, []byte(message), "commit-tree", strings.TrimSpace(tree), "-p", "origin/"+base, "-F", "-")
	if err != nil {
		return err
	}
	_, err = r.run(ctx, nil, "push", "--quiet", "origin", strings.TrimSpace(commit)+":refs/heads/"+branch)
	return err
}

var scpLike = regexp.MustCompile(`^(?:[^@/]+@)?([^:/]+):(.+)$`)

// CompareURL is the web page that opens a pull (or merge) request from branch into base, for the
// hosts Finnick knows; "" for others.
func (r *Repo) CompareURL(ctx context.Context, base, branch string) string {
	remote, err := r.run(ctx, nil, "remote", "get-url", "origin")
	if err != nil {
		return ""
	}
	return compareURL(strings.TrimSpace(remote), base, branch)
}

func compareURL(remote, base, branch string) string {
	var host, path string
	if u, err := url.Parse(remote); err == nil && u.Host != "" {
		host, path = u.Hostname(), u.Path
	} else if m := scpLike.FindStringSubmatch(remote); m != nil {
		host, path = m[1], m[2]
	} else {
		return ""
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	if path == "" {
		return ""
	}
	web := "https://" + host + "/" + path
	switch {
	case host == "github.com":
		ref := func(s string) string { return strings.ReplaceAll(url.PathEscape(s), "%2F", "/") }
		return web + "/compare/" + ref(base) + "..." + ref(branch) + "?expand=1"
	case host == "gitlab.com" || strings.HasPrefix(host, "gitlab."):
		q := url.Values{"merge_request[source_branch]": {branch}, "merge_request[target_branch]": {base}}
		return web + "/-/merge_requests/new?" + q.Encode()
	case host == "bitbucket.org":
		return web + "/pull-requests/new?source=" + url.QueryEscape(branch)
	}
	return ""
}
