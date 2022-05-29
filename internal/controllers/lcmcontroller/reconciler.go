/*
Copyright 2021 NDD.

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

package lcmcontroller

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	pkgmetav1 "github.com/yndd/ndd-core/apis/pkg/meta/v1"
	pkgv1 "github.com/yndd/ndd-core/apis/pkg/v1"
	"github.com/yndd/ndd-runtime/pkg/event"
	"github.com/yndd/ndd-runtime/pkg/logging"
	"github.com/yndd/ndd-runtime/pkg/meta"
	targetv1 "github.com/yndd/ndd-target-runtime/apis/dvr/v1"
	"github.com/yndd/ndd-target-runtime/pkg/resource"
	"github.com/yndd/ndd-target-runtime/pkg/shared"
	"github.com/yndd/provider-controller/pkg/inventory"
	"github.com/yndd/registrator/registrator"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	cevent "sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	// finalizer
	finalizerName = "finalizer.controller.srl.config.ndd.yndd.io"
	// timers
	defaultpollInterval = 1 * time.Minute
	shortWait           = 1 * time.Minute
	// errors
	errGetControllerConfig     = "cannot get controller config cr"
	errGetTargetList           = "cannot get target cr list"
	errGetPod                  = "cannot get pod cr"
	errGetPodList              = "cannot get pod cr list"
	errGetCrd                  = "cannot get crd"
	errUpdateStatus            = "cannot update status"
	errApplyStatfullSet        = "cannot apply statefulset"
	errApplyCertificate        = "cannot apply certificate"
	errApplyService            = "cannot apply service"
	errApplyMutatingWebhook    = "cannot apply mutating webhook"
	errApplyValidatingWebhook  = "cannot apply validating webhook"
	errApplyClusterRoles       = "cannot apply clusterrole"
	errApplyClusterRoleBinding = "cannot apply clusterrolebinding"
	errApplyServiceAccount     = "cannot apply service account"
	//event
	reasonCreatedStatefullSet event.Reason = "CreatedStatefullSet"
	reasonAllocatedPod        event.Reason = "AllocatedPod"
	reasonMaxReplicasReached  event.Reason = "MaxReplicasReached"
)

// ReconcilerOption is used to configure the Reconciler.
type ReconcilerOption func(*Reconciler)

// Reconciler reconciles packages.
type Reconciler struct {
	client    resource.ClientApplicator
	finalizer resource.Finalizer
	// servicediscovery registrator
	registrator registrator.Registrator
	inventory   inventory.Inventory
	//deployer    deployer.Deployer

	m             sync.Mutex
	watchers      map[string]context.CancelFunc
	newTargetList func() targetv1.TgList

	crdNames     []string
	pollInterval time.Duration

	log    logging.Logger
	record event.Recorder
}

// WithLogger specifies how the Reconciler logs messages.
func WithLogger(l logging.Logger) ReconcilerOption {
	return func(r *Reconciler) {
		r.log = l
	}
}

// WithRecorder specifies how the Reconciler records events.
func WithRecorder(er event.Recorder) ReconcilerOption {
	return func(r *Reconciler) {
		r.record = er
	}
}

// WithRegistrator specifies how the Reconciler registers and discover services
func WithRegistrator(reg registrator.Registrator) ReconcilerOption {
	return func(r *Reconciler) {
		r.registrator = reg
	}
}

// SetupProvider adds a controller that reconciles Providers.
func Setup(mgr ctrl.Manager, nddopts *shared.NddControllerOptions) (chan cevent.GenericEvent, error) {
	name := "config-controller/" + strings.ToLower(pkgv1.CompositeProviderGroupKind)

	e := make(chan cevent.GenericEvent)

	r := NewReconciler(mgr,
		WithRegistrator(nddopts.Registrator),
		WithLogger(nddopts.Logger),
		WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
	)

	CompositeProviderHandler := &EnqueueRequestForAllCompositeProvider{
		client: mgr.GetClient(),
		log:    nddopts.Logger,
		ctx:    context.Background(),
	}

	return e, ctrl.NewControllerManagedBy(mgr).
		WithOptions(nddopts.Copts).
		Named(name).
		For(&pkgv1.CompositeProvider{}).
		WithEventFilter(resource.IgnoreUpdateWithoutGenerationChangePredicate()).
		Watches(&source.Channel{Source: e}, CompositeProviderHandler).
		Complete(r)
}

// NewReconciler creates a new package reconciler.
func NewReconciler(m ctrl.Manager, opts ...ReconcilerOption) *Reconciler {
	tgl := func() targetv1.TgList { return &targetv1.TargetList{} }

	r := &Reconciler{
		client: resource.ClientApplicator{
			Client:     m.GetClient(),
			Applicator: resource.NewAPIPatchingApplicator(m.GetClient()),
		},
		watchers:      make(map[string]context.CancelFunc),
		newTargetList: tgl,
		pollInterval:  defaultpollInterval,
		log:           logging.NewNopLogger(),
		record:        event.NewNopRecorder(),
		finalizer:     resource.NewAPIFinalizer(m.GetClient(), finalizerName),
	}

	for _, f := range opts {
		f(r)
	}

	r.inventory = inventory.New(m.GetClient(), r.registrator)
	return r
}

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) { // nolint:gocyclo
	log := r.log.WithValues("NameSpaceName", req.NamespacedName)
	log.Debug("lcm reconciler start...")

	// get the controller config info
	cp := &pkgv1.CompositeProvider{}
	if err := r.client.Get(ctx, req.NamespacedName, cp); err != nil {
		// There's no need to requeue if we no longer exist. Otherwise we'll be
		// requeued implicitly because we return an error.
		log.Debug(errGetControllerConfig, "error", err)
		return reconcile.Result{}, errors.Wrap(resource.IgnoreNotFound(err), errGetControllerConfig)
	}

	record := r.record.WithAnnotations("external-name", meta.GetExternalName(cp))

	if meta.WasDeleted(cp) {
		// Delete the watcher
		// r.deleteWatcher(req.NamespacedName.String())
		// Delete finalizer after the object is deleted
		if err := r.finalizer.RemoveFinalizer(ctx, cp); err != nil {
			log.Debug("Cannot remove target cr finalizer", "error", err)
			return reconcile.Result{Requeue: true}, errors.Wrap(err, "cannot remove finalizer")
		}
		return reconcile.Result{Requeue: false}, errors.Wrap(r.client.Update(ctx, cp), "cannot remove finalizer")
	}

	// Add a finalizer
	if err := r.finalizer.AddFinalizer(ctx, cp); err != nil {
		log.Debug("cannot add finalizer", "error", err)
		return reconcile.Result{Requeue: true}, errors.Wrap(r.client.Update(ctx, cp), "cannot add finalizer")
	}

	log.Debug("ctrlMetaCfg", "ctrlMetaCfg spec", cp.Spec)
	// r.addWatcher(req.NamespacedName.String(), cc)

	for _, pkg := range cp.Spec.Packages {
		srvName := pkgv1.GetServiceName(cp.Name, pkg.Name)
		if !r.inventory.ISFull(ctx, cp, srvName) {
			continue
		}
		ss := &appsv1.StatefulSet{}
		if err := r.client.Get(ctx, types.NamespacedName{
			Namespace: cp.GetNamespace(),
			Name:      srvName,
		}, ss); err != nil {
			log.Debug("cannot get statefulset", "error", err)
			return reconcile.Result{Requeue: true}, errors.Wrap(r.client.Update(ctx, cp), "cannot get statefulset")
		}

		p := &pkgmetav1.Provider{}
		if err := r.client.Get(ctx, types.NamespacedName{
			Namespace: cp.GetNamespace(),
			Name:      srvName,
		}, p); err != nil {
			log.Debug("cannot get provider", "error", err)
			return reconcile.Result{Requeue: true}, errors.Wrap(r.client.Update(ctx, cp), "cannot get provider")
		}

		// scale out
		if *ss.Spec.Replicas < *p.Spec.Pod.MaxReplicas {
			// TODO add label to statefulsets
			//pkg.Replicas++
			continue
		}
		log.Info("service already at max replicas", "service", srvName)
		record.Event(cp, event.Warning(reasonMaxReplicasReached, nil, "service-name", srvName))
	}

	// we always deploy since this allows us to handle updates of the deploySpec
	// TODO crd
	/*
		if err := r.deployer.Deploy(ctx, cp); err != nil {
			log.Debug("cannot deploy", "error", err)
			return reconcile.Result{}, errors.Wrap(err, "cannot deploy")
		}
	*/

	// based on logic update replicas in the spec
	log.Debug("target allocation and validation successful")
	// a scale out/in action is triggered by periodic reconciliation (building inventory and deciding on the replicas)
	return reconcile.Result{RequeueAfter: r.pollInterval}, r.client.Update(ctx, cp)
}
