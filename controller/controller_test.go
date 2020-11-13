package controller

import (
	"fmt"
	"testing"
	"time"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/olli-ai/exposecontroller/exposestrategy"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeStrategy struct{
	testing       *testing.T
	tasks         []map[string]bool
	ignore        []map[string]bool
	syncFunc      func() error
	hasSyncedFunc func() bool
	addFunc       func(svc *v1.Service) error
	cleanFunc     func(svc *v1.Service) error
	deleteFunc    func(svc *v1.Service) error
}

func (s *fakeStrategy) checkTask(action string, svc *v1.Service) {
	t := s.testing
	if svc != nil {
		action = fmt.Sprintf("%s:%s/%s:%s", action, svc.Namespace, svc.Name, svc.ResourceVersion)
	}
	if len(s.tasks) > 0 &&  s.tasks[0][action] {
		delete(s.tasks[0], action)
		if len(s.tasks[0]) == 0 {
			s.tasks = s.tasks[1:]
			if len(s.ignore) > 1 {
				ignore := s.ignore[0]
				s.ignore = s.ignore[1:]
				if s.ignore[0] == nil {
					s.ignore[0] = ignore
				} else {
					for k := range ignore {
						s.ignore[0][k] = true
					}
				}
			}
		}
	} else if len(s.ignore) > 0 && s.ignore[0][action] {
		delete(s.ignore[0], action)
	} else {
		assert.Failf(t, "unexpected action", "unexpected action %s, expected %v", action, s.tasks)
	}
}

func (s *fakeStrategy) checkEnd() {
	assert.Empty(s.testing, s.tasks)
}

func (s *fakeStrategy) Sync() error {
	s.checkTask("Sync", nil)
			var err error
	if s.syncFunc != nil {
		err = s.syncFunc()
		assert.NoError(s.testing, err)
	}
	return nil
}

func (s *fakeStrategy) HasSynced() bool {
	if s.hasSyncedFunc != nil {
		return s.hasSyncedFunc()
	}
	return true
}

func (s *fakeStrategy) Add(svc *v1.Service) error {
	s.checkTask("Add", svc)
			var err error
	if s.addFunc != nil {
		err = s.addFunc(svc)
		assert.NoError(s.testing, err)
	}
	return nil
}

func (s *fakeStrategy) Clean(svc *v1.Service) error {
	s.checkTask("Clean", svc)
			var err error
	if s.cleanFunc != nil {
		err = s.cleanFunc(svc)
		assert.NoError(s.testing, err)
	}
	return nil
}

func (s *fakeStrategy) Delete(svc *v1.Service) error {
	s.checkTask("Delete", svc)
			var err error
	if s.deleteFunc != nil {
		err = s.deleteFunc(svc)
		assert.NoError(s.testing, err)
	}
	return nil
}

func TestRun_controllerSynced(t *testing.T) {
	services := []runtime.Object{
		&v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "main",
				Name:     "svc1",
				Annotations: map[string]string{
					exposestrategy.ExposeAnnotation.Key: exposestrategy.ExposeAnnotation.Value,
				},
				ResourceVersion: "1",
			},
		},
		&v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "other",
				Name:     "svc2",
				Annotations: map[string]string{
					exposestrategy.ExposeAnnotation.Key: exposestrategy.ExposeAnnotation.Value,
				},
				ResourceVersion: "2",
			},
		},
		&v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "main",
				Name:     "svc3",
				Annotations: map[string]string{
					"todo": "true",
				},
				ResourceVersion: "3",
			},
		},
		&v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "main",
				Name:     "svc4",
				Annotations: map[string]string{
					exposestrategy.ExposeAnnotation.Key: exposestrategy.ExposeAnnotation.Value,
				},
				ResourceVersion: "4",
			},
		},
	}
	client := fake.NewSimpleClientset(services...)

	strategy := fakeStrategy{
		testing: t,
		tasks: []map[string]bool{{
			"Sync": true,
		}, {
			"Add:main/svc1:1":   true,
			"Clean:main/svc3:3": true,
			"Add:main/svc4:4":   true,
		}},
		ignore: []map[string]bool{nil, nil, {
			"Add:main/svc1:1+": true,
			"Add:main/svc4:4+": true,
		}},
		addFunc: func(svc *v1.Service) error {
			var err error
			if svc.Annotations["checked"] == "" {
				svc = svc.DeepCopy()
				svc.Annotations["checked"] = "true"
				svc.ResourceVersion = svc.ResourceVersion + "+"
				_, err = client.CoreV1().Services(svc.Namespace).Update(svc)
			}
			return err
		},
		cleanFunc: func(svc *v1.Service) error {
			var err error
			if svc.Annotations["todo"] == "true" {
				svc = svc.DeepCopy()
				delete(svc.Annotations, "todo")
				svc.ResourceVersion = svc.ResourceVersion + "+"
				_, err = client.CoreV1().Services(svc.Namespace).Update(svc)
			}
			return err
		},
	}
	testStrategy = &strategy
	defer func() {
		testStrategy = nil
	}()

	err := Run(client, "main", &Config{}, time.Second)
	require.NoError(t, err)
	strategy.checkEnd()
}

func TestRun_strategySynced(t *testing.T) {
	services := []runtime.Object{
		&v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "main",
				Name:     "svc1",
				Annotations: map[string]string{
					exposestrategy.ExposeAnnotation.Key: exposestrategy.ExposeAnnotation.Value,
				},
				ResourceVersion: "1",
			},
		},
		&v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "other",
				Name:     "svc2",
				Annotations: map[string]string{
					exposestrategy.ExposeAnnotation.Key: exposestrategy.ExposeAnnotation.Value,
				},
				ResourceVersion: "2",
			},
		},
		&v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "main",
				Name:     "svc3",
				Annotations: map[string]string{
					"todo": "true",
				},
				ResourceVersion: "3",
			},
		},
		&v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "main",
				Name:     "svc4",
				Annotations: map[string]string{
					exposestrategy.ExposeAnnotation.Key: exposestrategy.ExposeAnnotation.Value,
					"todo": "true",
				},
				ResourceVersion: "4",
			},
		},
	}
	client := fake.NewSimpleClientset(services...)

	todo := map[string]bool{}
	strategy := fakeStrategy{
		testing: t,
		tasks: []map[string]bool{{
			"Sync": true,
		}, {
			"Add:main/svc1:1":   true,
			"Clean:main/svc3:3": true,
			"Add:main/svc4:4":   true,
		}, {
			"Add:main/svc1:1+": true,
		}},
		ignore: []map[string]bool{nil, nil, {
			"Add:main/svc4:4+": true,
		}, {
			"Add:main/svc1:1++": true,
		}},
		addFunc: func(svc *v1.Service) error {
			var err error
			if svc.Annotations["todo"] == "" {
				todo[svc.Name] = true
				svc = svc.DeepCopy()
				svc.Annotations["todo"] = "true"
				svc.ResourceVersion = svc.ResourceVersion + "+"
				_, err = client.CoreV1().Services(svc.Namespace).Update(svc)
			} else if svc.Annotations["todo"] == "true" {
				delete(todo, svc.Name)
				svc = svc.DeepCopy()
				svc.Annotations["todo"] = "false"
				svc.ResourceVersion = svc.ResourceVersion + "+"
				_, err = client.CoreV1().Services(svc.Namespace).Update(svc)
			}
			return err
		},
		cleanFunc: func(svc *v1.Service) error {
			var err error
			if svc.Annotations["todo"] == "true" {
				svc = svc.DeepCopy()
				delete(svc.Annotations, "todo")
				svc.ResourceVersion = svc.ResourceVersion + "+"
				_, err = client.CoreV1().Services(svc.Namespace).Update(svc)
			}
			return err
		},
		hasSyncedFunc: func() bool {
			return len(todo) == 0
		},
	}
	testStrategy = &strategy
	defer func() {
		testStrategy = nil
	}()

	err := Run(client, "main", &Config{}, time.Second)
	require.NoError(t, err)
	strategy.checkEnd()
}

func TestRun_timeout(t *testing.T) {
	services := []runtime.Object{
		&v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "main",
				Name:     "svc1",
				Annotations: map[string]string{
					exposestrategy.ExposeAnnotation.Key: exposestrategy.ExposeAnnotation.Value,
				},
				ResourceVersion: "1",
			},
		},
	}
	client := fake.NewSimpleClientset(services...)

	strategy := fakeStrategy{
		testing: t,
		tasks: []map[string]bool{{
			"Sync": true,
		}, {
			"Add:main/svc1:1": true,
		}},
		hasSyncedFunc: func() bool {
			return false
		},
	}
	testStrategy = &strategy
	defer func() {
		testStrategy = nil
	}()

	err := Run(client, "main", &Config{}, time.Second)
	require.Error(t, err)
	strategy.checkEnd()
}

func TestDaemon(t *testing.T) {
	services := []runtime.Object{
		&v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "main",
				Name:     "svc1",
				Annotations: map[string]string{
					exposestrategy.ExposeAnnotation.Key: exposestrategy.ExposeAnnotation.Value,
				},
				ResourceVersion: "1",
			},
		},
		&v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "other",
				Name:     "svc2",
				Annotations: map[string]string{
					exposestrategy.ExposeAnnotation.Key: exposestrategy.ExposeAnnotation.Value,
				},
				ResourceVersion: "2",
			},
		},
		&v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "main",
				Name:     "svc3",
				Annotations: map[string]string{
					"todo": "true",
				},
				ResourceVersion: "3",
			},
		},
		&v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "main",
				Name:     "svc4",
				Annotations: map[string]string{
					exposestrategy.ExposeAnnotation.Key: exposestrategy.ExposeAnnotation.Value,
					"todo": "true",
				},
				ResourceVersion: "4",
			},
		},
		&v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "main",
				Name:     "svc5",
				Annotations: map[string]string{
					exposestrategy.ExposeAnnotation.Key: exposestrategy.ExposeAnnotation.Value,
					"todo": "true",
				},
				ResourceVersion: "5",
			},
		},
	}
	client := fake.NewSimpleClientset(services...)

	todo := map[string]bool{}
	strategy := fakeStrategy{
		testing: t,
		tasks: []map[string]bool{{
			"Sync": true,
		}, {
			"Add:main/svc1:1":   true,
			"Clean:main/svc3:3": true,
			"Add:main/svc4:4":   true,
			"Add:main/svc5:5":   true,
		}, {
			"Add:main/svc1:1+":  true,
			"Add:main/svc1:1++": true,
			"Add:main/svc4:4+":  true,
			"Add:main/svc5:5+":  true,
		}},
		addFunc: func(svc *v1.Service) error {
			var err error
			if svc.Annotations["todo"] == "" {
				todo[svc.Name] = true
				svc = svc.DeepCopy()
				svc.Annotations["todo"] = "true"
				svc.ResourceVersion = svc.ResourceVersion + "+"
				_, err = client.CoreV1().Services(svc.Namespace).Update(svc)
			} else if svc.Annotations["todo"] == "true" {
				delete(todo, svc.Name)
				svc = svc.DeepCopy()
				svc.Annotations["todo"] = "false"
				svc.ResourceVersion = svc.ResourceVersion + "+"
				_, err = client.CoreV1().Services(svc.Namespace).Update(svc)
			}
			return err
		},
		cleanFunc: func(svc *v1.Service) error {
			var err error
			if svc.Annotations["todo"] == "true" {
				svc = svc.DeepCopy()
				delete(svc.Annotations, "todo")
				svc.ResourceVersion = svc.ResourceVersion + "+"
				_, err = client.CoreV1().Services(svc.Namespace).Update(svc)
			}
			return err
		},
		hasSyncedFunc: func() bool {
			return len(todo) == 0
		},
	}
	testStrategy = &strategy
	defer func() {
		testStrategy = nil
	}()

	controller, err := Daemon(client, "main", &Config{}, time.Hour)
	require.NoError(t, err)
	stopChan := make(chan struct{})
	defer close(stopChan)
	go controller.Run(stopChan)

	time.Sleep(500*time.Millisecond)
	strategy.checkEnd()

	strategy.tasks = []map[string]bool{{
		"Add:main/svc1:6":     true,
		"Add:main/svc3:7":     true,
		"Add:main/svc3:7+":    true,
		"Add:main/svc3:7++":   true,
		"Clean:main/svc4:8":   true,
		"Delete:main/svc5:5+": true,
	}}

	client.CoreV1().Services("main").Update(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "main",
			Name:     "svc1",
			Annotations: map[string]string{
				exposestrategy.ExposeAnnotation.Key: exposestrategy.ExposeAnnotation.Value,
				"todo": "false",
			},
			ResourceVersion: "6",
		},
	})
	client.CoreV1().Services("main").Update(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "main",
			Name:     "svc3",
			Annotations: map[string]string{
				exposestrategy.ExposeAnnotation.Key: exposestrategy.ExposeAnnotation.Value,
			},
			ResourceVersion: "7",
		},
	})
	client.CoreV1().Services("main").Update(&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "main",
			Name:     "svc4",
			Annotations: map[string]string{
				"todo": "true",
			},
			ResourceVersion: "8",
		},
	})
	client.CoreV1().Services("main").Delete("svc5", &metav1.DeleteOptions{})

	time.Sleep(500*time.Millisecond)
	strategy.checkEnd()
}
