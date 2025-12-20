package config

type Config struct {
	HTTP   HTTPConfig  `yaml:"http"`
	HTTPS  HTTPSConfig `yaml:"https"`
	TLS    TLSConfig   `yaml:"tls"`
	Routes []Route     `yaml:"routes"`
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

type Certificate struct {
	Domain string `yaml:"domain"`
	Cert   string `yaml:"cert"`
	Key    string `yaml:"key"`
}

type Route struct {
	Host     string `yaml:"host"`
	Upstream string `yaml:"upstream"`
}
