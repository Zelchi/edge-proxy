package config

type Config struct {
	RateLimit RateLimitConfig `yaml:"rate_limit"`
	HTTP      HTTPConfig      `yaml:"http"`
	HTTPS     HTTPSConfig     `yaml:"https"`
	TCP       TCPConfig       `yaml:"tcp"`
	UDP       UDPConfig       `yaml:"udp"`
}

type HTTPConfig struct {
	Address         string  `yaml:"address"`
	RedirectToHTTPS bool    `yaml:"redirect_to_https"`
	Dashboard       string  `yaml:"dashboard"`
	Routes          []Route `yaml:"routes"`
	Fallback        string  `yaml:"fallback"`
}

type HTTPSConfig struct {
	Address string    `yaml:"address"`
	TLS     TLSConfig `yaml:"tls"`
}

type TLSConfig struct {
	CertsDir      string   `yaml:"certs_dir"`
	CertsFallback string   `yaml:"certs_fallback"`
	Domains       []string `yaml:"domains"`
}

type RateLimitConfig struct {
	RequestsPerSecond float64 `yaml:"requests_per_second"`
	Burst             int     `yaml:"burst"`
}

type Route struct {
	Host         string `yaml:"host"`
	Upstream     string `yaml:"upstream"`
	PreserveHost bool   `yaml:"preserve_host"`
}

type TCPConfig struct {
	Address         string           `yaml:"address"`
	Router          string           `yaml:"router"`
	Upstream        string           `yaml:"upstream"`
	DefaultUpstream string           `yaml:"default_upstream"`
	MaxConnections  int              `yaml:"max_connections"`
	Routes          []TCPRouteConfig `yaml:"routes"`
}

type TCPRouteConfig struct {
	Host     string `yaml:"host"`
	Upstream string `yaml:"upstream"`
}

type UDPConfig struct {
	Address         string           `yaml:"address"`
	Router          string           `yaml:"router"`
	Upstream        string           `yaml:"upstream"`
	DefaultUpstream string           `yaml:"default_upstream"`
	MaxSessions     int              `yaml:"max_sessions"`
	Routes          []UDPRouteConfig `yaml:"routes"`
}

type UDPRouteConfig struct {
	Host     string `yaml:"host"`
	Upstream string `yaml:"upstream"`
}
