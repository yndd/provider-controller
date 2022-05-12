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

	"github.com/pkg/errors"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/types"
)

// getCrds retrieves the crds from the k8s api based on the crNames
// coming from the flags
func (r *Reconciler) getCrds(ctx context.Context, crdNames []string) ([]extv1.CustomResourceDefinition, error) {
	log := r.log.WithValues("crdNames", crdNames)
	log.Debug("getCrds...")

	crds := []extv1.CustomResourceDefinition{}
	for _, crdName := range crdNames {
		crd := &extv1.CustomResourceDefinition{}
		// namespace is not relevant for the crd, hence it is not supplied
		if err := r.client.Get(ctx, types.NamespacedName{Name: crdName}, crd); err != nil {
			return nil, errors.Wrap(err, errGetCrd)
		}
		crds = append(crds, *crd)
	}

	return crds, nil
}
