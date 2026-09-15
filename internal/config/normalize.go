package config

import "strings"

func (c *Config) Normalize() {
	for index := range c.HTTP.Routes {
		c.HTTP.Routes[index].Host = normalizeHostname(c.HTTP.Routes[index].Host)
	}
	for index := range c.TCP.Routes {
		c.TCP.Routes[index].Host = normalizeHostname(c.TCP.Routes[index].Host)
	}
	for index := range c.UDP.Routes {
		c.UDP.Routes[index].Host = normalizeHostname(c.UDP.Routes[index].Host)
	}
	for index := range c.HTTPS.TLS.Domains {
		c.HTTPS.TLS.Domains[index] = normalizeHostname(c.HTTPS.TLS.Domains[index])
	}
	c.HTTP.Dashboard = normalizeHostname(c.HTTP.Dashboard)
	c.HTTP.Fallback = normalizeHostname(c.HTTP.Fallback)
}

func normalizeHostname(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	return strings.TrimRight(host, ".")
}
