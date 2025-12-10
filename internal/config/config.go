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

func LoadEssentialsWithFallback(paths []string) (*EssentialsConfig, error) {
	var lastErr error
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		cfg, err := loadEssentialsFromPath(path)
		if err != nil {
			lastErr = err
			continue
		}
		return cfg, nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("no essentials.json found")
}

func loadEssentialsFromPath(path string) (*EssentialsConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg EssentialsConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse essentials.json: %w", err)
	}
	cfg.X.Network = normalizeNetwork(cfg.X.Network)
	return &cfg, nil
}

func normalizeNetwork(network string) string {
	if strings.TrimSpace(network) == "" {
		return "https://x.com"
	}
	return network
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
	src := c.featureSource(key)
	if src == nil {
		return "{}", nil
	}
	data, err := json.Marshal(src)
	if err != nil {
		return "{}", err
	}
	return string(data), nil
}

func (c *EssentialsConfig) featureSource(key string) any {
	switch key {
	case "user_by_screen_name":
		return c.Features.User
	case "user_media":
		return c.Features.Media
	default:
		return c.Features.User
	}
}

func (c *EssentialsConfig) BuildRequestHeaders(req *http.Request, ref string) {
	if c == nil || req == nil {
		return
	}
	httpx.ApplyConfiguredHeaders(req)
	c.applyConfiguredHeaders(req)
	c.applyRefererHeader(req, ref)
	c.applyAuthHeaders(req)
	c.applyCookieHeader(req)
}

func (c *EssentialsConfig) applyConfiguredHeaders(req *http.Request) {
	for key, value := range c.Headers {
		if value == "" {
			continue
		}
		if strings.EqualFold(key, "cookie") {
			continue
		}
		req.Header.Set(key, value)
	}
}

func (c *EssentialsConfig) applyRefererHeader(req *http.Request, ref string) {
	if ref == "" {
		return
	}
	req.Header.Set("Referer", ref)
}

func (c *EssentialsConfig) applyAuthHeaders(req *http.Request) {
	if c.Auth.Bearer != "" {
		req.Header.Set("Authorization", "Bearer "+c.Auth.Bearer)
	}
	if c.Auth.Cookies.Ct0 != "" {
		req.Header.Set("x-csrf-token", c.Auth.Cookies.Ct0)
	}
}

func (c *EssentialsConfig) applyCookieHeader(req *http.Request) {
	cookieHeader := c.buildCookieHeader()
	if cookieHeader == "" {
		return
	}
	req.Header.Set("Cookie", cookieHeader)
}

func (c *EssentialsConfig) buildCookieHeader() string {
	var parts []string
	if c.Auth.Cookies.GuestID != "" {
		parts = append(parts, "guest_id="+c.Auth.Cookies.GuestID)
	}
	if c.Auth.Cookies.AuthToken != "" {
		parts = append(parts, "auth_token="+c.Auth.Cookies.AuthToken)
	}
	if c.Auth.Cookies.Ct0 != "" {
		parts = append(parts, "ct0="+c.Auth.Cookies.Ct0)
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "; ")
}

type BrowserCookie struct {
	Domain string `json:"domain"`
	Name   string `json:"name"`
	Value  string `json:"value"`
	Path   string `json:"path"`
	Secure bool   `json:"secure"`
}

func ApplyCookiesFromFile(cfg *EssentialsConfig, path string) error {
	if cfg == nil {
		return fmt.Errorf("nil config")
	}
	if strings.TrimSpace(path) == "" {
		return nil
	}
	cookies, err := loadBrowserCookies(path)
	if err != nil {
		return err
	}
	cfg.applyBrowserCookies(cookies)
	return nil
}

func loadBrowserCookies(path string) ([]BrowserCookie, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read cookie file: %w", err)
	}
	var cookies []BrowserCookie
	if err := json.Unmarshal(data, &cookies); err != nil {
		return nil, fmt.Errorf("failed to parse cookie file: %w", err)
	}
	return cookies, nil
}

func (c *EssentialsConfig) applyBrowserCookies(cookies []BrowserCookie) {
	for _, cookie := range cookies {
		domain := normalizeDomain(cookie.Domain)
		if !strings.Contains(domain, "x.com") {
			continue
		}
		c.assignCookieValue(cookie)
	}
}

func normalizeDomain(domain string) string {
	return strings.ToLower(strings.TrimSpace(domain))
}

func (c *EssentialsConfig) assignCookieValue(cookie BrowserCookie) {
	switch strings.ToLower(cookie.Name) {
	case "guest_id":
		c.Auth.Cookies.GuestID = cookie.Value
	case "auth_token":
		c.Auth.Cookies.AuthToken = cookie.Value
	case "ct0":
		c.Auth.Cookies.Ct0 = cookie.Value
	}
}

func SaveEssentials(cfg *EssentialsConfig, path string) error {
	if cfg == nil {
		return fmt.Errorf("nil config")
	}
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("empty essentials path")
	}
	if err := ensureEssentialsDir(path); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal essentials: %w", err)
	}
	if err := writeEssentialsAtomically(path, data); err != nil {
		return err
	}
	return nil
}

func ensureEssentialsDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "" || dir == "." {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create essentials dir: %w", err)
	}
	return nil
}

func writeEssentialsAtomically(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("failed to write temporary essentials: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("failed to replace essentials: %w", err)
	}
	return nil
}

func ApplyCookiesFromFileAndPersist(cfg *EssentialsConfig, cookiePath, essentialsPath string) error {
	if err := ApplyCookiesFromFile(cfg, cookiePath); err != nil {
		return err
	}
	if strings.TrimSpace(essentialsPath) == "" {
		return nil
	}
	return SaveEssentials(cfg, essentialsPath)
}
