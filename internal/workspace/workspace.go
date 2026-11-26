// Package workspace reads a repository that keeps SOPS-encrypted files: its .sops.yaml,
// the encrypted files themselves, who can decrypt each one and who should.
package workspace

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/getsops/sops/v3"
	"github.com/getsops/sops/v3/aes"
	"github.com/getsops/sops/v3/cmd/sops/common"
	"github.com/getsops/sops/v3/cmd/sops/formats"
	sopsconfig "github.com/getsops/sops/v3/config"
)

// maxFileSize bounds how much we read while looking for encrypted files.
const maxFileSize = 4 << 20

// skipDirs are never descended into while scanning.
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, ".venv": true, "venv": true,
	".terraform": true, "dist": true, "build": true, "target": true, ".next": true, ".cache": true,
}

// Workspace is a repository with SOPS files, rooted where its .sops.yaml lives (or at the git root).
type Workspace struct {
	Root   string
	Config *Config // nil when the workspace has no .sops.yaml
}

// Open finds the .sops.yaml for dir the way SOPS does (dir, then its parents) and roots the
// workspace at the directory that holds it. Without a .sops.yaml it falls back to the root of
// the enclosing git repository, so encrypted files can still be listed; outside of both it
// refuses, rather than scanning an arbitrary directory tree such as $HOME.
func Open(dir string) (*Workspace, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if st, err := os.Stat(abs); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", dir)
	}
	res, err := sopsconfig.LookupConfigFile(filepath.Join(abs, ".sops.yaml"))
	if err == nil && res.Path != "" {
		cfg, err := LoadConfig(res.Path)
		if err != nil {
			return nil, err
		}
		return &Workspace{Root: filepath.Dir(res.Path), Config: cfg}, nil
	}
	if root := gitRoot(abs); root != "" {
		return &Workspace{Root: root}, nil
	}
	return nil, fmt.Errorf("no .sops.yaml or git repository found in %s or its parents: run finnick inside a repository", abs)
}

func gitRoot(dir string) string {
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// Drift is the difference between who can decrypt a file and who its creation rule says should.
type Drift struct {
	Missing []Recipient `json:"missing"` // in the rule, not in the file
	Extra   []Recipient `json:"extra"`   // in the file, not in the rule
}

// File is one SOPS-encrypted file. Nothing here is secret: key names are stored in clear by SOPS.
type File struct {
	Path         string        `json:"path"` // slash-separated, relative to the workspace root
	Format       string        `json:"format"`
	LastModified time.Time     `json:"lastModified"`
	SopsVersion  string        `json:"sopsVersion"`
	KeyGroups    [][]Recipient `json:"keyGroups"`
	Keys         []string      `json:"keys"`
	Rule         int           `json:"rule"` // matching creation rule, -1 when none
	Drift        *Drift        `json:"drift,omitempty"`
	CanDecrypt   bool          `json:"canDecrypt"`
}

// Scan walks the workspace and returns every SOPS-encrypted file, sorted by path.
func (ws *Workspace) Scan(local Identity) ([]File, error) {
	mine := map[string]bool{}
	for _, r := range local.Recipients {
		mine[r] = true
	}
	var files []File
	err := filepath.WalkDir(ws.Root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable entries are skipped, not fatal
		}
		if d.IsDir() {
			if path != ws.Root && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || d.Name() == ".sops.yaml" || d.Name() == ".sops.yml" {
			return nil
		}
		f, ok := ws.inspect(path, mine)
		if ok {
			files = append(files, f)
		}
		return nil
	})
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, err
}

func (ws *Workspace) inspect(path string, mine map[string]bool) (File, bool) {
	st, err := os.Stat(path)
	if err != nil || st.Size() > maxFileSize {
		return File{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil || !looksEncrypted(path, data) {
		return File{}, false
	}
	format := formats.FormatForPath(path)
	tree, err := loadTree(path, data)
	if err != nil {
		return File{}, false
	}
	rel, _ := filepath.Rel(ws.Root, path)
	f := File{
		Path:         filepath.ToSlash(rel),
		Format:       formatName(format),
		LastModified: tree.Metadata.LastModified,
		SopsVersion:  tree.Metadata.Version,
		Keys:         keyNames(tree.Branches),
		Rule:         ws.Config.RuleFor(rel),
	}
	for _, g := range tree.Metadata.KeyGroups {
		var group []Recipient
		for _, k := range g {
			r := Recipient{Type: k.TypeToIdentifier(), ID: normalizeID(k.TypeToIdentifier(), k.ToString())}
			r.Name = ws.Config.name(r.ID)
			if r.Type == "age" && mine[r.ID] {
				f.CanDecrypt = true
			}
			group = append(group, r)
		}
		f.KeyGroups = append(f.KeyGroups, group)
	}
	if f.Rule >= 0 {
		f.Drift = drift(f.KeyGroups, ws.Config.Rules[f.Rule].KeyGroups)
	}
	return f, true
}

// looksEncrypted is a cheap pre-filter before asking SOPS to parse the file.
func looksEncrypted(path string, data []byte) bool {
	if bytes.IndexByte(data, 0) >= 0 {
		return false
	}
	switch formats.FormatForPath(path) {
	case formats.Yaml:
		return bytes.HasPrefix(data, []byte("sops:")) || bytes.Contains(data, []byte("\nsops:"))
	case formats.Dotenv:
		return bytes.Contains(data, []byte("sops_mac="))
	case formats.Ini:
		return bytes.Contains(data, []byte("[sops]"))
	default: // json, and binary files (which SOPS stores as json)
		return bytes.Contains(data, []byte(`"sops"`)) && bytes.Contains(data, []byte(`"mac"`))
	}
}

func loadTree(path string, data []byte) (sops.Tree, error) {
	store := common.StoreForFormat(formats.FormatForPath(path), sopsconfig.NewStoresConfig())
	tree, err := store.LoadEncryptedFile(data)
	if err != nil {
		return sops.Tree{}, err
	}
	if tree.Metadata.MessageAuthenticationCode == "" {
		return sops.Tree{}, errors.New("no sops metadata")
	}
	tree.FilePath = path
	return tree, nil
}

func formatName(f formats.Format) string {
	switch f {
	case formats.Yaml:
		return "yaml"
	case formats.Json:
		return "json"
	case formats.Dotenv:
		return "dotenv"
	case formats.Ini:
		return "ini"
	default:
		return "binary"
	}
}

// normalizeID drops the parts of a key's string form that .sops.yaml does not carry (KMS context and profile).
func normalizeID(t, id string) string {
	if t == "kms" {
		if i := strings.IndexByte(id, '|'); i >= 0 {
			return id[:i]
		}
	}
	return id
}

// drift compares recipients as flat sets. Key-group structure (Shamir) is not compared yet.
func drift(actual, expected [][]Recipient) *Drift {
	have, want := map[string]bool{}, map[string]bool{}
	for _, g := range actual {
		for _, r := range g {
			have[r.key()] = true
		}
	}
	d := &Drift{Missing: []Recipient{}, Extra: []Recipient{}}
	for _, g := range expected {
		for _, r := range g {
			want[r.key()] = true
			if !have[r.key()] {
				d.Missing = append(d.Missing, r)
			}
		}
	}
	for _, g := range actual {
		for _, r := range g {
			if !want[r.key()] {
				d.Extra = append(d.Extra, r)
			}
		}
	}
	if len(d.Missing) == 0 && len(d.Extra) == 0 {
		return nil
	}
	return d
}

// Errors the UI can recognize (and translate) by code.
var (
	ErrNotRecipient = errors.New("none of your keys is a recipient of this file")
	ErrIntegrity    = errors.New("integrity check failed: the file was modified outside SOPS")
	ErrInvalidPath  = errors.New("invalid path")
)

// Entry is one leaf value of a decrypted file, addressed by a flattened key path.
type Entry struct {
	Key   string `json:"key"`
	Value string `json:"value,omitempty"`
}

// Reveal decrypts the file at rel (relative to the root) and returns its values.
// It verifies the MAC exactly like `sops decrypt` does.
func (ws *Workspace) Reveal(rel string) ([]Entry, error) {
	path, err := ws.resolve(rel)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	tree, err := loadTree(path, data)
	if err != nil {
		return nil, err
	}
	key, err := tree.Metadata.GetDataKey()
	if err != nil {
		return nil, fmt.Errorf("cannot decrypt %s: %w", rel, ErrNotRecipient)
	}
	cipher := aes.NewCipher()
	mac, err := tree.Decrypt(key, cipher)
	if err != nil {
		return nil, err
	}
	original, err := cipher.Decrypt(tree.Metadata.MessageAuthenticationCode, key, tree.Metadata.LastModified.Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt MAC: %w", err)
	}
	if original != mac {
		return nil, ErrIntegrity
	}
	var out []Entry
	walk(tree.Branches, func(k string, v any) { out = append(out, Entry{Key: k, Value: fmt.Sprint(v)}) })
	return out, nil
}

// resolve turns a workspace-relative path into an absolute one, refusing anything outside the root.
func (ws *Workspace) resolve(rel string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(rel))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w %q", ErrInvalidPath, rel)
	}
	return filepath.Join(ws.Root, clean), nil
}

func keyNames(branches sops.TreeBranches) []string {
	keys := []string{}
	walk(branches, func(k string, _ any) { keys = append(keys, k) })
	return keys
}

// walk visits every leaf value with a flattened path: nested maps join with ".", lists use [i].
func walk(branches sops.TreeBranches, visit func(string, any)) {
	for i, b := range branches {
		prefix := ""
		if len(branches) > 1 {
			prefix = fmt.Sprintf("doc[%d]", i)
		}
		walkValue(prefix, b, visit)
	}
}

func walkValue(prefix string, v any, visit func(string, any)) {
	switch t := v.(type) {
	case sops.TreeBranch:
		for _, item := range t {
			if _, isComment := item.Key.(sops.Comment); isComment {
				continue
			}
			walkValue(join(prefix, fmt.Sprint(item.Key)), item.Value, visit)
		}
	case []any:
		for i, e := range t {
			walkValue(fmt.Sprintf("%s[%d]", prefix, i), e, visit)
		}
	case sops.Comment:
	default:
		visit(prefix, t)
	}
}

func join(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}
