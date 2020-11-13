package exposestrategy

import (
	"fmt"
	"strconv"
	"strings"
	"reflect"

	"github.com/golang/glog"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"

	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
)

const (
	PathModeUsePath = "path"
)

type IngressStrategy struct {
	client         kubernetes.Interface
	namespace      string
	namePrefix     string
	domain         string
	internalDomain string
	tlsSecretName  string
	tlsUseWildcard bool
	http           bool
	tlsAcme        bool
	urltemplate    string
	pathMode       string
	ingressClass   string
	existing       map[string][]string
}

func NewIngressStrategy(client kubernetes.Interface, config *ExposeStrategyConfig) (ExposeStrategy, error) {

	var err error
	if len(config.Domain) == 0 {
		config.Domain, err = getAutoDefaultDomain(client)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get a domain")
		}
	}
	glog.Infof("Using domain: %s", config.Domain)

	var urlformat string
	urlformat, err = getURLFormat(config.URLTemplate)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get a url format")
	}
	glog.Infof("Using url template [%s] format [%s]", config.URLTemplate, urlformat)

	return &IngressStrategy{
		client:         client,
		namespace:      config.Namespace,
		namePrefix:     config.NamePrefix,
		domain:         config.Domain,
		internalDomain: config.InternalDomain,
		http:           config.HTTP,
		tlsAcme:        config.TLSAcme,
		tlsSecretName:  config.TLSSecretName,
		tlsUseWildcard: config.TLSUseWildcard,
		urltemplate:    urlformat,
		pathMode:       config.PathMode,
		ingressClass:   config.IngressClass,
	}, nil
}

func CleanIngressStrategy(client kubernetes.Interface, namespace string) error {
	// list all existing ingresses
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: map[string]string{"provider": "fabric8"},
	})
	if err != nil {
		return errors.Wrap(err, "failed to build selector")
	}
	listOptions := metav1.ListOptions{
		LabelSelector: selector.String(),
	}
	list, err := client.ExtensionsV1beta1().Ingresses(namespace).List(listOptions)
	if err != nil {
		return errors.Wrap(err, "failed to list ingresses")
	}
	// check which service is referencing each ingress
	for index := range list.Items {
		ingress := &list.Items[index]
		svc, del := getIngressService(ingress)
		if del || svc != "" {
			deleteIngress(client, ingress)
		}
	}
	return nil
}

func (s *IngressStrategy) Sync() error {
	// list all existing ingresses
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: map[string]string{"provider": "fabric8"},
	})
	if err != nil {
		return  errors.Wrap(err, "failed to build selector")
	}
	listOptions := metav1.ListOptions{
		LabelSelector: selector.String(),
	}
	list, err := s.client.ExtensionsV1beta1().Ingresses(s.namespace).List(listOptions)
	if err != nil {
		return errors.Wrap(err, "failed to list ingresses")
	}
	// check which service is referencing each ingress
	existing := map[string][]string{}
	for index := range list.Items {
		ingress := &list.Items[index]
		svc, del := getIngressService(ingress)
		if del {
			deleteIngress(s.client, ingress)
		} else if svc != "" {
			existing[svc] = append(existing[svc], ingress.Name)
		}
	}
	s.existing = existing
	return nil
}

func (s *IngressStrategy) HasSynced() bool {
	return true
}

func (s *IngressStrategy) Add(svc *v1.Service) error {
	// choose the name of the ingress
	appName := svc.Annotations["fabric8.io/ingress.name"]
	if appName == "" {
		if svc.Labels["release"] != "" {
			appName = strings.TrimPrefix(svc.Name, svc.Labels["release"]+"-")
		} else {
			appName = svc.Name
		}
	}
	ingressName := appName
	if s.namePrefix != "" {
		if strings.HasSuffix(s.namePrefix, "-") || strings.HasSuffix(s.namePrefix, ".") {
			ingressName = s.namePrefix + appName
		} else {
			ingressName = s.namePrefix + "-" + appName
		}
	}
	// choose the hostname and path of the ingress
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
	path := svc.Annotations["fabric8.io/ingress.path"]
	pathMode := svc.Annotations["fabric8.io/path.mode"]
	if pathMode == "" {
		pathMode = s.pathMode
	}
	if pathMode == PathModeUsePath {
		if path == "" {
			path = "/"
		}
		path = UrlJoin("/", svc.Namespace, appName, path)
		hostName = domain
	} else if path != "" && path[0] != '/' {
		path = "/" + path
	}
	// choose the target port
	exposePort := svc.Annotations[ExposePortAnnotationKey]
	if exposePort != "" {
		port, err := strconv.Atoi(exposePort)
		if err != nil {
			return errors.Wrapf(err, "port \"%s\" provided in the annotation \"%s\" is not a valid number in service %s/%s",
				exposePort, ExposePortAnnotationKey, svc.Namespace, svc.Name)
		}
		found := false
		for _, p := range svc.Spec.Ports {
			if port == int(p.Port) {
				found = true
				break
			}
		}
		if !found {
			glog.Warningf("port \"%s\" provided in the annotation \"%s\" is not available in the ports of service %s/%s",
				exposePort, ExposePortAnnotationKey, svc.Namespace, svc.Name)
			exposePort = ""
		}
	}
	// Pick the fist port available in the service if no expose port was configured
	if exposePort == "" && len(svc.Spec.Ports) > 0 {
		port := svc.Spec.Ports[0]
		exposePort = strconv.Itoa(int(port.Port))
	}
	servicePort, err := strconv.Atoi(exposePort)
	if err != nil {
		return errors.Wrapf(err, "failed to convert the exposed port \"%s\" to int in service %s/%s",
			exposePort, svc.Namespace, svc.Name)
	}
	glog.Infof("Exposing Port %d of Service %s/%s",
		servicePort, svc.Namespace, svc.Name)
	// gather the annotations of the ingress
	ingressAnnotations := map[string]string{}
	// ingress class annotation
	if s.ingressClass != "" {
		ingressAnnotations["kubernetes.io/ingress.class"] = s.ingressClass
		ingressAnnotations["nginx.ingress.kubernetes.io/ingress.class"] = s.ingressClass
	} else if pathMode == PathModeUsePath {
		ingressAnnotations["kubernetes.io/ingress.class"] = "nginx"
		ingressAnnotations["nginx.ingress.kubernetes.io/ingress.class"] = "nginx"
	}
	// check for tls
	tlsSecretName := s.tlsSecretName
	if s.tlsAcme {
		ingressAnnotations["kubernetes.io/tls-acme"] = "true"
		if tlsSecretName == "" {
			tlsSecretName = "tls-" + appName
		}
	}

	var tlsSpec []v1beta1.IngressTLS
	if tlsSecretName != "" {
		tlsSpec = []v1beta1.IngressTLS{
			{
				Hosts:      []string{tlsHostName},
				SecretName: tlsSecretName,
			},
		}
	}
	// add all the other annotations
	annotationsString := svc.Annotations["fabric8.io/ingress.annotations"]
	if annotationsString != "" {
		err := yaml.Unmarshal([]byte(annotationsString), ingressAnnotations)
		if err != nil {
			return errors.Wrapf(err, "failed to parse annotation \"fabric8.io/ingress.annotations\" in service %s/%s",
				svc.Namespace, svc.Name)
		}
	}
	// that annotations is important and cannot be overridden
	ingressAnnotations["fabric8.io/generated-by"] = "exposecontroller"
	// build the ingress
	ingress := v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   svc.Namespace,
			Name:        ingressName,
			Labels:      map[string]string{
				"provider": "fabric8",
			},
			Annotations: ingressAnnotations,
			OwnerReferences: []metav1.OwnerReference{{
				Kind:       svc.Kind,
				APIVersion: svc.APIVersion,
				Name:       svc.Name,
				UID:        svc.UID,
			}},
		},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{{
				Host: hostName,
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Backend: v1beta1.IngressBackend{
								ServiceName: svc.Name,
								ServicePort: intstr.FromInt(servicePort),
							},
							Path: path,
						}},
					},
				},
			}},
			TLS: tlsSpec,
		},
	}
	// clean the old ingresses of the service if they have a different name
	ingresses := s.client.ExtensionsV1beta1().Ingresses(svc.Namespace)
	svcKey := fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)

	for _, name := range s.existing[svcKey] {
		if name != ingress.Name {
			existing, err := ingresses.Get(name, metav1.GetOptions{})
			if err == nil {
				exKey, del := getIngressService(existing)
				if del || exKey == svcKey {
					deleteIngress(s.client, existing)
				}
			} else if !apierrors.IsNotFound(err) {
				glog.Errorf("error when getting ingress %s/%s: %s",
					svc.Namespace, name, err)
			}
		}
	}
	s.existing[svcKey] = []string{ingress.Name}
	// check for an existing ingress
	existing, err := ingresses.Get(ingress.Name, metav1.GetOptions{})

	if err == nil {
		// if the ingress is the same in all points, no need to update
		if reflect.DeepEqual(ingress.Labels, existing.Labels) &&
		reflect.DeepEqual(ingress.Annotations, existing.Annotations) &&
		reflect.DeepEqual(ingress.OwnerReferences, existing.OwnerReferences) &&
		reflect.DeepEqual(ingress.Spec, existing.Spec) {
			glog.Infof("ingress %s/%s already up to date for service %s/%s",
				ingress.Namespace, ingress.Name, svc.Namespace, svc.Name)
			return nil
		}
		// get the resource version for update
		ingress.ResourceVersion = existing.ResourceVersion
	} else if !apierrors.IsNotFound(err) {
		return errors.Wrapf(err, "could not check for existing ingress %s/%s", ingress.Namespace, ingress.Name)
	}
	// create or update the ingress
	glog.Infof("processing ingress %s/%s for service %s/%s with http: %v, path mode: %s, and path: %s",
		ingress.Namespace, ingress.Name, svc.Namespace, svc.Name, s.http, pathMode, path)

	if ingress.ResourceVersion == "" {
		_, err := ingresses.Create(&ingress)
		if err != nil {
			return errors.Wrapf(err, "failed to create ingress %s/%s", ingress.Namespace, ingress.Name)
		}
	} else {
		_, err := ingresses.Update(&ingress)
		if err != nil {
			return errors.Wrapf(err, "failed to update ingress %s/%s", ingress.Namespace, ingress.Name)
		}
	}
	// build the patch for the service annotations
	clone := svc.DeepCopy()
	if tlsSecretName != "" {
		err = addServiceAnnotationWithProtocol(clone, hostName, path, "https")
	} else {
		err = addServiceAnnotationWithProtocol(clone, hostName, path, "http")
	}
	if err != nil {
		return errors.Wrapf(err, "failed to add annotation to service %s/%s",
			svc.Namespace, svc.Name)
	}

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

func (s *IngressStrategy) Clean(svc *v1.Service) error {
	svcKey := fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)
	for _, name := range s.existing[svcKey] {
		existing, err := s.client.ExtensionsV1beta1().Ingresses(svc.Namespace).Get(name, metav1.GetOptions{})
		if err == nil {
			exKey, del := getIngressService(existing)
			if del || exKey == svcKey {
				deleteIngress(s.client, existing)
			}
		} else if !apierrors.IsNotFound(err) {
			glog.Errorf("error when getting ingress %s/%s: %s",
				svc.Namespace, name, err)
		}
	}
	delete(s.existing, svcKey)

	clone := svc.DeepCopy()
	if !removeServiceAnnotation(clone) {
		return nil
	}

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

func (s *IngressStrategy) Delete(svc *v1.Service) error {
	svcKey := fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)
	for _, name := range s.existing[svcKey] {
		existing, err := s.client.ExtensionsV1beta1().Ingresses(svc.Namespace).Get(name, metav1.GetOptions{})
		if err == nil {
			exKey, del := getIngressService(existing)
			if del || exKey == svcKey {
				deleteIngress(s.client, existing)
			}
		} else if !apierrors.IsNotFound(err) {
			glog.Errorf("error when getting ingress %s/%s: %s",
				svc.Namespace, name, err)
		}
	}
	delete(s.existing, svcKey)

	return nil
}

func deleteIngress(client kubernetes.Interface, ingress *v1beta1.Ingress) {
	options := metav1.DeleteOptions{
		Preconditions: &metav1.Preconditions{
			ResourceVersion: &ingress.ResourceVersion,
		},
	}
	glog.Infof("cleaning the ingress %s/%s", ingress.Namespace, ingress.Name)
	err := client.ExtensionsV1beta1().Ingresses(ingress.Namespace).Delete(ingress.Name, &options)
	if err != nil {
		glog.Errorf("error when deleting ingress %s/%s: %s",
			ingress.Namespace, ingress.Name, err)
	}
}

func getIngressService(ingress *v1beta1.Ingress) (string, bool) {
	if ingress.Labels["provider"] != "fabric8" || ingress.Annotations["fabric8.io/generated-by"] != "exposecontroller" {
		return "", false
	} else if len(ingress.OwnerReferences) != 1 {
		return "", true
	} else if owner := ingress.OwnerReferences[0]; owner.Kind != "Service" {
		return "", true
	} else {
		return fmt.Sprintf("%s/%s", ingress.Namespace, owner.Name), false
	}
}
