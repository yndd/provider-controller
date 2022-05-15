package inventory

import (
	"context"
	"strings"

	"github.com/hashicorp/consul/api"
	pkgmetav1 "github.com/yndd/ndd-core/apis/pkg/meta/v1"
	targetv1 "github.com/yndd/ndd-target-runtime/apis/dvr/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Inventory interface {
	Build(ctx context.Context, cc *pkgmetav1.ControllerConfig, consul bool) (map[string]map[string][]string, error)
}

type InvImpl struct {
	kClient client.Client
	cClient api.Client
}

func New() Inventory {
	return &InvImpl{}
}

func (i *InvImpl) Build(ctx context.Context, cc *pkgmetav1.ControllerConfig, consul bool) (map[string]map[string][]string, error) {
	if consul {
		return i.inventoryFromConsul(ctx, cc)
	}
	targets, err := i.getTargets(ctx, "") // TODO: add namespace to controllerConfig spec and use it here
	if err != nil {
		return nil, err
	}
	return i.inventoryFromTargets(targets)
}

func (i *InvImpl) getTargets(ctx context.Context, ns string) ([]targetv1.Target, error) {
	targets := &targetv1.TargetList{}
	err := i.kClient.List(ctx, targets, &client.ListOptions{
		Namespace: ns,
	})
	return targets.Items, err
}

func (i *InvImpl) inventoryFromTargets(targets []targetv1.Target) (map[string]map[string][]string, error) {
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

func (i *InvImpl) inventoryFromConsul(ctx context.Context, cc *pkgmetav1.ControllerConfig) (map[string]map[string][]string, error) {
	inv := make(map[string]map[string][]string)
	for _, serv := range cc.GetServicesInfoByKind(pkgmetav1.KindNone) { // kind none is targets
		consulServiceEntries, _, err := i.cClient.Health().ServiceMultipleTags(serv.ServiceName, []string{}, true, &api.QueryOptions{})
		if err != nil {
			return nil, err
		}
		for _, srvEntry := range consulServiceEntries {
			if inv[srvEntry.Service.Service] == nil {
				inv[srvEntry.Service.Service] = make(map[string][]string)
			}
			if inv[srvEntry.Service.Service][srvEntry.Service.ID] == nil {
				inv[srvEntry.Service.Service][srvEntry.Service.ID] = make([]string, 0)
			}

			for _, tag := range srvEntry.Service.Tags {
				if strings.HasPrefix(tag, "target=") {
					inv[srvEntry.Service.Service][srvEntry.Service.ID] = append(
						inv[srvEntry.Service.Service][srvEntry.Service.ID],
						strings.TrimPrefix(tag, "target="))
				}
			}
		}
	}

	return inv, nil
}
