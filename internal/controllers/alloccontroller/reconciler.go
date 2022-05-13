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
	"github.com/yndd/registrator/registrator"
	ctrl "sigs.k8s.io/controller-runtime"
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
	errGetCtrlMetaCfg     = "cannot get controller meta config cr"
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
	pollInterval time.Duration

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

// SetupProvider adds a controller that reconciles Providers.
func Setup(mgr ctrl.Manager, nddopts *shared.NddControllerOptions) error {
	name := "config-controller/" + strings.ToLower(targetv1.TargetGroupKind)
	tgl := func() targetv1.TgList { return &targetv1.TargetList{} }

	r := NewReconciler(mgr,
		WithRegistrator(nddopts.Registrator),
		WithLogger(nddopts.Logger),
		WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
	)

	ControllerConfigHandler := &EnqueueRequestForAllControllerConfig{
		client:        mgr.GetClient(),
		log:           nddopts.Logger,
		ctx:           context.Background(),
		newTargetList: tgl,
	}

	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(nddopts.Copts).
		Named(name).
		For(&targetv1.Target{}).
		WithEventFilter(resource.IgnoreUpdateWithoutGenerationChangePredicate()).
		Watches(&source.Kind{Type: &pkgmetav1.ControllerConfig{}}, ControllerConfigHandler).
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
	ctrlMetaCfgList := &pkgmetav1.ControllerConfigList{}
	if err := r.client.List(ctx, ctrlMetaCfgList); err != nil {
		log.Debug(errGetCtrlMetaCfgList, "error", err)
		return reconcile.Result{}, errors.Wrap(err, errGetCtrlMetaCfgList)
	}
	var ctrlMetaCfg *pkgmetav1.ControllerConfig
	for _, cmc := range ctrlMetaCfgList.Items {
		if cmc.Spec.VendorType == tspec.VendorType.String() {
			ctrlMetaCfg = &cmc
			break
		}
	}
	if ctrlMetaCfg == nil {
		// no controller config found for the target vendor type
		log.Debug("no controller config found for vendor Type", "error", err)
		return reconcile.Result{RequeueAfter: shortWait}, nil
	}

	// build inventory interface
	// -> per worker and reconciler we identify the targets for each and will be able to select

	// per service validate the allocation
	for _, serviceInfo := range ctrlMetaCfg.GetServicesInfo() {
		log.WithValues("serviceName", serviceInfo.ServiceName)
		// initialize allocation map if it is not initialized
		if tspec.Allocation == nil {
			tspec.Allocation = map[string]*ygotnddtarget.NddTarget_TargetEntry_Allocation{}
		}

		// check if the service was allocated
		allocatedServiceInstance, ok := tspec.Allocation[serviceInfo.ServiceName]
		if !ok {
			// service is not allocated
			log.Debug("service is not allocated")
			// check least fill
			// allocate the serviceInstance
			serviceInstance := &ygotnddtarget.NddTarget_TargetEntry_Allocation{
				ServiceName:     ygot.String(serviceInfo.ServiceName),
				ServiceIdentity: ygot.String("todo"),
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

			// When the
			if serviceInfo.Kind == pkgmetav1.KindWorker {
				targetServiceInfo := ctrlMetaCfg.GetTargetServiceInfo()

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
	return reconcile.Result{Requeue: true}, errors.Wrap(r.client.Update(ctx, t), "cannot update target")
}
