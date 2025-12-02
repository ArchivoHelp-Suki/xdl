package config

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ghostlawless/xdl/internal/httpx"
)

type GraphQLOperation struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

type GraphQLSection struct {
	Operations map[string]GraphQLOperation `json:"operations"`
}

type AuthCookies struct {
	GuestID   string `json:"guest_id"`
	AuthToken string `json:"auth_token"`
	Ct0       string `json:"ct0"`
}

type AuthSection struct {
	Bearer  string      `json:"bearer"`
	Cookies AuthCookies `json:"cookies"`
}

type FeaturesSection struct {
	User  map[string]any `json:"user"`
	Media map[string]any `json:"media"`
}

type PathsSection struct {
	Logs     string `json:"logs"`
	Debug    string `json:"debug"`
	DebugRaw string `json:"debug_raw"`
	Exports  string `json:"exports"`
}

type RuntimeSection struct {
	DebugEnabled   bool   `json:"debug_enabled"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	MaxRetries     int    `json:"max_retries"`
	LimiterSecret  string `json:"limiter_secret"`
}

type XSection struct {
	Network string `json:"network"`
}

type EssentialsConfig struct {
	X        XSection          `json:"x,omitempty"`
	GraphQL  GraphQLSection    `json:"graphql"`
	Auth     AuthSection       `json:"auth"`
	Headers  map[string]string `json:"headers"`
	Features FeaturesSection   `json:"features"`
	Paths    PathsSection      `json:"paths"`
	Runtime  RuntimeSection    `json:"runtime"`
}

func LoadEssentialsWithFallback(ps []string) (*EssentialsConfig, error) {
	var e error
	for _, p := range ps {
		if p == "" {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			e = err
			continue
		}
		var cfg EssentialsConfig
		if err := json.Unmarshal(b, &cfg); err != nil {
			return nil, fmt.Errorf("failed to parse essentials.json: %w", err)
		}
		if strings.TrimSpace(cfg.X.Network) == "" {
			cfg.X.Network = "https://x.com"
		}
		return &cfg, nil
	}
	if e != nil {
		return nil, e
	}
	return nil, fmt.Errorf("no essentials.json found")
}

func (c *EssentialsConfig) HTTPTimeout() time.Duration {
	if c == nil {
		return 15 * time.Second
	}
	if c.Runtime.TimeoutSeconds <= 0 {
		return 15 * time.Second
	}
	return time.Duration(c.Runtime.TimeoutSeconds) * time.Second
}

func (c *EssentialsConfig) GraphQLURL(key string) (string, error) {
	if c == nil {
		return "", fmt.Errorf("nil config")
	}
	if c.GraphQL.Operations == nil {
		return "", fmt.Errorf("graphql.operations is empty")
	}
	op, ok := c.GraphQL.Operations[key]
	if !ok || strings.TrimSpace(op.Path) == "" {
		return "", fmt.Errorf("unknown graphql operation: %s", key)
	}
	base := strings.TrimRight(c.X.Network, "/") + "/i/api/graphql"
	return base + "/" + op.Path, nil
}

func (c *EssentialsConfig) FeatureJSONFor(key string) (string, error) {
	if c == nil {
		return "{}", nil
	}
	var src any
	switch key {
	case "user_by_screen_name":
		src = c.Features.User
	case "user_media":
		src = c.Features.Media
	default:
		src = c.Features.User
	}
	if src == nil {
		return "{}", nil
	}
	b, err := json.Marshal(src)
	if err != nil {
		return "{}", err
	}
	return string(b), nil
}

func (c *EssentialsConfig) BuildRequestHeaders(req *http.Request, ref string) {
	if c == nil || req == nil {
		return
	}
	httpx.ApplyConfiguredHeaders(req)
	for k, v := range c.Headers {
		if v == "" {
			continue
		}
		if strings.EqualFold(k, "cookie") {
			continue
		}
		req.Header.Set(k, v)
	}
	if ref != "" {
		req.Header.Set("Referer", ref)
	}
	if c.Auth.Bearer != "" {
		req.Header.Set("Authorization", "Bearer "+c.Auth.Bearer)
	}
	if c.Auth.Cookies.Ct0 != "" {
		req.Header.Set("x-csrf-token", c.Auth.Cookies.Ct0)
	}
	var cs []string
	if c.Auth.Cookies.GuestID != "" {
		cs = append(cs, "guest_id="+c.Auth.Cookies.GuestID)
	}
	if c.Auth.Cookies.AuthToken != "" {
		cs = append(cs, "auth_token="+c.Auth.Cookies.AuthToken)
	}
	if c.Auth.Cookies.Ct0 != "" {
		cs = append(cs, "ct0="+c.Auth.Cookies.Ct0)
	}
	if len(cs) > 0 {
		req.Header.Set("Cookie", strings.Join(cs, "; "))
	}
}

type BrowserCookie struct {
	Domain string `json:"domain"`
	Name   string `json:"name"`
	Value  string `json:"value"`
	Path   string `json:"path"`
	Secure bool   `json:"secure"`
}

func ApplyCookiesFromFile(cfg *EssentialsConfig, p string) error {
	if cfg == nil {
		return fmt.Errorf("nil config")
	}
	if strings.TrimSpace(p) == "" {
		return nil
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return fmt.Errorf("failed to read cookie file: %w", err)
	}
	var cs []BrowserCookie
	if err := json.Unmarshal(b, &cs); err != nil {
		return fmt.Errorf("failed to parse cookie file: %w", err)
	}
	for _, c := range cs {
		d := strings.ToLower(strings.TrimSpace(c.Domain))
		if !strings.Contains(d, "x.com") {
			continue
		}
		switch strings.ToLower(c.Name) {
		case "guest_id":
			cfg.Auth.Cookies.GuestID = c.Value
		case "auth_token":
			cfg.Auth.Cookies.AuthToken = c.Value
		case "ct0":
			cfg.Auth.Cookies.Ct0 = c.Value
		}
	}
	return nil
}

func SaveEssentials(cfg *EssentialsConfig, p string) error {
	if cfg == nil {
		return fmt.Errorf("nil config")
	}
	if strings.TrimSpace(p) == "" {
		return fmt.Errorf("empty essentials path")
	}
	dir := filepath.Dir(p)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("failed to create essentials dir: %w", err)
		}
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal essentials: %w", err)
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return fmt.Errorf("failed to write temporary essentials: %w", err)
	}
	if err := os.Rename(tmp, p); err != nil {
		return fmt.Errorf("failed to replace essentials: %w", err)
	}
	return nil
}

func ApplyCookiesFromFileAndPersist(cfg *EssentialsConfig, cp, ep string) error {
	if err := ApplyCookiesFromFile(cfg, cp); err != nil {
		return err
	}
	if strings.TrimSpace(ep) == "" {
		return nil
	}
	return SaveEssentials(cfg, ep)
}
