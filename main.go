package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"

	v1beta1 "k8s.io/api/networking/v1beta1"
)

type Controller struct {
	queue           workqueue.RateLimitingInterface
	serviceIndexer  cache.Indexer
	serviceInformer cache.Controller
	ingressIndexer  cache.Indexer
	ingressInformer cache.Controller
}

func NewController(
	queue workqueue.RateLimitingInterface,
	serviceIndexer cache.Indexer,
	serviceInformer cache.Controller,
	ingressIndexer cache.Indexer,
	ingressInformer cache.Controller) *Controller {
	return &Controller{
		queue:           queue,
		serviceIndexer:  serviceIndexer,
		serviceInformer: serviceInformer,
		ingressIndexer:  ingressIndexer,
		ingressInformer: ingressInformer,
	}
}

func (c *Controller) processNextItem() bool {
	// Wait until there is a new item in the working queue
	key, quit := c.queue.Get()
	if quit {
		return false
	}

	defer c.queue.Done(key)

	splitKey := strings.Split(key.(string), "/")
	if splitKey[0] == "service" {
		err := c.syncToStdout(splitKey[1] + "/" + splitKey[2])
		// Handle the error if something went wrong during the execution of the business logic
		c.handleErr(err, key)
	} else {
		err := c.ingressSyncToStdout(splitKey[1] + "/" + splitKey[2])
		// Handle the error if something went wrong during the execution of the business logic
		c.handleErr(err, key)
	}

	return true
}

func (c *Controller) syncToStdout(key string) error {
	//obj, exists, err := c.indexer.GetByKey(key)
	_, exists, err := c.serviceIndexer.GetByKey(key)
	if err != nil {
		klog.Errorf("Fetching object with key %s from store failed with %v", key, err)
		return err
	}

	if !exists {
		fmt.Printf("Service %s does not exist anymore\n", key)
	} else {
		//fmt.Printf("\nSync/Add/Update for obj: %v\n", obj)
		fmt.Printf("Sync/Add/Update for: %v\n", key)
	}

	return nil
}

func (c *Controller) ingressSyncToStdout(key string) error {
	//obj, exists, err := c.ingressIndexer.GetByKey(key)
	_, exists, err := c.ingressIndexer.GetByKey(key)
	if err != nil {
		klog.Errorf("Fetching object with key %s from store failed with %v", key, err)
		return err
	}

	if !exists {
		fmt.Printf("Ingress %s does not exist anymore\n", key)
	} else {
		//fmt.Printf("\nSync/Add/Update for obj: %v\n", obj)
		fmt.Printf("Sync/Add/Update for: %v\n", key)
	}
	return nil
}

// handleErr checks if an error happened and makes sure we will retry later.
func (c *Controller) handleErr(err error, key interface{}) {
	if err == nil {
		// Forget about the #AddRateLimited history of the key on every successful synchronization.
		// This ensures that future processing of updates for this key is not delayed because of
		// an outdated error history.
		c.queue.Forget(key)
		return
	}

	// This controller retries 5 times if something goes wrong. After that, it stops trying.
	if c.queue.NumRequeues(key) < 5 {
		klog.Infof("Error syncing %v: %v", key, err)

		// Re-enqueue the key rate limited. Based on the rate limiter on the
		// queue and the re-enqueue history, the key will be processed later again.
		c.queue.AddRateLimited(key)
		return
	}

	c.queue.Forget(key)
	// Report to an external entity that, even after several retries, we could not successfully process this key
	runtime.HandleError(err)
	klog.Infof("Dropping %q out of the queue: %v", key, err)
}

func (c *Controller) Run(threadiness int, stopCh chan struct{}) {
	defer runtime.HandleCrash()

	// Let the workers stop when we are done
	defer c.queue.ShutDown()
	klog.Info("Starting controller")

	go c.serviceInformer.Run(stopCh)
	go c.ingressInformer.Run(stopCh)

	// Wait for all involved caches to be synced, before processing items from the queue is started
	if !cache.WaitForCacheSync(stopCh, c.serviceInformer.HasSynced) {
		runtime.HandleError(fmt.Errorf("Timed out waiting for caches to sync"))
		return
	}

	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	<-stopCh
	klog.Info("Stopping controller")
}

func (c *Controller) runWorker() {
	for c.processNextItem() {
	}
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}

func main() {
	var kubeconfig *string
	if home := homeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	cfAuthEmail = os.Getenv("CF_AUTH_EMAIL")
	cfAuthToken = os.Getenv("CF_AUTH_TOKEN")
	cfZoneID = os.Getenv("CF_ZONE_ID")

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err.Error())
	}

	// create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	// create the watchers
	serviceListWatcher := cache.NewListWatchFromClient(clientset.CoreV1().RESTClient(), "services", "", fields.Everything())
	ingressListWatcher := cache.NewListWatchFromClient(clientset.NetworkingV1beta1().RESTClient(), "ingresses", "", fields.Everything())

	// create the workqueue
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	serviceIndexer, serviceInformer := cache.NewIndexerInformer(serviceListWatcher, &v1.Service{}, 5*time.Second, cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			// fmt.Println("HERE:")
			// fmt.Println(obj.(*v1.Service).GetAnnotations())
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				queue.Add("service/" + key)
			}
		},
		UpdateFunc: func(old interface{}, new interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(new)
			if err == nil {
				queue.Add("service/" + key)
			}
		},
		DeleteFunc: func(obj interface{}) {
			// IndexerInformer uses a delta queue, therefore for deletes we have to use this
			// key function.
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			if err == nil {
				queue.Add("service/" + key)
			}
		},
	}, cache.Indexers{})

	ingressIndexer, ingressInformer := cache.NewIndexerInformer(ingressListWatcher, &v1beta1.Ingress{}, 5*time.Second, cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			// annotations := obj.(*v1beta1.Ingress).GetAnnotations()
			// if _, ok := annotations["test"]; !ok {
			// 	return
			// }
			// if annotations["test"] != "thiscoolvalue" {
			// 	return
			// }
			// fmt.Println("WE GOT TO HERE!!!!!!")
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				queue.Add("ingress/" + key)
			}
		},
		UpdateFunc: func(old interface{}, new interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(new)
			if err == nil {
				queue.Add("ingress/" + key)
			}
		},
		DeleteFunc: func(obj interface{}) {
			// IndexerInformer uses a delta queue, therefore for deletes we have to use this
			// key function.
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			if err == nil {
				queue.Add("ingress/" + key)
			}
		},
	}, cache.Indexers{})

	controller := NewController(queue, serviceIndexer, serviceInformer, ingressIndexer, ingressInformer)

	//Start the public ip watcher
	currentIP := CurrentIP{}
	go watchPublicIP(&currentIP)

	// Now let's start the controller
	stop := make(chan struct{})
	defer close(stop)
	go controller.Run(1, stop)

	// Wait forever
	select {}
}
