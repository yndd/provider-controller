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
	pkgmetav1 "github.com/yndd/ndd-core/apis/pkg/meta/v1"
	pkgv1 "github.com/yndd/ndd-core/apis/pkg/v1"
	"k8s.io/apimachinery/pkg/types"
)

// deploy deploys the k8s resources that are managed by this controller
// clusterrole, clusterrolebindings
// statefulset/deployments, serviceaccounts
// certificates, services, webhooks
func (r *Reconciler) deploy(ctx context.Context, ctrlMetaCfg *pkgmetav1.ControllerConfig, crdNames []string) error {
	log := r.log.WithValues("controller config name", ctrlMetaCfg.Name)
	log.Debug("deploy...")

	// getCrds retrieves the crds from the k8s api based on the crNames
	// coming from the flags
	crds, err := r.getCrds(ctx, r.crdNames)
	if err != nil {
		log.Debug("cannot get crds", "error", err)
		return errors.Wrap(err, "cannot get crds")
	}

	newPackageRevision := func() pkgv1.PackageRevision { return &pkgv1.ProviderRevision{} }
	// this is used to automatically cleanup the created resources when deleting the provider revision
	// By assigning the owner reference to the newly created resources we take care of this
	pr := newPackageRevision()
	if err := r.client.Get(ctx, types.NamespacedName{
		Namespace: r.revisionNamespace,
		Name:      r.revision}, pr); err != nil {
		return err
	}

	for _, podSpec := range ctrlMetaCfg.Spec.Pods {
		for _, c := range podSpec.Containers {
			for _, cr := range r.renderClusterRoles(ctrlMetaCfg, podSpec, c, pr, crds) {
				cr := cr // Pin range variable so we can take its address.
				log.WithValues("role-name", cr.GetName())
				if err := r.client.Apply(ctx, &cr); err != nil {
					return errors.Wrap(err, errApplyClusterRoles)
				}
				log.Debug("Applied RBAC ClusterRole")
			}

			crb := r.renderClusterRoleBinding(ctrlMetaCfg, podSpec, c, pr)
			if err := r.client.Apply(ctx, crb); err != nil {
				return errors.Wrap(err, errApplyClusterRoleBinding)
			}
			log.Debug("Applied RBAC ClusterRole")

			for _, extra := range c.Extras {
				if extra.Certificate {
					// deploy a certificate
					cert := r.renderCertificate(ctrlMetaCfg, podSpec, c, extra, pr)
					if err := r.client.Apply(ctx, cert); err != nil {
						return errors.Wrap(err, errApplyCertificate)
					}
				}
				if extra.Service {
					// deploy a webhook service
					s := r.renderService(ctrlMetaCfg, podSpec, c, extra, pr)
					if err := r.client.Apply(ctx, s); err != nil {
						return errors.Wrap(err, errApplyService)
					}
				}
				if extra.Webhook {
					// deploy a mutating webhook
					whMutate := r.renderWebhookMutate(ctrlMetaCfg, podSpec, c, extra, pr, crds)
					if err := r.client.Apply(ctx, whMutate); err != nil {
						return errors.Wrap(err, errApplyMutatingWebhook)
					}
					// deploy a validating webhook
					whValidate := r.renderWebhookValidate(ctrlMetaCfg, podSpec, c, extra, pr, crds)
					if err := r.client.Apply(ctx, whValidate); err != nil {
						return errors.Wrap(err, errApplyValidatingWebhook)
					}
				}
			}
		}
		switch podSpec.Type {
		case pkgmetav1.DeploymentTypeDeployment:
		case pkgmetav1.DeploymentTypeStatefulset:
			s := r.renderStatefulSet(ctrlMetaCfg, podSpec, pr)
			if err := r.client.Apply(ctx, s); err != nil {
				return errors.Wrap(err, errApplyStatfullSet)
			}
			sa := r.renderServiceAccount(ctrlMetaCfg, podSpec, pr)
			if err := r.client.Apply(ctx, sa); err != nil {
				return errors.Wrap(err, errApplyServiceAccount)
			}
		}
	}
	return nil
}
