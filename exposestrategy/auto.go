package exposestrategy

import (
	"strings"

	"github.com/pkg/errors"
	"k8s.io/klog"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	ingress            = "ingress"
	loadBalancer       = "loadbalancer"
	nodePort           = "nodeport"
	domainExt          = ".nip.io"
	stackpointNS       = "stackpoint-system"
	stackpointHAProxy  = "spc-balancer"
	stackpointIPEnvVar = "BALANCER_IP"
)

// NewAutoStrategy creates a new strategy, choose automatically
func NewAutoStrategy(client kubernetes.Interface, config *Config) (ExposeStrategy, error) {
	var err error
	config.Exposer, err = getAutoDefaultExposeRule(client)
	if err != nil {
		return nil, errors.Wrap(err, "failed to automatically get exposer rule.  consider setting 'exposer' type in config.yml")
	}
	klog.Infof("Using exposer strategy: %s", config.Exposer)

	// only try to get domain if we need wildcard dns and one wasn't given to us
	if config.Domain == "" && (strings.EqualFold(ingress, config.Exposer)) {
		config.Domain, err = getAutoDefaultDomain(client)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get a domain")
		}
		klog.Infof("Using domain: %s", config.Domain)
	}

	return New(client, config)
}

func getAutoDefaultExposeRule(c kubernetes.Interface) (string, error) {
	// lets default to Ingress on kubernetes for now
	/*
		nodes, err := c.CoreV1().Nodes().List(metav1.ListOptions{})
		if err != nil {
			return "", errors.Wrap(err, "failed to find any nodes")
		}
		if len(nodes.Items) == 1 {
			node := nodes.Items[0]
			if node.Name == "minishift" || node.Name == "minikube" {
				return nodePort, nil
			}
		}
	*/
	return ingress, nil
}

func getAutoDefaultDomain(c kubernetes.Interface) (string, error) {
	nodes, err := c.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		return "", errors.Wrap(err, "failed to find any nodes")
	}

	// if we're mini* then there's only one node, any router / ingress controller deployed has to be on this one
	if len(nodes.Items) == 1 {
		node := nodes.Items[0]
		if node.Name == "minishift" || node.Name == "minikube" {
			ip, err := getExternalIP(node)
			if err != nil {
				return "", err
			}
			return ip + domainExt, nil
		}
	}

	// check for a gofabric8 ingress labelled node
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchLabels: map[string]string{"fabric8.io/externalIP": "true"}})
	nodes, err = c.CoreV1().Nodes().List(metav1.ListOptions{LabelSelector: selector.String()})
	if len(nodes.Items) == 1 {
		node := nodes.Items[0]
		ip, err := getExternalIP(node)
		if err != nil {
			return "", err
		}
		return ip + domainExt, nil
	}

	// look for a stackpoint HA proxy
	pod, _ := c.CoreV1().Pods(stackpointNS).Get(stackpointHAProxy, metav1.GetOptions{})
	if pod != nil {
		containers := pod.Spec.Containers
		for _, container := range containers {
			if container.Name == stackpointHAProxy {
				for _, e := range container.Env {
					if e.Name == stackpointIPEnvVar {
						return e.Value + domainExt, nil
					}
				}
			}
		}
	}
	return "", errors.New("no known automatic ways to get an external ip to use with nip.  Please configure exposecontroller configmap manually see https://github.com/olli-ai/exposecontroller#configuration")
}

// copied from k8s.io/kubernetes/pkg/master/master.go
func getExternalIP(node v1.Node) (string, error) {
	var fallback string
	ann := node.Annotations
	if ann != nil {
		for k, v := range ann {
			if len(v) > 0 && strings.HasSuffix(k, "kubernetes.io/provided-node-ip") {
				return v, nil
			}
		}
	}
	for ix := range node.Status.Addresses {
		addr := &node.Status.Addresses[ix]
		if addr.Type == v1.NodeExternalIP {
			return addr.Address, nil
		}
		if fallback == "" && addr.Type == v1.NodeInternalIP {
			fallback = addr.Address
		}
	}
	if fallback != "" {
		return fallback, nil
	}
	return "", errors.New("no node ExternalIP found")
}
