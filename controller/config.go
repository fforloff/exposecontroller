package controller

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/golang/glog"
	"github.com/pkg/errors"
	yaml "gopkg.in/yaml.v2"
)

// LoadFile loads the config from yaml file
func LoadFile(path string) (*Config, bool, error) {
	content, err := ioutil.ReadFile(path)

	exists := true
	if err != nil {
		exists = false
		if !os.IsNotExist(err) {
			return nil, exists, errors.Wrap(err, "failed to read config file")
		}
		glog.Infof("No %s file found.  Will try to figure out defaults", path)
	}

	c, err := Load(string(content))
	if err != nil {
		return nil, false, errors.Wrap(err, "failed to read config file")
	}
	return c, exists, nil
}

// Load loads the config from yaml string
func Load(s string) (*Config, error) {
	cfg := &Config{}
	// If the entire config body is empty the UnmarshalYAML method is
	// never called. We thus have to set the DefaultConfig at the entry
	// point as well.
	*cfg = DefaultConfig
	err := yaml.Unmarshal([]byte(s), &cfg)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal config")
	}

	cfg.original = s

	return cfg, nil
}

// Config is the global config of the program
type Config struct {
	Domain                string   `yaml:"domain,omitempty" json:"domain"`
	InternalDomain        string   `yaml:"internal-domain,omitempty" json:"internal_domain"`
	Exposer               string   `yaml:"exposer" json:"exposer"`
	PathMode              string   `yaml:"path-mode" json:"path_mode"`
	NodeIP                string   `yaml:"node-ip,omitempty" json:"node_ip"`
	AuthorizePath         string   `yaml:"authorize-path,omitempty" json:"authorize_path"`
	WatchNamespaces       string   `yaml:"watch-namespaces" json:"watch_namespaces"`
	WatchCurrentNamespace bool     `yaml:"watch-current-namespace" json:"watch_current_namespace"`
	HTTP                  bool     `yaml:"http" json:"http"`
	TLSAcme               bool     `yaml:"tls-acme" json:"tls_acme"`
	TLSSecretName         string   `yaml:"tls-secret-name" json:"tls_secret_name"`
	TLSUseWildcard        bool     `yaml:"tls-use-wildcard" json:"tls_use_wildcard"`
	URLTemplate           string   `yaml:"urltemplate,omitempty" json:"url_template"`
	Services              []string `yaml:"services,omitempty" json:"services"`
	IngressClass          string   `yaml:"ingress-class" json:"ingress_class"`
	NamePrefix            string   `yaml:"name-prefix,omitempty" json:"name_prefix"`
	// original is the input from which the config was parsed.
	original string `json:"original"`
}

// DefaultConfig is the default values of Config
var (
	DefaultConfig = Config{}
)

// MapToConfig converts the ConfigMap data to a Config object
func MapToConfig(data map[string]string) (*Config, error) {
	answer := &Config{}

	b, err := yaml.Marshal(data)
	if err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(b, answer)
	return answer, err
}

func (c Config) String() string {
	if c.original != "" {
		return c.original
	}

	b, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Sprintf("<error creating config string: %s>", err)
	}
	return string(b)
}
