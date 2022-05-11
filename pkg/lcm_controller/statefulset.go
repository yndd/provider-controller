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
	"path/filepath"
	"strings"

	pkgmetav1 "github.com/yndd/ndd-core/apis/pkg/meta/v1"
	pkgv1 "github.com/yndd/ndd-core/apis/pkg/v1"
	"github.com/yndd/ndd-runtime/pkg/meta"
	"github.com/yndd/ndd-runtime/pkg/utils"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (r *Reconciler) renderStatefulSet(ctrlMetaCfg *pkgmetav1.ControllerConfig, podSpec pkgmetav1.PodSpec, revision pkgv1.PackageRevision) *appsv1.StatefulSet {
	s := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            getControllerPodKey(ctrlMetaCfg.Name, podSpec.Name),
			Namespace:       ctrlMetaCfg.Namespace,
			OwnerReferences: []metav1.OwnerReference{meta.AsController(meta.TypedReferenceTo(revision, pkgv1.ProviderRevisionGroupVersionKind))},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: utils.Int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: getLabels(ctrlMetaCfg, podSpec),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:      getControllerPodKey(ctrlMetaCfg.Name, podSpec.Name),
					Namespace: ctrlMetaCfg.Namespace,
					Labels:    getLabels(ctrlMetaCfg, podSpec),
				},
				Spec: corev1.PodSpec{
					SecurityContext:    getPodSecurityContext(),
					ServiceAccountName: r.renderServiceAccount(ctrlMetaCfg, podSpec, revision).GetName(),
					ImagePullSecrets:   revision.GetPackagePullSecrets(),
					Containers:         getContainers(ctrlMetaCfg, podSpec, revision.GetPackagePullPolicy()),
					Volumes:            getVolumes(ctrlMetaCfg, podSpec),
				},
			},
		},
	}

	return s
}

func getLabels(ctrlMetaCfg *pkgmetav1.ControllerConfig, podSpec pkgmetav1.PodSpec) map[string]string {
	labels := getRevisionLabel(ctrlMetaCfg.Name, podSpec)
	for _, container := range podSpec.Containers {
		for _, extra := range container.Extras {
			labels[getLabelKey(extra.Name)] = getServiceName(ctrlMetaCfg.Name, podSpec.Name, container.Container.Name, extra.Name)
		}
	}
	return labels
}

func getPodSecurityContext() *corev1.PodSecurityContext {
	return &corev1.PodSecurityContext{
		RunAsUser:    utils.Int64Ptr(userGroup),
		RunAsGroup:   utils.Int64Ptr(userGroup),
		RunAsNonRoot: utils.BoolPtr(true),
	}
}

func getSecurityContext() *corev1.SecurityContext {
	return &corev1.SecurityContext{
		RunAsUser:                utils.Int64Ptr(userGroup),
		RunAsGroup:               utils.Int64Ptr(userGroup),
		AllowPrivilegeEscalation: utils.BoolPtr(false),
		Privileged:               utils.BoolPtr(false),
		RunAsNonRoot:             utils.BoolPtr(true),
	}
}

func getEnv() []corev1.EnvVar {
	// environment parameters used in the deployment/statefulset
	envNameSpace := corev1.EnvVar{
		Name: "POD_NAMESPACE",
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{
				APIVersion: "v1",
				FieldPath:  "metadata.namespace",
			},
		},
	}
	envPodIP := corev1.EnvVar{
		Name: "POD_IP",
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{
				APIVersion: "v1",
				FieldPath:  "status.podIP",
			},
		},
	}
	envPodName := corev1.EnvVar{
		Name: "POD_NAME",
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{
				APIVersion: "v1",
				FieldPath:  "metadata.name",
			},
		},
	}
	envNodeName := corev1.EnvVar{
		Name: "NODE_NAME",
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{
				APIVersion: "v1",
				FieldPath:  "spec.nodeName",
			},
		},
	}
	envNodeIP := corev1.EnvVar{
		Name: "NODE_IP",
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{
				APIVersion: "v1",
				FieldPath:  "status.hostIP",
			},
		},
	}

	return []corev1.EnvVar{
		envNameSpace,
		envPodIP,
		envPodName,
		envNodeName,
		envNodeIP,
	}
}

func getContainers(ctrlMetaCfg *pkgmetav1.ControllerConfig, podSpec pkgmetav1.PodSpec, pullPolicy *corev1.PullPolicy) []corev1.Container {
	containers := []corev1.Container{}

	for _, container := range podSpec.Containers {
		if container.Container.Name == "kube-rbac-proxy" {
			containers = append(containers, getKubeProxyContainer())
		} else {
			containers = append(containers, getContainer(ctrlMetaCfg, container, pullPolicy))
		}
	}

	return containers
}

func getKubeProxyContainer() corev1.Container {
	return corev1.Container{
		Name:  "kube-rbac-proxy",
		Image: "gcr.io/kubebuilder/kube-rbac-proxy:v0.8.0",
		Args:  getProxyArgs(),
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: 8443,
				Name:          "https",
			},
		},
	}
}

func getProxyArgs() []string {
	return []string{
		"--secure-listen-address=0.0.0.0:8443",
		"--upstream=http://127.0.0.1:8080/",
		"--logtostderr=true",
		"--v=10",
	}
}

func getArgs(ctrlMetaCfg *pkgmetav1.ControllerConfig) []string {
	cnArg := strings.Join([]string{"--controller-name", ctrlMetaCfg.Name}, "=")
	dkArg := strings.Join([]string{"--deployment-kind", "distributed"}, "=")
	cnsArg := strings.Join([]string{"--consul-namespace", ctrlMetaCfg.Spec.ServiceDiscoveryNamespace}, "=")
	return []string{
		"start",
		cnArg,
		dkArg,
		cnsArg,
		"--debug",
	}
}

func getVolumeMounts(c pkgmetav1.ContainerSpec) []corev1.VolumeMount {
	volumes := []corev1.VolumeMount{}
	for _, extra := range c.Extras {
		if extra.Certificate {
			volumes = append(volumes, corev1.VolumeMount{
				Name:      extra.Name,
				MountPath: filepath.Join("tmp", strings.Join([]string{"k8s", extra.Name, "server"}, "-"), certPathSuffix),
				ReadOnly:  true,
			})
		} else {
			if extra.Volume {
				volumes = append(volumes, corev1.VolumeMount{
					Name:      extra.Name,
					MountPath: filepath.Join(extra.Name),
				})
			}
		}
	}
	return volumes
}

func getVolumes(ctrlMetaCfg *pkgmetav1.ControllerConfig, podSpec pkgmetav1.PodSpec) []corev1.Volume {
	volume := []corev1.Volume{}
	for _, c := range podSpec.Containers {
		for _, extra := range c.Extras {
			if extra.Certificate {
				volume = append(volume, corev1.Volume{
					Name: extra.Name,
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName:  getCertificateName(ctrlMetaCfg.Name, podSpec.Name, c.Container.Name, extra.Name),
							DefaultMode: utils.Int32Ptr(420),
						},
					},
				})
			} else {
				volume = append(volume, corev1.Volume{
					Name: extra.Name,
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				})
			}
		}
	}
	return volume
}

func getContainer(ctrlMetaCfg *pkgmetav1.ControllerConfig, c pkgmetav1.ContainerSpec, pullPolicy *corev1.PullPolicy) corev1.Container {
	return corev1.Container{
		Name:            c.Container.Name,
		Image:           c.Container.Image,
		ImagePullPolicy: *pullPolicy,
		SecurityContext: getSecurityContext(),
		Args:            getArgs(ctrlMetaCfg),
		Env:             getEnv(),
		Command: []string{
			containerStartupCmd,
		},
		VolumeMounts: getVolumeMounts(c),
	}
}
