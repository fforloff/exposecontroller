package exposestrategy

import (
	"github.com/pkg/errors"

	"k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

type LoadBalancerStrategy struct {
	client  kubernetes.Interface
}

var _ ExposeStrategy = &LoadBalancerStrategy{}

func NewLoadBalancerStrategy(client kubernetes.Interface) (*LoadBalancerStrategy, error) {
	return &LoadBalancerStrategy{
		client:  client,
	}, nil
}

func (s *LoadBalancerStrategy) Add(svc *v1.Service) error {
	var err error
	clone := svc.DeepCopy()
	clone.Spec.Type = v1.ServiceTypeLoadBalancer
	if len(clone.Spec.LoadBalancerIP) > 0 {
		clone, err = addServiceAnnotation(clone, clone.Spec.LoadBalancerIP)
		if err != nil {
			return errors.Wrap(err, "failed to add service annotation")
		}
	}

	patch, err := createServicePatch(svc, clone)
	if err != nil {
		return errors.Wrap(err, "failed to create patch")
	}
	if patch != nil {
		_, err = s.client.CoreV1().Services(svc.Namespace).
			Patch(svc.Name, patchType, patch)
		if err != nil {
			return errors.Wrap(err, "failed to send patch")
		}
	}

	return nil
}

func (s *LoadBalancerStrategy) Remove(svc *v1.Service) error {
	clone := svc.DeepCopy()
	clone = removeServiceAnnotation(clone)

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
