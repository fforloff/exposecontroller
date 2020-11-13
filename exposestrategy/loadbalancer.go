package exposestrategy

import (
	"fmt"

	"github.com/pkg/errors"

	"k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

// LoadBalancerStrategy is a strategy that changes the type of services to LoadBalancer
type LoadBalancerStrategy struct {
	client kubernetes.Interface
	// The services to wait for their load balancer IP
	todo   map[string]bool
}

// NewLoadBalancerStrategy a new LoadBalancerStrategy
func NewLoadBalancerStrategy(client kubernetes.Interface, config *Config) (ExposeStrategy, error) {
	return &LoadBalancerStrategy{
		client:  client,
	}, nil
}

// Sync is called before starting / resyncing
// init the todo map
func (s *LoadBalancerStrategy) Sync() error {
	s.todo = map[string]bool{}
	return nil
}

// HasSynced tells if the strategy is complete
// Complete when todo is empty
func (s *LoadBalancerStrategy) HasSynced() bool {
	return len(s.todo) == 0
}

// Add is called when an exposed service is created or updated
// Changes the service type and updates various annotations
// Adds the service to the todo list if the load balancer IP is unknown
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

// Clean is called when an exposed service is unexposed
// Restores the service type and cleans various annotations
// Clears the service form the todo list
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

// Delete is called when an exposed service is deleted
// Clears the service form the todo list
func (s *LoadBalancerStrategy) Delete(svc *v1.Service) error {
	delete(s.todo, fmt.Sprintf("%s/%s", svc.Namespace, svc.Name))

	return nil
}
