package config

import (
	"errors"
	"fmt"
)

func ValidateReload(current, next *Config) error {
	if current == nil || next == nil {
		return errors.New("current and next configurations are required")
	}
	if current.HTTP.Address != next.HTTP.Address {
		return fmt.Errorf("http.address changed from %q to %q", current.HTTP.Address, next.HTTP.Address)
	}
	if current.HTTPS.Address != next.HTTPS.Address {
		return fmt.Errorf("https.address changed from %q to %q", current.HTTPS.Address, next.HTTPS.Address)
	}
	if current.RateLimit != next.RateLimit {
		return errors.New("rate_limit configuration changed")
	}
	if current.TLS.CertsDir != next.TLS.CertsDir {
		return fmt.Errorf("tls.certs_dir changed from %q to %q", current.TLS.CertsDir, next.TLS.CertsDir)
	}
	if current.TLS.CertsFallback != next.TLS.CertsFallback {
		return errors.New("tls.certs_fallback changed")
	}
	if !sameStrings(current.TLS.Domains, next.TLS.Domains) {
		return errors.New("tls.domains changed")
	}
	return nil
}

func sameStrings(first, second []string) bool {
	if len(first) != len(second) {
		return false
	}
	values := make(map[string]int, len(first))
	for _, value := range first {
		values[value]++
	}
	for _, value := range second {
		if values[value] == 0 {
			return false
		}
		values[value]--
	}
	return true
}
