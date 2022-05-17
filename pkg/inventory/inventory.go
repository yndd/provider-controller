package inventory

import (
	"context"
	"strings"

	pkgmetav1 "github.com/yndd/ndd-core/apis/pkg/meta/v1"
	targetv1 "github.com/yndd/ndd-target-runtime/apis/dvr/v1"
	"github.com/yndd/registrator/registrator"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Option func(Inventory)
type Inventory interface {
	// Build builds a map of service --> serviceInstance --> []targets based on
	// the assignments in the target spec
	Build(ctx context.Context, cc *pkgmetav1.ControllerConfig) (map[string]map[string][]string, error)
	// BuildFromRegistry builds the same map as Build based on the target
	// registrations in the registrator
	BuildFromRegistry(ctx context.Context, cc *pkgmetav1.ControllerConfig) (map[string]map[string][]string, error)
	// GetLeastLoaded returns the least loaded instance of service serviceName
	GetLeastLoaded(ctx context.Context, cc *pkgmetav1.ControllerConfig, serviceName string) string
}

type invImpl struct {
	kClient client.Client
	reg     registrator.Registrator
}

func New(k client.Client, reg registrator.Registrator) Inventory {
	return &invImpl{
		kClient: k,
		reg:     reg,
	}
}

func (i *invImpl) Build(ctx context.Context, cc *pkgmetav1.ControllerConfig) (map[string]map[string][]string, error) {
	targets, err := i.getTargets(ctx, "") // TODO: add namespace to controllerConfig spec and use it here
	if err != nil {
		return nil, err
	}
	return i.inventoryFromTargets(targets)
}

func (i *invImpl) BuildFromRegistry(ctx context.Context, cc *pkgmetav1.ControllerConfig) (map[string]map[string][]string, error) {
	return i.inventoryFromRegistry(ctx, cc)
}

func (i *invImpl) GetLeastLoaded(ctx context.Context, cc *pkgmetav1.ControllerConfig, serviceName string) string {
	for _, pod := range cc.Spec.Pods {
		if serviceName != pkgmetav1.GetServiceName(cc.Name, pod.Name) {
			continue
		}
		inv, err := i.Build(ctx, cc)
		if err != nil {
			return ""
		}
		return findLeastLoaded(inv, serviceName)
	}
	return ""
}

func (i *invImpl) getTargets(ctx context.Context, ns string) ([]targetv1.Target, error) {
	targets := &targetv1.TargetList{}
	err := i.kClient.List(ctx, targets, &client.ListOptions{
		Namespace: ns,
	})
	return targets.Items, err
}

func (i *invImpl) inventoryFromTargets(targets []targetv1.Target) (map[string]map[string][]string, error) {
	// allocation -> serviceID -> []targets
	inv := make(map[string]map[string][]string)
	numTargets := len(targets)
	for _, t := range targets {
		te, err := t.GetSpec()
		if err != nil {
			return nil, err
		}
		for allocName, alloc := range te.Allocation {
			// if one of ServiceIdentity or ServiceName is not set,
			// it means the allocation is not done yet.
			if alloc == nil || alloc.ServiceIdentity == nil || alloc.ServiceName == nil {
				continue
			}
			if inv[allocName] == nil {
				inv[*alloc.ServiceIdentity] = make(map[string][]string)
			}
			if inv[allocName][*alloc.ServiceIdentity] == nil {
				inv[allocName][*alloc.ServiceIdentity] = make([]string, 0, numTargets)
			}
			inv[allocName][*alloc.ServiceIdentity] = append(inv[allocName][*alloc.ServiceIdentity], t.Name)
		}
	}
	return inv, nil
}

func (i *invImpl) inventoryFromRegistry(ctx context.Context, cc *pkgmetav1.ControllerConfig) (map[string]map[string][]string, error) {
	inv := make(map[string]map[string][]string)
	serv := cc.GetTargetServiceInfo()
	services, err := i.reg.Query(ctx, serv.ServiceName, []string{})
	if err != nil {
		return nil, err
	}
	for _, srvEntry := range services {
		if inv[srvEntry.Name] == nil {
			inv[srvEntry.Name] = make(map[string][]string)
		}
		if inv[srvEntry.Name][srvEntry.ID] == nil {
			inv[srvEntry.Name][srvEntry.ID] = make([]string, 0)
		}

		for _, tag := range srvEntry.Tags {
			if strings.HasPrefix(tag, "target=") {
				inv[srvEntry.Name][srvEntry.ID] = append(
					inv[srvEntry.Name][srvEntry.ID],
					strings.TrimPrefix(tag, "target="))
			}
		}
	}

	return inv, nil
}

func findLeastLoaded(inv map[string]map[string][]string, name string) string {
	leastLoaded := ""
	if srv, ok := inv[name]; ok {
		var minTargets *int
		for instance, targets := range srv {
			if minTargets == nil {
				minTargets = pointer.Int(len(targets))
				leastLoaded = instance
				continue
			}
			numTargets := len(targets)
			if numTargets <= *minTargets {
				minTargets = pointer.Int(numTargets)
				leastLoaded = instance
			}
		}
	}
	return leastLoaded
}
