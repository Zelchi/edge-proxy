package config

import "strings"

func (c *Config) Normalize() {
	for index := range c.TLS.Domains {
		c.TLS.Domains[index] = normalizeHostname(c.TLS.Domains[index])
	}
	for index := range c.Routes {
		c.Routes[index].Host = normalizeHostname(c.Routes[index].Host)
	}
	c.Fallback.Host = normalizeHostname(c.Fallback.Host)
	c.Dashboard.Host = normalizeHostname(c.Dashboard.Host)
}

func normalizeHostname(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	return strings.TrimRight(host, ".")
}
