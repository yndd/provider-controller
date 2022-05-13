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

package deployer

import (
	"context"

	"github.com/pkg/errors"
	pkgmetav1 "github.com/yndd/ndd-core/apis/pkg/meta/v1"
	pkgv1 "github.com/yndd/ndd-core/apis/pkg/v1"
	"github.com/yndd/ndd-runtime/pkg/logging"
	"github.com/yndd/ndd-target-runtime/pkg/resource"
	"k8s.io/apimachinery/pkg/types"
)

type Deployer interface {
	// add a logger to Collector
	WithLogger(log logging.Logger)
	// add a k8s client
	WithClient(c resource.ClientApplicator)
	// add a revision
	WithRevision(name, namespace string)
	// deploy
	Deploy(ctx context.Context, ctrlMetaCfg *pkgmetav1.ControllerConfig) error
}

// Option can be used to manipulate Deployer config.
type Option func(Deployer)

// WithLogger specifies how the deployer logs messages.
func WithLogger(log logging.Logger) Option {
	return func(d Deployer) {
		d.WithLogger(log)
	}
}

// WithClient specifies the k8s client of the deployer.
func WithClient(c resource.ClientApplicator) Option {
	return func(d Deployer) {
		d.WithClient(c)
	}
}

// WithRevision specifies the revision
func WithRevision(name, namespace string) Option {
	return func(d Deployer) {
		d.WithRevision(name, namespace)
	}
}

func New(opts ...Option) Deployer {
	d := &deployer{}

	for _, opt := range opts {
		opt(d)
	}
	return d
}

type deployer struct {
	ctrlMetaCfg       *pkgmetav1.ControllerConfig
	crdNames          []string // TODO to be updated
	revision          string
	revisionNamespace string

	client resource.ClientApplicator
	log    logging.Logger
}

func (d *deployer) WithLogger(log logging.Logger) {
	d.log = log
}

func (d *deployer) WithClient(c resource.ClientApplicator) {
	d.client = c
}

// WithRevision specifies the revision of the controller
func (d *deployer) WithRevision(name, namespace string) {
	d.revision = name
	d.revisionNamespace = namespace
}

func (d *deployer) Deploy(ctx context.Context, ctrlMetaCfg *pkgmetav1.ControllerConfig) error {
	log := d.log.WithValues("controller config name", ctrlMetaCfg.Name)
	log.Debug("deploy...")

	// getCrds retrieves the crds from the k8s api based on the crNames
	// coming from the flags
	//d.crdNames = []string{"srlconfig"}
	d.crdNames = []string{"srlconfigs.srl.config.ndd.yndd.io"}
	crds, err := d.getCrds(ctx, d.crdNames)
	if err != nil {
		log.Debug(errGetCrd, "error", err)
		return errors.Wrap(err, errGetCrd)
	}

	newPackageRevision := func() pkgv1.PackageRevision { return &pkgv1.ProviderRevision{} }
	// this is used to automatically cleanup the created resources when deleting the provider revision
	// By assigning the owner reference to the newly created resources we take care of this
	pr := newPackageRevision()
	if err := d.client.Get(ctx, types.NamespacedName{
		Namespace: d.revisionNamespace,
		Name:      d.revision}, pr); err != nil {
		log.Debug(errGetCrd, "error", err)
		return err
	}

	for _, podSpec := range ctrlMetaCfg.Spec.Pods {
		for _, c := range podSpec.Containers {
			for _, cr := range renderClusterRoles(ctrlMetaCfg, podSpec, c, pr, crds) {
				cr := cr // Pin range variable so we can take its address.
				log.WithValues("role-name", cr.GetName())
				if err := d.client.Apply(ctx, &cr); err != nil {
					return errors.Wrap(err, errApplyClusterRoles)
				}
				log.Debug("Applied RBAC ClusterRole")
			}

			crb := renderClusterRoleBinding(ctrlMetaCfg, podSpec, c, pr)
			if err := d.client.Apply(ctx, crb); err != nil {
				return errors.Wrap(err, errApplyClusterRoleBinding)
			}
			log.Debug("Applied RBAC ClusterRole")

			for _, extra := range c.Extras {
				if extra.Certificate {
					// deploy a certificate
					cert := renderCertificate(ctrlMetaCfg, podSpec, c, extra, pr)
					if err := d.client.Apply(ctx, cert); err != nil {
						return errors.Wrap(err, errApplyCertificate)
					}
				}
				if extra.Service {
					// deploy a webhook service
					s := renderService(ctrlMetaCfg, podSpec, c, extra, pr)
					if err := d.client.Apply(ctx, s); err != nil {
						return errors.Wrap(err, errApplyService)
					}
				}
				if extra.Webhook {
					// deploy a mutating webhook
					whMutate := renderWebhookMutate(ctrlMetaCfg, podSpec, c, extra, pr, crds)
					if err := d.client.Apply(ctx, whMutate); err != nil {
						return errors.Wrap(err, errApplyMutatingWebhook)
					}
					// deploy a validating webhook
					whValidate := renderWebhookValidate(ctrlMetaCfg, podSpec, c, extra, pr, crds)
					if err := d.client.Apply(ctx, whValidate); err != nil {
						return errors.Wrap(err, errApplyValidatingWebhook)
					}
				}
			}
		}
		switch podSpec.Type {
		case pkgmetav1.DeploymentTypeDeployment:
		case pkgmetav1.DeploymentTypeStatefulset:
			s := renderStatefulSet(ctrlMetaCfg, podSpec, pr)
			if err := d.client.Apply(ctx, s); err != nil {
				return errors.Wrap(err, errApplyStatfullSet)
			}
			sa := renderServiceAccount(ctrlMetaCfg, podSpec, pr)
			if err := d.client.Apply(ctx, sa); err != nil {
				return errors.Wrap(err, errApplyServiceAccount)
			}
		}
	}
	return nil
}
