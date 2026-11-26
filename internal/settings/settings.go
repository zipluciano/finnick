// Package settings stores UI preferences on disk.
//
// The UI runs on a random loopback port, so browser storage (scoped by origin, which
// includes the port) would forget preferences on every launch. They live in
// ~/.config/finnick/settings.json instead.
package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Settings struct {
	Theme  string `json:"theme"`  // system | light | dark
	Locale string `json:"locale"` // "" (follow the browser) | en | pt-BR | es
}

var (
	Themes  = []string{"system", "light", "dark"}
	Locales = []string{"en", "pt-BR", "es"}
)

func Default() Settings { return Settings{Theme: "system"} }

// Validate rejects unknown values so the file never holds something the UI cannot render.
func (s Settings) Validate() error {
	if !contains(Themes, s.Theme) {
		return fmt.Errorf("theme must be one of %v", Themes)
	}
	if s.Locale != "" && !contains(Locales, s.Locale) {
		return fmt.Errorf("locale must be one of %v", Locales)
	}
	return nil
}

// Path is ~/.config/finnick/settings.json (or under $XDG_CONFIG_HOME).
func Path() (string, error) {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "finnick", "settings.json"), nil
}

// Load returns the saved settings, or defaults when there is no (valid) file.
func Load() Settings {
	s := Default()
	p, err := Path()
	if err != nil {
		return s
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return s
	}
	var saved Settings
	if json.Unmarshal(data, &saved) != nil {
		return s
	}
	if saved.Theme == "" {
		saved.Theme = s.Theme
	}
	if saved.Validate() != nil {
		return s
	}
	return saved
}

func Save(s Settings) error {
	if err := s.Validate(); err != nil {
		return err
	}
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(s, "", "  ")
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
