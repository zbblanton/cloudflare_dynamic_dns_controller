package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
)

type Controller struct {
	currentIP       *CurrentIP
	cf              Cloudflare
	queue           workqueue.RateLimitingInterface
	serviceIndexer  cache.Indexer
	serviceInformer cache.Controller
	ingressIndexer  cache.Indexer
	ingressInformer cache.Controller
}

func NewController(
	currentIP *CurrentIP,
	cf Cloudflare,
	queue workqueue.RateLimitingInterface,
	serviceIndexer cache.Indexer,
	serviceInformer cache.Controller,
	ingressIndexer cache.Indexer,
	ingressInformer cache.Controller) *Controller {
	return &Controller{
		currentIP:       currentIP,
		cf:              cf,
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
	obj, exists, err := c.serviceIndexer.GetByKey(key)
	if err != nil {
		klog.Errorf("Fetching object with key %s from store failed with %v", key, err)
		return err
	}

	if !exists {
		fmt.Printf("Service %s does not exist anymore\n", key)

		records, err := c.cf.ListTXTRecords()
		if err != nil {
			fmt.Printf("Failed to get list of txt records:  %v\n", err)
			return nil
		}
		for _, record := range records {
			if record.Content == key {
				err = c.cf.DeleteRecordByName("A", record.Name)
				if err != nil {
					fmt.Printf("Failed to delete A record:  %v\n", err)
				}
				err = c.cf.DeleteRecordByName("TXT", record.Name)
				if err != nil {
					fmt.Printf("Failed to delete TXT record:  %v\n", err)
				}
			}
		}

		// annotations := obj.(*v1.Service).GetAnnotations()
		// if _, ok := annotations["cloudflare-dynamic-dns.alpha.kubernetes.io/hostname"]; ok {
		// 	hostname := annotations["cloudflare-dynamic-dns.alpha.kubernetes.io/hostname"]
		// 	err = c.cf.DeleteRecordByName(hostname)
		// 	if err != nil {
		// 		fmt.Printf("Failed to delete record:  %v\n", err)
		// 	}
		// }
	} else {
		//fmt.Printf("\nSync/Add/Update for obj: %v\n", obj)
		annotations := obj.(*v1.Service).GetAnnotations()

		if _, ok := annotations["cloudflare-dynamic-dns.alpha.kubernetes.io/hostname"]; !ok {
			fmt.Printf("Skipping: %v\n", key)
			return nil
		}
		hostname := annotations["cloudflare-dynamic-dns.alpha.kubernetes.io/hostname"]

		//Check if proxied is provided, if so convert to bool
		var proxied bool
		if _, ok := annotations["cloudflare-dynamic-dns.alpha.kubernetes.io/proxied"]; ok {
			proxied, err = strconv.ParseBool(annotations["cloudflare-dynamic-dns.alpha.kubernetes.io/proxied"])
			if err != nil {
				fmt.Printf("Could not convert cloudflare-dynamic-dns.alpha.kubernetes.io/proxied to bool for %v\n", key)
			}
			return nil
		}

		publicIP := c.currentIP.Get()

		// if annotations["test"] != "thiscoolvalue" {
		// 	fmt.Printf("Skipping: %v\n", key)
		// 	return nil
		// }
		_, err := c.cf.CreateARecord(hostname, publicIP, 1, proxied)
		if err != nil {
			fmt.Printf("Failed to create A record for %v: %v\n", key, err)
			return nil
		}
		_, err = c.cf.CreateTXTRecord(hostname, key, 1, proxied)
		if err != nil {
			fmt.Printf("Failed to create TXT record for %v: %v\n", key, err)
			return nil
		}
		fmt.Printf("Sync/Add/Update for %v, hostname: %v, ip: %v\n ", key, hostname, publicIP)
		// fmt.Printf("Sync/Add/Update for: %v\n", annotations)
		// fmt.Printf("Sync/Add/Update for: %v\n", key)
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

	cfAuthEmail := os.Getenv("CF_AUTH_EMAIL")
	cfAuthToken := os.Getenv("CF_AUTH_TOKEN")
	cfZoneID := os.Getenv("CF_ZONE_ID")
	cf := NewCloudflare(cfAuthEmail, cfAuthToken, cfZoneID)
	// fmt.Println(cf.GetRecord("testme2.blantontechnology.com"))
	// newRecord, err := cf.CreateRecord("testme3.blantontechnology.com", "192.168.0.1", 1, false)
	// if err != nil {
	// 	fmt.Println(err)
	// }
	// fmt.Println(newRecord)
	// err = cf.DeleteRecordByName("testme3.blantontechnology.com")
	// if err != nil {
	// 	fmt.Println(err)
	// }

	//Start the public ip watcher
	currentIP := CurrentIP{}
	go watchPublicIP(&currentIP)

	controller := NewController(&currentIP, cf, queue, serviceIndexer, serviceInformer, ingressIndexer, ingressInformer)

	// Now let's start the controller
	stop := make(chan struct{})
	defer close(stop)
	go controller.Run(1, stop)

	// Wait forever
	select {}
}
