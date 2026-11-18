package config

import (
	"errors"
	"fmt"
	"reflect"
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
	if current.HTTP.Dashboard != next.HTTP.Dashboard {
		return fmt.Errorf("http.dashboard changed from %q to %q", current.HTTP.Dashboard, next.HTTP.Dashboard)
	}
	if current.RateLimit != next.RateLimit {
		return errors.New("rate_limit configuration changed")
	}
	if current.HTTPS.TLS.CertsDir != next.HTTPS.TLS.CertsDir {
		return fmt.Errorf("https.tls.certs_dir changed from %q to %q", current.HTTPS.TLS.CertsDir, next.HTTPS.TLS.CertsDir)
	}
	if current.HTTPS.TLS.CertsFallback != next.HTTPS.TLS.CertsFallback {
		return errors.New("https.tls.certs_fallback changed")
	}
	if !sameStrings(current.HTTPS.TLS.Domains, next.HTTPS.TLS.Domains) {
		return errors.New("https.tls.domains changed")
	}
	if !reflect.DeepEqual(current.TCP, next.TCP) {
		return errors.New("tcp configuration changed; restart is required")
	}
	if !reflect.DeepEqual(current.UDP, next.UDP) {
		return errors.New("udp configuration changed; restart is required")
	}
	if !reflect.DeepEqual(current.TURN, next.TURN) {
		return errors.New("turn configuration changed; restart is required")
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
