package pkg

import (
	"context"
	"fmt"
	v110 "k8s.io/api/apps/v1"
	v19 "k8s.io/api/core/v1"
	v18 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v17 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/workqueue"
	"log"
	"time"

	v16 "k8s.io/client-go/informers/apps/v1"
	v14 "k8s.io/client-go/informers/core/v1"
	v15 "k8s.io/client-go/informers/networking/v1"
	"k8s.io/client-go/kubernetes"
	v13 "k8s.io/client-go/listers/apps/v1"
	v12 "k8s.io/client-go/listers/core/v1"
	v1 "k8s.io/client-go/listers/networking/v1"
	"k8s.io/client-go/tools/cache"
	"reflect"
)

const (
	workNum  = 5
	maxRetry = 10
)

type controller struct {
	ingressLister  v1.IngressLister
	serviceLister  v12.ServiceLister
	client         kubernetes.Interface
	deploymentList v13.DeploymentLister
	queue          workqueue.RateLimitingInterface
}

func (c *controller) deleteIngress(obj interface{}) {
	ingress := obj.(*v18.Ingress)
	ownerReference := v17.GetControllerOf(ingress)
	if ownerReference == nil {
		return
	}
	if ownerReference.Kind != "service" {
		return
	}
	c.queue.Add(ingress.Namespace + "/" + ingress.Name)
	fmt.Println(ingress.Namespace + "/" + ingress.Name)
}

func (c *controller) updateService(obj interface{}, obj2 interface{}) {
	if reflect.DeepEqual(obj, obj2) {
		return
	}
	c.enqueue(obj2)
}

func (c *controller) addService(obj interface{}) {
	c.enqueue(obj)
}
func (c *controller) deleteService(obj interface{}) {
	service := obj.(*v19.Service)
	c.queue.Add(service.Namespace + "/" + service.Name)
}
func (c *controller) updateDeployment(obj interface{}, obj2 interface{}) {
	if reflect.DeepEqual(obj, obj2) {
		return
	}
	c.enqueue(obj2)
}

func (c *controller) addDeployment(obj interface{}) {
	c.enqueue(obj)
}

func (c *controller) enqueue(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	fmt.Println(key)
	if err != nil {
		log.Fatalln("获取workqueue的key失败")
	}
	c.queue.Add(key)
}

func (c *controller) Run(stopCh chan struct{}) {
	for i := 0; i < workNum; i++ {
		go wait.Until(c.worker, time.Minute, stopCh)

	}
	<-stopCh
}

func (c *controller) worker() {
	for c.processNextItem() {

	}

}

func (c *controller) processNextItem() bool {
	item, shutdown := c.queue.Get()
	if shutdown {
		return false
	}
	defer c.queue.Done(item)
	key := item.(string)
	err := c.syncService(key)
	if err != nil {
		c.handlerError(key, err)
	}
	err = c.syncIngress(key)
	if err != nil {
		c.handlerError(key, err)
	}
	return true
}

func (c *controller) syncService(key string) error {
	namespaceKey, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return nil
	}
	//deployment逻辑处理
	deployment, err := c.deploymentList.Deployments(namespaceKey).Get(name)
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	//service逻辑处理
	service, err := c.serviceLister.Services(namespaceKey).Get(name)
	_, ok := deployment.GetAnnotations()["houyazhen/svc"]
	if err != nil && !errors.IsNotFound(err) {
		return err
	}
	if ok && errors.IsNotFound(err) {
		svc := c.constructService(deployment)
		_, err := c.client.CoreV1().Services(namespaceKey).Create(context.TODO(), svc, v17.CreateOptions{})
		if err != nil {
			return nil
		} else if !ok && service != nil {
			err := c.client.CoreV1().Services(namespaceKey).Delete(context.TODO(), name, v17.DeleteOptions{})
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (c *controller) syncIngress(key string) error {
	namespaceKey, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return nil
	}
	service, err := c.serviceLister.Services(namespaceKey).Get(name)
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	//ingress逻辑处理
	ingress, err := c.ingressLister.Ingresses(namespaceKey).Get(name)
	_, ok := service.GetAnnotations()["houyazhen/svc"]

	if err != nil && !errors.IsNotFound(err) {
		return err
	}
	if ok && errors.IsNotFound(err) {
		ig := c.constructIngress(service)
		_, err := c.client.NetworkingV1().Ingresses(namespaceKey).Create(context.TODO(), ig, v17.CreateOptions{})
		if err != nil {
			return nil

		}
	} else if !ok && ingress != nil {
		err := c.client.NetworkingV1().Ingresses(namespaceKey).Delete(context.TODO(), name, v17.DeleteOptions{})
		if err != nil {
			return nil
		}

	}

	return nil
}

func (c *controller) handlerError(key string, err error) {
	if c.queue.NumRequeues(key) <= maxRetry {
		c.queue.AddRateLimited(key)
		return
	}
	runtime.HandleError(err)
	c.queue.Forget(key)
}

func (c *controller) constructService(deployment *v110.Deployment) *v19.Service {
	service := v19.Service{}
	service.Name = deployment.Name
	service.Namespace = deployment.Namespace
	service.Annotations = map[string]string{
		"houyazhen/svc": "true",
	}
	service.ObjectMeta.OwnerReferences = []v17.OwnerReference{
		*v17.NewControllerRef(deployment, v18.SchemeGroupVersion.WithKind("deployment")),
	}
	service.Spec = v19.ServiceSpec{
		Selector: map[string]string{
			"app": deployment.Name,
		},
		Ports: []v19.ServicePort{
			{
				Name:     "http",
				Port:     80,
				Protocol: v19.ProtocolTCP,
			},
		},
	}
	return &service
}

func (c *controller) constructIngress(service *v19.Service) *v18.Ingress {
	ingress := v18.Ingress{}
	ingress.ObjectMeta.OwnerReferences = []v17.OwnerReference{
		*v17.NewControllerRef(service, v18.SchemeGroupVersion.WithKind("service")),
	}
	ingress.Name = service.Name
	ingress.Namespace = service.Namespace
	pathType := v18.PathTypePrefix
	icn := "nginx"
	ingress.Spec = v18.IngressSpec{
		IngressClassName: &icn,
		Rules: []v18.IngressRule{
			{
				Host: "houyazhen.com",
				IngressRuleValue: v18.IngressRuleValue{
					HTTP: &v18.HTTPIngressRuleValue{
						Paths: []v18.HTTPIngressPath{
							{
								Path:     "/",
								PathType: &pathType,
								Backend: v18.IngressBackend{
									Service: &v18.IngressServiceBackend{
										Name: service.Name,
										Port: v18.ServiceBackendPort{
											Number: 80,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	return &ingress
}

func NewController(client kubernetes.Interface, serviceInformer v14.ServiceInformer, ingressInformer v15.IngressInformer, deploymentInformer v16.DeploymentInformer) controller {
	c := controller{
		client:         client,
		ingressLister:  ingressInformer.Lister(),
		serviceLister:  serviceInformer.Lister(),
		deploymentList: deploymentInformer.Lister(),
		queue:          workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "ingressManager"),
	}
	deploymentInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.addDeployment,
		UpdateFunc: c.updateDeployment,
	})
	serviceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.addService,
		UpdateFunc: c.updateService,
		DeleteFunc: c.deleteService,
	})
	ingressInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		DeleteFunc: c.deleteIngress,
	})
	return c
}
