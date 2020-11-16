package exposestrategy

import (
	"testing"

	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetIngressService(t *testing.T) {
	examples := []struct{
		name string
		meta metav1.ObjectMeta
		svc  string
		del  bool
	}{{
		name: "empty",
	}, {
		name: "missing label",
		meta: metav1.ObjectMeta{
			Name:        "test-ingress",
			Namespace:   "test-namespace",
			Annotations: map[string]string{
				"fabric8.io/generated-by": "exposecontroller",
			},
		},
	}, {
		name: "missing annotation",
		meta: metav1.ObjectMeta{
			Name:        "test-ingress",
			Namespace:   "test-namespace",
			Labels: map[string]string{
				"provider": "fabric8",
			},
		},
	}, {
		name: "no owner",
		meta: metav1.ObjectMeta{
			Name:        "test-ingress",
			Namespace:   "test-namespace",
			Labels: map[string]string{
				"provider": "fabric8",
			},
			Annotations: map[string]string{
				"fabric8.io/generated-by": "exposecontroller",
			},
		},
		del:  true,
	}, {
		name: "empty owner",
		meta: metav1.ObjectMeta{
			Name:        "test-ingress",
			Namespace:   "test-namespace",
			Labels: map[string]string{
				"provider": "fabric8",
			},
			Annotations: map[string]string{
				"fabric8.io/generated-by": "exposecontroller",
			},
			OwnerReferences: []metav1.OwnerReference{},
		},
		del:  true,
	}, {
		name: "owner not a service",
		meta: metav1.ObjectMeta{
			Name:        "test-ingress",
			Namespace:   "test-namespace",
			Labels: map[string]string{
				"provider": "fabric8",
			},
			Annotations: map[string]string{
				"fabric8.io/generated-by": "exposecontroller",
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "v1",
				Kind:       "Deployment",
				Name:       "test-deployment",
			}},
		},
		del:  true,
	}, {
		name: "right",
		meta: metav1.ObjectMeta{
			Name:        "test-ingress",
			Namespace:   "test-namespace",
			Labels: map[string]string{
				"provider": "fabric8",
			},
			Annotations: map[string]string{
				"fabric8.io/generated-by": "exposecontroller",
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "v1",
				Kind:       "Service",
				Name:       "test-service",
			}},
		},
		svc:  "test-namespace/test-service",
	}, {
		name: "too many owners",
		meta: metav1.ObjectMeta{
			Name:        "test-ingress",
			Namespace:   "test-namespace",
			Labels: map[string]string{
				"provider": "fabric8",
			},
			Annotations: map[string]string{
				"fabric8.io/generated-by": "exposecontroller",
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "v1",
				Kind:       "Service",
				Name:       "test-service-1",
			}, {
				APIVersion: "v1",
				Kind:       "Service",
				Name:       "test-service-2",
			}},
		},
		del:  true,
	}}
	for _, example := range examples {
		svc, del := getIngressService(&v1beta1.Ingress{
			ObjectMeta: example.meta,
		})
		assert.Equal(t, example.svc, svc, example.name)
		assert.Equal(t, example.del, del, example.name)
	}
}

func TestIngressStrategy_Sync(t *testing.T) {
	objects := []runtime.Object{&v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "main",
			Name: "ingress1",
			Labels: map[string]string{
				"provider": "fabric8",
			},
			Annotations: map[string]string{
				"fabric8.io/generated-by": "exposecontroller",
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "v1",
				Kind:       "Service",
				Name:       "service1",
			}},
			ResourceVersion: "1",
		},
	}, &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "main",
			Name: "ingress2",
			Labels: map[string]string{
				"provider": "fabric8",
			},
			Annotations: map[string]string{
				"fabric8.io/generated-by": "not-exposecontroller",
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "v1",
				Kind:       "Service",
				Name:       "service2",
			}},
			ResourceVersion: "2",
		},
	}, &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "main",
			Name: "ingress3",
			Labels: map[string]string{
				"provider": "fabric8",
			},
			Annotations: map[string]string{
				"fabric8.io/generated-by": "exposecontroller",
			},
			ResourceVersion: "3",
		},
	}, &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "other",
			Name: "ingress4",
			Labels: map[string]string{
				"provider": "fabric8",
			},
			Annotations: map[string]string{
				"fabric8.io/generated-by": "not-exposecontroller",
			},
			ResourceVersion: "4",
		},
	}, &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "main",
			Name: "ingress5",
			Labels: map[string]string{
				"provider": "fabric8",
			},
			Annotations: map[string]string{
				"fabric8.io/generated-by": "exposecontroller",
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "v1",
				Kind:       "Service",
				Name:       "service1",
			}},
			ResourceVersion: "5",
		},
	}, &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "main",
			Name: "ingress6",
			Labels: map[string]string{
				"provider": "fabric8",
			},
			Annotations: map[string]string{
				"fabric8.io/generated-by": "exposecontroller",
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "v1",
				Kind:       "Service",
				Name:       "service2",
			}},
			ResourceVersion: "6",
		},
	}}
	client := fake.NewSimpleClientset(objects...)

	strategy := IngressStrategy{
		client:         client,
		namespace:      "main",
	}
	strategy.Sync()

	existing := map[string]map[string]bool{}
	for svc, slice := range strategy.existing {
		names := map[string]bool{}
		for _, name := range slice {
			assert.Falsef(t, names[name], "%s %s already present", svc, name)
			names[name] = true
		}
		existing[svc] = names
	}
	expectedE := map[string]map[string]bool{
		"main/service1": map[string]bool{
			"ingress1": true,
			"ingress5": true,
		},
		"main/service2": map[string]bool{
			"ingress6": true,
		},
	}
	assert.Equal(t, expectedE, existing, "strategy.existing")

	found := map[string]bool{}
	list, err := client.ExtensionsV1beta1().Ingresses("").List(metav1.ListOptions{})
	if assert.NoError(t, err) {
		for _, ingress := range list.Items {
			found[ingress.Name] = true
		}
	}
	expectedF := map[string]bool{
		"ingress1": true,
		"ingress2": true,
		"ingress4": true,
		"ingress5": true,
		"ingress6": true,
	}
	assert.Equal(t, expectedF, found, "found ingresses")
}

func TestIngressStrategy_Add(t *testing.T) {
	service := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "main",
			Name: "source",
			Annotations: map[string]string {
				ExposeAnnotation.Key: ExposeAnnotation.Value,
			},
			ResourceVersion: "1",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Port: 1234,
			}},
		},
	}
	objects := []runtime.Object{service, &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "main",
			Name: "ingress1",
			Labels: map[string]string{
				"provider": "fabric8",
			},
			Annotations: map[string]string{
				"fabric8.io/generated-by": "exposecontroller",
			},
			OwnerReferences: []metav1.OwnerReference{{
				Kind:       "Service",
				APIVersion: "v1",
				Name:       "source",
			}},
			ResourceVersion: "2",
		},
	}, &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "main",
			Name: "ingress2",
			Labels: map[string]string{
				"provider": "fabric8",
			},
			Annotations: map[string]string{
				"fabric8.io/generated-by": "not-exposecontroller",
			},
			OwnerReferences: []metav1.OwnerReference{{
				Kind:       "Service",
				APIVersion: "v1",
				Name:       "source",
			}},
			ResourceVersion: "3",
		},
	}, &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "main",
			Name: "ingress3",
			Labels: map[string]string{
				"provider": "fabric8",
			},
			Annotations: map[string]string{
				"fabric8.io/generated-by": "exposecontroller",
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "v1",
				Kind:       "Service",
				Name:       "other",
			}},
			ResourceVersion: "4",
		},
	}, &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "main",
			Name: "ingress4",
			Labels: map[string]string{
				"provider": "fabric8",
			},
			Annotations: map[string]string{
				"fabric8.io/generated-by": "exposecontroller",
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "v1",
				Kind:       "Service",
				Name:       "other",
			}, {
				APIVersion: "v1",
				Kind:       "Service",
				Name:       "another",
			}},
			ResourceVersion: "5",
		},
	}, &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "main",
			Name: "source",
			Labels: map[string]string{
				"provider": "fabric8",
			},
			Annotations: map[string]string{
				"fabric8.io/generated-by": "exposecontroller",
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "v1",
				Kind:       "Service",
				Name:       "source",
			}},
			ResourceVersion: "6",
		},
	}}
	client := fake.NewSimpleClientset(objects...)

	strategy := IngressStrategy{
		client:         client,
		namespace:      "main",
		domain:         "my-domain.com",
		urltemplate:    "%[1]s.%[2]s.%[3]s",
		existing: map[string][]string{
			"main/source": []string{
				"ingress1",
				"ingress2",
				"ingress3",
				"ingress4",
				"source",
			},
		},
	}
	err := strategy.Add(service)
	require.NoError(t, err)

	found := map[string]bool{}
	list, err := client.ExtensionsV1beta1().Ingresses("").List(metav1.ListOptions{})
	if assert.NoError(t, err) {
		for _, ingress := range list.Items {
			found[ingress.Name] = true
		}
	}
	expectedF := map[string]bool{
		"ingress2": true,
		"ingress3": true,
		"source": true,
	}
	assert.Equal(t, expectedF, found, "found ingresses")

	ingress, err := client.ExtensionsV1beta1().Ingresses("main").Get("source", metav1.GetOptions{})
	if assert.NoError(t, err, "get ingress") {
		expectedI := &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "main",
				Name: "source",
				Labels: map[string]string{
					"provider": "fabric8",
				},
				Annotations: map[string]string{
					"fabric8.io/generated-by": "exposecontroller",
				},
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: "v1",
					Kind:       "Service",
					Name:       "source",
				}},
				ResourceVersion: "6",
			},
			Spec: v1beta1.IngressSpec{
				Rules: []v1beta1.IngressRule{{
					Host: "source.main.my-domain.com",
					IngressRuleValue: v1beta1.IngressRuleValue{
						HTTP: &v1beta1.HTTPIngressRuleValue{
							Paths: []v1beta1.HTTPIngressPath{{
								Backend: v1beta1.IngressBackend{
									ServiceName: "source",
									ServicePort: intstr.FromInt(1234),
								},
								Path: "",
							}},
						},
					},
				}},
			},
		}
		assert.Equalf(t, expectedI, ingress, "ingress")
	}

	service, err = client.CoreV1().Services("main").Get("source", metav1.GetOptions{})
	if assert.NoError(t, err, "get service") {
		expectedS := &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "main",
				Name: "source",
				Annotations: map[string]string {
					ExposeAnnotation.Key: ExposeAnnotation.Value,
					ExposeAnnotationKey: "http://source.main.my-domain.com",
				},
				ResourceVersion: "1",
			},
			Spec: v1.ServiceSpec{
				Ports: []v1.ServicePort{{
					Port: 1234,
				}},
			},
		}
		assert.Equalf(t, expectedS, service, "service")
	}
}

func TestIngressStrategy_Clean(t *testing.T) {
	objects := []runtime.Object{
		&v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns1",
				Name: "svc1",
				Annotations: map[string]string{
					ExposeAnnotationKey: "url",
					"other": "other",
				},
			},
		},
		&v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns1",
				Name: "ingress1",
				Labels: map[string]string{
					"provider": "fabric8",
				},
				Annotations: map[string]string{
					"fabric8.io/generated-by": "exposecontroller",
				},
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: "v1",
					Kind:       "Service",
					Name:       "svc1",
				}},
			},
		},
		&v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns1",
				Name: "ingress2",
				Labels: map[string]string{
					"provider": "fabric8",
				},
				Annotations: map[string]string{
					"fabric8.io/generated-by": "exposecontroller",
				},
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: "v1",
					Kind:       "Service",
					Name:       "other",
				}},
			},
		},
		&v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns1",
				Name: "ingress3",
				Labels: map[string]string{
					"provider": "fabric8",
				},
				Annotations: map[string]string{
					"fabric8.io/generated-by": "exposecontroller",
				},
			},
		},
		&v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns1",
				Name: "ingress4",
				Labels: map[string]string{
					"provider": "fabric8",
				},
				Annotations: map[string]string{
					"fabric8.io/generated-by": "not-exposecontroller",
				},
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: "v1",
					Kind:       "Service",
					Name:       "svc1",
				}},
			},
		},
		&v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns1",
				Name: "ingress5",
				Labels: map[string]string{
					"provider": "fabric8",
				},
				Annotations: map[string]string{
					"fabric8.io/generated-by": "exposecontroller",
				},
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: "v1",
					Kind:       "Service",
					Name:       "svc1",
				}},
			},
		},
		&v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns1",
				Name: "ingress6",
				Labels: map[string]string{
					"provider": "fabric8",
				},
				Annotations: map[string]string{
					"fabric8.io/generated-by": "exposecontroller",
				},
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: "v1",
					Kind:       "Service",
					Name:       "svc1",
				}},
			},
		},
		&v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns2",
				Name: "ingress7",
				Labels: map[string]string{
					"provider": "fabric8",
				},
				Annotations: map[string]string{
					"fabric8.io/generated-by": "exposecontroller",
				},
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: "v1",
					Kind:       "Service",
					Name:       "svc2",
				}},
			},
		},
	}

	client := fake.NewSimpleClientset(objects...)
	strategy := IngressStrategy{
		client: client,
		existing: map[string][]string{
			"ns1/svc1": []string{
				"ingress1",
				"ingress2",
				"ingress3",
				"ingress4",
				"ingress5",
			},
			"ns2/svc2": []string{
				"ingress7",
			},
			"ns3/svc3": []string{
				"other",
			},
		},
	}

	err := strategy.Clean(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns1",
			Name: "svc1",
			Annotations: map[string]string{
				ExposeAnnotationKey: "url",
			},
		},
	})
	assert.NoError(t, err, "clean and patch svc1")
	err = strategy.Clean(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns2",
			Name: "svc2",
		},
	})
	assert.NoError(t, err, "clean svc2")

	list, err := client.ExtensionsV1beta1().Ingresses("").List(metav1.ListOptions{})
	if assert.NoError(t, err, "get ingresses") {
		found := map[string]bool{}
		for _, ingress := range list.Items {
			found[ingress.Name] = true
		}
		expectedF := map[string]bool{
			"ingress2": true,
			"ingress4": true,
			"ingress6": true,
		}
		assert.Equal(t, expectedF, found, "ingresses")
	}
	expectedE := map[string][]string{
		"ns3/svc3": []string{
			"other",
		},
	}
	assert.Equal(t, expectedE, strategy.existing, "strategy.existing")
}

func TestIngressStrategy_IngressTLSAcme(t *testing.T) {
	service := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "main",
			Name: "my-service",
			Annotations: map[string]string {
				ExposeAnnotation.Key: ExposeAnnotation.Value,
			},
			ResourceVersion: "1",
			UID: "my-service-uid",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Port: 123,
			}, {
				Port: 456,
			}, {
				Port: 789,
			}},
		},
	}
	client := fake.NewSimpleClientset(service)

	strategy, err := NewIngressStrategy(client, &Config{
		Exposer:        "ingress",
		Namespace:      "main",
		NamePrefix:     "prefix",
		Domain:         "my-domain.com",
		InternalDomain: "my-internal-domain.com",
		URLTemplate:    "{{.Service}}-{{.Namespace}}.{{.Domain}}",
		TLSAcme:        true,
		IngressClass:   "myIngressClass",
	})
	require.NoError(t, err)
	err = strategy.Sync()
	require.NoError(t, err)
	err = strategy.Add(service)
	require.NoError(t, err)

	service, err = client.CoreV1().Services("main").Get("my-service", metav1.GetOptions{})
	if assert.NoError(t, err, "get service") {
		expectedS := &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "main",
				Name: "my-service",
				Annotations: map[string]string {
					ExposeAnnotation.Key: ExposeAnnotation.Value,
					ExposeAnnotationKey: "https://my-service-main.my-domain.com",
				},
				ResourceVersion: "1",
				UID: "my-service-uid",
			},
			Spec: v1.ServiceSpec{
				Ports: []v1.ServicePort{{
					Port: 123,
				}, {
					Port: 456,
				}, {
					Port: 789,
				}},
			},
		}
		assert.Equalf(t, expectedS, service, "service")
	}

	ingress, err := client.ExtensionsV1beta1().Ingresses("main").Get("prefix-my-service", metav1.GetOptions{})
	if assert.NoError(t, err, "get ingress") {
		expectedI := &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "main",
				Name: "prefix-my-service",
				Labels:      map[string]string{
					"provider": "fabric8",
				},
				Annotations: map[string]string {
					"fabric8.io/generated-by": "exposecontroller",
					"kubernetes.io/ingress.class": "myIngressClass",
					"nginx.ingress.kubernetes.io/ingress.class": "myIngressClass",
					"kubernetes.io/tls-acme": "true",
				},
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: "v1",
					Kind:       "Service",
					Name:       "my-service",
					UID:        "my-service-uid",
				}},
			},
			Spec: v1beta1.IngressSpec{
				Rules: []v1beta1.IngressRule{{
					Host: "my-service-main.my-domain.com",
					IngressRuleValue: v1beta1.IngressRuleValue{
						HTTP: &v1beta1.HTTPIngressRuleValue{
							Paths: []v1beta1.HTTPIngressPath{{
								Backend: v1beta1.IngressBackend{
									ServiceName: "my-service",
									ServicePort: intstr.FromInt(123),
								},
								Path: "",
							}},
						},
					},
				}},
				TLS: []v1beta1.IngressTLS{{
					Hosts:      []string{"my-service-main.my-domain.com"},
					SecretName: "tls-my-service",
				}},
			},
		}
		assert.Equalf(t, expectedI, ingress, "ingress")
	}
}

func TestIngressStrategy_IngressTLSSecretName(t *testing.T) {
	service := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "main",
			Name: "my-service",
			Labels: map[string]string {
				"release": "my",
			},
			Annotations: map[string]string {
				ExposeAnnotation.Key: ExposeAnnotation.Value,
			},
			ResourceVersion: "1",
			UID: "my-service-uid",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Port: 123,
			}, {
				Port: 456,
			}, {
				Port: 789,
			}},
		},
	}
	client := fake.NewSimpleClientset(service)

	strategy, err := NewIngressStrategy(client, &Config{
		Exposer:        "ingress",
		Namespace:      "main",
		Domain:         "my-domain.com",
		InternalDomain: "my-internal-domain.com",
		URLTemplate:    "{{.Service}}.{{.Namespace}}.{{.Domain}}",
		TLSSecretName:  "my-tls-secret",
		TLSUseWildcard: true,
		PathMode:       PathModeUsePath,
	})
	require.NoError(t, err)
	err = strategy.Sync()
	require.NoError(t, err)
	err = strategy.Add(service)
	require.NoError(t, err)

	service, err = client.CoreV1().Services("main").Get("my-service", metav1.GetOptions{})
	if assert.NoError(t, err, "get service") {
		expectedS := &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "main",
				Name: "my-service",
				Labels: map[string]string {
					"release": "my",
				},
				Annotations: map[string]string {
					ExposeAnnotation.Key: ExposeAnnotation.Value,
					ExposeAnnotationKey: "https://my-domain.com/main/service/",
				},
				ResourceVersion: "1",
				UID: "my-service-uid",
			},
			Spec: v1.ServiceSpec{
				Ports: []v1.ServicePort{{
					Port: 123,
				}, {
					Port: 456,
				}, {
					Port: 789,
				}},
			},
		}
		assert.Equalf(t, expectedS, service, "service")
	}

	ingress, err := client.ExtensionsV1beta1().Ingresses("main").Get("service", metav1.GetOptions{})
	if assert.NoError(t, err, "get ingress") {
		expectedI := &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "main",
				Name: "service",
				Labels:      map[string]string{
					"provider": "fabric8",
				},
				Annotations: map[string]string {
					"fabric8.io/generated-by": "exposecontroller",
					"kubernetes.io/ingress.class": "nginx",
					"nginx.ingress.kubernetes.io/ingress.class": "nginx",
				},
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: "v1",
					Kind:       "Service",
					Name:       "my-service",
					UID:        "my-service-uid",
				}},
			},
			Spec: v1beta1.IngressSpec{
				Rules: []v1beta1.IngressRule{{
					Host: "my-domain.com",
					IngressRuleValue: v1beta1.IngressRuleValue{
						HTTP: &v1beta1.HTTPIngressRuleValue{
							Paths: []v1beta1.HTTPIngressPath{{
								Backend: v1beta1.IngressBackend{
									ServiceName: "my-service",
									ServicePort: intstr.FromInt(123),
								},
								Path: "/main/service/",
							}},
						},
					},
				}},
				TLS: []v1beta1.IngressTLS{{
					Hosts:      []string{"*.my-domain.com"},
					SecretName: "my-tls-secret",
				}},
			},
		}
		assert.Equalf(t, expectedI, ingress, "ingress")
	}
}

const testIngressAnnotations = `
sentence:  sentence with spaces
  # ignored comment
quoted:    " quoted sentence "
multiline: |-
  multi line
  sentence

fabric8.io/generated-by: other

kubernetes.io/ingress.class: other
`

func TestIngressStrategy_IngressAnnotations(t *testing.T) {
	service := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "main",
			Name: "my-service",
			Annotations: map[string]string {
				ExposeAnnotation.Key: ExposeAnnotation.Value,
				"fabric8.io/ingress.name": "my-ingress",
				"fabric8.io/host.name": "my-hostname",
				"fabric8.io/use.internal.domain": "true",
				"fabric8.io/ingress.path": "my/path",
				"fabric8.io/path.mode": "other",
				ExposePortAnnotationKey: "456",
				ExposeHostNameAsAnnotationKey: "my-exposed-hostname",
				"fabric8.io/ingress.annotations": testIngressAnnotations,
			},
			ResourceVersion: "1",
			UID: "my-service-uid",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Port: 123,
			}, {
				Port: 456,
			}, {
				Port: 789,
			}},
		},
	}
	client := fake.NewSimpleClientset(service)

	strategy, err := NewIngressStrategy(client, &Config{
		Exposer:        "ingress",
		Namespace:      "main",
		Domain:         "my-domain.com",
		InternalDomain: "my-internal-domain.com",
		URLTemplate:    "{{.Namespace}}.{{.Service}}.{{.Domain}}",
		PathMode:       PathModeUsePath,
		IngressClass:   "my-class",
	})
	require.NoError(t, err)
	err = strategy.Sync()
	require.NoError(t, err)
	err = strategy.Add(service)
	require.NoError(t, err)

	service, err = client.CoreV1().Services("main").Get("my-service", metav1.GetOptions{})
	if assert.NoError(t, err, "get service") {
		expectedS := &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "main",
				Name: "my-service",
				Annotations: map[string]string {
					ExposeAnnotation.Key: ExposeAnnotation.Value,
					"fabric8.io/ingress.name": "my-ingress",
					"fabric8.io/host.name": "my-hostname",
					"fabric8.io/use.internal.domain": "true",
					"fabric8.io/ingress.path": "my/path",
					"fabric8.io/path.mode": "other",
					ExposePortAnnotationKey: "456",
					ExposeHostNameAsAnnotationKey: "my-exposed-hostname",
					"fabric8.io/ingress.annotations": testIngressAnnotations,

					"my-exposed-hostname": "main.my-hostname.my-internal-domain.com",
					ExposeAnnotationKey: "http://main.my-hostname.my-internal-domain.com/my/path",
				},
				ResourceVersion: "1",
				UID: "my-service-uid",
			},
			Spec: v1.ServiceSpec{
				Ports: []v1.ServicePort{{
					Port: 123,
				}, {
					Port: 456,
				}, {
					Port: 789,
				}},
			},
		}
		assert.Equalf(t, expectedS, service, "service")
	}

	ingress, err := client.ExtensionsV1beta1().Ingresses("main").Get("my-ingress", metav1.GetOptions{})
	if assert.NoError(t, err, "get ingress") {
		expectedI := &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "main",
				Name: "my-ingress",
				Labels:      map[string]string{
					"provider": "fabric8",
				},
				Annotations: map[string]string {
					"fabric8.io/generated-by": "exposecontroller",
					"kubernetes.io/ingress.class": "other",
					"nginx.ingress.kubernetes.io/ingress.class": "my-class",
					"sentence": "sentence with spaces",
					"quoted": " quoted sentence ",
					"multiline": "multi line\nsentence",
				},
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: "v1",
					Kind:       "Service",
					Name:       "my-service",
					UID:        "my-service-uid",
				}},
			},
			Spec: v1beta1.IngressSpec{
				Rules: []v1beta1.IngressRule{{
					Host: "main.my-hostname.my-internal-domain.com",
					IngressRuleValue: v1beta1.IngressRuleValue{
						HTTP: &v1beta1.HTTPIngressRuleValue{
							Paths: []v1beta1.HTTPIngressPath{{
								Backend: v1beta1.IngressBackend{
									ServiceName: "my-service",
									ServicePort: intstr.FromInt(456),
								},
								Path: "/my/path",
							}},
						},
					},
				}},
			},
		}
		assert.Equalf(t, expectedI, ingress, "ingress")
	}
}

func TestIngressStrategy_update(t *testing.T) {
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name: "svc",
			Annotations: map[string]string {
				ExposeAnnotation.Key: ExposeAnnotation.Value,
			},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Port: 1234,
			}, {
				Port: 5678,
			}},
		},
	}
	client := fake.NewSimpleClientset(svc)
	strategy, err := NewIngressStrategy(client, &Config{
		Exposer:        "ingress",
		Namespace:      "main",
		Domain:         "my-domain.com",
		URLTemplate:    "{{.Service}}.{{.Namespace}}.{{.Domain}}",
	})
	require.NoError(t, err)
	require.NoError(t, strategy.Sync())

	err = strategy.Add(svc.DeepCopy())
	require.NoError(t, err)

	expected := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name: "svc",
			Labels: map[string]string{
				"provider": "fabric8",
			},
			Annotations: map[string]string{
				"fabric8.io/generated-by": "exposecontroller",
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "v1",
				Kind:       "Service",
				Name:       "svc",
			}},
		},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{{
				Host: "svc.ns.my-domain.com",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Backend: v1beta1.IngressBackend{
								ServiceName: "svc",
								ServicePort: intstr.FromInt(1234),
							},
							Path: "",
						}},
					},
				},
			}},
		},
	}

	ingresses, err := client.ExtensionsV1beta1().Ingresses("ns").List(metav1.ListOptions{})
	require.NoError(t, err)
	if assert.Equal(t, 1, len(ingresses.Items)) {
		assert.Equal(t, expected, &ingresses.Items[0])
	}

	expected.ResourceVersion = "1"
	expected.UID = "test"
	client.ExtensionsV1beta1().Ingresses("ns").Update(expected.DeepCopy())
	err = strategy.Add(svc.DeepCopy())
	require.NoError(t, err)
	ingress, err := client.ExtensionsV1beta1().Ingresses("ns").Get(expected.Name, metav1.GetOptions{})
	if assert.NoError(t, err) {
		assert.Equal(t, expected, ingress)
	}

	svc = &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name: "svc",
			Annotations: map[string]string {
				ExposeAnnotation.Key: ExposeAnnotation.Value,
				ExposePortAnnotationKey: "5678",
			},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Port: 1234,
			}, {
				Port: 5678,
			}},
		},
	}
	err = strategy.Add(svc.DeepCopy())
	require.NoError(t, err)

	ingresses, err = client.ExtensionsV1beta1().Ingresses("ns").List(metav1.ListOptions{})
	require.NoError(t, err)
	expected = &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name: "svc",
			Labels: map[string]string{
				"provider": "fabric8",
			},
			Annotations: map[string]string{
				"fabric8.io/generated-by": "exposecontroller",
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "v1",
				Kind:       "Service",
				Name:       "svc",
			}},
			ResourceVersion: "1",
		},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{{
				Host: "svc.ns.my-domain.com",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Backend: v1beta1.IngressBackend{
								ServiceName: "svc",
								ServicePort: intstr.FromInt(5678),
							},
							Path: "",
						}},
					},
				},
			}},
		},
	}
	if assert.Equal(t, 1, len(ingresses.Items)) {
		assert.Equal(t, expected, &ingresses.Items[0])
	}

	expected.ResourceVersion = "2"
	expected.UID = "test"
	client.ExtensionsV1beta1().Ingresses("ns").Update(expected.DeepCopy())
	err = strategy.Add(svc.DeepCopy())
	require.NoError(t, err)
	ingress, err = client.ExtensionsV1beta1().Ingresses("ns").Get(expected.Name, metav1.GetOptions{})
	if assert.NoError(t, err) {
		assert.Equal(t, expected, ingress)
	}

	svc = &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name: "svc",
			Annotations: map[string]string {
				ExposeAnnotation.Key: ExposeAnnotation.Value,
				ExposePortAnnotationKey: "5678",
				"fabric8.io/ingress.name": "ingress",
			},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Port: 1234,
			}, {
				Port: 5678,
			}},
		},
	}
	err = strategy.Add(svc.DeepCopy())
	require.NoError(t, err)

	ingresses, err = client.ExtensionsV1beta1().Ingresses("ns").List(metav1.ListOptions{})
	require.NoError(t, err)
	expected = &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name: "ingress",
			Labels: map[string]string{
				"provider": "fabric8",
			},
			Annotations: map[string]string{
				"fabric8.io/generated-by": "exposecontroller",
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "v1",
				Kind:       "Service",
				Name:       "svc",
			}},
		},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{{
				Host: "ingress.ns.my-domain.com",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Backend: v1beta1.IngressBackend{
								ServiceName: "svc",
								ServicePort: intstr.FromInt(5678),
							},
							Path: "",
						}},
					},
				},
			}},
		},
	}
	if assert.Equal(t, 1, len(ingresses.Items)) {
		assert.Equal(t, expected, &ingresses.Items[0])
	}

	expected.ResourceVersion = "3"
	expected.UID = "test"
	client.ExtensionsV1beta1().Ingresses("ns").Update(expected.DeepCopy())
	err = strategy.Add(svc.DeepCopy())
	require.NoError(t, err)
	ingress, err = client.ExtensionsV1beta1().Ingresses("ns").Get(expected.Name, metav1.GetOptions{})
	if assert.NoError(t, err) {
		assert.Equal(t, expected, ingress)
	}

	svc = &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name: "svc",
			Annotations: map[string]string {
				ExposeAnnotation.Key: ExposeAnnotation.Value,
				ExposePortAnnotationKey: "5678",
				"fabric8.io/ingress.name": "ingress",
				"fabric8.io/ingress.annotations": "custom: \"true\"\n",
			},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Port: 1234,
			}, {
				Port: 5678,
			}},
		},
	}
	err = strategy.Add(svc.DeepCopy())
	require.NoError(t, err)

	ingresses, err = client.ExtensionsV1beta1().Ingresses("ns").List(metav1.ListOptions{})
	require.NoError(t, err)
	expected = &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name: "ingress",
			Labels: map[string]string{
				"provider": "fabric8",
			},
			Annotations: map[string]string{
				"fabric8.io/generated-by": "exposecontroller",
				"custom": "true",
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "v1",
				Kind:       "Service",
				Name:       "svc",
			}},
			ResourceVersion: "3",
		},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{{
				Host: "ingress.ns.my-domain.com",
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Backend: v1beta1.IngressBackend{
								ServiceName: "svc",
								ServicePort: intstr.FromInt(5678),
							},
							Path: "",
						}},
					},
				},
			}},
		},
	}
	if assert.Equal(t, 1, len(ingresses.Items)) {
		assert.Equal(t, expected, &ingresses.Items[0])
	}

	expected.ResourceVersion = "4"
	expected.UID = "test"
	client.ExtensionsV1beta1().Ingresses("ns").Update(expected.DeepCopy())
	err = strategy.Add(svc.DeepCopy())
	require.NoError(t, err)
	ingress, err = client.ExtensionsV1beta1().Ingresses("ns").Get(expected.Name, metav1.GetOptions{})
	if assert.NoError(t, err) {
		assert.Equal(t, expected, ingress)
	}

	svc = &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name: "svc",
			Annotations: map[string]string {
				ExposePortAnnotationKey: "5678",
				"fabric8.io/ingress.name": "ingress",
				"fabric8.io/ingress.annotations": "custom: \"true\"\n",
			},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Port: 1234,
			}, {
				Port: 5678,
			}},
		},
	}
	err = strategy.Clean(svc.DeepCopy())
	require.NoError(t, err)

	ingresses, err = client.ExtensionsV1beta1().Ingresses("ns").List(metav1.ListOptions{})
	require.NoError(t, err)
	assert.Equal(t, 0, len(ingresses.Items))
}
