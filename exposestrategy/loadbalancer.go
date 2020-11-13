package exposestrategy

import (
	"fmt"

	"github.com/pkg/errors"

	"k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

type LoadBalancerStrategy struct {
	client kubernetes.Interface
	todo   map[string]bool
}

func NewLoadBalancerStrategy(client kubernetes.Interface, config *ExposeStrategyConfig) (ExposeStrategy, error) {
	return &LoadBalancerStrategy{
		client:  client,
	}, nil
}

func (s *LoadBalancerStrategy) Sync() error {
	s.todo = map[string]bool{}
	return nil
}

func (s *LoadBalancerStrategy) HasSynced() bool {
	return len(s.todo) == 0
}

func (s *LoadBalancerStrategy) Add(svc *v1.Service) error {
	delete(s.todo, fmt.Sprintf("%s/%s", svc.Namespace, svc.Name))

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

	if clone.Spec.LoadBalancerIP == "" {
		s.todo[fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)] = true
	}
	return nil
}

func (s *LoadBalancerStrategy) Clean(svc *v1.Service) error {
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

func (s *LoadBalancerStrategy) Delete(svc *v1.Service) error {
	delete(s.todo, fmt.Sprintf("%s/%s", svc.Namespace, svc.Name))

	return nil
}
