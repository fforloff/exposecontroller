package exposestrategy

import (
	"testing"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/stretchr/testify/assert"
)

func TestAmbassadorStrategy_Add(t *testing.T) {
	examples := []struct{
		name     string
		config   ExposeStrategyConfig
		svc      v1.Service
		expected map[string]string
	}{{
		name:     "simple",
		config:   ExposeStrategyConfig{
			Domain:      "my-domain.com",
			URLTemplate: "{{.Service}}.{{.Namespace}}.{{.Domain}}",
		},
		svc:      v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "main",
				Name:      "svc",
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
		},
		expected: map[string]string{
			ExposeAnnotationKey: "http://svc.main.my-domain.com",
			"getambassador.io/config":
`apiVersion: ambassador/v1
kind: Mapping
host: svc.main.my-domain.com
name: svc.main.my-domain.com_main_mapping
service: svc.main:123
prefix: /
---`,
		},
	}, {
		name:     "TLSAcme",
		config:   ExposeStrategyConfig{
			Domain:      "my-domain.com",
			URLTemplate: "{{.Service}}.{{.Namespace}}.{{.Domain}}",
			NamePrefix:  "my-prefix",
			TLSAcme:     true,
			PathMode:    PathModeUsePath,
		},
		svc:      v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:   "main",
				Name:        "my-prefix-svc",
				Annotations: map[string]string{
					ExposePortAnnotationKey: "456",
				},
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
		},
		expected: map[string]string{
			ExposeAnnotationKey: "https://my-domain.com/main/svc",
			"getambassador.io/config":
`apiVersion: ambassador/v1
kind: Mapping
host: my-domain.com
name: my-domain.com_main_mapping
service: my-prefix-svc.main:456
prefix: /main/svc
---
apiVersion: ambassador/v1
kind: Module
name: tls
config:
  server:
    enabled: True
    secret: tls-svc
---`,
		},
	}, {
		name:     "TLSSecretName",
		config:   ExposeStrategyConfig{
			Domain:        "my-domain.com",
			URLTemplate:   "{{.Service}}-{{.Namespace}}.{{.Domain}}",
			NamePrefix:    "my-prefix",
			TLSSecretName: "my-tls-secret",
		},
		svc:      v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "main",
				Name:      "svc",
				Annotations: map[string]string{
					"fabric8.io/ingress.path": "/my/path",
				},
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
		},
		expected: map[string]string{
			ExposeAnnotationKey: "https://svc.main.my-domain.com/my/path",
			"getambassador.io/config":
`apiVersion: ambassador/v1
kind: Mapping
host: my-domain.com
name: svc.main.my-domain.com
service: svc.main:123
prefix: /my/path
---
apiVersion: ambassador/v1
kind: Module
name: tls
config:
  server:
    enabled: True
    secret: my-tls-secret
---`,
		},
	}}
	for _, example := range examples {
		client := fake.NewSimpleClientset(example.svc.DeepCopy())
		strategy, err := NewAmbassadorStrategy(client, &example.config)
		if assert.NoError(t, err, example.name) {
			continue
		}

		err = strategy.Add(example.svc.DeepCopy())
		if assert.NoError(t, err, example.name) {
			continue
		}

		svc, err := client.CoreV1().Services(example.svc.Namespace).Get(example.svc.Name, metav1.GetOptions{})
		if assert.NoError(t, err, example.name) {
			continue
		}

		expected := example.svc.DeepCopy()
		expected.Annotations = example.expected
		assert.Equal(t, expected, svc, example.name)
	}
}

func TestAmbassadorStrategy_Clean(t *testing.T) {
	examples := []struct{
		name     string
		svc      v1.Service
		expected map[string]string
	}{{
		name:     "to clean",
		svc:      v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "main",
				Name:      "svc",
				Annotations: map[string]string{
					ExposeAnnotationKey:       "http://svc.main.my-domain.com",
					"getambassador.io/config": "anything",
				},
			},
		},
		expected: map[string]string{},
	}, {
		name:     "to skip",
		svc:      v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "main",
				Name:      "svc",
				Annotations: map[string]string{
					"getambassador.io/config": "anything",
				},
			},
		},
		expected: map[string]string{
			"getambassador.io/config": "anything",
		},
	}}
	for _, example := range examples {
		client := fake.NewSimpleClientset(example.svc.DeepCopy())
		svc := example.svc.DeepCopy()
		svc.Annotations["test"] = "test"
		strategy := &AmbassadorStrategy{client: client}

		err := strategy.Clean(example.svc.DeepCopy())
		if assert.NoError(t, err, example.name) {
			continue
		}

		svc, err = client.CoreV1().Services(example.svc.Namespace).Get(example.svc.Name, metav1.GetOptions{})
		if assert.NoError(t, err, example.name) {
			continue
		}

		expected := example.svc.DeepCopy()
		expected.Annotations = example.expected
		expected.Annotations["test"] = "test"
		assert.Equal(t, expected, svc, example.name)
	}
}
