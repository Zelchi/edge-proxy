package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateAcceptsValidConfig(t *testing.T) {
	cfg := validConfig()
	cfg.Fallback = FallbackConfig{Host: "app.example", StatusCode: 308}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsInvalidConfig(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Config)
		want string
	}{
		{
			name: "duplicate route host",
			edit: func(cfg *Config) {
				cfg.Routes = append(cfg.Routes, cfg.Routes[0])
			},
			want: "duplicates",
		},
		{
			name: "invalid upstream scheme",
			edit: func(cfg *Config) {
				cfg.Routes[0].Upstream = "ftp://upstream.example"
			},
			want: "must use http or https",
		},
		{
			name: "route missing TLS domain",
			edit: func(cfg *Config) {
				cfg.Routes[0].Host = "missing.example"
			},
			want: "is not listed in tls.domains",
		},
		{
			name: "invalid fallback",
			edit: func(cfg *Config) {
				cfg.Fallback = FallbackConfig{Host: "other.example", StatusCode: 308}
			},
			want: "must be a route host",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := validConfig()
			test.edit(&cfg)

			err := cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestValidateReload(t *testing.T) {
	current := validConfig()
	next := validConfig()
	next.Routes[0].Upstream = "http://new-upstream.example:8080"
	next.HTTP.RedirectToHTTPS = false
	next.Fallback = FallbackConfig{Host: "app.example", StatusCode: 308}

	if err := ValidateReload(&current, &next); err != nil {
		t.Fatalf("ValidateReload() error = %v", err)
	}

	next.HTTPS.Address = ":8443"
	if err := ValidateReload(&current, &next); err == nil {
		t.Fatal("ValidateReload() accepted an HTTPS address change")
	}
}

func TestLoadNormalizesHosts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	contents := []byte(`
http:
  address: ":80"
https:
  address: ":443"
tls:
  certs_dir: /tmp/certs
  domains: ["APP.EXAMPLE."]
routes:
  - host: "APP.EXAMPLE."
    upstream: http://upstream.example:8080
dashboard:
  host: "APP.EXAMPLE."
fallback:
  host: "APP.EXAMPLE."
`)
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TLS.Domains[0] != "app.example" || cfg.Routes[0].Host != "app.example" || cfg.Fallback.Host != "app.example" || cfg.Dashboard.Host != "app.example" {
		t.Fatalf("hosts were not normalized: %#v", cfg)
	}
}

func validConfig() Config {
	return Config{
		HTTP:  HTTPConfig{Address: ":80", RedirectToHTTPS: true},
		HTTPS: HTTPSConfig{Address: ":443"},
		TLS: TLSConfig{
			CertsDir: "/tmp/certs",
			Domains:  []string{"app.example"},
		},
		Dashboard: DashboardConfig{Host: "app.example"},
		Routes:    []Route{{Host: "app.example", Upstream: "http://upstream.example:8080"}},
	}
}
