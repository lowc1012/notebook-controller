/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	appsinformers "k8s.io/client-go/informers/apps/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	appslisters "k8s.io/client-go/listers/apps/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	"github.com/lowc1012/notebook-controller/pkg/apis/notebookcontroller/v1alpha1"
	clientset "github.com/lowc1012/notebook-controller/pkg/generated/clientset/versioned"
	scheme "github.com/lowc1012/notebook-controller/pkg/generated/clientset/versioned/scheme"
	informers "github.com/lowc1012/notebook-controller/pkg/generated/informers/externalversions/notebookcontroller/v1alpha1"
	listers "github.com/lowc1012/notebook-controller/pkg/generated/listers/notebookcontroller/v1alpha1"
)

const controllerAgentName = "notebook-controller"
const DefaultContainerPort = 8888
const DefaultServingPort = 80

const (
	// SuccessSynced is used as part of the Event 'reason' when a Notebook is synced
	SuccessSynced = "Synced"
	// ErrResourceExists is used as part of the Event 'reason' when a Notebook fails
	// to sync due to a Deployment of the same name already existing.
	ErrResourceExists = "ErrResourceExists"

	// MessageResourceExists is the message used for Events when a resource
	// fails to sync due to a Deployment already existing
	MessageResourceExists = "Resource %q already exists and is not managed by Notebook"
	// MessageResourceSynced is the message used for an Event fired when a Notebook
	// is synced successfully
	MessageResourceSynced = "Notebook synced successfully"
)

// Controller is the controller implementation for Notebook resources
type Controller struct {
	// kubeclientset is a standard kubernetes clientset
	kubeclientset kubernetes.Interface
	// sampleclientset is a clientset for our own API group
	sampleclientset clientset.Interface

	statefulsetsLister appslisters.StatefulSetLister
	statefulsetsSynced cache.InformerSynced
	notebooksLister    listers.NotebookLister
	notebooksSynced    cache.InformerSynced

	// workqueue is a rate limited work queue. This is used to queue work to be
	// processed instead of performing it as soon as a change happens. This
	// means we can ensure we only process a fixed amount of resources at a
	// time, and makes it easy to ensure we are never processing the same item
	// simultaneously in two different workers.
	workqueue workqueue.RateLimitingInterface
	// recorder is an event recorder for recording Event resources to the
	// Kubernetes API.
	recorder record.EventRecorder
}

// NewController returns a new sample controller
func NewController(
	kubeclientset kubernetes.Interface,
	sampleclientset clientset.Interface,
	statefulsetInformer appsinformers.StatefulSetInformer,
	NotebookInformer informers.NotebookInformer) *Controller {

	// Create event broadcaster
	// Add sample-controller types to the default Kubernetes Scheme so Events can be
	// logged for sample-controller types.
	utilruntime.Must(scheme.AddToScheme(scheme.Scheme))
	klog.V(4).Info("Creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartStructuredLogging(0)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclientset.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	controller := &Controller{
		kubeclientset:      kubeclientset,
		sampleclientset:    sampleclientset,
		statefulsetsLister: statefulsetInformer.Lister(),
		statefulsetsSynced: statefulsetInformer.Informer().HasSynced,
		notebooksLister:    NotebookInformer.Lister(),
		notebooksSynced:    NotebookInformer.Informer().HasSynced,
		workqueue:          workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Notebooks"),
		recorder:           recorder,
	}

	klog.Info("Setting up event handlers")
	// Set up an event handler for when Notebook resources change
	NotebookInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.enqueueNotebook,
		UpdateFunc: func(old, new interface{}) {
			controller.enqueueNotebook(new)
		},
	})
	// Set up an event handler for when Deployment resources change. This
	// handler will lookup the owner of the given Deployment, and if it is
	// owned by a Notebook resource will enqueue that Notebook resource for
	// processing. This way, we don't need to implement custom logic for
	// handling Deployment resources. More info on this pattern:
	// https://github.com/kubernetes/community/blob/8cafef897a22026d42f5e5bb3f104febe7e29830/contributors/devel/controllers.md
	statefulsetInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.handleObject,
		UpdateFunc: func(old, new interface{}) {
			newStatefulset := new.(*appsv1.StatefulSet)
			oldStatefulset := old.(*appsv1.StatefulSet)
			if newStatefulset.ResourceVersion == oldStatefulset.ResourceVersion {
				// Periodic resync will send update events for all known Deployments.
				// Two different versions of the same Deployment will always have different RVs.
				return
			}
			controller.handleObject(new)
		},
		DeleteFunc: controller.handleObject,
	})

	return controller
}

// Run will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers. It will block until stopCh
// is closed, at which point it will shutdown the workqueue and wait for
// workers to finish processing their current work items.
func (c *Controller) Run(threadiness int, stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()

	// Start the informer factories to begin populating the informer caches
	klog.Info("Starting Notebook controller")

	// Wait for the caches to be synced before starting workers
	klog.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, c.statefulsetsSynced, c.notebooksSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	klog.Info("Starting workers")
	// Launch two workers to process Notebook resources
	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	klog.Info("Started workers")
	<-stopCh
	klog.Info("Shutting down workers")

	return nil
}

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the
// workqueue.
func (c *Controller) runWorker() {
	for c.processNextWorkItem() {
	}
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the syncHandler.
func (c *Controller) processNextWorkItem() bool {
	obj, shutdown := c.workqueue.Get()

	if shutdown {
		return false
	}

	// We wrap this block in a func so we can defer c.workqueue.Done.
	err := func(obj interface{}) error {
		// We call Done here so the workqueue knows we have finished
		// processing this item. We also must remember to call Forget if we
		// do not want this work item being re-queued. For example, we do
		// not call Forget if a transient error occurs, instead the item is
		// put back on the workqueue and attempted again after a back-off
		// period.
		defer c.workqueue.Done(obj)
		var key string
		var ok bool
		// We expect strings to come off the workqueue. These are of the
		// form namespace/name. We do this as the delayed nature of the
		// workqueue means the items in the informer cache may actually be
		// more up to date that when the item was initially put onto the
		// workqueue.
		if key, ok = obj.(string); !ok {
			// As the item in the workqueue is actually invalid, we call
			// Forget here else we'd go into a loop of attempting to
			// process a work item that is invalid.
			c.workqueue.Forget(obj)
			utilruntime.HandleError(fmt.Errorf("expected string in workqueue but got %#v", obj))
			return nil
		}
		// Run the syncHandler, passing it the namespace/name string of the
		// Notebook resource to be synced.
		if err := c.syncHandler(key); err != nil {
			// Put the item back on the workqueue to handle any transient errors.
			c.workqueue.AddRateLimited(key)
			return fmt.Errorf("error syncing '%s': %s, requeuing", key, err.Error())
		}
		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		c.workqueue.Forget(obj)
		klog.Infof("Successfully synced '%s'", key)
		return nil
	}(obj)

	if err != nil {
		utilruntime.HandleError(err)
		return true
	}

	return true
}

// syncHandler compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the Notebook resource
// with the current status of the resource.
func (c *Controller) syncHandler(key string) error {
	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	// Get the Notebook resource with this namespace/name
	notebook, err := c.notebooksLister.Notebooks(namespace).Get(name)
	if err != nil {
		// The Notebook resource may no longer exist, in which case we stop
		// processing.
		if errors.IsNotFound(err) {
			utilruntime.HandleError(fmt.Errorf("notebook '%s' in work queue no longer exists", key))
			return nil
		}

		return err
	}

	statefulsetName := notebook.Spec.StatefulsetName
	if statefulsetName == "" {
		// We choose to absorb the error here as the worker would requeue the
		// resource otherwise. Instead, the next time the resource is updated
		// the resource will be queued again.
		utilruntime.HandleError(fmt.Errorf("%s: statefulset name must be specified", key))
		return nil
	}

	// Get the statefulset with the name specified in Notebook.spec
	statefulSet, err := c.statefulsetsLister.StatefulSets(notebook.Namespace).Get(statefulsetName)
	// If the resource doesn't exist, we'll create it
	if errors.IsNotFound(err) {
		statefulSet, err = c.kubeclientset.AppsV1().StatefulSets(notebook.Namespace).Create(context.TODO(), newStatefulset(notebook), metav1.CreateOptions{})
	}

	// If an error occurs during Get/Create, we'll requeue the item so we can
	// attempt processing again later. This could have been caused by a
	// temporary network failure, or any other transient reason.
	if err != nil {
		return err
	}

	// If the Statefulset is not controlled by this Notebook resource, we should log
	// a warning to the event recorder and return error msg.
	if !metav1.IsControlledBy(statefulSet, notebook) {
		msg := fmt.Sprintf(MessageResourceExists, statefulSet.Name)
		c.recorder.Event(notebook, corev1.EventTypeWarning, ErrResourceExists, msg)
		return fmt.Errorf(msg)
	}

	// If this number of the replicas on the Notebook resource is specified, and the
	// number does not equal the current desired replicas on the Deployment, we
	// should update the Statefulset resource.
	if notebook.Spec.Replicas != nil && *notebook.Spec.Replicas != *statefulSet.Spec.Replicas {
		klog.V(4).Infof("Notebook %s replicas: %d, statefulSet replicas: %d", name, *notebook.Spec.Replicas, *statefulSet.Spec.Replicas)
		statefulSet, err = c.kubeclientset.AppsV1().StatefulSets(notebook.Namespace).Update(context.TODO(), newStatefulset(notebook), metav1.UpdateOptions{})
	}

	// If an error occurs during Update, we'll requeue the item so we can
	// attempt processing again later. This could have been caused by a
	// temporary network failure, or any other transient reason.
	if err != nil {
		return err
	}

	// Reconcile service

	// Check if the Service already exists

	// Update the foundService object and write the result back if there are any changes

	// Reconcile IngressRoute if we use Traefik
	//if os.Getenv("USE_Traefik") == "true" {
	//
	//}

	// Finally, we update the status block of the Notebook resource to reflect the
	// current state of the world
	err = c.updateNotebookStatus(notebook, statefulSet)
	if err != nil {
		return err
	}

	c.recorder.Event(notebook, corev1.EventTypeNormal, SuccessSynced, MessageResourceSynced)
	return nil
}

func (c *Controller) updateNotebookStatus(notebook *v1alpha1.Notebook, statefulSet *appsv1.StatefulSet) error {
	// NEVER modify objects from the store. It's a read-only, local cache.
	// You can use DeepCopy() to make a deep copy of original object and modify this copy
	// Or create a copy manually for better performance
	notebookCopy := notebook.DeepCopy()
	notebookCopy.Status.ReadyReplicas = statefulSet.Status.ReadyReplicas
	// If the CustomResourceSubresources feature gate is not enabled,
	// we must use Update instead of UpdateStatus to update the Status block of the Notebook resource.
	// UpdateStatus will not allow changes to the Spec of the resource,
	// which is ideal for ensuring nothing other than resource status has been updated.
	_, err := c.sampleclientset.SamplecontrollerV1alpha1().Foos(notebook.Namespace).Update(context.TODO(), notebookCopy, metav1.UpdateOptions{})
	return err
}

// enqueueNotebook takes a Notebook resource and converts it into a namespace/name
// string which is then put onto the work queue. This method should *not* be
// passed resources of any type other than Notebook.
func (c *Controller) enqueueNotebook(obj interface{}) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.workqueue.Add(key)
}

// handleObject will take any resource implementing metav1.Object and attempt
// to find the Notebook resource that 'owns' it. It does this by looking at the
// objects metadata.ownerReferences field for an appropriate OwnerReference.
// It then enqueues that Notebook resource to be processed. If the object does not
// have an appropriate OwnerReference, it will simply be skipped.
func (c *Controller) handleObject(obj interface{}) {
	var object metav1.Object
	var ok bool
	if object, ok = obj.(metav1.Object); !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("error decoding object, invalid type"))
			return
		}
		object, ok = tombstone.Obj.(metav1.Object)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("error decoding object tombstone, invalid type"))
			return
		}
		klog.V(4).Infof("Recovered deleted object '%s' from tombstone", object.GetName())
	}
	klog.V(4).Infof("Processing object: %s", object.GetName())
	if ownerRef := metav1.GetControllerOf(object); ownerRef != nil {
		// If this object is not owned by a Notebook, we should not do anything more
		// with it.
		if ownerRef.Kind != "Notebook" {
			return
		}

		foo, err := c.notebooksLister.Notebooks(object.GetNamespace()).Get(ownerRef.Name)
		if err != nil {
			klog.V(4).Infof("ignoring orphaned object '%s' of Notebook '%s'", object.GetSelfLink(), ownerRef.Name)
			return
		}

		c.enqueueNotebook(foo)
		return
	}
}

// newStatefulset creates a new Statefulset for a Notebook resource. It also sets
// the appropriate OwnerReferences on the resource so handleObject can discover
// the Notebook resource that 'owns' it.
func newStatefulset(notebook *v1alpha1.Notebook) *appsv1.StatefulSet {
	replica := int32(1)

	statefulset := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      notebook.Spec.StatefulsetName,
			Namespace: notebook.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(notebook, v1alpha1.SchemeGroupVersion.WithKind("Notebook")),
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replica,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"statefulset": notebook.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
					"statefulset":   notebook.Name,
					"notebook-name": notebook.Name,
				}},
				Spec: notebook.Spec.Template.Spec,
			},
		},
	}

	// copy all of the Notebook labels to the pod including poddefault related labels
	l := &statefulset.Spec.Template.ObjectMeta.Labels
	for k, v := range notebook.ObjectMeta.Labels {
		(*l)[k] = v
	}

	podSpec := &statefulset.Spec.Template.Spec
	container := &podSpec.Containers[0]
	if container.WorkingDir == "" {
		container.WorkingDir = "/home/jovyan"
	}

	if container.Ports == nil {
		container.Ports = []corev1.ContainerPort{
			{
				ContainerPort: DefaultContainerPort,
				Name:          "notebook-port",
				Protocol:      "TCP",
			},
		}
	}
	container.Env = append(container.Env, corev1.EnvVar{
		Name:  "NB_PREFIX",
		Value: "/notebook/" + notebook.Namespace + "/" + notebook.Name,
	})
	return statefulset
}

func newService() *corev1.Service {
	return nil
}
