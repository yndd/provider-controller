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

package lcm_controller

import (
	"context"
	"strings"
	"time"

	"github.com/yndd/ndd-runtime/pkg/event"
	"github.com/yndd/ndd-runtime/pkg/logging"
	targetv1 "github.com/yndd/ndd-target-runtime/apis/dvr/v1"
	"github.com/yndd/ndd-target-runtime/pkg/resource"
	"github.com/yndd/ndd-target-runtime/pkg/shared"
	"github.com/yndd/registrator/registrator"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	// finalizer
	finalizerName = "finalizer.controller.srl.config.ndd.yndd.io"
	// timers
	defaultpollInterval = 1 * time.Minute
	shortWait           = 1 * time.Minute
	// errors
	errGetTarget               = "cannot get target cr"
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

	crdNames []string

	newTarget func() targetv1.Tg
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

	r := NewReconciler(mgr,
		WithRegistrator(nddopts.Registrator),
		WithCrdNames(nddopts.CrdNames),
		WithLogger(nddopts.Logger),
		WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
	)

	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(nddopts.Copts).
		Named(name).
		For(&targetv1.Target{}).
		Complete(r)
}

// NewReconciler creates a new package reconciler.
func NewReconciler(m ctrl.Manager, opts ...ReconcilerOption) *Reconciler {
	tg := func() targetv1.Tg { return &targetv1.Target{} }
	//pr := func() pkgv1.PackageRevision { return &pkgv1.ProviderRevision{} }

	r := &Reconciler{
		client: resource.ClientApplicator{
			Client:     m.GetClient(),
			Applicator: resource.NewAPIPatchingApplicator(m.GetClient()),
		},
		//pollInterval: defaultpollInterval,
		log:       logging.NewNopLogger(),
		record:    event.NewNopRecorder(),
		newTarget: tg,
		//newProviderRevision: pr,
		finalizer: resource.NewAPIFinalizer(m.GetClient(), finalizerName),
	}

	for _, f := range opts {
		f(r)
	}

	return r
}

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) { // nolint:gocyclo
	log := r.log.WithValues("NameSpaceName", req.NamespacedName)
	log.Debug("allocation reconciler start...")

	// build inventory interface
	// -> per worker and reconciler we identify the targets for each and will be able to select

	
	// if the allocation in the spec is not done
	// get least loaded instance per serviceName
	// 


	return reconcile.Result{}, nil
}
