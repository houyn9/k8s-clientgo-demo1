package main

import (
	"clientgo01/pkg"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"log"
)

func main() {
	//1.set config
	config, err := clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
	if err != nil {
		inClusterConfig, err := rest.InClusterConfig()
		if err != nil {
			log.Fatalln("can't find config")
		}
		config = inClusterConfig
	}
	//2.creat clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalln("can't create clientset")
	}
	//3.create informer
	factory := informers.NewSharedInformerFactory(clientset, 0)
	//informers.NewSharedInformerFactoryWithOptions(clientset,0,informers.WithNamespace("default"))  基于namespace的控制
	serviceInformer := factory.Core().V1().Services()
	ingressInformer := factory.Networking().V1().Ingresses()
	deploymentInformer := factory.Apps().V1().Deployments()
	//4.add event handler
	controller := pkg.NewController(clientset, serviceInformer, ingressInformer, deploymentInformer)

	//5.informer start
	stopCh := make(chan struct{})
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	//6.controller run
	controller.Run(stopCh)
}
