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
	"fmt"
	"strings"

	pkgmetav1 "github.com/yndd/ndd-core/apis/pkg/meta/v1"
	pkgv1 "github.com/yndd/ndd-core/apis/pkg/v1"
)

const (
	serviceSuffix       = "svc"
	certSuffix          = "serving-cert"
	certPathSuffix      = "serving-certs"
	containerStartupCmd = "/manager"

	statefulsetKey = "statefulset"

	userGroup = 2000
)

// getControllerPodKey returns a controller pod key
func getControllerPodKey(ctrlCfgName, podName string) string {
	return strings.Join([]string{ctrlCfgName, podName}, "-")
}

func getDnsName(ctrlCfgNamespace, serviceName string, x ...string) string {
	s := []string{serviceName, ctrlCfgNamespace, serviceSuffix}
	if len(x) > 0 {
		s = append(s, x...)
	}
	return strings.Join(s, ".")
}

func getRoleName(ctrlCfgName, podName, containerName string) string {
	return strings.Join([]string{ctrlCfgName, podName, containerName}, "-")
}

func getRevisionLabelString(ctrlCfgName string, podSpec pkgmetav1.PodSpec) string {
	return fmt.Sprintf("%s=%s", getLabelKey(statefulsetKey), getControllerPodKey(ctrlCfgName, podSpec.Name))

}

func getRevisionLabel(ctrlCfgName string, podSpec pkgmetav1.PodSpec) map[string]string {
	return map[string]string{getLabelKey(statefulsetKey): getControllerPodKey(ctrlCfgName, podSpec.Name)}
}

func getCertificateName(ctrlCfgName, podName, containerName, extraName string) string {
	return strings.Join([]string{ctrlCfgName, podName, containerName, extraName, certSuffix}, "-")
}

func getServiceName(ctrlCfgName, podName, containerName, extraName string) string {
	return strings.Join([]string{ctrlCfgName, podName, containerName, extraName, serviceSuffix}, "-")
}

func getMutatingWebhookName(crdSingular, crdGroup string) string {
	return strings.Join([]string{"m" + crdSingular, crdGroup}, ".")
}

func getValidatingWebhookName(crdSingular, crdGroup string) string {
	return strings.Join([]string{"v" + crdSingular, crdGroup}, ".")
}

func getFqTargetName(namespace, name string) string {
	return strings.Join([]string{namespace, name}, ".")
}

// SystemClusterProviderRoleName returns the name of the 'system' cluster role - i.e.
// the role that a provider's ServiceAccount should be bound to.
func systemClusterProviderRoleName(roleName string) string {
	return nameProviderPrefix + roleName + nameSuffixSystem
}

func getLabelKey(extraName string) string {
	return strings.Join([]string{pkgv1.PackageNamespace, extraName}, "/")

}
