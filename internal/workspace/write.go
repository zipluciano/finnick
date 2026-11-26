package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/getsops/sops/v3"
	"github.com/getsops/sops/v3/aes"
	"github.com/getsops/sops/v3/cmd/sops/common"
	"github.com/getsops/sops/v3/cmd/sops/formats"
	sopsconfig "github.com/getsops/sops/v3/config"
	"github.com/getsops/sops/v3/keyservice"
	"github.com/getsops/sops/v3/version"
)

// Errors for writes the UI can recognize (and translate) by code.
var (
	ErrStale       = errors.New("the file changed on disk since it was read: reload it and try again")
	ErrUnsupported = errors.New("this file cannot be edited here: use sops edit")
	ErrKeyExists   = errors.New("a key with that name already exists")
	ErrKeyNotFound = errors.New("key not found")
	ErrInvalidKey  = errors.New("invalid key name")
	ErrNoRule      = errors.New("no creation rule in .sops.yaml matches this path")
	ErrFileExists  = errors.New("a file already exists at this path")
)

// Version identifies the exact bytes of a file, so a write can refuse to overwrite changes it has not seen.
func Version(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// opened is a SOPS file read from disk and decrypted, ready to be changed and written back.
type opened struct {
	path    string
	format  formats.Format
	store   common.Store
	data    []byte
	tree    sops.Tree
	dataKey []byte
}

// stores returns the store settings of .sops.yaml (YAML/JSON indentation), like the sops CLI.
func (ws *Workspace) stores() *sopsconfig.StoresConfig {
	if ws.Config != nil {
		if sc, err := sopsconfig.LoadStoresConfig(ws.Config.Path); err == nil {
			return sc
		}
	}
	return sopsconfig.NewStoresConfig()
}

// open reads and decrypts the file at path, verifying its MAC exactly like `sops decrypt`.
func (ws *Workspace) open(rel, path string) (*opened, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	format := formats.FormatForPath(path)
	store := common.StoreForFormat(format, ws.stores())
	tree, err := store.LoadEncryptedFile(data)
	if err != nil {
		return nil, err
	}
	if tree.Metadata.MessageAuthenticationCode == "" {
		return nil, errors.New("no sops metadata")
	}
	tree.FilePath = path
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
	return &opened{path: path, format: format, store: store, data: data, tree: tree, dataKey: key}, nil
}

// encrypt encrypts the tree with its data key (new lastmodified and MAC) and returns the file's bytes
// and its entries.
func (o *opened) encrypt() ([]byte, []Entry, error) {
	entries := entriesOf(o.tree.Branches)
	if err := common.EncryptTree(common.EncryptTreeOpts{Tree: &o.tree, Cipher: aes.NewCipher(), DataKey: o.dataKey}); err != nil {
		return nil, nil, err
	}
	out, err := o.store.EmitEncryptedFile(o.tree)
	return out, entries, err
}

// save encrypts the tree again with its data key, as `sops edit` does, and replaces the file on disk.
// It returns the new version and the entries of the saved file.
func (o *opened) save() (string, []Entry, error) {
	out, entries, err := o.encrypt()
	if err != nil {
		return "", nil, err
	}
	if err := replaceFile(o.path, out); err != nil {
		return "", nil, err
	}
	return Version(out), entries, nil
}

// Create makes a new encrypted file at rel with the keys added by edits (all of them "add"). Its
// recipients and encryption settings come from the creation rule that matches the path, as with
// `sops encrypt`; without a matching rule nothing is written and ErrNoRule is returned.
func (ws *Workspace) Create(rel string, edits []Edit) (string, []Entry, error) {
	path, err := ws.resolveWrite(rel)
	if err != nil {
		return "", nil, err
	}
	if _, err := os.Lstat(path); err == nil {
		return "", nil, ErrFileExists
	}
	format := formats.FormatForPath(path)
	if format == formats.Binary {
		return "", nil, fmt.Errorf("%w: new files must end in .env, .yaml, .yml, .json or .ini", ErrUnsupported)
	}
	if ws.Config == nil {
		return "", nil, ErrNoRule
	}
	rule, err := sopsconfig.LoadCreationRuleForFile(ws.Config.Path, path, map[string]*string{})
	if err != nil || rule == nil || len(rule.KeyGroups) == 0 {
		return "", nil, ErrNoRule
	}
	doc := sops.TreeBranch{}
	for _, e := range edits {
		if e.Op != "add" {
			return "", nil, fmt.Errorf("a new file only takes \"add\" edits, got %q", e.Op)
		}
		if doc, err = applyEdit(doc, format, e); err != nil {
			return "", nil, err
		}
	}
	if len(doc) == 0 {
		return "", nil, fmt.Errorf("%w: add at least one key", ErrInvalidKey)
	}
	tree := sops.Tree{
		Branches: sops.TreeBranches{doc},
		FilePath: path,
		Metadata: sops.Metadata{
			KeyGroups:               rule.KeyGroups,
			ShamirThreshold:         rule.ShamirThreshold,
			UnencryptedSuffix:       rule.UnencryptedSuffix,
			EncryptedSuffix:         rule.EncryptedSuffix,
			UnencryptedRegex:        rule.UnencryptedRegex,
			EncryptedRegex:          rule.EncryptedRegex,
			UnencryptedCommentRegex: rule.UnencryptedCommentRegex,
			EncryptedCommentRegex:   rule.EncryptedCommentRegex,
			MACOnlyEncrypted:        rule.MACOnlyEncrypted,
			Version:                 version.Version,
		},
	}
	key, errs := tree.GenerateDataKeyWithKeyServices([]keyservice.KeyServiceClient{keyservice.NewLocalClient()})
	if len(errs) > 0 {
		return "", nil, fmt.Errorf("could not encrypt the data key for every recipient: %v", errs)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", nil, err
	}
	o := &opened{path: path, format: format, store: common.StoreForFormat(format, ws.stores()), tree: tree, dataKey: key}
	return o.saveNew()
}

// saveNew is save for a file that must not exist yet: it fails instead of replacing one created meanwhile.
func (o *opened) saveNew() (string, []Entry, error) {
	out, entries, err := o.encrypt()
	if err != nil {
		return "", nil, err
	}
	f, err := os.OpenFile(o.path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, os.ErrExist) {
		return "", nil, ErrFileExists
	}
	if err != nil {
		return "", nil, err
	}
	if _, err := f.Write(out); err != nil {
		f.Close()
		os.Remove(o.path)
		return "", nil, err
	}
	if err := f.Close(); err != nil {
		os.Remove(o.path)
		return "", nil, err
	}
	return Version(out), entries, nil
}

// replaceFile writes data next to path and renames it over path, so a crash never leaves a
// half-written file. The file keeps its permissions.
func replaceFile(path string, data []byte) error {
	mode := os.FileMode(0o644)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".finnick-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// resolveWrite is resolve for paths Finnick writes to: it also refuses .sops.yaml, anything under
// .git, symlinks, and directories that lead outside the workspace through a symlink.
func (ws *Workspace) resolveWrite(rel string) (string, error) {
	path, err := ws.resolve(rel)
	if err != nil {
		return "", err
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel)))
	base := filepath.Base(path)
	if clean == "." || base == ".sops.yaml" || base == ".sops.yml" {
		return "", fmt.Errorf("%w %q", ErrInvalidPath, rel)
	}
	for _, part := range strings.Split(clean, "/") {
		if part == ".git" {
			return "", fmt.Errorf("%w %q", ErrInvalidPath, rel)
		}
	}
	if st, err := os.Lstat(path); err == nil && !st.Mode().IsRegular() {
		return "", fmt.Errorf("%w %q: not a regular file", ErrInvalidPath, rel)
	}
	root, err := filepath.EvalSymlinks(ws.Root)
	if err != nil {
		return "", err
	}
	// The closest existing ancestor must stay inside the root once symlinks are resolved.
	dir := filepath.Dir(path)
	for {
		if _, err := os.Lstat(dir); err == nil {
			break
		}
		dir = filepath.Dir(dir)
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", err
	}
	if real != root && !strings.HasPrefix(real, root+string(filepath.Separator)) {
		return "", fmt.Errorf("%w %q: outside the workspace", ErrInvalidPath, rel)
	}
	return path, nil
}

// Edit is one change to a key of a file. Path addresses the key: map keys are strings, list
// positions are numbers, as in the Path of an Entry.
type Edit struct {
	// Op is "set" (change an existing value), "add" (a new key; missing parents are created),
	// "delete", or "rename" (the last path element becomes To).
	Op    string `json:"op"`
	Path  []any  `json:"path"`
	Value string `json:"value,omitempty"`
	To    string `json:"to,omitempty"`
}

// Edit applies edits to the file at rel and encrypts it again with the same data key, like
// `sops edit`. version must be the Version of the file the edits were based on; if the file
// changed since, nothing is written and ErrStale is returned. All edits apply, or none.
func (ws *Workspace) Edit(rel, version string, edits []Edit) (string, []Entry, error) {
	path, err := ws.resolveWrite(rel)
	if err != nil {
		return "", nil, err
	}
	o, err := ws.open(rel, path)
	if err != nil {
		return "", nil, err
	}
	if version != Version(o.data) {
		return "", nil, ErrStale
	}
	if o.format == formats.Binary || len(o.tree.Branches) != 1 {
		return "", nil, ErrUnsupported
	}
	branch := o.tree.Branches[0]
	for _, e := range edits {
		if branch, err = applyEdit(branch, o.format, e); err != nil {
			return "", nil, err
		}
	}
	o.tree.Branches[0] = branch
	return o.save()
}

// applyEdit validates one edit against the file's format and applies it to the document.
func applyEdit(doc sops.TreeBranch, format formats.Format, e Edit) (sops.TreeBranch, error) {
	path, err := normalizePath(e.Path)
	if err != nil {
		return nil, err
	}
	if e.Op == "add" {
		if err := checkKeyPath(path, format); err != nil {
			return nil, err
		}
	}
	if e.Op == "rename" {
		renamed := append(append([]any{}, path[:len(path)-1]...), e.To)
		if err := checkKeyPath(renamed, format); err != nil {
			return nil, err
		}
	}
	parent, last := path[:len(path)-1], path[len(path)-1]
	var change func(container any) (any, error)
	switch e.Op {
	case "set":
		change = func(c any) (any, error) {
			return setLeaf(c, last, func(old any, found bool) (any, error) {
				if !found {
					return nil, keyErr(ErrKeyNotFound, path)
				}
				return typedLike(old, e.Value), nil
			})
		}
	case "add":
		change = func(c any) (any, error) {
			return setLeaf(c, last, func(_ any, found bool) (any, error) {
				if found {
					return nil, keyErr(ErrKeyExists, path)
				}
				return e.Value, nil
			})
		}
	case "delete":
		change = func(c any) (any, error) { return deleteLeaf(c, last, path) }
	case "rename":
		change = func(c any) (any, error) { return renameLeaf(c, last, e.To, path) }
	default:
		return nil, fmt.Errorf("unknown edit %q", e.Op)
	}
	out, err := at(doc, parent, e.Op == "add", e.Op == "delete", change)
	if err != nil {
		return nil, err
	}
	return out.(sops.TreeBranch), nil
}

// normalizePath turns a path decoded from JSON (strings and float64) into strings and ints.
func normalizePath(p []any) ([]any, error) {
	if len(p) == 0 {
		return nil, ErrInvalidKey
	}
	out := make([]any, len(p))
	for i, s := range p {
		switch v := s.(type) {
		case string:
			out[i] = v
		case int:
			out[i] = v
		case float64:
			if v < 0 || v != float64(int(v)) {
				return nil, ErrInvalidKey
			}
			out[i] = int(v)
		default:
			return nil, ErrInvalidKey
		}
	}
	return out, nil
}

// checkKeyPath validates the names of a key about to be created, for the shape each format allows.
func checkKeyPath(path []any, format formats.Format) error {
	for _, s := range path {
		name, ok := s.(string)
		if !ok || strings.TrimSpace(name) == "" || strings.ContainsAny(name, "\x00\r\n") {
			return ErrInvalidKey
		}
	}
	first := path[0].(string)
	if first == "sops" {
		return fmt.Errorf("%w: %q is reserved by SOPS", ErrInvalidKey, first)
	}
	switch format {
	case formats.Dotenv:
		if len(path) != 1 || strings.ContainsAny(first, "= \t") || strings.HasPrefix(first, "#") {
			return fmt.Errorf("%w: a .env name has no spaces, '=' or leading '#'", ErrInvalidKey)
		}
		if strings.HasPrefix(first, "sops_") {
			return fmt.Errorf("%w: names starting with sops_ are reserved by SOPS", ErrInvalidKey)
		}
	case formats.Ini:
		if len(path) != 2 {
			return fmt.Errorf("%w: an .ini key is section.name", ErrInvalidKey)
		}
	}
	return nil
}

func keyErr(err error, path []any) error {
	return fmt.Errorf("%w: %s", err, pathString(path))
}

func pathString(path []any) string {
	var b strings.Builder
	for i, s := range path {
		if n, ok := s.(int); ok {
			fmt.Fprintf(&b, "[%d]", n)
			continue
		}
		if i > 0 {
			b.WriteByte('.')
		}
		b.WriteString(s.(string))
	}
	return b.String()
}

// at walks node along path to a map or list and replaces it with change(container). With create,
// missing maps along the way are created. With prune, maps left empty by the change are removed.
func at(node any, path []any, create, prune bool, change func(any) (any, error)) (any, error) {
	if len(path) == 0 {
		switch node.(type) {
		case sops.TreeBranch, []any:
			return change(node)
		}
		return nil, ErrInvalidKey
	}
	switch n := node.(type) {
	case sops.TreeBranch:
		name, ok := path[0].(string)
		if !ok {
			return nil, ErrKeyNotFound
		}
		i := indexOf(n, name)
		if i < 0 {
			if !create {
				return nil, ErrKeyNotFound
			}
			child, err := at(sops.TreeBranch{}, path[1:], create, prune, change)
			if err != nil {
				return nil, err
			}
			return append(n, sops.TreeItem{Key: name, Value: child}), nil
		}
		child, err := at(n[i].Value, path[1:], create, prune, change)
		if err != nil {
			return nil, err
		}
		if b, ok := child.(sops.TreeBranch); prune && ok && len(b) == 0 {
			return append(n[:i:i], n[i+1:]...), nil
		}
		n[i].Value = child
		return n, nil
	case []any:
		pos, ok := path[0].(int)
		if !ok || pos >= len(n) {
			return nil, ErrKeyNotFound
		}
		child, err := at(n[pos], path[1:], create, prune, change)
		if err != nil {
			return nil, err
		}
		n[pos] = child
		return n, nil
	}
	return nil, ErrInvalidKey // a value where a map was expected
}

// indexOf finds a key in a map, skipping comments. Keys are compared by their text, so a YAML
// key written as 1 is found by "1".
func indexOf(b sops.TreeBranch, name string) int {
	for i, item := range b {
		if _, isComment := item.Key.(sops.Comment); isComment {
			continue
		}
		if fmt.Sprint(item.Key) == name {
			return i
		}
	}
	return -1
}

func isLeaf(v any) bool {
	switch v.(type) {
	case sops.TreeBranch, []any:
		return false
	}
	return true
}

// setLeaf sets one value of a map or list through value(old, found).
func setLeaf(c any, key any, value func(old any, found bool) (any, error)) (any, error) {
	switch n := c.(type) {
	case sops.TreeBranch:
		name, ok := key.(string)
		if !ok {
			return nil, ErrInvalidKey
		}
		i := indexOf(n, name)
		if i >= 0 && !isLeaf(n[i].Value) {
			return nil, ErrInvalidKey // replacing a whole map or list is not a value edit
		}
		var old any
		if i >= 0 {
			old = n[i].Value
		}
		v, err := value(old, i >= 0)
		if err != nil {
			return nil, err
		}
		if i >= 0 {
			n[i].Value = v
			return n, nil
		}
		return append(n, sops.TreeItem{Key: name, Value: v}), nil
	case []any:
		pos, ok := key.(int)
		found := ok && pos < len(n)
		if found && !isLeaf(n[pos]) {
			return nil, ErrInvalidKey
		}
		var old any
		if found {
			old = n[pos]
		}
		v, err := value(old, found)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, ErrInvalidKey // lists grow only through sops edit
		}
		n[pos] = v
		return n, nil
	}
	return nil, ErrInvalidKey
}

func deleteLeaf(c any, key any, path []any) (any, error) {
	switch n := c.(type) {
	case sops.TreeBranch:
		name, _ := key.(string)
		i := indexOf(n, name)
		if i < 0 {
			return nil, keyErr(ErrKeyNotFound, path)
		}
		if !isLeaf(n[i].Value) {
			return nil, ErrInvalidKey
		}
		return append(n[:i:i], n[i+1:]...), nil
	case []any:
		pos, ok := key.(int)
		if !ok || pos >= len(n) {
			return nil, keyErr(ErrKeyNotFound, path)
		}
		if !isLeaf(n[pos]) {
			return nil, ErrInvalidKey
		}
		return append(n[:pos:pos], n[pos+1:]...), nil
	}
	return nil, ErrInvalidKey
}

func renameLeaf(c any, key any, to string, path []any) (any, error) {
	n, ok := c.(sops.TreeBranch)
	name, isName := key.(string)
	if !ok || !isName {
		return nil, ErrInvalidKey // list positions cannot be renamed
	}
	i := indexOf(n, name)
	if i < 0 {
		return nil, keyErr(ErrKeyNotFound, path)
	}
	if !isLeaf(n[i].Value) {
		return nil, ErrInvalidKey
	}
	if to == name {
		return n, nil
	}
	if indexOf(n, to) >= 0 {
		return nil, keyErr(ErrKeyExists, append(append([]any{}, path[:len(path)-1]...), to))
	}
	n[i].Key = to
	return n, nil
}
