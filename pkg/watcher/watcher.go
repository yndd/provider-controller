package watcher

import (
	"context"

	pkgv1 "github.com/yndd/ndd-core/apis/pkg/v1"
	"github.com/yndd/ndd-runtime/pkg/logging"
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
	WithRegistrator(reg registrator.Registrator)
	// Watch
	Watch(ctx context.Context, cc *pkgv1.CompositeProvider)
}

// WithLogger adds a logger to the Watcher
func WithLogger(l logging.Logger) Option {
	return func(o Watcher) {
		o.WithLogger(l)
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
	eventCh     []chan event.GenericEvent
	registrator registrator.Registrator
	log         logging.Logger
}

func New(eventCh []chan event.GenericEvent, opts ...Option) Watcher {
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

func (w *watcher) WithRegistrator(reg registrator.Registrator) {
	w.registrator = reg
}

func (w *watcher) Watch(ctx context.Context, cc *pkgv1.CompositeProvider) {
	go w.watch(ctx, cc)
}

func (w *watcher) watch(ctx context.Context, cc *pkgv1.CompositeProvider) {
START:
	/*
		var r registrator.Registrator
		switch cc.Spec.ServiceDiscovery {
		case pkgmetav1.ServiceDiscoveryTypeConsul:
			r = registrator.NewConsulRegistrator(ctx, cc.Spec.ServiceDiscoveryNamespace, "kind-dc1",
				registrator.WithLogger(w.log),
				registrator.WithClient(w.client),
			)
		case pkgmetav1.ServiceDiscoveryTypeK8s:
		}
	*/

	ch := make(chan *registrator.ServiceResponse)
	for _, pkg := range cc.Spec.Packages {
		w.log.Debug("podInfo", "pod", pkg)
		for _, serviceInfo := range cc.GetServicesInfoByKind(pkg.Kind) {
			w.log.Debug("serviceInfo", "serviceInfo", serviceInfo)
			go w.registrator.WatchCh(ctx, serviceInfo.ServiceName, []string{}, registrator.WatchOptions{}, ch)
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
				for _, ch := range w.eventCh {
					ch <- event.GenericEvent{
						Object: &pkgv1.CompositeProvider{
							ObjectMeta: metav1.ObjectMeta{Name: cc.Name, Namespace: cc.Namespace},
						},
					}
				}
			}
		}
	}
}
