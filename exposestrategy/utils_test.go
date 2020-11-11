package exposestrategy

import (
	"testing"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
)

func TestAddServiceAnnotationWithProtocol(t *testing.T) {
	tests := []struct {
		name                string
		svc                 *v1.Service
		hostName            string
		path                string
		protocol            string
		expectedAnnotations map[string]string
	}{
		{
			name:     "http",
			svc:      &v1.Service{},
			hostName: "example.com",
			protocol: "http",
			expectedAnnotations: map[string]string{
				ExposeAnnotationKey: "http://example.com",
			},
		},
		{
			name:     "https",
			svc:      &v1.Service{},
			hostName: "example.com",
			protocol: "https",
			expectedAnnotations: map[string]string{
				ExposeAnnotationKey: "https://example.com",
			},
		},
		{
			name:     "path",
			svc:      &v1.Service{},
			hostName: "example.com",
			path:     "some/path",
			protocol: "http",
			expectedAnnotations: map[string]string{
				ExposeAnnotationKey: "http://example.com/some/path",
			},
		},
		{
			name:     "full",
			svc: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						ApiServicePathAnnotationKey: "some/path",
					},
				},
			},
			hostName: "example.com",
			path:     "other/path",
			protocol: "https",
			expectedAnnotations: map[string]string{
				ApiServicePathAnnotationKey: "some/path",
				ExposeAnnotationKey:         "https://example.com/some/path",
			},
		},
		{
			name:     "osiris.deislabs.io",
			svc: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						ExposeHostNameAsAnnotationKey: "osiris.deislabs.io/ingressHostname",
					},
				},
			},
			hostName: "example.com",
			protocol: "http",
			expectedAnnotations: map[string]string{
				ExposeHostNameAsAnnotationKey:        "osiris.deislabs.io/ingressHostname",
				"osiris.deislabs.io/ingressHostname": "example.com",
				ExposeAnnotationKey:                  "http://example.com",
			},
		},
	}

	for _, test := range tests {
		err := addServiceAnnotationWithProtocol(test.svc, test.hostName, test.path, test.protocol)
		assert.NoError(t, err, test.name)
		assert.Equal(t, test.expectedAnnotations, test.svc.Annotations, test.name)
	}
}

func TestRemoveServiceAnnotation(t *testing.T) {
	tests := []struct {
		name                string
		svc                 *v1.Service
		expectedAnnotations map[string]string
		ok                  bool
	}{
		{
			name:                "empty",
			svc:                 &v1.Service{},
			expectedAnnotations: nil,
			ok:                  false,
		},
		{
			name: "expose annotation",
			svc: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						ExposeAnnotationKey: "http://example.com",
						"some-key":          "some value",
					},
				},
			},
			expectedAnnotations: map[string]string{
				"some-key": "some value",
			},
			ok:                  true,
		},
		{
			name: "expose hostname annotation",
			svc: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						ExposeHostNameAsAnnotationKey:        "osiris.deislabs.io/ingressHostname",
						"osiris.deislabs.io/ingressHostname": "example.com",
						ApiServicePathAnnotationKey:          "some/path",
						ExposeAnnotationKey:                  "http://example.com/some/path",
					},
				},
			},
			expectedAnnotations: map[string]string{
				ExposeHostNameAsAnnotationKey: "osiris.deislabs.io/ingressHostname",
				ApiServicePathAnnotationKey:   "some/path",
			},
			ok:                  true,
		},
	}

	for _, test := range tests {
		ok := removeServiceAnnotation(test.svc)
		assert.Equal(t, test.ok, ok, test.name)
		assert.Equal(t, test.expectedAnnotations, test.svc.Annotations, test.name)
	}
}
