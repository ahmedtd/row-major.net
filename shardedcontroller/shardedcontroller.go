package main

import (
	"context"
	"crypto/sha512"
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	coreinformersv1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

var (
	shardNamespace = flag.String("shard_namespace", "", "The Kubernetes namespace where we will find our other processing shards.")
	shardSelector  = flag.String("shard_selector", "", "A Kubernetes label selector to find the pods that form our processing shards.")
	selfPodName    = flag.String("self_pod_name", "", "The name of the pod we are currently running as.")
)

// rendezvous selects a shard from shards to handle item.
func rendezvous(item string, shards []string) string {
	maxWeight := uint64(0)
	maxShard := ""
	for _, shard := range shards {
		hash := sha512.Sum512_256(append([]byte(item), shard...))
		weight := uint64(0)
		for i := 0; i < 8; i++ {
			weight += uint64(hash[i]) << i * 8
		}

		if maxShard == "" {
			maxWeight = weight
			maxShard = shard
			continue
		}

		if weight > maxWeight {
			maxWeight = weight
			maxShard = shard
			continue
		}

		if weight == maxWeight && strings.Compare(shard, maxShard) > 0 {
			maxWeight = weight
			maxShard = shard
			continue
		}
	}

	return maxShard
}

type Sharder struct {
	shardNamespace string
	shardSelector  string

	podName string

	kc          *kubernetes.Clientset
	podInformer cache.SharedIndexInformer
}

func NewSharder(shardNamespace, shardSelector, podName string, kc *kubernetes.Clientset) *Sharder {
	c := &Sharder{
		shardNamespace: shardNamespace,
		shardSelector:  shardSelector,
		podName:        podName,
		kc:             kc,
	}

	c.podInformer = coreinformersv1.NewFilteredPodInformer(
		kc,
		shardNamespace,
		24*time.Hour,
		cache.Indexers{
			cache.NamespaceIndex: cache.MetaNamespaceIndexFunc,
		},
		func(opts *metav1.ListOptions) {
			opts.LabelSelector = shardSelector
		},
	)

	return c
}

func (c *Sharder) Run(ctx context.Context) {
	go c.podInformer.Run(ctx.Done())

	if !cache.WaitForCacheSync(ctx.Done(), c.podInformer.HasSynced) {
		return
	}

	<-ctx.Done()
	return
}

func (c *Sharder) DoIOwnItem(item string) bool {
	shards := c.podInformer.GetStore().ListKeys()
	return rendezvous(item, shards) == c.shardNamespace+"/"+c.podName
}

type ShardedConfigMapWatcher struct {
	kc *kubernetes.Clientset

	sharder *Sharder

	cmInformer cache.SharedIndexInformer
	queue      workqueue.RateLimitingInterface
}

func NewShardedConfigMapWatcher(kc *kubernetes.Clientset, sharder *Sharder) *ShardedConfigMapWatcher {
	c := &ShardedConfigMapWatcher{
		kc:      kc,
		sharder: sharder,
	}

	c.cmInformer = coreinformersv1.NewConfigMapInformer(
		kc,
		corev1.NamespaceAll,
		24*time.Hour,
		cache.Indexers{
			cache.NamespaceIndex: cache.MetaNamespaceIndexFunc,
		},
	)

	c.queue = workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "ShardedConfigMapWatcher")

	c.cmInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(new interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(new)
			if err != nil {
				return
			}
			c.queue.Add(key)
		},
		UpdateFunc: func(old, new interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(new)
			if err != nil {
				return
			}
			c.queue.Add(key)
		},
		DeleteFunc: func(old interface{}) {
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(old)
			if err != nil {
				return
			}
			c.queue.Add(key)
		},
	})

	return c
}

func (c *ShardedConfigMapWatcher) Run(ctx context.Context) {
	go c.cmInformer.Run(ctx.Done())

	if !cache.WaitForCacheSync(ctx.Done(), c.cmInformer.HasSynced) {
		return
	}

	go wait.Until(func() { c.runWorker(ctx) }, time.Second, ctx.Done())

	<-ctx.Done()
	return
}

func (c *ShardedConfigMapWatcher) runWorker(ctx context.Context) {
	for c.processNextWorkItem(ctx) {
	}
}

func (c *ShardedConfigMapWatcher) processNextWorkItem(ctx context.Context) bool {
	key, quit := c.queue.Get()
	if quit {
		return false
	}

	defer c.queue.Done(key)

	if c.sharder.DoIOwnItem(key.(string)) {
		log.Printf("ShardedConfigMapWatcher owns configmap %q", key.(string))
	}

	// Processed this item successfully.  Remove from queue.
	c.queue.Forget(key)
	return true
}

func main() {
	flag.Parse()
	log.Printf("shardedcontroller booting up")
	log.Printf("Flags:")
	log.Printf("--shard_namespace=%v", *shardNamespace)
	log.Printf("--shard_selector=%v", *shardSelector)
	log.Printf("--self_pod_name=%v", *selfPodName)

	kconfig, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("Error creating in-cluster Kubernetes config: %v", err)
	}

	kc, err := kubernetes.NewForConfig(kconfig)
	if err != nil {
		log.Fatalf("Error creating Kubernetes clientset: %v", err)
	}

	sharder := NewSharder(*shardNamespace, *shardSelector, *selfPodName, kc)

	cmWatcher := NewShardedConfigMapWatcher(kc, sharder)

	ctx, cancel := context.WithCancel(context.Background())

	go sharder.Run(ctx)
	go cmWatcher.Run(ctx)

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)

	<-signalCh
	cancel()
}
