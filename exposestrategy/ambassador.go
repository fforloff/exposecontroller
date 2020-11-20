package exposestrategy

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	yaml "gopkg.in/yaml.v2"
	"k8s.io/klog"

	"k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

// const (
// 	PathModeUsePath = "path"
// )

// AmbassadorStrategy is a strategy that adds the ambassador annotations
type AmbassadorStrategy struct {
	client  kubernetes.Interface

	domain        string
	tlsSecretName string
	http          bool
	tlsAcme       bool
	urltemplate   string
	pathMode      string
}

// NewAmbassadorStrategy creates a new AmbassadorStrategy
func NewAmbassadorStrategy(client kubernetes.Interface, config *Config) (ExposeStrategy, error) {

	var err error
	if config.Domain == "" {
		config.Domain, err = getAutoDefaultDomain(client)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get a domain")
		}
	}
	klog.Infof("Using domain: %s", config.Domain)

	var urlformat string
	urlformat, err = getURLFormat(config.URLTemplate)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get a url format")
	}
	klog.Infof("Using url template [%s] format [%s]", config.URLTemplate, urlformat)

	return &AmbassadorStrategy{
		client:        client,
		domain:        config.Domain,
		http:          config.HTTP,
		tlsAcme:       config.TLSAcme,
		tlsSecretName: config.TLSSecretName,
		urltemplate:   urlformat,
		pathMode:      config.PathMode,
	}, nil
}

// Sync is called before starting / resyncing
// Nothing to do
func (s *AmbassadorStrategy) Sync() error {
	return nil
}

// HasSynced tells if the strategy is complete
// Nothing to do
func (s *AmbassadorStrategy) HasSynced() bool {
	return true
}

// Add is called when an exposed service is created or updated
// Sets the ambassador annotations and various annotations
func (s *AmbassadorStrategy) Add(svc *v1.Service) error {
	appName := svc.Annotations["fabric8.io/ingress.name"]
	if appName == "" {
		if svc.Labels["release"] != "" {
			appName = strings.Replace(svc.Name, svc.Labels["release"]+"-", "", 1)
		} else {
			appName = svc.Name
		}
	}

	hostName := svc.Annotations["fabric8.io/host.name"]
	if hostName == "" {
		hostName = appName
	}

	hostName = fmt.Sprintf(s.urltemplate, hostName, svc.Namespace, s.domain)
	path := svc.Annotations["fabric8.io/ingress.path"]
	pathMode := svc.Annotations["fabric8.io/path.mode"]
	if pathMode == "" {
		pathMode = s.pathMode
	}
	if pathMode == PathModeUsePath {
		if path == "" {
			path = "/"
		}
		path = URLJoin("/", svc.Namespace, appName, path)
		hostName = s.domain
	} else if path == "" || path[0] != '/' {
		path = "/" + path
	}

	exposePort := svc.Annotations[ExposePortAnnotationKey]
	if exposePort != "" {
		port, err := strconv.Atoi(exposePort)
		if err == nil {
			found := false
			for _, p := range svc.Spec.Ports {
				if port == int(p.Port) {
					found = true
					break
				}
			}
			if !found {
				klog.Warningf("Port '%s' provided in the annotation '%s' is not available in the ports of service '%s'",
					exposePort, ExposePortAnnotationKey, svc.GetName())
				exposePort = ""
			}
		} else {
			klog.Warningf("Port '%s' provided in the annotation '%s' is not a valid number",
				exposePort, ExposePortAnnotationKey)
			exposePort = ""
		}
	}
	// Pick the fist port available in the service if no expose port was configured
	if exposePort == "" {
		port := svc.Spec.Ports[0]
		exposePort = strconv.Itoa(int(port.Port))
	}

	servicePort, err := strconv.Atoi(exposePort)
	if err != nil {
		return errors.Wrapf(err, "failed to convert the exposed port '%s' to int", exposePort)
	}

	tlsSecretName := s.tlsSecretName
	if s.tlsAcme && tlsSecretName == "" {
		tlsSecretName = "tls-" + appName
	}
	if svc.Annotations["jenkins-x.io/skip.tls"] == "true" {
		tlsSecretName = ""
	}

	klog.Infof("Exposing Port %d of Service %s", servicePort, svc.Name)

	clone := svc.DeepCopy()
	if !s.http && tlsSecretName != "" {
		err = addServiceAnnotationWithProtocol(clone, hostName, path, "https")
	} else {
		err = addServiceAnnotationWithProtocol(clone, hostName, path, "http")
	}
	if err != nil {
		return errors.Wrapf(err, "failed to add annotation to service %s/%s",
			svc.Namespace, svc.Name)
	}
	// Here's where we start adding the annotations to our service
	ambassadorAnnotations := map[string]interface{}{
		"apiVersion": "ambassador/v1",
		"kind":       "Mapping",
		"host":       hostName,
		"name":       fmt.Sprintf("%s_%s_mapping", hostName, svc.Namespace),
		"service":    fmt.Sprintf("%s.%s:%s", svc.Name, svc.Namespace, strconv.Itoa(servicePort)),
		"prefix":     path,
	}

	joinedAnnotations := new(bytes.Buffer)
	fmt.Fprintf(joinedAnnotations, "---\n")
	yamlAnnotation, err := yaml.Marshal(&ambassadorAnnotations)
	if err != nil {
		return err
	}
	fmt.Fprintf(joinedAnnotations, "%s", string(yamlAnnotation))

	if tlsSecretName != "" {
		// we need to prepare the tls module config
		ambassadorAnnotations = map[string]interface{}{
			"apiVersion": "ambassador/v1",
			"kind":       "Module",
			"name":       "tls",
			"config": map[string]interface{}{
				"server": map[string]interface{}{
					"enabled": "True",
					"secret":  tlsSecretName}}}

		yamlAnnotation, err = yaml.Marshal(&ambassadorAnnotations)
		if err != nil {
			return err
		}

		fmt.Fprintf(joinedAnnotations, "---\n")
		fmt.Fprintf(joinedAnnotations, "%s", string(yamlAnnotation))
	}
	clone.Annotations["getambassador.io/config"] = joinedAnnotations.String()

	patch, err := createServicePatch(svc, clone)
	if err != nil {
		return errors.Wrapf(err, "failed to create patch for service %s/%s",
			svc.Namespace, svc.Name)
	}
	// patch the service
	if patch != nil {
		_, err = s.client.CoreV1().Services(svc.Namespace).
			Patch(svc.Name, patchType, patch)
		if err != nil {
			return errors.Wrapf(err, "failed to send patch %s/%s",
				svc.Namespace, svc.Name)
		}
	}

	return nil
}

// Clean is called when an exposed service is unexposed
// Cleans the ambassador annotations and and various annotations
func (s *AmbassadorStrategy) Clean(svc *v1.Service) error {
	clone := svc.DeepCopy()
	if !removeServiceAnnotation(clone) {
		return nil
	}
	delete(svc.Annotations, "getambassador.io/config")

	patch, err := createServicePatch(svc, clone)
	if err != nil {
		return errors.Wrapf(err, "failed to create patch for service %s/%s",
			svc.Namespace, svc.Name)
	}
	// patch the service
	if patch != nil {
		_, err = s.client.CoreV1().Services(svc.Namespace).
			Patch(svc.Name, patchType, patch)
		if err != nil {
			return errors.Wrapf(err, "failed to send patch %s/%s",
				svc.Namespace, svc.Name)
		}
	}
	return nil
}

// Delete is called when an exposed service is deleted
// Nothing to do
func (s *AmbassadorStrategy) Delete(svc *v1.Service) error {
	return nil
}
