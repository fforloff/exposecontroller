package exposestrategy

import (
	"strings"

	"github.com/pkg/errors"

	"k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

// ExposeStrategy represents a strategy
type ExposeStrategy interface {
	Sync() error
	HasSynced() bool
	Add(svc *v1.Service) error
	Clean(svc *v1.Service) error
	Delete(svc *v1.Service) error
}

// Config is the common config to all strategies
type Config struct {
	Exposer        string
	Namespace      string
	NamePrefix     string
	Domain         string
	InternalDomain string
	NodeIP         string
	TLSSecretName  string
	TLSUseWildcard bool
	HTTP           bool
	TLSAcme        bool
	URLTemplate    string
	PathMode       string
	IngressClass   string
}

type label struct {
	Key   string
	Value string
}

var (
	// ExposeLabel label tells that the service is exposed
	ExposeLabel                   = label{Key: "expose", Value: "true"}
	// ExposeAnnotation anotations tells that the service is exposed
	ExposeAnnotation              = label{Key: "fabric8.io/expose", Value: "true"}
	// InjectAnnotation anotations tells that the service is exposed
	InjectAnnotation              = label{Key: "fabric8.io/inject", Value: "true"}
	// ExposeHostNameAsAnnotationKey annotation sets the hostname to use
	ExposeHostNameAsAnnotationKey = "fabric8.io/exposeHostNameAs"
	// ExposeAnnotationKey annotation will be created with the exposed url
	ExposeAnnotationKey           = "fabric8.io/exposeURL"
	// ExposePortAnnotationKey annotation sets the service port to export
	ExposePortAnnotationKey       = "fabric8.io/exposePort"
	// APIServicePathAnnotationKey annotation sets the path to export
	APIServicePathAnnotationKey   = "api.service.kubernetes.io/path"
)

type exposeStrategyFunc = func(client kubernetes.Interface, config *Config) (ExposeStrategy, error)
var exposeStrategyFuncs map[string]exposeStrategyFunc = map[string]exposeStrategyFunc{
	"ambassador":   NewAmbassadorStrategy,
	"ingress":      NewIngressStrategy,
	"loadbalancer": NewLoadBalancerStrategy,
	"nodeport":     NewNodePortStrategy,
}

// New creates a new strategy
func New(client kubernetes.Interface, config *Config) (ExposeStrategy, error) {
	exposer := strings.ToLower(config.Exposer)
	if exposer == "" || exposer == "auto" {
		return NewAutoStrategy(client, config)
	}

	f, ok := exposeStrategyFuncs[exposer]
	if ok {
		strategy, err := f(client, config)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create %s expose strategy", exposer)
		}
		return strategy, nil
	}
	strategies := make([]string, 1, 1+len(exposeStrategyFuncs))
	strategies[0] = "auto"
	for s := range exposeStrategyFuncs {
		strategies = append(strategies, s)
	}
	return nil, errors.Errorf("unknown expose strategy \"%s\", must be one of \"%s\"", exposer, strings.Join(strategies, "\", \""))
}
