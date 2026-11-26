package workspace

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Recipient is one master key that can (or should) decrypt a file.
type Recipient struct {
	// Type follows SOPS' own identifiers: age, pgp, kms, gcp_kms, azure_kv, hc_vault, hckms.
	Type string `json:"type"`
	ID   string `json:"id"`
	// Name is the YAML anchor the key was declared with in .sops.yaml (e.g. &alice), if any.
	Name string `json:"name,omitempty"`
}

func (r Recipient) key() string { return r.Type + "\x00" + r.ID }

// Rule is one entry of creation_rules, in file order.
type Rule struct {
	Index     int           `json:"index"`
	PathRegex string        `json:"pathRegex"`
	KeyGroups [][]Recipient `json:"keyGroups"`
	re        *regexp.Regexp
}

func (r Rule) matches(rel string) bool {
	return r.re == nil || r.re.MatchString(rel)
}

// Config is the parsed .sops.yaml.
type Config struct {
	Path  string `json:"path"`
	Rules []Rule `json:"rules"`
	// aliases maps a recipient ID to the anchor name it was declared with.
	aliases map[string]string
}

// RuleFor returns the index of the creation rule SOPS would apply to the file at rel
// (slash- or OS-separated, relative to the config directory), or -1 when none matches.
// Like SOPS, the first matching rule wins and a rule without path_regex matches everything.
func (c *Config) RuleFor(rel string) int {
	if c == nil {
		return -1
	}
	for _, r := range c.Rules {
		if r.matches(rel) {
			return r.Index
		}
	}
	return -1
}

func (c *Config) name(id string) string {
	if c == nil {
		return ""
	}
	return c.aliases[id]
}

type rawGroup struct {
	Age []string `yaml:"age"`
	PGP []string `yaml:"pgp"`
	KMS []struct {
		Arn  string `yaml:"arn"`
		Role string `yaml:"role"`
	} `yaml:"kms"`
	GCPKMS []struct {
		ResourceID string `yaml:"resource_id"`
	} `yaml:"gcp_kms"`
	AzureKV []struct {
		VaultURL string `yaml:"vaultUrl"`
		Key      string `yaml:"key"`
		Version  string `yaml:"version"`
	} `yaml:"azure_keyvault"`
	Vault []string `yaml:"hc_vault"`
	HCKMS []struct {
		KeyID string `yaml:"key_id"`
	} `yaml:"hckms"`
}

type rawRule struct {
	PathRegex string     `yaml:"path_regex"`
	Age       any        `yaml:"age"`
	PGP       any        `yaml:"pgp"`
	KMS       any        `yaml:"kms"`
	GCPKMS    any        `yaml:"gcp_kms"`
	AzureKV   any        `yaml:"azure_keyvault"`
	Vault     any        `yaml:"hc_vault_transit_uri"`
	HCKMS     []string   `yaml:"hckms"`
	KeyGroups []rawGroup `yaml:"key_groups"`
}

type rawConfig struct {
	CreationRules []rawRule `yaml:"creation_rules"`
}

// LoadConfig parses a .sops.yaml file.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseConfig(path, data)
}

func parseConfig(path string, data []byte) (*Config, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	cfg := &Config{Path: path, aliases: map[string]string{}}
	collectAnchors(&doc, cfg.aliases)

	var raw rawConfig
	if err := doc.Decode(&raw); err != nil && doc.Kind != 0 {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	for i, rr := range raw.CreationRules {
		rule := Rule{Index: i, PathRegex: rr.PathRegex}
		if rr.PathRegex != "" {
			re, err := regexp.Compile(rr.PathRegex)
			if err != nil {
				return nil, fmt.Errorf("creation_rules[%d]: invalid path_regex: %w", i, err)
			}
			rule.re = re
		}
		rule.KeyGroups = cfg.groupsFor(rr)
		cfg.Rules = append(cfg.Rules, rule)
	}
	return cfg, nil
}

// collectAnchors records every anchored scalar (e.g. `- &alice age1...`) so keys can be shown by name.
func collectAnchors(n *yaml.Node, out map[string]string) {
	if n.Kind == yaml.ScalarNode && n.Anchor != "" {
		if _, exists := out[n.Value]; !exists {
			out[n.Value] = n.Anchor
		}
	}
	for _, c := range n.Content {
		collectAnchors(c, out)
	}
}

func (c *Config) groupsFor(rr rawRule) [][]Recipient {
	if len(rr.KeyGroups) > 0 {
		var groups [][]Recipient
		for _, g := range rr.KeyGroups {
			var group []Recipient
			add := func(t, id string) { group = append(group, c.recipient(t, id)) }
			for _, k := range g.Age {
				add("age", k)
			}
			for _, k := range g.PGP {
				add("pgp", k)
			}
			for _, k := range g.KMS {
				id := k.Arn
				if k.Role != "" {
					id += "+" + k.Role
				}
				add("kms", id)
			}
			for _, k := range g.GCPKMS {
				add("gcp_kms", k.ResourceID)
			}
			for _, k := range g.AzureKV {
				add("azure_kv", fmt.Sprintf("%s/keys/%s/%s", k.VaultURL, k.Key, k.Version))
			}
			for _, k := range g.Vault {
				add("hc_vault", k)
			}
			for _, k := range g.HCKMS {
				add("hckms", k.KeyID)
			}
			groups = append(groups, group)
		}
		return groups
	}

	// Flat form: a single implicit key group.
	var group []Recipient
	for _, f := range []struct {
		t string
		v any
	}{{"age", rr.Age}, {"pgp", rr.PGP}, {"kms", rr.KMS}, {"gcp_kms", rr.GCPKMS}, {"azure_kv", rr.AzureKV}, {"hc_vault", rr.Vault}} {
		for _, id := range splitKeys(f.v) {
			group = append(group, c.recipient(f.t, id))
		}
	}
	for _, id := range rr.HCKMS {
		group = append(group, c.recipient("hckms", id))
	}
	if len(group) == 0 {
		return nil
	}
	return [][]Recipient{group}
}

func (c *Config) recipient(t, id string) Recipient {
	id = strings.TrimSpace(id)
	return Recipient{Type: t, ID: id, Name: c.name(id)}
}

// splitKeys accepts SOPS' "string or list of strings" fields; strings may be comma-separated.
func splitKeys(v any) []string {
	var parts []string
	switch t := v.(type) {
	case string:
		parts = strings.Split(t, ",")
	case []any:
		for _, e := range t {
			if s, ok := e.(string); ok {
				parts = append(parts, strings.Split(s, ",")...)
			}
		}
	}
	var out []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
