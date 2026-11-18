package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"edge-proxy/internal/turnproxy"
)

func (c *Config) Validate() error {
	c.Normalize()
	var validationErrors []error

	validationErrors = append(validationErrors, validateAddress("http.address", c.HTTP.Address, true))
	validationErrors = append(validationErrors, validateAddress("https.address", c.HTTPS.Address, false))
	if c.RateLimit.RequestsPerSecond <= 0 {
		validationErrors = append(validationErrors, errors.New("rate_limit.requests_per_second must be greater than zero"))
	}
	if c.RateLimit.Burst < 1 {
		validationErrors = append(validationErrors, errors.New("rate_limit.burst must be at least one"))
	}

	domains := make(map[string]struct{}, len(c.HTTPS.TLS.Domains))
	for _, domain := range c.HTTPS.TLS.Domains {
		if err := validateHostname("https.tls.domains", domain); err != nil {
			validationErrors = append(validationErrors, err)
			continue
		}
		if _, exists := domains[domain]; exists {
			validationErrors = append(validationErrors, fmt.Errorf("https.tls.domains contains duplicate domain %q", domain))
		}
		domains[domain] = struct{}{}
	}

	if c.HTTPS.Address != "" {
		if c.HTTPS.TLS.CertsDir == "" {
			validationErrors = append(validationErrors, errors.New("https.tls.certs_dir is required when HTTPS is enabled"))
		}
		if len(domains) == 0 {
			validationErrors = append(validationErrors, errors.New("https.tls.domains requires at least one domain when HTTPS is enabled"))
		}
	}
	if err := validateHostname("http.dashboard", c.HTTP.Dashboard); err != nil {
		validationErrors = append(validationErrors, err)
	} else if _, exists := domains[c.HTTP.Dashboard]; !exists {
		validationErrors = append(validationErrors, fmt.Errorf("http.dashboard %q is not listed in https.tls.domains", c.HTTP.Dashboard))
	}

	routeHosts := make(map[string]struct{}, len(c.HTTP.Routes))
	for index, route := range c.HTTP.Routes {
		field := fmt.Sprintf("http.routes[%d]", index)
		if err := validateHostname(field+".host", route.Host); err != nil {
			validationErrors = append(validationErrors, err)
		} else {
			if _, exists := routeHosts[route.Host]; exists {
				validationErrors = append(validationErrors, fmt.Errorf("%s.host duplicates %q", field, route.Host))
			}
			routeHosts[route.Host] = struct{}{}
			if c.HTTPS.Address != "" {
				if _, exists := domains[route.Host]; !exists {
					validationErrors = append(validationErrors, fmt.Errorf("%s.host %q is not listed in https.tls.domains", field, route.Host))
				}
			}
		}
		if err := validateUpstream(field+".upstream", route.Upstream); err != nil {
			validationErrors = append(validationErrors, err)
		}
	}

	if c.HTTP.Fallback != "" {
		if err := validateHostname("http.fallback", c.HTTP.Fallback); err != nil {
			validationErrors = append(validationErrors, err)
		} else {
			_, hasRoute := routeHosts[c.HTTP.Fallback]
			_, hasCertificate := domains[c.HTTP.Fallback]
			if !hasRoute && !hasCertificate {
				validationErrors = append(validationErrors, fmt.Errorf("http.fallback %q must be a route host or be listed in https.tls.domains", c.HTTP.Fallback))
			}
		}
	}

	validationErrors = append(validationErrors, validateTCPConfig(c.TCP)...)
	validationErrors = append(validationErrors, validateUDPConfig(c.UDP)...)
	validationErrors = append(validationErrors, validateTURNConfig(c.TURN)...)
	validationErrors = append(validationErrors, validateAddressConflicts(c)...)

	return errors.Join(validationErrors...)
}

func validateAddressConflicts(c *Config) []error {
	var validationErrors []error
	if c.HTTPS.Address != "" && c.HTTP.Address == c.HTTPS.Address {
		validationErrors = append(validationErrors, errors.New("http.address conflicts with https.address"))
	}
	if c.TCP.Address != "" {
		if c.TCP.Address == c.HTTP.Address {
			validationErrors = append(validationErrors, errors.New("tcp.address conflicts with http.address"))
		}
		if c.HTTPS.Address != "" && c.TCP.Address == c.HTTPS.Address {
			validationErrors = append(validationErrors, errors.New("tcp.address conflicts with https.address"))
		}
	}
	if c.UDP.Address != "" {
		_, port, err := net.SplitHostPort(c.UDP.Address)
		if err == nil && port == "443" {
			validationErrors = append(validationErrors, errors.New("udp.address uses UDP port 443 reserved by HTTP/3"))
		}
	}
	if hasTURNConfig(c.TURN) {
		tcpPort, tcpErr := addressPort(c.TURN.TCPAddress)
		udpPort, udpErr := addressPort(c.TURN.UDPAddress)
		if tcpErr == nil {
			for field, address := range map[string]string{
				"http.address": c.HTTP.Address, "https.address": c.HTTPS.Address, "tcp.address": c.TCP.Address,
			} {
				if port, err := addressPort(address); err == nil && port == tcpPort {
					validationErrors = append(validationErrors, fmt.Errorf("turn.tcp_address conflicts with %s", field))
				}
			}
		}
		if udpErr == nil {
			if udpPort == 443 {
				validationErrors = append(validationErrors, errors.New("turn.udp_address uses UDP port 443 reserved by HTTP/3"))
			}
			if port, err := addressPort(c.UDP.Address); err == nil && port == udpPort {
				validationErrors = append(validationErrors, errors.New("turn.udp_address conflicts with udp.address"))
			}
		}
		for field, address := range map[string]string{"HTTP/3": serverHTTP3Address(), "udp.address": c.UDP.Address} {
			if port, err := addressPort(address); err == nil && port >= c.TURN.RelayMinPort && port <= c.TURN.RelayMaxPort {
				validationErrors = append(validationErrors, fmt.Errorf("turn relay range conflicts with %s", field))
			}
		}
		if udpErr == nil && udpPort >= c.TURN.RelayMinPort && udpPort <= c.TURN.RelayMaxPort {
			validationErrors = append(validationErrors, errors.New("turn.udp_address conflicts with the TURN relay range"))
		}
	}
	return validationErrors
}

func serverHTTP3Address() string {
	return ":443"
}

func addressPort(address string) (int, error) {
	if address == "" {
		return 0, errors.New("address is empty")
	}
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return 0, err
	}
	value, err := strconv.Atoi(port)
	if err != nil || value < 1 || value > 65535 {
		return 0, fmt.Errorf("invalid port %q", port)
	}
	return value, nil
}

func validateTCPConfig(config TCPConfig) []error {
	if !hasTCPConfig(config) {
		return nil
	}
	var validationErrors []error
	validationErrors = append(validationErrors, validateAddress("tcp.address", config.Address, true))
	if config.MaxConnections < 0 {
		validationErrors = append(validationErrors, errors.New("tcp.max_connections must not be negative"))
	}

	router := strings.ToLower(strings.TrimSpace(config.Router))
	if router == "" {
		router = "hostname"
	}
	switch router {
	case "static":
		if config.Upstream == "" {
			validationErrors = append(validationErrors, errors.New("tcp.upstream is required for a static router"))
		}
		if len(config.Routes) > 0 || config.DefaultUpstream != "" {
			validationErrors = append(validationErrors, errors.New("tcp.routes/default_upstream require router hostname"))
		}
	case "hostname":
		if config.Upstream != "" {
			validationErrors = append(validationErrors, errors.New("tcp.upstream must be empty for router hostname"))
		}
		if len(config.Routes) == 0 && config.DefaultUpstream == "" {
			validationErrors = append(validationErrors, errors.New("tcp.routes or tcp.default_upstream is required for router hostname"))
		}
	default:
		validationErrors = append(validationErrors, fmt.Errorf("tcp.router %q is not supported", config.Router))
	}
	if config.Upstream != "" {
		validationErrors = append(validationErrors, validateStreamUpstream("tcp.upstream", config.Upstream))
	}
	if config.DefaultUpstream != "" {
		validationErrors = append(validationErrors, validateStreamUpstream("tcp.default_upstream", config.DefaultUpstream))
	}
	validationErrors = append(validationErrors, validateTCPRoutes(config.Routes)...)
	return validationErrors
}

func validateTCPRoutes(routes []TCPRouteConfig) []error {
	var validationErrors []error
	routeHosts := make(map[string]struct{}, len(routes))
	for index, route := range routes {
		field := fmt.Sprintf("tcp.routes[%d]", index)
		if err := validateHostname(field+".host", route.Host); err != nil {
			validationErrors = append(validationErrors, err)
		} else if _, exists := routeHosts[route.Host]; exists {
			validationErrors = append(validationErrors, fmt.Errorf("%s.host duplicates %q", field, route.Host))
		} else {
			routeHosts[route.Host] = struct{}{}
		}
		validationErrors = append(validationErrors, validateStreamUpstream(field+".upstream", route.Upstream))
	}
	return validationErrors
}

func validateUDPConfig(config UDPConfig) []error {
	if !hasUDPConfig(config) {
		return nil
	}
	var validationErrors []error
	validationErrors = append(validationErrors, validateAddress("udp.address", config.Address, true))
	if config.MaxSessions < 0 {
		validationErrors = append(validationErrors, errors.New("udp.max_sessions must not be negative"))
	}

	router := strings.ToLower(strings.TrimSpace(config.Router))
	if router == "" {
		if len(config.Routes) > 0 || config.DefaultUpstream != "" {
			router = "hostname"
		} else {
			router = "static"
		}
	}
	switch router {
	case "static":
		if config.Upstream == "" {
			validationErrors = append(validationErrors, errors.New("udp.upstream is required for a static router"))
		}
		if len(config.Routes) > 0 || config.DefaultUpstream != "" {
			validationErrors = append(validationErrors, errors.New("udp.routes/default_upstream require router hostname"))
		}
	case "hostname":
		if config.Upstream != "" {
			validationErrors = append(validationErrors, errors.New("udp.upstream must be empty for router hostname"))
		}
		if len(config.Routes) == 0 && config.DefaultUpstream == "" {
			validationErrors = append(validationErrors, errors.New("udp.routes or udp.default_upstream is required for router hostname"))
		}
		if len(config.Routes) > 1 {
			validationErrors = append(validationErrors, errors.New("udp cannot distinguish multiple hostname routes"))
		}
	default:
		validationErrors = append(validationErrors, fmt.Errorf("udp.router %q is not supported", config.Router))
	}
	if config.Upstream != "" {
		validationErrors = append(validationErrors, validateStreamUpstream("udp.upstream", config.Upstream))
	}
	if config.DefaultUpstream != "" {
		validationErrors = append(validationErrors, validateStreamUpstream("udp.default_upstream", config.DefaultUpstream))
	}
	validationErrors = append(validationErrors, validateUDPRoutes(config.Routes)...)
	return validationErrors
}

func validateTURNConfig(config TURNProxyConfig) []error {
	if !hasTURNConfig(config) {
		return nil
	}
	var validationErrors []error
	publicIP := net.ParseIP(config.PublicIP)
	if publicIP == nil || !turnproxy.IsPublicIPv4(publicIP) {
		validationErrors = append(validationErrors, errors.New("turn.public_ip must be a public IPv4 address"))
	}
	validationErrors = append(validationErrors, validateAddress("turn.tcp_address", config.TCPAddress, true))
	validationErrors = append(validationErrors, validateAddress("turn.udp_address", config.UDPAddress, true))
	tcpPort, tcpErr := addressPort(config.TCPAddress)
	udpPort, udpErr := addressPort(config.UDPAddress)
	if tcpErr == nil && udpErr == nil && tcpPort != udpPort {
		validationErrors = append(validationErrors, errors.New("turn.tcp_address and turn.udp_address must use the same port"))
	}
	validationErrors = append(validationErrors, validateStreamUpstream("turn.control_upstream", config.ControlUpstream))
	if config.RelayMinPort < 1024 || config.RelayMinPort > 65535 ||
		config.RelayMaxPort < config.RelayMinPort || config.RelayMaxPort > 65535 {
		validationErrors = append(validationErrors, errors.New("turn relay port range must be valid"))
	}
	if config.RelayUpstreamHost == "" || strings.TrimSpace(config.RelayUpstreamHost) != config.RelayUpstreamHost ||
		strings.ContainsAny(config.RelayUpstreamHost, ":/ ") {
		validationErrors = append(validationErrors, errors.New("turn.relay_upstream_host must be a hostname or IP"))
	}
	count := config.RelayMaxPort - config.RelayMinPort + 1
	for field, base := range map[string]int{
		"turn.relay_upstream_base_port": config.RelayUpstreamBasePort,
		"turn.relay_command_base_port":  config.RelayCommandBasePort,
	} {
		if base < 1 || base+count-1 > 65535 {
			validationErrors = append(validationErrors, fmt.Errorf("%s range must fit in UDP ports 1-65535", field))
		}
	}
	if config.RelayUpstreamBasePort > 0 && config.RelayCommandBasePort > 0 {
		upstreamEnd := config.RelayUpstreamBasePort + count - 1
		commandEnd := config.RelayCommandBasePort + count - 1
		if config.RelayUpstreamBasePort <= commandEnd && config.RelayCommandBasePort <= upstreamEnd {
			validationErrors = append(validationErrors, errors.New("TURN relay upstream and command port ranges overlap"))
		}
	}
	if config.MaxSessions < 0 {
		validationErrors = append(validationErrors, errors.New("turn.max_sessions must not be negative"))
	}
	return validationErrors
}

func validateUDPRoutes(routes []UDPRouteConfig) []error {
	var validationErrors []error
	routeHosts := make(map[string]struct{}, len(routes))
	for index, route := range routes {
		field := fmt.Sprintf("udp.routes[%d]", index)
		if err := validateHostname(field+".host", route.Host); err != nil {
			validationErrors = append(validationErrors, err)
		} else if _, exists := routeHosts[route.Host]; exists {
			validationErrors = append(validationErrors, fmt.Errorf("%s.host duplicates %q", field, route.Host))
		} else {
			routeHosts[route.Host] = struct{}{}
		}
		validationErrors = append(validationErrors, validateStreamUpstream(field+".upstream", route.Upstream))
	}
	return validationErrors
}

func hasTCPConfig(config TCPConfig) bool {
	return config.Address != "" || config.Router != "" || config.Upstream != "" || config.DefaultUpstream != "" || config.MaxConnections != 0 || len(config.Routes) > 0
}

func hasUDPConfig(config UDPConfig) bool {
	return config.Address != "" || config.Router != "" || config.Upstream != "" || config.DefaultUpstream != "" || config.MaxSessions != 0 || len(config.Routes) > 0
}

func hasTURNConfig(config TURNProxyConfig) bool {
	return config.PublicIP != "" || config.TCPAddress != "" || config.UDPAddress != "" ||
		config.ControlUpstream != "" || config.RelayMinPort != 0 || config.RelayMaxPort != 0 ||
		config.RelayUpstreamHost != "" || config.RelayUpstreamBasePort != 0 ||
		config.RelayCommandBasePort != 0 || config.MaxSessions != 0
}

func validateStreamUpstream(field, upstream string) error {
	host, port, err := net.SplitHostPort(upstream)
	if err != nil || host == "" || strings.TrimSpace(host) != host || strings.ContainsAny(host, "/ ") {
		return fmt.Errorf("%s must be a host:port upstream", field)
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return fmt.Errorf("%s has invalid port %q", field, port)
	}
	return nil
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
