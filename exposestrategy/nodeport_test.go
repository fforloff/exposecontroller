package exposestrategy

import (
	"testing"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNodePortStrategy_NodeIP(t *testing.T) {
	client := fake.NewSimpleClientset(&v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-node",
			Labels: map[string]string{
				ExternalIPLabel: "my-external-ip",
			},
		},
	})
	strategy, err := NewNodePortStrategy(client, &ExposeStrategyConfig{})
	if assert.NoError(t, err) {
		assert.Equal(t, "my-external-ip", strategy.(*NodePortStrategy).nodeIP)
	}
	strategy, err = NewNodePortStrategy(client, &ExposeStrategyConfig{
		NodeIP: "my-node-ip",
	})
	if assert.NoError(t, err) {
		assert.Equal(t, "my-node-ip", strategy.(*NodePortStrategy).nodeIP)
	}

	client = fake.NewSimpleClientset(&v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-node",
		},
		Status: v1.NodeStatus{
			Addresses: []v1.NodeAddress{{
				Type:    v1.NodeInternalIP,
				Address: "192.168.1.100",
			}, {
				Type:    v1.NodeExternalIP,
				Address: "192.168.1.200",
			}},
		},
	})
	strategy, err = NewNodePortStrategy(client, &ExposeStrategyConfig{})
	if assert.NoError(t, err) {
		assert.Equal(t, "192.168.1.200", strategy.(*NodePortStrategy).nodeIP)
	}
	strategy, err = NewNodePortStrategy(client, &ExposeStrategyConfig{
		NodeIP: "my-node-ip",
	})
	if assert.NoError(t, err) {
		assert.Equal(t, "my-node-ip", strategy.(*NodePortStrategy).nodeIP)
	}

	client = fake.NewSimpleClientset(&v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-node",
		},
		Status: v1.NodeStatus{
			Addresses: []v1.NodeAddress{{
				Type:    v1.NodeInternalIP,
				Address: "192.168.1.100",
			}},
		},
	})
	strategy, err = NewNodePortStrategy(client, &ExposeStrategyConfig{})
	if assert.NoError(t, err) {
		assert.Equal(t, "192.168.1.100", strategy.(*NodePortStrategy).nodeIP)
	}
	strategy, err = NewNodePortStrategy(client, &ExposeStrategyConfig{
		NodeIP: "my-node-ip",
	})
	if assert.NoError(t, err) {
		assert.Equal(t, "my-node-ip", strategy.(*NodePortStrategy).nodeIP)
	}

	client = fake.NewSimpleClientset(&v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-node",
		},
	})
	strategy, err = NewNodePortStrategy(client, &ExposeStrategyConfig{})
	assert.Error(t, err)
	strategy, err = NewNodePortStrategy(client, &ExposeStrategyConfig{
		NodeIP: "my-node-ip",
	})
	if assert.NoError(t, err) {
		assert.Equal(t, "my-node-ip", strategy.(*NodePortStrategy).nodeIP)
	}
}

func TestNodePortStrategy_Add(t *testing.T) {
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
			Ports: []v1.ServicePort{{
				Port: 1234,
			}},
		},
	})
	strategy, err := NewNodePortStrategy(client, &ExposeStrategyConfig{
		NodeIP: "my-node-ip",
	})
	require.NoError(t, err)
	err = strategy.Add(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "svc",
		},
		Spec: v1.ServiceSpec{
			Type:           v1.ServiceTypeClusterIP,
			Ports: []v1.ServicePort{{
				Port: 1234,
			}},
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
				Type:      v1.ServiceTypeNodePort,
				ClusterIP: "my-cluster-ip",
				Ports: []v1.ServicePort{{
					Port: 1234,
				}},
			},
		}
		assert.Equal(t, expected, svc)
	}
	_, err = client.CoreV1().Services("ns").Update(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   "ns",
			Name:        "svc",
			Annotations: map[string]string{
				"test":              "test",
				ExposeAnnotationKey: "",
			},
		},
		Spec: v1.ServiceSpec{
			Type:      v1.ServiceTypeNodePort,
			ClusterIP: "my-cluster-ip",
			Ports: []v1.ServicePort{{
				Port:     1234,
				NodePort: 5678,
			}},
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
			Type:      v1.ServiceTypeNodePort,
			ClusterIP: "my-cluster-ip",
			Ports: []v1.ServicePort{{
				Port:     1234,
				NodePort: 5678,
			}},
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
					ExposeAnnotationKey: "http://my-node-ip:5678",
				},
			},
			Spec: v1.ServiceSpec{
				Type:           v1.ServiceTypeNodePort,
				ClusterIP:      "my-cluster-ip",
				Ports: []v1.ServicePort{{
					Port: 1234,
					NodePort: 5678,
				}},
			},
		}
		assert.Equal(t, expected, svc)
	}
}

func TestNodePortStrategy_Remove(t *testing.T) {
	svc1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   "ns1",
			Name:        "svc1",
			Annotations: map[string]string{
				"test": "test",
				ExposeAnnotationKey: "http://my-cluster-ip:5678",
			},
		},
		Spec: v1.ServiceSpec{
			Type:           v1.ServiceTypeNodePort,
			Ports: []v1.ServicePort{{
				Port: 1234,
			}},
		},
	}
	svc2 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   "ns2",
			Name:        "svc2",
			Annotations: map[string]string{},
		},
		Spec: v1.ServiceSpec{
			Type:           v1.ServiceTypeNodePort,
			Ports: []v1.ServicePort{{
				Port:     1234,
				NodePort: 5678,
			}},
		},
	}

	client := fake.NewSimpleClientset(svc1.DeepCopy(), svc2.DeepCopy())
	strategy, err := NewNodePortStrategy(client, &ExposeStrategyConfig{
		NodeIP: "my-node-ip",
	})
	require.NoError(t, err)

	err = strategy.Remove(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   "ns1",
			Name:        "svc1",
			Annotations: map[string]string{
				ExposeAnnotationKey: "http://my-cluster-ip:5678",
			},
		},
		Spec: v1.ServiceSpec{
			Type:           v1.ServiceTypeNodePort,
			Ports: []v1.ServicePort{{
				Port: 1234,
			}},
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
				Ports: []v1.ServicePort{{
					Port: 1234,
				}},
			},
		}
		assert.Equal(t, expected, svc, "managed")
	}

	svc, err = client.CoreV1().Services("ns2").Get("svc2", metav1.GetOptions{})
	if assert.NoError(t, err) {
		assert.Equal(t, svc2, svc, "unmanaged")
	}
}
