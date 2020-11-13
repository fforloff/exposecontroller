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
	err = strategy.Sync()
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
	assert.False(t, strategy.HasSynced(), "unsynced")
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
	assert.True(t, strategy.HasSynced(), "synced")
}

func TestLoadBalancerStrategy_Clean(t *testing.T) {
	svc1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   "ns1",
			Name:        "svc1",
			Annotations: map[string]string{
				"test": "test",
				ExposeAnnotationKey: "",
			},
		},
		Spec: v1.ServiceSpec{
			Type: v1.ServiceTypeLoadBalancer,
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

	client := fake.NewSimpleClientset(svc2.DeepCopy())
	strategy, err := NewLoadBalancerStrategy(client, &ExposeStrategyConfig{})
	require.NoError(t, err)
	err = strategy.Sync()
	require.NoError(t, err)

	err = strategy.Add(svc1.DeepCopy())
	assert.NoError(t, err)
	assert.False(t, strategy.HasSynced(), "unsynced")
	// Add it late to be sure it wasn't updated by the strategy
	_, err = client.CoreV1().Services("ns1").Create(svc1.DeepCopy())
	require.NoError(t, err)

	err = strategy.Clean(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   "ns1",
			Name:        "svc1",
			Annotations: map[string]string{
				ExposeAnnotationKey: "",
			},
		},
		Spec: v1.ServiceSpec{
			Type: v1.ServiceTypeLoadBalancer,
		},
	})
	assert.NoError(t, err)
	err = strategy.Clean(svc2.DeepCopy())
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
				Type: v1.ServiceTypeClusterIP,
			},
		}
		assert.Equal(t, expected, svc, "managed")
	}

	svc, err = client.CoreV1().Services("ns2").Get("svc2", metav1.GetOptions{})
	if assert.NoError(t, err) {
		assert.Equal(t, svc2, svc, "unmanaged")
	}

	assert.True(t, strategy.HasSynced(), "synced")
}

func TestLoadBalancerStrategy_Delete(t *testing.T) {
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   "ns",
			Name:        "svc",
			Annotations: map[string]string{
				ExposeAnnotationKey: "",
			},
		},
		Spec: v1.ServiceSpec{
			Type: v1.ServiceTypeLoadBalancer,
		},
	}

	client := fake.NewSimpleClientset()
	strategy, err := NewLoadBalancerStrategy(client, &ExposeStrategyConfig{})
	require.NoError(t, err)
	err = strategy.Sync()
	require.NoError(t, err)

	err = strategy.Add(svc.DeepCopy())
	assert.NoError(t, err)
	assert.False(t, strategy.HasSynced(), "unsynced")
	// Add it late to be sure it wasn't updated by the strategy
	_, err = client.CoreV1().Services("ns").Create(svc.DeepCopy())
	require.NoError(t, err)

	err = client.CoreV1().Services(svc.Namespace).Delete(svc.Name, &metav1.DeleteOptions{})
	require.NoError(t, err)
	err = strategy.Delete(svc.DeepCopy())
	require.NoError(t, err)
	assert.True(t, strategy.HasSynced(), "usynced")
}
