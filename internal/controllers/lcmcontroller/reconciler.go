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
	"github.com/yndd/ndd-runtime/pkg/event"
	"github.com/yndd/ndd-runtime/pkg/logging"
	"github.com/yndd/ndd-runtime/pkg/meta"
	targetv1 "github.com/yndd/ndd-target-runtime/apis/dvr/v1"
	"github.com/yndd/ndd-target-runtime/pkg/resource"
	"github.com/yndd/ndd-target-runtime/pkg/shared"
	"github.com/yndd/provider-controller/internal/deployer"
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
	errGetControllerConfig     = "cannot get controller config cr"
	errGetTargetList           = "cannot get target cr list"
	errGetPod                  = "cannot get pod cr"
	errGetPodList              = "cannot get pod cr list"
	errGetCtrlMetaCfg          = "cannot get controller meta config cr"
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
)

// ReconcilerOption is used to configure the Reconciler.
type ReconcilerOption func(*Reconciler)

// Reconciler reconciles packages.
type Reconciler struct {
	client    resource.ClientApplicator
	finalizer resource.Finalizer
	// servicediscovery registrator
	registrator registrator.Registrator
	watcher     watcher.Watcher
	deployer    deployer.Deployer

	m             sync.Mutex
	watchers      map[string]context.CancelFunc
	newTargetList func() targetv1.TgList

	crdNames          []string
	revision          string
	revisionNamespace string
	pollInterval      time.Duration

	//newProviderRevision func() pkgv1.PackageRevision
	log    logging.Logger
	record event.Recorder
}

// WithCrdNames specifies the crdNames in the reconciler
func WithCrdNames(n []string) ReconcilerOption {
	return func(r *Reconciler) {
		r.crdNames = n
	}
}

// WithRevision specifies the revision
func WithRevision(name, namespace string) ReconcilerOption {
	return func(r *Reconciler) {
		r.revision = name
		r.revisionNamespace = namespace
	}
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
func Setup(mgr ctrl.Manager, nddopts *shared.NddControllerOptions) error {
	name := "config-controller/" + strings.ToLower(pkgmetav1.ControllerConfigGroupKind)

	events := make(chan cevent.GenericEvent)

	r := NewReconciler(mgr,
		WithRegistrator(nddopts.Registrator),
		WithWatcher(watcher.New(events,
			watcher.WithRegistrator(nddopts.Registrator),
			//watcher.WithClient(resource.ClientApplicator{
			//	Client:     mgr.GetClient(),
			//	Applicator: resource.NewAPIPatchingApplicator(mgr.GetClient()),
			//}),
			watcher.WithLogger(nddopts.Logger))),
		WithRevision(nddopts.Revision, nddopts.RevisionNamespace),
		WithCrdNames(nddopts.CrdNames),
		WithLogger(nddopts.Logger),
		WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
	)

	ControllerConfigHandler := &EnqueueRequestForAllControllerConfig{
		client: mgr.GetClient(),
		log:    nddopts.Logger,
		ctx:    context.Background(),
	}

	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(nddopts.Copts).
		Named(name).
		For(&pkgmetav1.ControllerConfig{}).
		Owns(&pkgmetav1.ControllerConfig{}).
		WithEventFilter(resource.IgnoreUpdateWithoutGenerationChangePredicate()).
		Watches(&source.Channel{Source: events}, ControllerConfigHandler).
		Complete(r)
}

// NewReconciler creates a new package reconciler.
func NewReconciler(m ctrl.Manager, opts ...ReconcilerOption) *Reconciler {
	//tg := func() targetv1.Tg { return &targetv1.Target{} }
	//pr := func() pkgv1.PackageRevision { return &pkgv1.ProviderRevision{} }
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

	r.deployer = deployer.New(
		deployer.WithClient(
			resource.ClientApplicator{
				Client:     m.GetClient(),
				Applicator: resource.NewAPIPatchingApplicator(m.GetClient()),
			},
		),
		deployer.WithLogger(r.log),
		deployer.WithRevision(r.revision, r.revisionNamespace),
	)

	return r
}

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) { // nolint:gocyclo
	log := r.log.WithValues("NameSpaceName", req.NamespacedName)
	log.Debug("lcm reconciler start...")

	// get the controller config info
	ctrlMetaCfg := &pkgmetav1.ControllerConfig{}
	if err := r.client.Get(ctx, req.NamespacedName, ctrlMetaCfg); err != nil {
		// There's no need to requeue if we no longer exist. Otherwise we'll be
		// requeued implicitly because we return an error.
		log.Debug(errGetControllerConfig, "error", err)
		return reconcile.Result{}, errors.Wrap(resource.IgnoreNotFound(err), errGetControllerConfig)
	}

	//record := r.record.WithAnnotations("external-name", meta.GetExternalName(ctrlMetaCfg))

	if meta.WasDeleted(ctrlMetaCfg) {
		// Delete the watcher
		r.deleteWatcher(req.NamespacedName.String())
		// Delete finalizer after the object is deleted
		if err := r.finalizer.RemoveFinalizer(ctx, ctrlMetaCfg); err != nil {
			log.Debug("Cannot remove target cr finalizer", "error", err)
			return reconcile.Result{Requeue: true}, errors.Wrap(err, "cannot remove finalizer")
		}
		return reconcile.Result{Requeue: false}, errors.Wrap(r.client.Update(ctx, ctrlMetaCfg), "cannot remove finalizer")
	}

	// Add a finalizer
	if err := r.finalizer.AddFinalizer(ctx, ctrlMetaCfg); err != nil {
		log.Debug("cannot add finalizer", "error", err)
		return reconcile.Result{Requeue: true}, errors.Wrap(r.client.Update(ctx, ctrlMetaCfg), "cannot add finalizer")
	}

	log.Debug("ctrlMetaCfg", "ctrlMetaCfg spec", ctrlMetaCfg.Spec)
	r.addWatcher(req.NamespacedName.String(), ctrlMetaCfg)

	// we always deploy since this allows us to handle updates of the deploySpec
	// TODO crd
	if err := r.deployer.Deploy(ctx, ctrlMetaCfg); err != nil {
		log.Debug("cannot deploy", "error", err)
		return reconcile.Result{}, errors.Wrap(err, "cannot deploy")
	}

	/*
		for _, serviceInfo := range ctrlMetaCfg.GetServicesInfo() {
			// check inventory
			// scale out based on allocation -> set the replicaset number
		}
	*/

	// based on logic update replicas in the spec
	log.Debug("target allocation and validation successfull")
	// a scale out/in action is triggered by periodic reconciliation (building inventory and deciding on the replicas)
	return reconcile.Result{RequeueAfter: r.pollInterval}, nil
}

func (r *Reconciler) addWatcher(nsName string, ctrlMetaCfg *pkgmetav1.ControllerConfig) {
	r.m.Lock()
	defer r.m.Unlock()
	if _, ok := r.watchers[nsName]; !ok {
		ctx, cfn := context.WithCancel(context.Background())
		r.watchers[nsName] = cfn
		r.watcher.Watch(ctx, ctrlMetaCfg)
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
