package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	v1beta1 "k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
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

	err := c.cloudflareSync(key.(string))
	c.handleErr(err, key)

	return true
}

//Delete both TXT and A record for key
func (c *Controller) cloudflareDeleteRecordPair(key string) error {
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

	return nil
}

//Create both TXT and A record for key
func (c *Controller) cloudflareSyncRecordPair(key, hostname, ip string, proxied bool) error {
	err := c.cf.syncRecord("TXT", hostname, key, 1, false)
	if err != nil {
		fmt.Printf("Failed trying to get TXT record for %v: %v\n", key, err)
		return nil
	}

	publicIP := c.currentIP.Get()
	err = c.cf.syncRecord("A", hostname, publicIP, 1, proxied)
	if err != nil {
		fmt.Printf("Failed trying to get TXT record for %v: %v\n", key, err)
		return nil
	}

	return nil
}

func (c *Controller) cloudflareSync(key string) error {
	var obj interface{}
	var exists bool
	var err error

	splitKey := strings.Split(key, "/")
	resource := splitKey[1] + "/" + splitKey[2]
	if splitKey[0] == "service" {
		obj, exists, err = c.serviceIndexer.GetByKey(resource)
	} else {
		obj, exists, err = c.ingressIndexer.GetByKey(resource)
	}

	if err != nil {
		klog.Errorf("Fetching object with key %s from store failed with %v", key, err)
		return err
	}

	if !exists {
		fmt.Printf("Service %s does not exist anymore\n", key)
		c.cloudflareDeleteRecordPair(key)
	} else {
		var annotations map[string]string
		if splitKey[0] == "service" {
			annotations = obj.(*v1.Service).GetAnnotations()
		} else {
			annotations = obj.(*v1beta1.Ingress).GetAnnotations()
		}

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
				return nil
			}
		}

		publicIP := c.currentIP.Get()
		c.cloudflareSyncRecordPair(key, hostname, publicIP, proxied)
		fmt.Printf("Sync/Add/Update %v, hostname: %v, ip: %v\n", key, hostname, publicIP)
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
