package config

type Config struct {
	HTTP      HTTPConfig      `yaml:"http"`
	HTTPS     HTTPSConfig     `yaml:"https"`
	TLS       TLSConfig       `yaml:"tls"`
	Dashboard DashboardConfig `yaml:"dashboard"`
	Routes    []Route         `yaml:"routes"`
	Fallback  FallbackConfig  `yaml:"fallback"`
}

type HTTPConfig struct {
	Address         string `yaml:"address"`
	RedirectToHTTPS bool   `yaml:"redirect_to_https"`
}

type HTTPSConfig struct {
	Address string `yaml:"address"`
}

type TLSConfig struct {
	CertsDir string   `yaml:"certs_dir"`
	Domains  []string `yaml:"domains"`
}

type DashboardConfig struct {
	Host string `yaml:"host"`
}

type Route struct {
	Host         string `yaml:"host"`
	Upstream     string `yaml:"upstream"`
	PreserveHost bool   `yaml:"preserve_host"`
}

type FallbackConfig struct {
	Host       string `yaml:"host"`
	StatusCode int    `yaml:"status_code"`
}
