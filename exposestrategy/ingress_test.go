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
				Kind:       "Deployment",
				APIVersion: "extensions/v1beta1",
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
				Kind:       "Service",
				APIVersion: "v1",
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
				Kind:       "Service",
				APIVersion: "v1",
				Name:       "test-service-1",
			}, {
				Kind:       "Service",
				APIVersion: "v1",
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
				Kind:       "Service",
				APIVersion: "v1",
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
				Kind:       "Service",
				APIVersion: "v1",
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
				Kind:       "Service",
				APIVersion: "v1",
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
				Kind:       "Service",
				APIVersion: "v1",
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
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
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
				Kind:       "Service",
				APIVersion: "v1",
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
				Kind:       "Service",
				APIVersion: "v1",
				Name:       "other",
			}, {
				Kind:       "Service",
				APIVersion: "v1",
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
				Kind:       "Service",
				APIVersion: "v1",
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
					Kind:       "Service",
					APIVersion: "v1",
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
			TypeMeta: metav1.TypeMeta{
				Kind:       "Service",
				APIVersion: "v1",
			},
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

func TestIngressStrategy_IngressTLS(t *testing.T) {
	service := &v1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
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

	strategy := IngressStrategy{
		client:         client,
		namespace:      "main",
		namePrefix:     "prefix",
		domain:         "my-domain.com",
		internalDomain: "my-internal-domain.com",
		urltemplate:    "%[1]s-%[2]s.%[3]s",
		tlsAcme:        true,
		ingressClass:   "myIngressClass",
		existing:       map[string][]string{},
	}
	err := strategy.Add(service)
	require.NoError(t, err)

	service, err = client.CoreV1().Services("main").Get("my-service", metav1.GetOptions{})
	if assert.NoError(t, err, "get service") {
		expectedS := &v1.Service{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Service",
				APIVersion: "v1",
			},
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
					Kind:       "Service",
					APIVersion: "v1",
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
