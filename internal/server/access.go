package server

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/zipluciano/finnick/internal/git"
	"github.com/zipluciano/finnick/internal/workspace"
)

var errNotYourKey = fmt.Errorf("%w: access can be requested for your own key only", workspace.ErrInvalidRecipient)

func (s *Server) handleGenerateIdentity(w http.ResponseWriter, r *http.Request) {
	id, err := workspace.GenerateIdentity()
	if err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	writeJSON(w, id)
}

// diffLine is one line of a change to .sops.yaml: Op is " " (kept), "+" (added) or "-" (removed).
type diffLine struct {
	Op   string `json:"op"`
	Text string `json:"text"`
}

// handleAccess proposes adding the local key to .sops.yaml as a new branch on origin, built from the
// default branch there (the local checkout is not touched). Without push it only returns the diff.
func (s *Server) handleAccess(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name  string   `json:"name"`
		Key   string   `json:"key"`
		Rules []string `json:"rules"` // path_regex of each rule, so they are found again on origin
		Push  bool     `json:"push"`
	}
	if !readJSON(w, r, &req) || req.Name == "" || req.Key == "" || len(req.Rules) == 0 {
		writeError(w, http.StatusBadRequest, errors.New("body must be {\"name\", \"key\", \"rules\": [...]}"))
		return
	}
	mine := false
	for _, k := range workspace.LocalIdentity().Recipients {
		mine = mine || k == req.Key
	}
	if !mine {
		writeError(w, http.StatusBadRequest, errNotYourKey)
		return
	}
	repo, err := git.Open(s.ws.Root)
	if err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	ctx := r.Context()
	base, err := repo.Base(ctx)
	if err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	cfgPath := s.ws.ConfigPath()
	rel, err := relativeTo(repo.Dir, cfgPath)
	if err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	data, err := repo.Show(ctx, base, rel)
	if err != nil {
		writeError(w, statusFor(workspace.ErrNoConfig), fmt.Errorf("%w on origin/%s", workspace.ErrNoConfig, base))
		return
	}
	ops := []workspace.ConfigOp{{Op: "addPerson", Name: req.Name, Key: req.Key}}
	_, cfg, err := workspace.ApplyConfigOps(cfgPath, data, nil) // just to read the rules on origin
	if err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	for _, re := range req.Rules {
		i := -1
		for _, rule := range cfg.Rules {
			if rule.PathRegex == re {
				i = rule.Index
				break
			}
		}
		if i < 0 {
			writeError(w, statusFor(workspace.ErrRuleNotFound), fmt.Errorf("%w on origin/%s: %q", workspace.ErrRuleNotFound, base, re))
			return
		}
		ops = append(ops, workspace.ConfigOp{Op: "grant", Rule: i, Key: req.Key})
	}
	out, _, err := workspace.ApplyConfigOps(cfgPath, data, ops)
	if err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	branch := "finnick/access-" + req.Name
	for n := 2; ; n++ {
		exists, err := repo.BranchExists(ctx, branch)
		if err != nil {
			writeError(w, statusFor(err), err)
			return
		}
		if !exists {
			break
		}
		branch = fmt.Sprintf("finnick/access-%s-%d", req.Name, n)
	}
	resp := map[string]any{"base": base, "branch": branch, "file": filepath.ToSlash(rel), "diff": lineDiff(string(data), string(out))}
	if req.Push {
		msg := fmt.Sprintf("Add %s to %s\n\nAdds the age public key of %s to %s, for the rules:\n\n- %s\n\n"+
			"Proposed with Finnick. Check that the key really is theirs before merging. After the merge,\n"+
			"someone who can decrypt the files updates their keys (sops updatekeys, or Update keys in Finnick).\n",
			req.Name, filepath.ToSlash(rel), req.Name, filepath.ToSlash(rel), strings.Join(req.Rules, "\n- "))
		if err := repo.Propose(ctx, base, branch, rel, out, msg); err != nil {
			writeError(w, statusFor(err), err)
			return
		}
		resp["pushed"] = true
		resp["url"] = repo.CompareURL(ctx, base, branch)
	}
	writeJSON(w, resp)
}

// relativeTo is path relative to dir, both with symlinks resolved (macOS's /tmp is /private/tmp).
func relativeTo(dir, path string) (string, error) {
	d, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", err
	}
	p, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(d, filepath.Join(p, filepath.Base(path)))
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("%w: .sops.yaml is outside the git repository", workspace.ErrNoConfig)
	}
	return rel, nil
}

// lineDiff lists the lines of a and b in order, marking what was removed and added (longest common
// subsequence; .sops.yaml files are small).
func lineDiff(a, b string) []diffLine {
	x := strings.Split(strings.TrimSuffix(a, "\n"), "\n")
	y := strings.Split(strings.TrimSuffix(b, "\n"), "\n")
	if a == "" {
		x = nil
	}
	lcs := make([][]int, len(x)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(y)+1)
	}
	for i := len(x) - 1; i >= 0; i-- {
		for j := len(y) - 1; j >= 0; j-- {
			if x[i] == y[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var out []diffLine
	i, j := 0, 0
	for i < len(x) || j < len(y) {
		switch {
		case i < len(x) && j < len(y) && x[i] == y[j]:
			out = append(out, diffLine{" ", x[i]})
			i++
			j++
		case i < len(x) && (j == len(y) || lcs[i+1][j] >= lcs[i][j+1]):
			out = append(out, diffLine{"-", x[i]}) // removals first, as in a unified diff
			i++
		default:
			out = append(out, diffLine{"+", y[j]})
			j++
		}
	}
	return out
}
