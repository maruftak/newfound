// Package config loads and validates a reconsentry scope file.
package config

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/maruftak/reconsentry/internal/model"
)

// Notify holds notification settings for a scope. Each field is a list of
// destination URLs rendered in that platform's format, so a single scope can
// fan out to a generic endpoint, Slack, and Discord at the same time.
type Notify struct {
	Webhooks []string `yaml:"webhooks"` // generic JSON POST
	Slack    []string `yaml:"slack"`    // Slack incoming-webhook URLs
	Discord  []string `yaml:"discord"`  // Discord webhook URLs
}

// Endpoints returns every configured destination URL.
func (n Notify) Endpoints() []string {
	out := make([]string, 0, len(n.Webhooks)+len(n.Slack)+len(n.Discord))
	out = append(out, n.Webhooks...)
	out = append(out, n.Slack...)
	out = append(out, n.Discord...)
	return out
}

// Config is a single monitoring scope.
type Config struct {
	Name        string   `yaml:"name"`
	Targets     []string `yaml:"targets"`
	Exclude     []string `yaml:"exclude"`
	MinPriority string   `yaml:"min_priority"` // low | medium | high
	TrackIP     bool     `yaml:"track_ip"`     // alert on IP changes (noisy on CDNs); off by default
	Notify      Notify   `yaml:"notify"`
}

// Load reads and validates a scope config from path.
func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	return Parse(b)
}

// Parse validates raw YAML bytes into a Config.
func Parse(b []byte) (*Config, error) {
	b = bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF}) // strip leading UTF-8 BOM (e.g. Notepad-saved files)
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	c.normalize()
	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) validate() error {
	if strings.TrimSpace(c.Name) == "" {
		return fmt.Errorf("config: name is required")
	}
	if len(c.Targets) == 0 {
		return fmt.Errorf("config: at least one target is required")
	}
	for _, t := range c.Targets {
		if t == "" {
			return fmt.Errorf("config: empty target not allowed")
		}
		if strings.ContainsAny(t, "/:") {
			return fmt.Errorf("config: target %q must be a bare domain (no scheme or path)", t)
		}
	}
	switch c.MinPriority {
	case "low", "medium", "high":
	default:
		return fmt.Errorf("config: min_priority must be low|medium|high, got %q", c.MinPriority)
	}
	for _, u := range c.Notify.Endpoints() {
		if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
			return fmt.Errorf("config: notify URL %q must start with http:// or https://", u)
		}
	}
	return nil
}

func (c *Config) normalize() {
	if c.MinPriority == "" {
		c.MinPriority = "low"
	}
	c.MinPriority = strings.ToLower(strings.TrimSpace(c.MinPriority))
	for i, t := range c.Targets {
		c.Targets[i] = strings.ToLower(model.TrimInvisible(t))
	}
	for i, e := range c.Exclude {
		c.Exclude[i] = strings.ToLower(model.TrimInvisible(e))
	}
}

// Excluded reports whether host matches any exclude entry, by exact match or
// as a subdomain suffix (dev.example.com excludes api.dev.example.com).
func (c *Config) Excluded(host string) bool {
	host = strings.ToLower(host)
	for _, e := range c.Exclude {
		if e == "" {
			continue
		}
		if host == e || strings.HasSuffix(host, "."+e) {
			return true
		}
	}
	return false
}
