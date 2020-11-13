package exposestrategy

import (
	"fmt"
	"net"
	"strconv"

	"github.com/pkg/errors"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type NodePortStrategy struct {
	client  kubernetes.Interface

	nodeIP string
	todo   map[string]bool
}

const ExternalIPLabel = "fabric8.io/externalIP"

func NewNodePortStrategy(client kubernetes.Interface, config *ExposeStrategyConfig) (ExposeStrategy, error) {
	ip := config.NodeIP
	if len(ip) == 0 {
		l, err := client.CoreV1().Nodes().List(metav1.ListOptions{})
		if err != nil {
			return nil, errors.Wrap(err, "failed to list nodes")
		}

		if len(l.Items) != 1 {
			return nil, errors.Errorf("node port strategy can only be used with single node clusters - found %d nodes", len(l.Items))
		}

		n := l.Items[0]
		ip = n.ObjectMeta.Labels[ExternalIPLabel]
		if len(ip) == 0 {
			addr, err := getNodeHostIP(n)
			if err != nil {
				return nil, errors.Wrap(err, "cannot discover node IP")
			}
			ip = addr.String()
		}
	}

	return &NodePortStrategy{
		client:  client,
		nodeIP:  ip,
	}, nil
}

// getNodeHostIP returns the provided node's IP, based on the priority:
// 1. NodeExternalIP
// 2. NodeInternalIP
func getNodeHostIP(node v1.Node) (net.IP, error) {
	addresses := node.Status.Addresses
	addressMap := make(map[v1.NodeAddressType][]v1.NodeAddress)
	for i := range addresses {
		addressMap[addresses[i].Type] = append(addressMap[addresses[i].Type], addresses[i])
	}
	if addresses, ok := addressMap[v1.NodeExternalIP]; ok {
		return net.ParseIP(addresses[0].Address), nil
	}
	if addresses, ok := addressMap[v1.NodeInternalIP]; ok {
		return net.ParseIP(addresses[0].Address), nil
	}
	return nil, fmt.Errorf("host IP unknown; known addresses: %v", addresses)
}

func (s *NodePortStrategy) Sync() error {
	s.todo = map[string]bool{}
	return nil
}

func (s *NodePortStrategy) HasSynced() bool {
	return len(s.todo) == 0
}

func (s *NodePortStrategy) Add(svc *v1.Service) error {
	delete(s.todo, fmt.Sprintf("%s/%s", svc.Namespace, svc.Name))

	var err error
	clone := svc.DeepCopy()
	clone.Spec.Type = v1.ServiceTypeNodePort
	clone.Spec.ExternalIPs = nil

	if len(svc.Spec.Ports) == 0 {
		return errors.Errorf(
			"service %s/%s has no ports specified. Node port strategy requires a node port",
			svc.Namespace, svc.Name,
		)
	}

	if len(svc.Spec.Ports) > 1 {
		return errors.Errorf(
			"service %s/%s has multiple ports specified (%v). Node port strategy can only be used with single port services",
			svc.Namespace, svc.Name, svc.Spec.Ports,
		)
	}

	port := svc.Spec.Ports[0]
	portInt := int(port.NodePort)
	if portInt > 0 {
		nodePort := strconv.Itoa(portInt)
		hostName := net.JoinHostPort(s.nodeIP, nodePort)
		err = addServiceAnnotation(clone, hostName)
	} else {
		s.todo[fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)] = true
		err = addServiceAnnotation(clone, "")
	}
	if err != nil {
		return errors.Wrap(err, "failed to add service annotation")
	}
	patch, err := createServicePatch(svc, clone)
	if err != nil {
		return errors.Wrap(err, "failed to create patch")
	}
	if patch != nil {
		_, err = s.client.CoreV1().Services(svc.Namespace).
			Patch(svc.Name, patchType, patch)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("failed to send patch for %s/%s patch %s", svc.Namespace, svc.Name, string(patch)))
		}
	}

	if portInt <= 0 {
		s.todo[fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)] = true
	}
	return nil
}

func (s *NodePortStrategy) Clean(svc *v1.Service) error {
	delete(s.todo, fmt.Sprintf("%s/%s", svc.Namespace, svc.Name))
	clone := svc.DeepCopy()
	if !removeServiceAnnotation(clone) {
		return nil
	}
	clone.Spec.Type = v1.ServiceTypeClusterIP

	patch, err := createServicePatch(svc, clone)
	if err != nil {
		return errors.Wrap(err, "failed to create patch")
	}
	if patch != nil {
		_, err = s.client.CoreV1().Services(clone.Namespace).
			Patch(clone.Name, patchType, patch)
		if err != nil {
			return errors.Wrap(err, "failed to send patch")
		}
	}

	return nil
}

func (s *NodePortStrategy) Delete(svc *v1.Service) error {
	delete(s.todo, fmt.Sprintf("%s/%s", svc.Namespace, svc.Name))

	return nil
}
