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
	"k8s.io/apimachinery/pkg/util/intstr"
	v12 "k8s.io/client-go/informers/core/v1"
	v1 "k8s.io/client-go/listers/core/v1"
	"time"

	"golang.org/x/time/rate"

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

	samplev1alpha1 "k8s.io/sample-controller/pkg/apis/calico/v1alpha1"
	clientset "k8s.io/sample-controller/pkg/generated/clientset/versioned"
	samplescheme "k8s.io/sample-controller/pkg/generated/clientset/versioned/scheme"
	informers "k8s.io/sample-controller/pkg/generated/informers/externalversions/calico/v1alpha1"
	listers "k8s.io/sample-controller/pkg/generated/listers/calico/v1alpha1"
)

const controllerAgentName = "sample-controller"

const (
	// SuccessSynced is used as part of the Event 'reason' when a Bookstore is synced
	SuccessSynced = "Synced"
	// ErrResourceExists is used as part of the Event 'reason' when a Bookstore fails
	// to sync due to a Deployment of the same name already existing.
	ErrResourceExists = "ErrResourceExists"

	// MessageResourceExists is the message used for Events when a resource
	// fails to sync due to a Deployment already existing
	MessageResourceExists = "Resource %q already exists and is not managed by Bookstore"
	// MessageResourceSynced is the message used for an Event fired when a Bookstore
	// is synced successfully
	MessageResourceSynced = "Bookstore synced successfully"
)

// Controller is the controller implementation for Bookstore resources
type Controller struct {
	// kubeclientset is a standard kubernetes clientset
	kubeclientset kubernetes.Interface
	// sampleclientset is a clientset for our own API group
	sampleclientset clientset.Interface

	serviceLister     v1.ServiceLister
	serviceSynced     cache.InformerSynced
	deploymentsLister appslisters.DeploymentLister
	deploymentsSynced cache.InformerSynced
	bookstoresLister  listers.BookstoreLister
	bookstoresSynced  cache.InformerSynced

	// workqueue is a rate limited work queue. This is used to queue work to be
	// processed instead of performing it as soon as a change happens. This
	// means we can ensure we only process a fixed amount of resources at a
	// time, and makes it easy to ensure we are never processing the same item
	// simultaneously in two different workers.
	workqueue workqueue.TypedRateLimitingInterface[string]
	// recorder is an event recorder for recording Event resources to the
	// Kubernetes API.
	recorder record.EventRecorder
}

// NewController returns a new sample controller
func NewController(
	ctx context.Context,
	kubeclientset kubernetes.Interface,
	sampleclientset clientset.Interface,
	deploymentInformer appsinformers.DeploymentInformer,
	serviceInformer v12.ServiceInformer,
	bookstoreInformer informers.BookstoreInformer) *Controller {
	logger := klog.FromContext(ctx)

	// Create event broadcaster
	// Add sample-controller types to the default Kubernetes Scheme so Events can be
	// logged for sample-controller types.
	utilruntime.Must(samplescheme.AddToScheme(scheme.Scheme))
	logger.V(4).Info("Creating event broadcaster")

	eventBroadcaster := record.NewBroadcaster(record.WithContext(ctx))
	eventBroadcaster.StartStructuredLogging(0)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclientset.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})
	ratelimiter := workqueue.NewTypedMaxOfRateLimiter(
		workqueue.NewTypedItemExponentialFailureRateLimiter[string](5*time.Millisecond, 1000*time.Second),
		&workqueue.TypedBucketRateLimiter[string]{Limiter: rate.NewLimiter(rate.Limit(50), 300)},
	)

	controller := &Controller{
		kubeclientset:     kubeclientset,
		sampleclientset:   sampleclientset,
		serviceLister:     serviceInformer.Lister(),
		serviceSynced:     serviceInformer.Informer().HasSynced,
		deploymentsLister: deploymentInformer.Lister(),
		deploymentsSynced: deploymentInformer.Informer().HasSynced,
		bookstoresLister:  bookstoreInformer.Lister(),
		bookstoresSynced:  bookstoreInformer.Informer().HasSynced,
		workqueue:         workqueue.NewTypedRateLimitingQueue(ratelimiter),
		recorder:          recorder,
	}

	logger.Info("Setting up event handlers")
	// Set up an event handler for when Bookstore resources change
	bookstoreInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.enqueueBookstore,
		UpdateFunc: func(old, new interface{}) {
			controller.enqueueBookstore(new)
		},
	})
	// Set up an event handler for when Deployment resources change. This
	// handler will lookup the owner of the given Deployment, and if it is
	// owned by a Bookstore resource then the handler will enqueue that Bookstore resource for
	// processing. This way, we don't need to implement custom logic for
	// handling Deployment resources. More info on this pattern:
	// https://github.com/kubernetes/community/blob/8cafef897a22026d42f5e5bb3f104febe7e29830/contributors/devel/controllers.md
	deploymentInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.handleObject,
		UpdateFunc: func(old, new interface{}) {
			newDepl := new.(*appsv1.Deployment)
			oldDepl := old.(*appsv1.Deployment)
			if newDepl.ResourceVersion == oldDepl.ResourceVersion {
				// Periodic resync will send update events for all known Deployments.
				// Two different versions of the same Deployment will always have different RVs.
				return
			}
			controller.handleObject(new)
		},
		DeleteFunc: controller.handleObject,
	})

	serviceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.handleObject,
		UpdateFunc: func(old, new interface{}) {
			oldSVC := old.(*corev1.Service)
			newSVC := new.(*corev1.Service)
			if oldSVC.ResourceVersion == newSVC.ResourceVersion {
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
func (c *Controller) Run(ctx context.Context, workers int) error {
	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()
	logger := klog.FromContext(ctx)

	// Start the informer factories to begin populating the informer caches
	logger.Info("Starting Bookstore controller")

	// Wait for the caches to be synced before starting workers
	logger.Info("Waiting for informer caches to sync")

	if ok := cache.WaitForCacheSync(ctx.Done(), c.deploymentsSynced, c.bookstoresSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	logger.Info("Starting workers", "count", workers)
	// Launch two workers to process Bookstore resources
	for i := 0; i < workers; i++ {
		go wait.UntilWithContext(ctx, c.runWorker, time.Second)
	}

	logger.Info("Started workers")
	<-ctx.Done()
	logger.Info("Shutting down workers")

	return nil
}

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the
// workqueue.
func (c *Controller) runWorker(ctx context.Context) {
	for c.processNextWorkItem(ctx) {
	}
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the syncHandler.
func (c *Controller) processNextWorkItem(ctx context.Context) bool {
	obj, shutdown := c.workqueue.Get()
	logger := klog.FromContext(ctx)

	if shutdown {
		return false
	}

	// We wrap this block in a func so we can defer c.workqueue.Done.
	err := func(key string) error {
		// We call Done here so the workqueue knows we have finished
		// processing this item. We also must remember to call Forget if we
		// do not want this work item being re-queued. For example, we do
		// not call Forget if a transient error occurs, instead the item is
		// put back on the workqueue and attempted again after a back-off
		// period.
		defer c.workqueue.Done(key)
		// Run the syncHandler, passing it the namespace/name string of the
		// Bookstore resource to be synced.
		if err := c.syncHandler(ctx, key); err != nil {
			// Put the item back on the workqueue to handle any transient errors.
			c.workqueue.AddRateLimited(key)
			return fmt.Errorf("error syncing '%s': %s, requeuing", key, err.Error())
		}
		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		c.workqueue.Forget(obj)
		logger.Info("Successfully synced", "resourceName", key)
		return nil
	}(obj)

	if err != nil {
		utilruntime.HandleError(err)
		return true
	}

	return true
}

// syncHandler compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the Bookstore resource
// with the current status of the resource.
func (c *Controller) syncHandler(ctx context.Context, key string) error {
	// Convert the namespace/name string into a distinct namespace and name
	logger := klog.LoggerWithValues(klog.FromContext(ctx), "resourceName", key)

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		logger.Error(err, "invalid resource key")
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	// Get the Booksore resource with this namespace/name
	bookstore, err := c.bookstoresLister.Bookstores(namespace).Get(name)
	if err != nil {
		// The Bookstore resource may no longer exist, in which case we stop
		// processing.
		if errors.IsNotFound(err) {
			utilruntime.HandleError(fmt.Errorf("bookstore '%s' in work queue no longer exists", key))
			return nil
		}
		logger.Error(err, "bookstore lister unknown error")
		return err
	}

	if bookstore.Spec.DeploymentName == "" {
		// We choose to absorb the error here as the worker would requeue the
		// resource otherwise. Instead, the next time the resource is updated
		// the resource will be queued again.
		utilruntime.HandleError(fmt.Errorf("%s: deployment name must be specified", key))
		return nil
	}

	// Get the deployment with the name specified in Bookstore.spec
	deployment, err := c.deploymentsLister.Deployments(bookstore.Namespace).Get(bookstore.Spec.DeploymentName)
	// If the resource doesn't exist, we'll create it
	if errors.IsNotFound(err) {
		deployment, err = c.kubeclientset.AppsV1().Deployments(bookstore.Namespace).Create(context.TODO(), newDeployment(bookstore), metav1.CreateOptions{})
	}

	// If an error occurs during Get/Create, we'll requeue the item so we can
	// attempt processing again later. This could have been caused by a
	// temporary network failure, or any other transient reason.
	if err != nil {
		logger.Error(err, "error creating deployments")
		return err
	}

	// If the Deployment is not controlled by this Bookstore resource, we should log
	// a warning to the event recorder and return error msg.
	if !metav1.IsControlledBy(deployment, bookstore) {
		msg := fmt.Sprintf(MessageResourceExists, deployment.Name)
		c.recorder.Event(bookstore, corev1.EventTypeWarning, ErrResourceExists, msg)
		return fmt.Errorf("%s", msg)
	}

	// If this number of the replicas on the Bookstore resource is specified, and the
	// number does not equal the current desired replicas on the Deployment, we
	// should update the Deployment resource.
	if bookstore.Spec.Replicas != nil && *bookstore.Spec.Replicas != *deployment.Spec.Replicas {
		logger.V(4).Info("Update deployment resource", "currentReplicas", *bookstore.Spec.Replicas, "desiredReplicas", *deployment.Spec.Replicas)
		deployment, err = c.kubeclientset.AppsV1().Deployments(bookstore.Namespace).Update(context.TODO(), newDeployment(bookstore), metav1.UpdateOptions{})
	}

	// If an error occurs during Update, we'll requeue the item so we can
	// attempt processing again later. This could have been caused by a
	// temporary network failure, or any other transient reason.
	if err != nil {
		logger.Error(err, "error updating deployment")
		return err
	}

	if bookstore.Spec.ServiceName == "" {
		utilruntime.HandleError(fmt.Errorf("%s: service name must be specified", key))
		return nil
	}

	service, err := c.serviceLister.Services(bookstore.Namespace).Get(bookstore.Spec.ServiceName)
	if errors.IsNotFound(err) {
		service, err = c.kubeclientset.CoreV1().Services(bookstore.Namespace).Create(context.TODO(), newService(bookstore), metav1.CreateOptions{})
	}

	if err != nil {
		logger.Error(err, "error creating service")
		return err
	}

	if !metav1.IsControlledBy(service, bookstore) {
		msg := fmt.Sprintf(MessageResourceExists, service.Name)
		c.recorder.Event(bookstore, corev1.EventTypeWarning, ErrResourceExists, msg)
		return fmt.Errorf("%s", msg)
	}

	// Finally, we update the status block of the Bookstore resource to reflect the
	// current state of the world
	err = c.updateBookstoreStatus(bookstore, deployment)
	if err != nil {
		logger.Error(err, "error updating bookstore status")
		return err
	}

	//fmt.Println("\n-------------------------------------------------------\nNO ERROR\n-------------------------------------------------------\n")
	c.recorder.Event(bookstore, corev1.EventTypeNormal, SuccessSynced, MessageResourceSynced)
	return nil
}

func (c *Controller) updateBookstoreStatus(bookstore *samplev1alpha1.Bookstore, deployment *appsv1.Deployment) error {
	// NEVER modify objects from the store. It's a read-only, local cache.
	// You can use DeepCopy() to make a deep copy of original object and modify this copy
	// Or create a copy manually for better performance
	bookstoreCopy := bookstore.DeepCopy()
	bookstoreCopy.Status.AvailableReplicas = deployment.Status.AvailableReplicas
	// If the CustomResourceSubresources feature gate is not enabled,
	// we must use Update instead of UpdateStatus to update the Status block of the Bookstore resource.
	// UpdateStatus will not allow changes to the Spec of the resource,
	// which is ideal for ensuring nothing other than resource status has been updated.
	_, err := c.sampleclientset.CalicoV1alpha1().Bookstores(bookstore.Namespace).UpdateStatus(context.TODO(), bookstoreCopy, metav1.UpdateOptions{})
	return err
}

// enqueueBookstore takes a Bookstore resource and converts it into a namespace/name
// string which is then put onto the work queue. This method should *not* be
// passed resources of any type other than Bookstore.
func (c *Controller) enqueueBookstore(obj interface{}) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.workqueue.Add(key)
}

// handleObject will take any resource implementing metav1.Object and attempt
// to find the Bookstore resource that 'owns' it. It does this by looking at the
// objects metadata.ownerReferences field for an appropriate OwnerReference.
// It then enqueues that Bookstore resource to be processed. If the object does not
// have an appropriate OwnerReference, it will simply be skipped.
func (c *Controller) handleObject(obj interface{}) {
	var object metav1.Object
	var ok bool
	logger := klog.FromContext(context.Background())
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
		logger.V(4).Info("Recovered deleted object", "resourceName", object.GetName())
	}
	logger.V(4).Info("Processing object", "object", klog.KObj(object))
	if ownerRef := metav1.GetControllerOf(object); ownerRef != nil {
		// If this object is not owned by a Bookstore, we should not do anything more
		// with it.
		if ownerRef.Kind != "Bookstore" {
			return
		}

		bookstore, err := c.bookstoresLister.Bookstores(object.GetNamespace()).Get(ownerRef.Name)
		if err != nil {
			logger.V(4).Info("Ignore orphaned object", "object", klog.KObj(object), "bookstore", ownerRef.Name)
			return
		}

		c.enqueueBookstore(bookstore)
		return
	}
}

// newDeployment creates a new Deployment for a Bookstore resource. It also sets
// the appropriate OwnerReferences on the resource so handleObject can discover
// the Bookstore resource that 'owns' it.
func newDeployment(bookstore *samplev1alpha1.Bookstore) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      bookstore.Spec.DeploymentName,
			Namespace: bookstore.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(bookstore, samplev1alpha1.SchemeGroupVersion.WithKind("Bookstore")),
			},
		},

		Spec: appsv1.DeploymentSpec{
			Replicas: bookstore.Spec.Replicas,

			Selector: &metav1.LabelSelector{
				MatchLabels: bookstore.GetSelectorLabels(),
			},

			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: bookstore.GetSelectorLabels(),
				},

				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            bookstore.Spec.DeploymentName,
							Image:           bookstore.Spec.DeploymentImageName + ":" + bookstore.Spec.DeploymentImageTag,
							ImagePullPolicy: corev1.PullPolicy(bookstore.Spec.ImagePullPolicy),

							Ports: []corev1.ContainerPort{
								{
									ContainerPort: bookstore.Spec.ContainerPort,
								},
							},

							Env: []corev1.EnvVar{
								{
									Name: "AdminUsername",
									//Value: "admin22",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: "env-secrets",
											},
											Key: bookstore.Spec.EnvAdminUsername,
											Optional: func() *bool {
												var flag = false
												return &flag
											}(),
										},
									},
								},

								{
									Name: "AdminPassword",
									//Value: "admin72",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: "env-secrets",
											},
											Key: bookstore.Spec.EnvAdminPassword,
											Optional: func() *bool {
												var flag = false
												return &flag
											}(),
										},
									},
								},

								{
									Name: "JWTSECRET",
									//Value: "orangeCat",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: "env-secrets",
											},
											Key: bookstore.Spec.EnvJWTSECRET,
											Optional: func() *bool {
												var flag = false
												return &flag
											}(),
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
}

func newService(bookstore *samplev1alpha1.Bookstore) *corev1.Service {
	fmt.Println("CREATING SERVICE")
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "core/v1",
		},

		ObjectMeta: metav1.ObjectMeta{
			Name:   bookstore.Spec.ServiceName,
			Labels: bookstore.GetSelectorLabels(),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(bookstore, samplev1alpha1.SchemeGroupVersion.WithKind("Bookstore")),
			},
		},

		Spec: corev1.ServiceSpec{
			Selector: bookstore.GetSelectorLabels(),
			Type:     corev1.ServiceType(bookstore.Spec.ServiceType),
			Ports: []corev1.ServicePort{
				{
					Protocol:   corev1.ProtocolTCP,
					Port:       bookstore.Spec.ContainerPort,
					TargetPort: intstr.FromInt32(bookstore.Spec.TargetPort),
					NodePort:   bookstore.Spec.NodePort,
				},
			},
		},
	}
}
