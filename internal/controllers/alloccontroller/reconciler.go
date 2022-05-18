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

package alloccontroller

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/openconfig/ygot/ygot"
	"github.com/pkg/errors"
	pkgmetav1 "github.com/yndd/ndd-core/apis/pkg/meta/v1"
	"github.com/yndd/ndd-runtime/pkg/event"
	"github.com/yndd/ndd-runtime/pkg/logging"
	"github.com/yndd/ndd-runtime/pkg/meta"
	targetv1 "github.com/yndd/ndd-target-runtime/apis/dvr/v1"
	"github.com/yndd/ndd-target-runtime/pkg/resource"
	"github.com/yndd/ndd-target-runtime/pkg/shared"
	"github.com/yndd/ndd-target-runtime/pkg/ygotnddtarget"
	"github.com/yndd/provider-controller/pkg/inventory"
	"github.com/yndd/provider-controller/pkg/watcher"
	"github.com/yndd/registrator/registrator"
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
	errGetTarget          = "cannot get target cr"
	errGetTargetSpec      = "cannot get target spec"
	errGetTargetList      = "cannot get target cr list"
	errGetPod             = "cannot get pod cr"
	errGetPodList         = "cannot get pod cr list"
	//errGetCtrlMetaCfg     = "cannot get controller meta config cr"
	errGetCtrlMetaCfgList = "cannot get controller meta config cr list"
	errGetCrd             = "cannot get crd"
	errUpdateStatus       = "cannot update status"
	//errApplyStatfullSet        = "cannot apply statefulset"
	//errApplyCertificate        = "cannot apply certificate"
	//errApplyService            = "cannot apply service"
	//errApplyMutatingWebhook    = "cannot apply mutating webhook"
	//errApplyValidatingWebhook  = "cannot apply validating webhook"
	//errApplyClusterRoles       = "cannot apply clusterrole"
	//errApplyClusterRoleBinding = "cannot apply clusterrolebinding"
	//errApplyServiceAccount     = "cannot apply service account"
	//event
	reasonCreatedStatefullSet event.Reason = "CreatedStatefullSet"
	reasonAllocatedPod        event.Reason = "AllocatedPod"
)

// ReconcilerOption is used to configure the Reconciler.
type ReconcilerOption func(*Reconciler)

// Reconciler reconciles packages.
type Reconciler struct {
	client    resource.ClientApplicator
	finalizer resource.Finalizer
	// servicediscovery registrator
	registrator  registrator.Registrator
	watcher      watcher.Watcher
	inventory    inventory.Inventory
	pollInterval time.Duration

	m        sync.Mutex
	watchers map[string]context.CancelFunc

	newTarget func() targetv1.Tg
	//newProviderRevision func() pkgv1.PackageRevision
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

// WithWatcher specifies how the Reconciler watches services
func WithWatcher(w watcher.Watcher) ReconcilerOption {
	return func(r *Reconciler) {
		r.watcher = w
	}
}

// SetupProvider adds a controller that reconciles Providers.
func Setup(mgr ctrl.Manager, nddopts *shared.NddControllerOptions) (chan cevent.GenericEvent, error) {
	name := "config-controller/" + strings.ToLower(targetv1.TargetGroupKind)
	tgl := func() targetv1.TgList { return &targetv1.TargetList{} }

	e := make(chan cevent.GenericEvent)

	r := NewReconciler(mgr,
		WithRegistrator(nddopts.Registrator),
		WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
	)

	ControllerConfigHandler := &EnqueueRequestForAllControllerConfig{
		client:        mgr.GetClient(),
		log:           nddopts.Logger,
		ctx:           context.Background(),
		newTargetList: tgl,
	}

	return e, ctrl.NewControllerManagedBy(mgr).
		WithOptions(nddopts.Copts).
		Named(name).
		For(&targetv1.Target{}).
		WithEventFilter(resource.IgnoreUpdateWithoutGenerationChangePredicate()).
		Watches(&source.Kind{Type: &pkgmetav1.ControllerConfig{}}, ControllerConfigHandler).
		Watches(&source.Channel{Source: e}, ControllerConfigHandler).
		Complete(r)
}

// NewReconciler creates a new package reconciler.
func NewReconciler(m ctrl.Manager, opts ...ReconcilerOption) *Reconciler {
	tg := func() targetv1.Tg { return &targetv1.Target{} }

	r := &Reconciler{
		client: resource.ClientApplicator{
			Client:     m.GetClient(),
			Applicator: resource.NewAPIPatchingApplicator(m.GetClient()),
		},
		pollInterval: defaultpollInterval,
		log:          logging.NewNopLogger(),
		record:       event.NewNopRecorder(),
		newTarget:    tg,
		finalizer:    resource.NewAPIFinalizer(m.GetClient(), finalizerName),
	}

	for _, f := range opts {
		f(r)
	}
	r.inventory = inventory.New(m.GetClient(), r.registrator)
	return r
}

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) { // nolint:gocyclo
	log := r.log.WithValues("NameSpaceName", req.NamespacedName)
	log.Debug("allocation reconciler start...")

	// get the target info
	t := r.newTarget()
	if err := r.client.Get(ctx, req.NamespacedName, t); err != nil {
		// There's no need to requeue if we no longer exist. Otherwise we'll be
		// requeued implicitly because we return an error.
		log.Debug(errGetTarget, "error", err)
		return reconcile.Result{}, errors.Wrap(resource.IgnoreNotFound(err), errGetTarget)
	}

	if meta.WasDeleted(t) {
		return reconcile.Result{Requeue: false}, errors.Wrap(r.client.Update(ctx, t), errUpdateStatus)
	}

	// get the spec using ygot structs
	tspec, err := t.GetSpec()
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, errGetTargetSpec)
	}

	// get the proper controller config spec matching the vendor type
	ccList := &pkgmetav1.ControllerConfigList{}
	if err := r.client.List(ctx, ccList); err != nil {
		log.Debug(errGetCtrlMetaCfgList, "error", err)
		return reconcile.Result{}, errors.Wrap(err, errGetCtrlMetaCfgList)
	}
	var cc *pkgmetav1.ControllerConfig
	for _, cmc := range ccList.Items {
		if cmc.Spec.VendorType == tspec.VendorType.String() {
			cc = &cmc
			break
		}
	}
	if cc == nil {
		// no controller config found for the target vendor type
		log.Debug("no controller config found for vendor Type", "error", err)
		return reconcile.Result{RequeueAfter: shortWait}, nil
	}

	// per service validate the allocation
	for _, serviceInfo := range cc.GetServicesInfo() {
		log = log.WithValues("serviceName", serviceInfo.ServiceName)
		// initialize allocation map if it is not initialized
		if tspec.Allocation == nil {
			tspec.Allocation = map[string]*ygotnddtarget.NddTarget_TargetEntry_Allocation{}
		}

		// check if the service was allocated
		allocatedServiceInstance, ok := tspec.Allocation[serviceInfo.ServiceName]
		if !ok {
			// service is not allocated
			log.Debug("service is not allocated")
			assigned := r.inventory.GetLeastLoaded(ctx, cc, serviceInfo.ServiceName)
			if assigned == "" {
				return reconcile.Result{}, fmt.Errorf("could not assign an instance for service %q", serviceInfo.ServiceName)
			}
			// allocate the serviceInstance
			serviceInstance := &ygotnddtarget.NddTarget_TargetEntry_Allocation{
				ServiceName:     ygot.String(serviceInfo.ServiceName),
				ServiceIdentity: ygot.String(assigned),
			}
			log.Debug("service allocated", "serviceInstance", serviceInstance)
			tspec.AppendAllocation(serviceInstance)
		} else {
			// service is allocated
			log.Debug("service is allocated")
			// check if the service is alive
			// check if the service is alive -> if not delete the allocation
			availableServiceInstances, err := r.registrator.Query(ctx, serviceInfo.ServiceName, []string{})
			if err != nil {
				log.Debug("cannot query service registry", "error", err)
				return reconcile.Result{}, errors.Wrap(err, "cannot query service registry")
			}
			found := false
			for _, availableServiceInstance := range availableServiceInstances {
				if availableServiceInstance.Name == *allocatedServiceInstance.ServiceIdentity {
					found = true
					break
				}
			}
			if !found {
				// delete the allocation
				log.Debug("service is not alive -> deallocate")
				tspec.DeleteAllocation(serviceInfo.ServiceName)
			}

			// When the service is found
			if serviceInfo.Kind == pkgmetav1.KindWorker {
				targetServiceInfo := cc.GetTargetServiceInfo()

				availableTargetServiceInstances, err := r.registrator.Query(
					ctx,
					targetServiceInfo.ServiceName,
					[]string{fmt.Sprintf("target=%s/%s", t.GetNamespace(), t.GetName())},
				)
				if err != nil {
					log.Debug("cannot query service registry", "error", err)
					return reconcile.Result{}, errors.Wrap(err, "cannot query service registry")
				}
				if len(availableTargetServiceInstances) == 0 {
					// delete the allocation since the target is not alive
					log.Debug("target service is not alive -> deallocate")
					tspec.DeleteAllocation(targetServiceInfo.ServiceName)
				}
			}
		}
	}
	return reconcile.Result{
		Requeue:      true,
		RequeueAfter: time.Minute,
	}, errors.Wrap(r.client.Update(ctx, t), "cannot update target")
}

func (r *Reconciler) addWatcher(nsName string, cc *pkgmetav1.ControllerConfig) {
	r.m.Lock()
	defer r.m.Unlock()
	if _, ok := r.watchers[nsName]; !ok {
		ctx, cfn := context.WithCancel(context.Background())
		r.watchers[nsName] = cfn
		r.watcher.Watch(ctx, cc)
	}
}

func (r *Reconciler) deleteWatcher(nsName string) {
	r.m.Lock()
	defer r.m.Unlock()
	cfn, ok := r.watchers[nsName]
	if ok {
		cfn()
	}
}
