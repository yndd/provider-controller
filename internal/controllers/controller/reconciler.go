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

package controller

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	pkgmetav1 "github.com/yndd/ndd-core/apis/pkg/meta/v1"
	"github.com/yndd/provider-controller/pkg/watcher"
	"github.com/yndd/registrator/registrator"

	nddevent "github.com/yndd/ndd-runtime/pkg/event"
	"github.com/yndd/ndd-runtime/pkg/logging"
	"github.com/yndd/ndd-runtime/pkg/meta"
	"github.com/yndd/ndd-target-runtime/pkg/resource"
	"github.com/yndd/ndd-target-runtime/pkg/shared"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
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
)

// ReconcilerOption is used to configure the Reconciler.
type ReconcilerOption func(*Reconciler)

// Reconciler reconciles packages.
type Reconciler struct {
	client      resource.ClientApplicator
	finalizer   resource.Finalizer
	m           *sync.Mutex
	watchers    map[string]context.CancelFunc
	registrator registrator.Registrator

	allocGenericEventCh chan event.GenericEvent
	lcmGenericEventCh   chan event.GenericEvent

	pollInterval time.Duration

	log    logging.Logger
	record nddevent.Recorder
}

// WithLogger specifies how the Reconciler logs messages.
func WithLogger(l logging.Logger) ReconcilerOption {
	return func(r *Reconciler) {
		r.log = l
	}
}

// WithRecorder specifies how the Reconciler records events.
func WithRecorder(er nddevent.Recorder) ReconcilerOption {
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

func WithAllocGenericEventChs(geCh chan event.GenericEvent) ReconcilerOption {
	return func(r *Reconciler) {
		r.allocGenericEventCh = geCh
	}
}

func WithLCMGenericEventChs(geCh chan event.GenericEvent) ReconcilerOption {
	return func(r *Reconciler) {
		r.lcmGenericEventCh = geCh
	}
}

// SetupProvider adds a controller that reconciles Providers.
func Setup(mgr ctrl.Manager, nddopts *shared.NddControllerOptions, lcmCh, allocCh chan event.GenericEvent) error {
	name := "config-controller/" + strings.ToLower(pkgmetav1.ControllerConfigGroupKind)

	r := NewReconciler(mgr,
		WithLogger(nddopts.Logger),
		WithRecorder(nddevent.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
		WithRegistrator(nddopts.Registrator),
		WithLCMGenericEventChs(lcmCh),
		WithAllocGenericEventChs(allocCh),
	)

	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(nddopts.Copts).
		Named(name).
		For(&pkgmetav1.ControllerConfig{}).
		Owns(&pkgmetav1.ControllerConfig{}).
		Complete(r)
}

// NewReconciler creates a new package reconciler.
func NewReconciler(m ctrl.Manager, opts ...ReconcilerOption) *Reconciler {
	r := &Reconciler{
		client: resource.ClientApplicator{
			Client:     m.GetClient(),
			Applicator: resource.NewAPIPatchingApplicator(m.GetClient()),
		},
		m:            new(sync.Mutex),
		watchers:     make(map[string]context.CancelFunc),
		pollInterval: defaultpollInterval,
		log:          logging.NewNopLogger(),
		record:       nddevent.NewNopRecorder(),
		finalizer:    resource.NewAPIFinalizer(m.GetClient(), finalizerName),
	}

	for _, f := range opts {
		f(r)
	}

	return r
}

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) { // nolint:gocyclo
	log := r.log.WithValues("NameSpaceName", req.NamespacedName)
	log.Debug("controller reconciler start...")

	// get the controller config info
	cc := &pkgmetav1.ControllerConfig{}
	if err := r.client.Get(ctx, req.NamespacedName, cc); err != nil {
		// There's no need to requeue if we no longer exist. Otherwise we'll be
		// requeued implicitly because we return an error.
		log.Debug(errGetControllerConfig, "error", err)
		return reconcile.Result{}, errors.Wrap(resource.IgnoreNotFound(err), errGetControllerConfig)
	}

	// record := r.record.WithAnnotations("external-name", meta.GetExternalName(cc))

	if meta.WasDeleted(cc) {
		// Stop watcher
		r.deleteWatcher(req.NamespacedName.String())
		// Delete finalizer after the object is deleted
		if err := r.finalizer.RemoveFinalizer(ctx, cc); err != nil {
			log.Debug("Cannot remove controller config finalizer", "error", err)
			return reconcile.Result{Requeue: true}, errors.Wrap(err, "cannot remove finalizer")
		}
		return reconcile.Result{Requeue: false}, errors.Wrap(r.client.Update(ctx, cc), "cannot remove finalizer")
	}

	// Add a finalizer
	if err := r.finalizer.AddFinalizer(ctx, cc); err != nil {
		log.Debug("cannot add finalizer", "error", err)
		return reconcile.Result{Requeue: true}, errors.Wrap(r.client.Update(ctx, cc), "cannot add finalizer")
	}

	// TODO: add diff logic to decide when to delete/create the watcher
	// required only when when pod names/kind change in controllerConfig

	// stop watchers
	r.deleteWatcher(req.NamespacedName.String())
	// start watchers
	r.addWatcher(req.NamespacedName.String(), cc)
	// trigger generic event to LCM controller
	r.lcmGenericEventCh <- event.GenericEvent{Object: cc}
	log.Debug("ctrlMetaCfg", "ctrlMetaCfg spec", cc.Spec)

	return reconcile.Result{RequeueAfter: r.pollInterval}, r.client.Update(ctx, cc)
}

func (r *Reconciler) addWatcher(nsName string, cc *pkgmetav1.ControllerConfig) {
	r.m.Lock()
	defer r.m.Unlock()
	if _, ok := r.watchers[nsName]; !ok {
		w := watcher.New(
			[]chan event.GenericEvent{r.lcmGenericEventCh, r.allocGenericEventCh},
			watcher.WithLogger(r.log),
			watcher.WithRegistrator(r.registrator))

		ctx, cfn := context.WithCancel(context.Background())
		r.watchers[nsName] = cfn
		w.Watch(ctx, cc)
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
