package exposestrategy

import (
	"testing"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadBalancerStrategy_Add(t *testing.T) {
	client := fake.NewSimpleClientset(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   "ns",
			Name:        "svc",
			Annotations: map[string]string{
				"test": "test",
			},
		},
		Spec: v1.ServiceSpec{
			Type:      v1.ServiceTypeClusterIP,
			ClusterIP: "my-cluster-ip",
		},
	})
	strategy, err := NewLoadBalancerStrategy(client, &ExposeStrategyConfig{})
	require.NoError(t, err)
	err = strategy.Add(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "svc",
		},
	})
	assert.NoError(t, err)
	svc, err := client.CoreV1().Services("ns").Get("svc", metav1.GetOptions{})
	if assert.NoError(t, err) {
		expected := &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:   "ns",
				Name:        "svc",
				Annotations: map[string]string{
					"test":              "test",
					ExposeAnnotationKey: "",
				},
			},
			Spec: v1.ServiceSpec{
				Type:      v1.ServiceTypeLoadBalancer,
				ClusterIP: "my-cluster-ip",
			},
		}
		assert.Equal(t, expected, svc)
	}
	_, err = client.CoreV1().Services("ns").Update(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   "ns",
			Name:        "svc",
			Annotations: map[string]string{
				"test": "test",
				ExposeAnnotationKey: "",
			},
		},
		Spec: v1.ServiceSpec{
			Type:           v1.ServiceTypeLoadBalancer,
			ClusterIP:      "my-cluster-ip",
			LoadBalancerIP: "my-lb-ip",
		},
	})
	require.NoError(t, err)
	err = strategy.Add(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   "ns",
			Name:        "svc",
			Annotations: map[string]string{
				ExposeAnnotationKey: "",
			},
		},
		Spec: v1.ServiceSpec{
			Type:           v1.ServiceTypeLoadBalancer,
			LoadBalancerIP: "my-lb-ip",
		},
	})
	assert.NoError(t, err)
	svc, err = client.CoreV1().Services("ns").Get("svc", metav1.GetOptions{})
	if assert.NoError(t, err) {
		expected := &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:   "ns",
				Name:        "svc",
				Annotations: map[string]string{
					"test":              "test",
					ExposeAnnotationKey: "http://my-lb-ip",
				},
			},
			Spec: v1.ServiceSpec{
				Type:           v1.ServiceTypeLoadBalancer,
				ClusterIP:      "my-cluster-ip",
				LoadBalancerIP: "my-lb-ip",
			},
		}
		assert.Equal(t, expected, svc)
	}
}

func TestLoadBalancerStrategy_Remove(t *testing.T) {
	svc1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   "ns1",
			Name:        "svc1",
			Annotations: map[string]string{
				"test": "test",
				ExposeAnnotationKey: "http://my-lb-ip",
			},
		},
		Spec: v1.ServiceSpec{
			Type:           v1.ServiceTypeLoadBalancer,
			LoadBalancerIP: "my-lb-ip",
		},
	}
	svc2 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   "ns2",
			Name:        "svc2",
			Annotations: map[string]string{},
		},
		Spec: v1.ServiceSpec{
			Type:           v1.ServiceTypeLoadBalancer,
			LoadBalancerIP: "my-lb-ip",
		},
	}

	client := fake.NewSimpleClientset(svc1.DeepCopy(), svc2.DeepCopy())
	strategy, err := NewLoadBalancerStrategy(client, &ExposeStrategyConfig{})
	require.NoError(t, err)

	err = strategy.Remove(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   "ns1",
			Name:        "svc1",
			Annotations: map[string]string{
				ExposeAnnotationKey: "http://my-lb-ip",
			},
		},
		Spec: v1.ServiceSpec{
			Type:           v1.ServiceTypeLoadBalancer,
			LoadBalancerIP: "my-lb-ip",
		},
	})
	assert.NoError(t, err)
	err = strategy.Remove(svc2.DeepCopy())
	assert.NoError(t, err)

	svc, err := client.CoreV1().Services("ns1").Get("svc1", metav1.GetOptions{})
	if assert.NoError(t, err) {
		expected := &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:   "ns1",
				Name:        "svc1",
				Annotations: map[string]string{
					"test": "test",
				},
			},
			Spec: v1.ServiceSpec{
				Type:           v1.ServiceTypeClusterIP,
				LoadBalancerIP: "my-lb-ip",
			},
		}
		assert.Equal(t, expected, svc, "managed")
	}

	svc, err = client.CoreV1().Services("ns2").Get("svc2", metav1.GetOptions{})
	if assert.NoError(t, err) {
		assert.Equal(t, svc2, svc, "unmanaged")
	}
}
