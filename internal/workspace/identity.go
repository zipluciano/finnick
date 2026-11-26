package workspace

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"

	"filippo.io/age"
)

// Identity describes the local age key(s) SOPS will use to decrypt.
type Identity struct {
	// Source is where the key was found: a file path, or "SOPS_AGE_KEY".
	Source string `json:"source,omitempty"`
	// Recipients are the public keys (age1...) derived from the local private keys.
	Recipients []string `json:"recipients"`
	Error      string   `json:"error,omitempty"`
}

// PrepareAgeEnv makes SOPS find keys at ~/.config/sops/age/keys.txt on macOS too.
//
// On macOS SOPS defaults to ~/Library/Application Support/sops/age/keys.txt unless
// XDG_CONFIG_HOME is set, while most guides (and Linux) use ~/.config. When the user
// configured nothing and only the ~/.config file exists, point SOPS_AGE_KEY_FILE at it.
func PrepareAgeEnv() {
	if os.Getenv("SOPS_AGE_KEY") != "" || os.Getenv("SOPS_AGE_KEY_FILE") != "" || os.Getenv("SOPS_AGE_KEY_CMD") != "" {
		return
	}
	if fileExists(defaultAgeKeyFile()) {
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	if alt := filepath.Join(home, ".config", "sops", "age", "keys.txt"); fileExists(alt) {
		os.Setenv("SOPS_AGE_KEY_FILE", alt)
	}
}

// defaultAgeKeyFile mirrors SOPS' own default location.
func defaultAgeKeyFile() string {
	dir := ""
	if runtime.GOOS == "darwin" {
		dir = os.Getenv("XDG_CONFIG_HOME")
	}
	if dir == "" {
		d, err := os.UserConfigDir()
		if err != nil {
			return ""
		}
		dir = d
	}
	return filepath.Join(dir, "sops", "age", "keys.txt")
}

// LocalIdentity reads the local age identities the same way SOPS does (env first, then default file).
func LocalIdentity() Identity {
	var id Identity
	var data []byte
	switch {
	case os.Getenv("SOPS_AGE_KEY") != "":
		id.Source = "SOPS_AGE_KEY"
		data = []byte(os.Getenv("SOPS_AGE_KEY"))
	default:
		path := os.Getenv("SOPS_AGE_KEY_FILE")
		if path == "" {
			path = defaultAgeKeyFile()
		}
		id.Source = path
		b, err := os.ReadFile(path)
		if err != nil {
			id.Error = "no age key found at " + path
			return id
		}
		data = b
	}
	identities, err := age.ParseIdentities(bytes.NewReader(data))
	if err != nil {
		id.Error = "could not parse age key: " + err.Error()
		return id
	}
	for _, i := range identities {
		if x, ok := i.(*age.X25519Identity); ok {
			id.Recipients = append(id.Recipients, x.Recipient().String())
		}
	}
	return id
}

func fileExists(p string) bool {
	if p == "" {
		return false
	}
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
