package exposestrategy

import (
	"strings"

	"github.com/golang/glog"
	"github.com/pkg/errors"

	"k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

type ExposeStrategy interface {
	Add(svc *v1.Service) error
	Remove(svc *v1.Service) error
}

type Label struct {
	Key   string
	Value string
}

var (
	ExposeLabel                   = Label{Key: "expose", Value: "true"}
	ExposeAnnotation              = Label{Key: "fabric8.io/expose", Value: "true"}
	InjectAnnotation              = Label{Key: "fabric8.io/inject", Value: "true"}
	ExposeHostNameAsAnnotationKey = "fabric8.io/exposeHostNameAs"
	ExposeAnnotationKey           = "fabric8.io/exposeUrl"
	ExposePortAnnotationKey       = "fabric8.io/exposePort"
	ApiServicePathAnnotationKey   = "api.service.kubernetes.io/path"
)

func New(exposer, domain, internalDomain, urltemplate, nodeIP, pathMode string, http, tlsAcme bool, tlsSecretName string, tlsUseWildcard bool, ingressClass string, client kubernetes.Interface, namespace string) (ExposeStrategy, error) {
	switch strings.ToLower(exposer) {
	case "ambassador":
		strategy, err := NewAmbassadorStrategy(client, domain, http, tlsAcme, tlsSecretName, urltemplate, pathMode)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create ambassador expose strategy")
		}
		return strategy, nil
	case "loadbalancer":
		strategy, err := NewLoadBalancerStrategy(client)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create load balancer expose strategy")
		}
		return strategy, nil
	case "nodeport":
		strategy, err := NewNodePortStrategy(client, nodeIP)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create node port expose strategy")
		}
		return strategy, nil
	case "ingress":
		glog.Infof("stratagy.New %v", http)
		strategy, err := NewIngressStrategy(client, namespace, domain, internalDomain, http, tlsAcme, tlsSecretName, tlsUseWildcard, urltemplate, pathMode, ingressClass)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create ingress expose strategy")
		}
		return strategy, nil
	case "":
		strategy, err := NewAutoStrategy(exposer, domain, internalDomain, urltemplate, nodeIP, pathMode, http, tlsAcme, tlsSecretName, tlsUseWildcard, ingressClass, client, namespace)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create auto expose strategy")
		}
		return strategy, nil
	default:
		return nil, errors.Errorf("unknown expose strategy '%s', must be one of %v", exposer, []string{"Auto", "Ingress", "Route", "NodePort", "LoadBalancer"})
	}
}
