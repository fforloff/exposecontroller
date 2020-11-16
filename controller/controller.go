package controller

import (
	"bytes"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	yaml "gopkg.in/yaml.v2"
	"k8s.io/klog"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"github.com/olli-ai/exposecontroller/exposestrategy"
)

const (
	// ExposeConfigURLProtocol annotation holds the field to export the protocol to
	ExposeConfigURLProtocol                       = "expose.config.fabric8.io/url-protocol"
	// ExposeConfigURLKeyAnnotation annotation holds the field to export the url to
	ExposeConfigURLKeyAnnotation                  = "expose.config.fabric8.io/url-key"
	// ExposeConfigHostKeyAnnotation annotation holds the field to export the host to
	ExposeConfigHostKeyAnnotation                 = "expose.config.fabric8.io/host-key"
	// ExposeConfigClusterPathKeyAnnotation annotation holds the field to export the path to
	ExposeConfigClusterPathKeyAnnotation          = "expose.config.fabric8.io/path-key"
	// ExposeConfigClusterIPKeyAnnotation annotation holds the field to export the .Spec.ClusterIP field to
	ExposeConfigClusterIPKeyAnnotation            = "expose.config.fabric8.io/clusterip-key"
	// ExposeConfigClusterIPPortKeyAnnotation annotation holds the field to export the clusterIP with port to
	ExposeConfigClusterIPPortKeyAnnotation        = "expose.config.fabric8.io/clusterip-port-key"
	// ExposeConfigClusterIPPortIfEmptyKeyAnnotation annotation holds the field to export the clusterIP with port to, but if the configmap's field is empty
	ExposeConfigClusterIPPortIfEmptyKeyAnnotation = "expose.config.fabric8.io/clusterip-port-if-empty-key"
	// ExposeConfigYamlAnnotation annotation holds the field to export the service's url as a yaml object
	ExposeConfigYamlAnnotation = "expose.config.fabric8.io/config-yaml"

	updateOnChangeAnnotation = "configmap.fabric8.io/update-on-change"
)

// Run runs the controller until synced or timeout
func Run(client kubernetes.Interface, namespace string, config *Config, timeout time.Duration) error {
	var hasSyncedTimeout <-chan time.Time
	if timeout > 0*time.Second {
		hasSyncedTimeout = time.After(timeout)
	} else {
		hasSyncedTimeout = make(chan time.Time)
	}
	hasSynced := make(chan struct{})
	hasSyncedController := make(chan struct{})
	hasSyncedStrategy := make(chan struct{})

	controller, err := createController(client, namespace, config, time.Hour, hasSyncedController, hasSyncedStrategy)
	if err != nil {
		return err
	}

	go func() {
		select {
		case <- hasSyncedTimeout:
			err = fmt.Errorf("timeout")
		case <- hasSyncedController:
		case <- hasSyncedStrategy:
		}
		close(hasSynced)
	}()
	controller.Run(hasSynced)
	return err
}

// Daemon returns a controller for a daemon run
func Daemon(client kubernetes.Interface, namespace string, config *Config, resyncPeriod time.Duration) (cache.Controller, error) {
	return createController(client, namespace, config, resyncPeriod, nil, nil)
}

func createController(client kubernetes.Interface, namespace string, config *Config, resyncPeriod time.Duration, hasSyncedController, hasSyncedStrategy chan struct{}) (cache.Controller, error) {
	strategy, err := getStrategy(client, namespace, config)
	if err != nil {
		return nil, err
	}

	var controller cache.Controller
	isSyncing := false
	needCheckSynced := false
	checkSynced := func() {
		needCheckSynced = true
		if isSyncing && controller.HasSynced() {
			isSyncing = false
			if hasSyncedController != nil && strategy.HasSynced() {
				close(hasSyncedController)
				hasSyncedController = nil
			}
		}
	}

	handlers := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			svc := obj.(*v1.Service)
			if shouldExposeService(svc) {
				if !isServiceWhitelisted(svc.Name, config) {
					return
				}
				err := strategy.Add(svc)
				if err != nil {
					klog.Errorf("Add failed: %v", err)
				}
				updateRelatedResources(client, svc, config)
			} else if isSyncing {
				if !isServiceWhitelisted(svc.Name, config) {
					return
				}
				err := strategy.Clean(svc)
				if err != nil {
					klog.Errorf("Remove failed: %v", err)
				}
				if (needCheckSynced) {
					needCheckSynced = false
					go checkSynced()
				}
			} else {
				return
			}
			if hasSyncedStrategy != nil && strategy.HasSynced() {
				close(hasSyncedStrategy)
				hasSyncedStrategy = nil
			}
		},
		UpdateFunc: func(oldObj interface{}, newObj interface{}) {
			svc := newObj.(*v1.Service)
			if shouldExposeService(svc) {
				if !isServiceWhitelisted(svc.Name, config) {
					return
				}
				err := strategy.Add(svc)
				if err != nil {
					klog.Errorf("Add failed: %v", err)
				}
				updateRelatedResources(client, svc, config)
			} else if shouldExposeService(oldObj.(*v1.Service)) {
				if !isServiceWhitelisted(svc.Name, config) {
					return
				}
				err := strategy.Clean(svc)
				if err != nil {
					klog.Errorf("Remove failed: %v", err)
				}
			} else {
				return
			}
			if hasSyncedStrategy != nil && strategy.HasSynced() {
				close(hasSyncedStrategy)
				hasSyncedStrategy = nil
			}
		},
		DeleteFunc: func(obj interface{}) {
			svc := obj.(*v1.Service)
			if shouldExposeService(svc) {
				if !isServiceWhitelisted(svc.Name, config) {
					return
				}
				err := strategy.Delete(svc)
				if err != nil {
					klog.Errorf("Remove failed: %v", err)
				}
			} else {
				return
			}
			if hasSyncedStrategy != nil && strategy.HasSynced() {
				close(hasSyncedStrategy)
				hasSyncedStrategy = nil
			}
		},
	}

	services := client.CoreV1().Services(namespace)

	_, controller = cache.NewInformer(
		&cache.ListWatch{
			ListFunc:  func(options metav1.ListOptions) (runtime.Object, error) {
				err := strategy.Sync()
				if err != nil {
					return nil, err
				}
				list, err := services.List(options)
				if err != nil {
					return nil, err
				}
				isSyncing = true
				go checkSynced()
				return list, nil
			},
			WatchFunc: services.Watch,
		},
		&v1.Service{},
		resyncPeriod,
		handlers,
	)

	return controller, nil
}

// for testing only
var testStrategy exposestrategy.ExposeStrategy

func getStrategy(client kubernetes.Interface, namespace string, config *Config) (exposestrategy.ExposeStrategy, error) {
	// for testing only
	if testStrategy != nil {
		return testStrategy, nil
	}
	strategy, err := exposestrategy.New(client, &exposestrategy.Config{
		Exposer:        config.Exposer,
		Namespace:      namespace,
		NamePrefix:     config.NamePrefix,
		Domain:         config.Domain,
		InternalDomain: config.InternalDomain,
		NodeIP:         config.NodeIP,
		TLSSecretName:  config.TLSSecretName,
		TLSUseWildcard: config.TLSUseWildcard,
		HTTP:           config.HTTP,
		TLSAcme:        config.TLSAcme,
		URLTemplate:    config.URLTemplate,
		PathMode:       config.PathMode,
		IngressClass:   config.IngressClass,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create new strategy")
	}
	return strategy, nil
}

func shouldExposeService(svc *v1.Service) bool {
	return svc.Labels[exposestrategy.ExposeLabel.Key] == exposestrategy.ExposeLabel.Value ||
		svc.Annotations[exposestrategy.ExposeAnnotation.Key] == exposestrategy.ExposeAnnotation.Value ||
		svc.Annotations[exposestrategy.InjectAnnotation.Key] == exposestrategy.InjectAnnotation.Value
}

// isServiceWhitelisted checks if a service is white-listed in the controller configuration, allow all services if
// the white-list is empty
func isServiceWhitelisted(service string, config *Config) bool {
	services := config.Services
	if len(services) == 0 {
		return true
	}
	for _, s := range services {
		if s == service {
			return true
		}
	}
	return false
}

func updateRelatedResources(c kubernetes.Interface, svc *v1.Service, config *Config) {
	updateServiceConfigMap(c, svc, config)

	exposeURL := svc.Annotations[exposestrategy.ExposeAnnotationKey]
	if len(exposeURL) > 0 {
		updateOtherConfigMaps(c, svc, config, exposeURL)
	}
}

func getServicePort(svc *v1.Service) string {
	for _, port := range svc.Spec.Ports {
		tp := port.TargetPort.StrVal
		if tp != "" {
			return tp
		}
		i := port.TargetPort.IntVal
		if i > 0 {
			return strconv.Itoa(int(i))
		}
	}
	return ""
}

type configYaml struct {
	Key        string
	Expression string
	Prefix     string
	Suffix     string
}

func updateServiceConfigMap(c kubernetes.Interface, svc *v1.Service, config *Config) {
	name := svc.Name
	ns := svc.Namespace
	cm, err := c.CoreV1().ConfigMaps(ns).Get(name, metav1.GetOptions{})
	if err == nil {
		updated := false

		clusterIP := svc.Spec.ClusterIP
		if clusterIP != "" {
			clusterIPKey := firstMapValue(ExposeConfigClusterIPKeyAnnotation, svc.Annotations, cm.Annotations)
			clusterIPPortKey := firstMapValue(ExposeConfigClusterIPPortKeyAnnotation, svc.Annotations, cm.Annotations)
			clusterIPPortIfEmptyKey := firstMapValue(ExposeConfigClusterIPPortIfEmptyKeyAnnotation, svc.Annotations, cm.Annotations)

			if clusterIPKey != "" {
				if cm.Data[clusterIPKey] != clusterIP {
					cm.Data[clusterIPKey] = clusterIP
					updated = true
				}
			}

			port := getServicePort(svc)
			if port != "" {
				clusterIPAndPort := clusterIP + ":" + port

				if clusterIPPortKey != "" {
					if cm.Data[clusterIPPortKey] != clusterIPAndPort {
						cm.Data[clusterIPPortKey] = clusterIPAndPort
						updated = true
					}
				}
				if clusterIPPortIfEmptyKey != "" {
					if cm.Data[clusterIPPortIfEmptyKey] == "" {
						cm.Data[clusterIPPortIfEmptyKey] = clusterIPAndPort
						updated = true
					}
				}
			}
		}
		exposeURL := svc.Annotations[exposestrategy.ExposeAnnotationKey]
		if exposeURL != "" {
			host := ""
			url, err := url.Parse(exposeURL)
			if err != nil {
				klog.Errorf("Failed to parse expose URL %s for service %s  error: %v", exposeURL, name, err)

			} else {
				host = url.Host
			}
			urlKey := cm.Annotations[ExposeConfigURLKeyAnnotation]
			domainKey := cm.Annotations[ExposeConfigHostKeyAnnotation]
			if urlKey != "" {
				if cm.Data[urlKey] != exposeURL {
					cm.Data[urlKey] = exposeURL
					updated = true
				}
			}
			if host != "" && domainKey != "" {
				if cm.Data[domainKey] != host {
					cm.Data[domainKey] = host
					updated = true
				}
			}

			pathKey := cm.Annotations[ExposeConfigClusterPathKeyAnnotation]
			if pathKey != "" {
				path := urlPath(exposeURL)
				if cm.Data[pathKey] != path {
					cm.Data[pathKey] = path
					updated = true
				}
				klog.Infof("Found key %s and has path %s\n", pathKey, path)
			}

			configYamlS := svc.Annotations[ExposeConfigYamlAnnotation]
			if configYamlS != "" {
				fmt.Printf("Processing yaml config on service %s\n", svc.Name)
				configs := []configYaml{}
				err := yaml.Unmarshal([]byte(configYamlS), &configs)
				if err != nil {
					klog.Errorf("Failed to unmarshal Config YAML on service %s due to %s : YAML: %s", svc.Name, err, configYamlS)
				} else {
					values := map[string]string{
						"host":              host,
						"url":               exposeURL,
					}
					fmt.Printf("Loading yaml config %#v\n", configs)
					for _, c := range configs {
						if c.updateConfigMap(cm, values) {
							updated = true
						}
					}
				}
			}
		}
		if updated {
			klog.Infof("Updating ConfigMap %s/%s", ns, name)
			_, err = c.CoreV1().ConfigMaps(ns).Update(cm)
			if err != nil {
				klog.Errorf("Failed to update ConfigMap %s error: %v", name, err)
			}
			err = rollingUpgradeDeployments(cm, c)
			if err != nil {
				klog.Errorf("Failed to update Deployments after change to ConfigMap %s error: %v", name, err)
			}
		}
	}
}

// returns the path starting with a `/` character for the given URL
func urlPath(urlText string) string {
	answer := "/"
	u, err := url.Parse(urlText)
	if err != nil {
		klog.Warningf("Could not parse exposeURL: %s due to: %s", urlText, err)
	} else {
		if u.Path != "" {
			answer = u.Path
		}
		if !strings.HasPrefix(answer, "/") {
			answer = "/" + answer
		}
	}
	return answer
}

// firstMapValue returns the first value in the map which is not empty
func firstMapValue(key string, maps ...map[string]string) string {
	for _, m := range maps {
		if m != nil {
			v := m[key]
			if v != "" {
				return v
			}
		}
	}
	return ""
}

func (c *configYaml) updateConfigMap(configMap *v1.ConfigMap, values map[string]string) bool {
	key := c.Key
	if key == "" {
		klog.Warningf("ConfigMap %s does not have a key in yaml config %#v\n", configMap.Name, c)
		return false
	}
	expValue := values[c.Expression]
	if expValue == "" {
		klog.Warningf("Could not calculate expression %s from the yaml config %#v possible values are %v\n", c.Expression, c, values)
		return false
	}
	value := configMap.Data[key]
	if value == "" {
		klog.Warningf("ConfigMap %s does not have a key %s when trying to apply the yaml config %#v\n", configMap.Name, key, c)
		return false
	}
	lines := strings.Split(value, "\n")
	var buffer bytes.Buffer
	for _, line := range lines {
		if strings.HasPrefix(line, c.Prefix) {
			buffer.WriteString(c.Prefix + expValue + c.Suffix)
		} else {
			buffer.WriteString(line)
		}
		buffer.WriteString("\n")
	}
	newValue := buffer.String()
	if newValue != value {
		configMap.Data[key] = newValue
		return true
	}
	return false
}

// updateOtherConfigMaps lets update all other configmaps which want to be injected by this svc exposeURL
func updateOtherConfigMaps(c kubernetes.Interface, svc *v1.Service, config *Config, exposeURL string) error {
	serviceName := svc.Name
	annotationKey := "expose.service-key.config.fabric8.io/" + serviceName
	annotationFullKey := "expose-full.service-key.config.fabric8.io/" + serviceName
	annotationNoProtocolKey := "expose-no-protocol.service-key.config.fabric8.io/" + serviceName
	annotationNoPathKey := "expose-no-path.service-key.config.fabric8.io/" + serviceName
	annotationFullNoProtocolKey := "expose-full-no-protocol.service-key.config.fabric8.io/" + serviceName
	ns := svc.Namespace
	cms, err := c.CoreV1().ConfigMaps(ns).List(metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, cm := range cms.Items {
		update := false
		updateKey := cm.Annotations[annotationKey]
		if cm.Data == nil {
			cm.Data = map[string]string{}
		}
		if len(updateKey) > 0 {
			exposeURL = strings.TrimSuffix(exposeURL, "/")
			keys := strings.Split(updateKey, ",")
			for _, key := range keys {
				value := cm.Data[key]
				if value != exposeURL {
					cm.Data[key] = exposeURL
					klog.Infof("Updating ConfigMap %s in namespace %s with key %s", cm.Name, ns, key)
					update = true
				}
			}
		}
		updateKey = cm.Annotations[annotationFullKey]
		if len(updateKey) > 0 {
			if !strings.HasSuffix(exposeURL, "/") {
				exposeURL += "/"
			}
			keys := strings.Split(updateKey, ",")
			for _, key := range keys {
				value := cm.Data[key]
				if value != exposeURL {
					cm.Data[key] = exposeURL
					klog.Infof("Updating ConfigMap %s in namespace %s with key %s", cm.Name, ns, key)
					update = true
				}
			}
		}
		updateKey = cm.Annotations[annotationNoPathKey]
		if cm.Data == nil {
			cm.Data = map[string]string{}
		}
		if len(updateKey) > 0 {
			u, err := url.Parse(exposeURL)
			if err != nil {
				klog.Warningf("Failed to parse URL %s due to %s", exposeURL, err)
			} else {
				u.Path = "/"
				noPathURL := u.String()
				keys := strings.Split(updateKey, ",")
				for _, key := range keys {
					value := cm.Data[key]
					if value != noPathURL {
						cm.Data[key] = noPathURL
						klog.Infof("Updating ConfigMap %s in namespace %s with key %s", cm.Name, ns, key)
						update = true
					}
				}
			}
		}
		updateKey = cm.Annotations[annotationNoProtocolKey]
		if cm.Data == nil {
			cm.Data = map[string]string{}
		}
		if len(updateKey) > 0 {
			exposeURL = strings.TrimSuffix(exposeURL, "/")
			exposeURL = strings.TrimPrefix(exposeURL, "http://")
			exposeURL = strings.TrimPrefix(exposeURL, "https://")
			keys := strings.Split(updateKey, ",")
			for _, key := range keys {
				value := cm.Data[key]
				if value != exposeURL {
					cm.Data[key] = exposeURL
					klog.Infof("Updating ConfigMap %s in namespace %s with key %s", cm.Name, ns, key)
					update = true
				}
			}
		}
		updateKey = cm.Annotations[annotationFullNoProtocolKey]
		if len(updateKey) > 0 {
			if !strings.HasSuffix(exposeURL, "/") {
				exposeURL += "/"
			}
			exposeURL = strings.TrimPrefix(exposeURL, "http://")
			exposeURL = strings.TrimPrefix(exposeURL, "https://")
			keys := strings.Split(updateKey, ",")
			for _, key := range keys {
				value := cm.Data[key]
				if value != exposeURL {
					cm.Data[key] = exposeURL
					klog.Infof("Updating ConfigMap %s in namespace %s with key %s", cm.Name, ns, key)
					update = true
				}
			}
		}
		updateKey = cm.Annotations[ExposeConfigURLProtocol]
		if len(updateKey) > 0 {
			protocol := "https"
			if config.HTTP {
				protocol = "http"
			}
			keys := strings.Split(updateKey, ",")
			for _, key := range keys {
				value := cm.Data[key]
				if value != protocol {
					cm.Data[key] = protocol
					klog.Infof("Updating ConfigMap %s in namespace %s with key %s", cm.Name, ns, key)
					update = true
				}
			}
		}
		if update {
			_, err = c.CoreV1().ConfigMaps(ns).Update(&cm)
			if err != nil {
				return fmt.Errorf("Failed to update ConfigMap %s in namespace %s with key %s due to %v", cm.Name, ns, updateKey, err)
			}
		}
	}
	return nil
}
