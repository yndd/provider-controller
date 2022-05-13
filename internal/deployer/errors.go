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

const (
	// errors
	errGetControllerConfig     = "cannot get controller config cr"
	errGetTargetList           = "cannot get target cr list"
	errGetPod                  = "cannot get pod cr"
	errGetPodList              = "cannot get pod cr list"
	errGetCtrlMetaCfg          = "cannot get controller meta config cr"
	errGetCrd                  = "cannot get crd"
	errUpdateStatus            = "cannot update status"
	errApplyStatfullSet        = "cannot apply statefulset"
	errApplyCertificate        = "cannot apply certificate"
	errApplyService            = "cannot apply service"
	errApplyMutatingWebhook    = "cannot apply mutating webhook"
	errApplyValidatingWebhook  = "cannot apply validating webhook"
	errApplyClusterRoles       = "cannot apply clusterrole"
	errApplyClusterRoleBinding = "cannot apply clusterrolebinding"
	errApplyServiceAccount     = "cannot apply service account"
)