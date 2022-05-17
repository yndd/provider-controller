/*
Copyright 2022 NDD.

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

package controllerwatcher

import (
	"context"

	pkgmetav1 "github.com/yndd/ndd-core/apis/pkg/meta/v1"
	"github.com/yndd/ndd-runtime/pkg/logging"
	targetv1 "github.com/yndd/ndd-target-runtime/apis/dvr/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type adder interface {
	Add(item interface{})
}
type EnqueueRequestForAllControllerConfig struct {
	client        client.Client
	log           logging.Logger
	ctx           context.Context
	newTargetList func() targetv1.TgList
}

// Create enqueues a request for all infrastructures which pertains to the topology.
func (e *EnqueueRequestForAllControllerConfig) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	e.add(evt.Object, q)
}

// Create enqueues a request for all infrastructures which pertains to the topology.
func (e *EnqueueRequestForAllControllerConfig) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	e.add(evt.ObjectOld, q)
	e.add(evt.ObjectNew, q)
}

// Create enqueues a request for all infrastructures which pertains to the topology.
func (e *EnqueueRequestForAllControllerConfig) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	e.add(evt.Object, q)
}

// Create enqueues a request for all infrastructures which pertains to the topology.
func (e *EnqueueRequestForAllControllerConfig) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
	e.add(evt.Object, q)
}

func (e *EnqueueRequestForAllControllerConfig) add(obj runtime.Object, queue adder) {
	cr, ok := obj.(*pkgmetav1.ControllerConfig)
	if !ok {
		return
	}
	log := e.log.WithValues("event handler", "ControllerConfig", "name", cr.GetName())
	log.Debug("handleEvent")

	tl := e.newTargetList()
	if err := e.client.List(e.ctx, tl); err != nil {
		log.Debug("cannot list targets", "error", err)
		return
	}

	for _, t := range tl.GetTargets() {
		tspec, err := t.GetSpec()
		if err != nil {
			log.Debug("cannot get target spec", "error", err)
		}
		if cr.Spec.VendorType == tspec.VendorType.String() {
			queue.Add(reconcile.Request{NamespacedName: types.NamespacedName{
				Namespace: t.GetNamespace(),
				Name:      t.GetName()}})
		}
	}
}
