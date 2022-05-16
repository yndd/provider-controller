package watcher

import (
	"context"

	pkgmetav1 "github.com/yndd/ndd-core/apis/pkg/meta/v1"
	"github.com/yndd/ndd-runtime/pkg/logging"
	"github.com/yndd/ndd-target-runtime/pkg/resource"
	"github.com/yndd/registrator/registrator"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

// Option can be used to manipulate Watcher config.
type Option func(Watcher)

type Watcher interface {
	// add a logger to the Registrator
	WithLogger(log logging.Logger)
	// add a k8s client to the Registrator
	WithClient(c resource.ClientApplicator)
	// add a k8s client to the Registrator
	WithRegistrator(reg registrator.Registrator)
	// Watch
	Watch(ctx context.Context, ctrlMetaCfg *pkgmetav1.ControllerConfig)
}

// WithLogger adds a logger to the Watcher
func WithLogger(l logging.Logger) Option {
	return func(o Watcher) {
		o.WithLogger(l)
	}
}

// WithClient adds a k8s client to the Watcher.
func WithClient(c resource.ClientApplicator) Option {
	return func(o Watcher) {
		o.WithClient(c)
	}
}

// WithRegistrator specifies how the Reconciler registers and discover services
func WithRegistrator(reg registrator.Registrator) Option {
	return func(o Watcher) {
		o.WithRegistrator(reg)
	}
}

type watcher struct {
	// k8s event channel
	eventCh     chan event.GenericEvent
	registrator registrator.Registrator
	client      resource.ClientApplicator
	log         logging.Logger
}

func New(eventCh chan event.GenericEvent, opts ...Option) Watcher {
	w := &watcher{
		eventCh: eventCh,
	}

	for _, opt := range opts {
		opt(w)
	}

	return w
}

func (w *watcher) WithLogger(l logging.Logger) {
	w.log = l
}

func (w *watcher) WithClient(rc resource.ClientApplicator) {
	w.client = rc
}

func (w *watcher) WithRegistrator(reg registrator.Registrator) {
	w.registrator = reg
}

func (w *watcher) Watch(ctx context.Context, ctrlMetaCfg *pkgmetav1.ControllerConfig) {
	go w.watch(ctx, ctrlMetaCfg)
}

func (w *watcher) watch(ctx context.Context, ctrlMetaCfg *pkgmetav1.ControllerConfig) {
START:
	/*
		var r registrator.Registrator
		switch ctrlMetaCfg.Spec.ServiceDiscovery {
		case pkgmetav1.ServiceDiscoveryTypeConsul:
			r = registrator.NewConsulRegistrator(ctx, ctrlMetaCfg.Spec.ServiceDiscoveryNamespace, "kind-dc1",
				registrator.WithLogger(w.log),
				registrator.WithClient(w.client),
			)
		case pkgmetav1.ServiceDiscoveryTypeK8s:
		}
	*/

	ch := make(chan *registrator.ServiceResponse)
	for _, pod := range ctrlMetaCfg.Spec.Pods {
<<<<<<< HEAD
		for _, serviceInfo := range ctrlMetaCfg.GetServicesInfoByKind(pod.Kind) {
			go r.WatchCh(ctx, serviceInfo.ServiceName, []string{}, ch)
=======
		w.log.Debug("podInfo", "pod", pod)
		for _, serviceInfo := range ctrlMetaCfg.GetServicesInfoByKind(pod.Kind) {
			w.log.Debug("serviceInfo", "serviceInfo", serviceInfo)
			go w.registrator.WatchCh(ctx, serviceInfo.ServiceName, []string{}, ch)
>>>>>>> 9a18e89119ebe70fdc77116a851fa26251fe04d8
		}
	}

	for {
		select {
		case <-ctx.Done():
			w.registrator.StopWatch("")
			return
		case serviceResp, ok := <-ch:
			if !ok {
				// someone closed the channel so we cannot continue
				w.registrator.StopWatch("")
				return
			}
			if serviceResp.Err != nil {
				// when an error is returned we stop and restart all watches again
				w.registrator.StopWatch("")
				goto START
			}
			if serviceResp != nil {
				w.eventCh <- event.GenericEvent{
					Object: &pkgmetav1.ControllerConfig{
						ObjectMeta: metav1.ObjectMeta{Name: ctrlMetaCfg.Name, Namespace: ctrlMetaCfg.Namespace},
					},
				}
			}
		}
	}
}
