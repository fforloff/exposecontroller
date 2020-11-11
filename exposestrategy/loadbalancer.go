package exposestrategy

import (
	"github.com/pkg/errors"

	"k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

type LoadBalancerStrategy struct {
	client  kubernetes.Interface
}

func NewLoadBalancerStrategy(client kubernetes.Interface, config *ExposeStrategyConfig) (ExposeStrategy, error) {
	return &LoadBalancerStrategy{
		client:  client,
	}, nil
}

func (s *LoadBalancerStrategy) Sync() error {
	return nil
}

func (s *LoadBalancerStrategy) Add(svc *v1.Service) error {
	var err error
	clone := svc.DeepCopy()
	clone.Spec.Type = v1.ServiceTypeLoadBalancer
	err = addServiceAnnotation(clone, clone.Spec.LoadBalancerIP)
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
			return errors.Wrap(err, "failed to send patch")
		}
	}

	return nil
}

func (s *LoadBalancerStrategy) Remove(svc *v1.Service) error {
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
