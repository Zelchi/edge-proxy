package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

func (c *Config) Validate() error {
	var validationErrors []error

	validationErrors = append(validationErrors, validateAddress("http.address", c.HTTP.Address, true))
	validationErrors = append(validationErrors, validateAddress("https.address", c.HTTPS.Address, false))
	if c.RateLimit.RequestsPerSecond <= 0 {
		validationErrors = append(validationErrors, errors.New("rate_limit.requests_per_second must be greater than zero"))
	}
	if c.RateLimit.Burst < 1 {
		validationErrors = append(validationErrors, errors.New("rate_limit.burst must be at least one"))
	}

	domains := make(map[string]struct{}, len(c.TLS.Domains))
	for _, domain := range c.TLS.Domains {
		if err := validateHostname("tls.domains", domain); err != nil {
			validationErrors = append(validationErrors, err)
			continue
		}
		if _, exists := domains[domain]; exists {
			validationErrors = append(validationErrors, fmt.Errorf("tls.domains contains duplicate domain %q", domain))
		}
		domains[domain] = struct{}{}
	}

	if c.HTTPS.Address != "" {
		if c.TLS.CertsDir == "" {
			validationErrors = append(validationErrors, errors.New("tls.certs_dir is required when HTTPS is enabled"))
		}
		if len(domains) == 0 {
			validationErrors = append(validationErrors, errors.New("tls.domains requires at least one domain when HTTPS is enabled"))
		}
	}
	if err := validateHostname("dashboard.host", c.Dashboard.Host); err != nil {
		validationErrors = append(validationErrors, err)
	} else if _, exists := domains[c.Dashboard.Host]; !exists {
		validationErrors = append(validationErrors, fmt.Errorf("dashboard.host %q is not listed in tls.domains", c.Dashboard.Host))
	}

	routeHosts := make(map[string]struct{}, len(c.Routes))
	for index, route := range c.Routes {
		field := fmt.Sprintf("routes[%d]", index)
		if err := validateHostname(field+".host", route.Host); err != nil {
			validationErrors = append(validationErrors, err)
		} else {
			if _, exists := routeHosts[route.Host]; exists {
				validationErrors = append(validationErrors, fmt.Errorf("%s.host duplicates %q", field, route.Host))
			}
			routeHosts[route.Host] = struct{}{}
			if c.HTTPS.Address != "" {
				if _, exists := domains[route.Host]; !exists {
					validationErrors = append(validationErrors, fmt.Errorf("%s.host %q is not listed in tls.domains", field, route.Host))
				}
			}
		}
		if err := validateUpstream(field+".upstream", route.Upstream); err != nil {
			validationErrors = append(validationErrors, err)
		}
	}

	if c.Fallback.Host == "" {
		if c.Fallback.StatusCode != 0 {
			validationErrors = append(validationErrors, errors.New("fallback.status_code requires fallback.host"))
		}
	} else {
		if err := validateHostname("fallback.host", c.Fallback.Host); err != nil {
			validationErrors = append(validationErrors, err)
		} else {
			_, hasRoute := routeHosts[c.Fallback.Host]
			_, hasCertificate := domains[c.Fallback.Host]
			if !hasRoute && !hasCertificate {
				validationErrors = append(validationErrors, fmt.Errorf("fallback.host %q must be a route host or be listed in tls.domains", c.Fallback.Host))
			}
		}
		if c.Fallback.StatusCode != 0 && !isRedirectStatus(c.Fallback.StatusCode) {
			validationErrors = append(validationErrors, fmt.Errorf("fallback.status_code %d is not a supported redirect status", c.Fallback.StatusCode))
		}
	}

	return errors.Join(validationErrors...)
}

func validateAddress(field, address string, required bool) error {
	if address == "" {
		if required {
			return fmt.Errorf("%s is required", field)
		}
		return nil
	}

	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("%s must be a host:port address: %w", field, err)
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return fmt.Errorf("%s has invalid port %q", field, port)
	}
	return nil
}

func validateHostname(field, host string) error {
	if host == "" {
		return fmt.Errorf("%s is required", field)
	}
	if host != strings.ToLower(host) || strings.HasSuffix(host, ".") {
		return fmt.Errorf("%s must be lowercase and must not have a trailing dot", field)
	}
	if len(host) > 253 || strings.ContainsAny(host, ":/ ") {
		return fmt.Errorf("%s %q is not a valid hostname", field, host)
	}

	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return fmt.Errorf("%s %q is not a valid hostname", field, host)
		}
		for _, character := range label {
			if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '-' {
				return fmt.Errorf("%s %q is not a valid hostname", field, host)
			}
		}
	}
	return nil
}

func validateUpstream(field, rawURL string) error {
	upstream, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("%s is invalid: %w", field, err)
	}
	if upstream.Scheme != "http" && upstream.Scheme != "https" {
		return fmt.Errorf("%s must use http or https", field)
	}
	if upstream.Host == "" {
		return fmt.Errorf("%s requires a host", field)
	}
	if upstream.User != nil || upstream.Fragment != "" || upstream.RawQuery != "" {
		return fmt.Errorf("%s must not contain credentials, a query, or a fragment", field)
	}
	return nil
}

func isRedirectStatus(status int) bool {
	switch status {
	case 301, 302, 303, 307, 308:
		return true
	default:
		return false
	}
}
