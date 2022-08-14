/*
Copyright 2022. projectsveltos.io. All rights reserved.

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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// ClusterFeatureFinalizer allows ClusterFeatureReconciler to clean up resources associated with
	// ClusterFeature before removing it from the apiserver.
	ClusterFeatureFinalizer = "clusterfeaturefinalizer.projectsveltos.io"
)

type Selector string

// SyncMode specifies how features are synced in a workload cluster.
//+kubebuilder:validation:Enum:=OneTime;Continuous
type SyncMode string

const (
	// SyncModeOneTime indicates feature sync should happen only once
	SyncModeOneTime = SyncMode("OneTime")

	// SyncModeContinuous indicates feature sync should continuously happen
	SyncModeContinuous = SyncMode("Continuous")
)

// MGIANLUC: Kyverno generate ClusterRoleBinding https://kyverno.io/docs/writing-policies/generate/

type KyvernoConfiguration struct {
	// +kubebuilder:default:=1
	// Replicas is the number of kyverno replicas required
	Replicas uint `json:"replicas,omitempty"`

	// PolicyRef references ConfigMaps containing the Kyverno policies
	// that need to be deployed in the workload cluster.
	PolicyRefs []corev1.ObjectReference `json:"policyRef,omitempty"`
}

// InstallationMode specifies how prometheus is deployed in a CAPI Cluster.
//+kubebuilder:validation:Enum:=KubeStateMetrics;KubePrometheus;Custom
type InstallationMode string

const (
	// InstallationModeCustom will cause Prometheus Operator to be installed
	// and any PolicyRefs.
	InstallationModeCustom = InstallationMode("Custom")

	// InstallationModeKubeStateMetrics will cause Prometheus Operator to be installed
	// and any PolicyRefs. On top of that, KubeStateMetrics will also be installed
	// and a Promethus CRD instance will be created to scrape KubeStateMetrics metrics.
	InstallationModeKubeStateMetrics = InstallationMode("KubeStateMetrics")

	// InstallationModeKubePrometheus will cause the Kube-Prometheus stack to be deployed.
	// Any PolicyRefs will be installed after that.
	// Kube-Prometheus stack includes KubeStateMetrics.
	InstallationModeKubePrometheus = InstallationMode("KubePrometheus")
)

type PrometheusConfiguration struct {
	// InstallationMode indicates what type of resources will be deployed in a
	// CAPI Cluster.
	// +kubebuilder:default:=Custom
	// +optional
	InstallationMode InstallationMode `json:"installationMode,omitempty"`

	// storageClassName is the name of the StorageClass Prometheus will use to claim storage.
	// +optional
	StorageClassName *string `json:"storageClassName,omitempty"`

	// StorageQuantity indicates the amount of storage Prometheus will request from storageclass
	// if defined. (40Gi for instance)
	// If not defined and StorageClassName is defined, 40Gi will be used.
	// +optional
	StorageQuantity *resource.Quantity `json:"storageQuantity,omitempty"`

	// PolicyRef references ConfigMaps containing the Prometheus operator policies
	// that need to be deployed in the workload cluster. This includes:
	// - Prometheus, Alertmanager, ThanosRuler, ServiceMonitor, PodMonitor, Probe,
	// PrometheusRule, AlertmanagerConfig CRD instances;
	// -  Any other configuration needed for prometheus (like storageclass configuration)
	PolicyRefs []corev1.ObjectReference `json:"policyRef,omitempty"`
}

// ClusterFeatureSpec defines the desired state of ClusterFeature
type ClusterFeatureSpec struct {
	// ClusterSelector identifies ClusterAPI clusters to associate to.
	ClusterSelector Selector `json:"clusterSelector"`

	// SyncMode specifies how features are synced in a matching workload cluster.
	// - OneTime means, first time a workload cluster matches the ClusterFeature,
	// features will be deployed in such cluster. Any subsequent feature configuration
	// change won't be applied into the matching workload clusters;
	// - Continuous means first time a workload cluster matches the ClusterFeature,
	// features will be deployed in such a cluster. Any subsequent feature configuration
	// change will be applied into the matching workload clusters.
	// +kubebuilder:default:=OneTime
	// +optional
	SyncMode SyncMode `json:"syncMode,omitempty"`

	// WorkloadRoleRefs references all the WorkloadRoles that will be used
	// to create ClusterRole/Role in the workload cluster.
	// +optional
	WorkloadRoleRefs []corev1.ObjectReference `json:"workloadRoleRefs,omitempty"`

	// KyvernoConfiguration contains the Kyverno configuration.
	// If not nil, Kyverno will be deployed in the workload cluster along with, if any,
	// specified Kyverno policies.
	// +optional
	KyvernoConfiguration *KyvernoConfiguration `json:"kyvernoConfiguration,omitempty"`

	// PrometheusConfiguration contains the Prometheus configuration.
	// If not nil, at the very least Prometheus operator will be deployed in the workload cluster
	// +optional
	PrometheusConfiguration *PrometheusConfiguration `json:"prometheusConfiguration,omitempty"`
}

// ClusterFeatureStatus defines the observed state of ClusterFeature
type ClusterFeatureStatus struct {
	// MatchingClusterRefs reference all the cluster-api Cluster currently matching
	// ClusterFeature ClusterSelector
	MatchingClusterRefs []corev1.ObjectReference `json:"matchinClusters,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:resource:path=clusterfeatures,scope=Cluster
//+kubebuilder:subresource:status

// ClusterFeature is the Schema for the clusterfeatures API
type ClusterFeature struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterFeatureSpec   `json:"spec,omitempty"`
	Status ClusterFeatureStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ClusterFeatureList contains a list of ClusterFeature
type ClusterFeatureList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterFeature `json:"items"`
}

// nolint: gochecknoinits // forced pattern, can't workaround
func init() {
	SchemeBuilder.Register(&ClusterFeature{}, &ClusterFeatureList{})
}
