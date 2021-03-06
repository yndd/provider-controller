/*
Copyright 2022.

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

package controllers

import (
	"github.com/yndd/ndd-target-runtime/pkg/shared"
	"github.com/yndd/provider-controller/internal/controllers/alloccontroller"
	controllerc "github.com/yndd/provider-controller/internal/controllers/controller"
	"github.com/yndd/provider-controller/internal/controllers/lcmcontroller"
	ctrl "sigs.k8s.io/controller-runtime"
)

// Setup controllers.
func Setup(mgr ctrl.Manager, nddcopts *shared.NddControllerOptions) error {
	lcmCh, err := lcmcontroller.Setup(mgr, nddcopts)
	if err != nil {
		return err
	}
	allcCh, err := alloccontroller.Setup(mgr, nddcopts)
	if err != nil {
		return err
	}
	return controllerc.Setup(mgr, nddcopts, lcmCh, allcCh)
}
