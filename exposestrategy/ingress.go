package exposestrategy

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/golang/glog"
	"github.com/pkg/errors"

	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
)

const (
	PathModeUsePath = "path"
)

type IngressStrategy struct {
	client  kubernetes.Interface

	domain         string
	internalDomain string
	tlsSecretName  string
	tlsUseWildcard bool
	http           bool
	tlsAcme        bool
	urltemplate    string
	pathMode       string
	ingressClass   string
}

var _ ExposeStrategy = &IngressStrategy{}

func NewIngressStrategy(client kubernetes.Interface, domain string, internalDomain string, http, tlsAcme bool, tlsSecretName string, tlsUseWildcard bool, urltemplate, pathMode string, ingressClass string) (*IngressStrategy, error) {
	glog.Infof("NewIngressStrategy 1 %v", http)

	var err error
	if len(domain) == 0 {
		domain, err = getAutoDefaultDomain(client)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get a domain")
		}
	}
	glog.Infof("Using domain: %s", domain)

	var urlformat string
	urlformat, err = getURLFormat(urltemplate)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get a url format")
	}
	glog.Infof("Using url template [%s] format [%s]", urltemplate, urlformat)

	return &IngressStrategy{
		client:         client,
		domain:         domain,
		internalDomain: internalDomain,
		http:           http,
		tlsAcme:        tlsAcme,
		tlsSecretName:  tlsSecretName,
		tlsUseWildcard: tlsUseWildcard,
		urltemplate:    urlformat,
		pathMode:       pathMode,
		ingressClass:   ingressClass,
	}, nil
}

func (s *IngressStrategy) Add(svc *v1.Service) error {
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

	domain := s.domain
	if svc.Annotations["fabric8.io/use.internal.domain"] == "true" {
		domain = s.internalDomain
	}

	hostName = fmt.Sprintf(s.urltemplate, hostName, svc.Namespace, domain)
	tlsHostName := hostName
	if s.tlsUseWildcard {
		tlsHostName = "*." + domain
	}
	fullHostName := hostName
	path := svc.Annotations["fabric8.io/ingress.path"]
	pathMode := svc.Annotations["fabric8.io/path.mode"]
	if pathMode == "" {
		pathMode = s.pathMode
	}
	if pathMode == PathModeUsePath {
		suffix := path
		if len(suffix) == 0 {
			suffix = "/"
		}
		path = UrlJoin("/", svc.Namespace, appName, suffix)
		hostName = domain
		fullHostName = UrlJoin(hostName, path)
	}

	ingress, err := s.client.ExtensionsV1beta1().Ingresses(svc.Namespace).Get(appName, metav1.GetOptions{})
	createIngress := false
	if err != nil {
		if apierrors.IsNotFound(err) {
			createIngress = true
			ingress = &v1beta1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: svc.Namespace,
					Name:      appName,
				},
			}
		} else {
			return errors.Wrapf(err, "could not check for existing ingress %s/%s", svc.Namespace, appName)
		}
	}

	if ingress.Labels == nil {
		ingress.Labels = map[string]string{}
		ingress.Labels["provider"] = "fabric8"
	}

	if ingress.Annotations == nil {
		ingress.Annotations = map[string]string{}
		ingress.Annotations["fabric8.io/generated-by"] = "exposecontroller"
	}

	hasOwner := false
	for _, o := range ingress.OwnerReferences {
		if o.UID == svc.UID {
			hasOwner = true
			break
		}
	}
	if !hasOwner {
		ingress.OwnerReferences = append(ingress.OwnerReferences, metav1.OwnerReference{
			APIVersion: "v1",
			Kind:       "Service",
			Name:       svc.Name,
			UID:        svc.UID,
		})
	}

	if s.ingressClass != "" {
		ingress.Annotations["kubernetes.io/ingress.class"] = s.ingressClass
		ingress.Annotations["nginx.ingress.kubernetes.io/ingress.class"] = s.ingressClass
	}

	if pathMode == PathModeUsePath {
		if ingress.Annotations["kubernetes.io/ingress.class"] == "" {
			ingress.Annotations["kubernetes.io/ingress.class"] = "nginx"
		}
		if ingress.Annotations["nginx.ingress.kubernetes.io/ingress.class"] == "" {
			ingress.Annotations["nginx.ingress.kubernetes.io/ingress.class"] = "nginx"
		}
		/*		if ingress.Annotations["nginx.ingress.kubernetes.io/rewrite-target"] == "" {
					ingress.Annotations["nginx.ingress.kubernetes.io/rewrite-target"] = "/"
				}
		*/
	}
	var tlsSecretName string

	if s.tlsAcme {
		ingress.Annotations["kubernetes.io/tls-acme"] = "true"
		if s.tlsSecretName == "" {
			tlsSecretName = "tls-" + appName
		} else {
			tlsSecretName = s.tlsSecretName
		}
	}

	annotationsForIngress := svc.Annotations["fabric8.io/ingress.annotations"]
	if annotationsForIngress != "" {
		annotations := strings.Split(annotationsForIngress, "\n")
		for _, element := range annotations {
			annotation := strings.SplitN(element, ":", 2)
			key, value := annotation[0], strings.TrimSpace(annotation[1])
			ingress.Annotations[key] = value
		}
	}

	glog.Infof("Processing Ingress for Service %s with http: %v path mode: %s and path: %s", svc.Name, s.http, pathMode, path)

	backendPaths := []v1beta1.HTTPIngressPath{}
	if ingress.Spec.Rules != nil {
		backendPaths = ingress.Spec.Rules[0].HTTP.Paths
	}

	// check incase we already have this backend path listed
	for _, backendPath := range backendPaths {
		if backendPath.Backend.ServiceName == svc.Name && backendPath.Path == path {
			return nil
		}
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
				glog.Warningf("Port '%s' provided in the annotation '%s' is not available in the ports of service '%s'",
					exposePort, ExposePortAnnotationKey, svc.GetName())
				exposePort = ""
			}
		} else {
			glog.Warningf("Port '%s' provided in the annotation '%s' is not a valid number",
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
	glog.Infof("Exposing Port %d of Service %s", servicePort, svc.Name)

	ingressPaths := []v1beta1.HTTPIngressPath{}
	ingressPath := v1beta1.HTTPIngressPath{
		Backend: v1beta1.IngressBackend{
			ServiceName: svc.Name,
			ServicePort: intstr.FromInt(servicePort),
		},
		Path: path,
	}
	ingressPaths = append(ingressPaths, ingressPath)
	ingressPaths = append(ingressPaths, backendPaths...)

	ingress.Spec.Rules = []v1beta1.IngressRule{}
	rule := v1beta1.IngressRule{
		Host: hostName,
		IngressRuleValue: v1beta1.IngressRuleValue{
			HTTP: &v1beta1.HTTPIngressRuleValue{
				Paths: ingressPaths,
			},
		},
	}
	ingress.Spec.Rules = append(ingress.Spec.Rules, rule)

	if s.isTLSEnabled(svc) {
		ingress.Spec.TLS = []v1beta1.IngressTLS{
			{
				Hosts:      []string{tlsHostName},
				SecretName: tlsSecretName,
			},
		}
	}

	if createIngress {
		_, err := s.client.ExtensionsV1beta1().Ingresses(ingress.Namespace).Create(ingress)
		if err != nil {
			return errors.Wrapf(err, "failed to create ingress %s/%s", ingress.Namespace, ingress.Name)
		}
	} else {
		_, err := s.client.ExtensionsV1beta1().Ingresses(svc.Namespace).Update(ingress)
		if err != nil {
			return errors.Wrapf(err, "failed to update ingress %s/%s", ingress.Namespace, ingress.Name)
		}
	}

	clone := svc.DeepCopy()
	if s.isTLSEnabled(svc) {
		clone, err = addServiceAnnotationWithProtocol(clone, fullHostName, "https")
	} else {
		clone, err = addServiceAnnotationWithProtocol(clone, fullHostName, "http")
	}

	if err != nil {
		return errors.Wrap(err, "failed to add service annotation")
	}
	patch, err := createPatch(svc, clone, v1.Service{})
	if err != nil {
		return errors.Wrap(err, "failed to create patch")
	}
	if patch != nil {
		_, err = s.client.CoreV1().Services(svc.Namespace).
			Patch(svc.Name, types.StrategicMergePatchType, patch)
		if err != nil {
			return errors.Wrap(err, "failed to send patch")
		}
	}

	return nil
}

func (s *IngressStrategy) Remove(svc *v1.Service) error {
	var appName string
	if svc.Labels["release"] != "" {
		appName = strings.Replace(svc.Name, svc.Labels["release"]+"-", "", 1)
	} else {
		appName = svc.Name
	}
	err := s.client.ExtensionsV1beta1().Ingresses(svc.Namespace).Delete(appName, nil)
	if err != nil && !apierrors.IsNotFound(err) {
		return errors.Wrap(err, "failed to delete ingress")
	}

	clone := svc.DeepCopy()
	clone = removeServiceAnnotation(clone)

	patch, err := createPatch(svc, clone, v1.Service{})
	if err != nil {
		return errors.Wrap(err, "failed to create patch")
	}
	if patch != nil {
		_, err = s.client.CoreV1().Services(clone.Namespace).
			Patch(clone.Name, types.StrategicMergePatchType, patch)
		if err != nil {
			return errors.Wrap(err, "failed to send patch")
		}
	}

	return nil
}

func (s *IngressStrategy) isTLSEnabled(svc *v1.Service) bool {
	if svc != nil && svc.Annotations["jenkins-x.io/skip.tls"] == "true" {
		return false
	}

	if len(s.tlsSecretName) > 0 || s.tlsAcme {
		return true
	}

	return false
}
