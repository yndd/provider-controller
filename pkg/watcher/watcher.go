package watcher

import (
	"context"
	"strings"

	pkgmetav1 "github.com/yndd/ndd-core/apis/pkg/meta/v1"
	"github.com/yndd/registrator/registrator"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

type Watcher interface {
	Watch(ctx context.Context, ctrlMetaCfg *pkgmetav1.ControllerConfig)
}

type watcher struct {
	// k8s event channel
	eventCh  chan event.GenericEvent
	identity string
}

func New(identity string, eventCh chan event.GenericEvent) Watcher {
	return &watcher{
		eventCh: eventCh,
	}
}

func (w *watcher) Watch(ctx context.Context, ctrlMetaCfg *pkgmetav1.ControllerConfig) {
	go w.watch(ctx, ctrlMetaCfg)
}

func (w *watcher) watch(ctx context.Context, ctrlMetaCfg *pkgmetav1.ControllerConfig) {
START:
	var r registrator.Registrator
	switch ctrlMetaCfg.Spec.ServiceDiscovery {
	case pkgmetav1.ServiceDiscoveryTypeConsul:
		r = registrator.NewConsulRegistrator(ctx, ctrlMetaCfg.Spec.ServiceDiscoveryNamespace, "kinddc1")
	case pkgmetav1.ServiceDiscoveryTypeK8s:
	}

	ch := make(chan *registrator.ServiceResponse)
	for _, serviceInfo := range ctrlMetaCfg.GetAllServicesInfo() {
		go r.WatchCh(ctx, serviceInfo.ServiceName, []string{}, ch)
	}

	for {
		select {
		case <-ctx.Done():
			r.StopWatch("")
			return
		case serviceResp, ok := <-ch:
			if !ok {
				// someone closed the channel so we cannot continue
				r.StopWatch("")
				return
			}
			if serviceResp.Err != nil {
				// when an error is returned we stop and restart all watches again
				r.StopWatch("")
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

func (w *watcher) getServices(ctrlMetaCfg *pkgmetav1.ControllerConfig) []string {
	services := make([]string, 0, len(ctrlMetaCfg.Spec.Pods)+1)
	for _, pod := range ctrlMetaCfg.Spec.Pods {
		for _, watcher := range pod.Watchers {
			if w.identity != watcher {
				continue
			}
		}
		services = append(services, strings.Join([]string{ctrlMetaCfg.Name, pod.Name}, "-"))
		break
	}
	services = append(services, strings.Join([]string{ctrlMetaCfg.Name, "target"}, "-"))
	return services
}
