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

package controllers

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/gdexlab/go-render/render"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configv1alpha1 "github.com/projectsveltos/cluster-api-feature-manager/api/v1alpha1"
	"github.com/projectsveltos/cluster-api-feature-manager/internal/kyverno"
	"github.com/projectsveltos/cluster-api-feature-manager/pkg/logs"
	"github.com/projectsveltos/cluster-api-feature-manager/pkg/scope"
)

func deployKyverno(ctx context.Context, c client.Client,
	clusterNamespace, clusterName, applicant, _ string,
	logger logr.Logger) error {

	// Get ClusterSummary that requested this
	clusterSummary, clusterClient, err := getClusterSummaryAndCAPIClusterClient(ctx, applicant, c, logger)
	if err != nil {
		return err
	}

	// First verify if kyverno is installed, if not install it
	present, ready, err := isKyvernoReady(ctx, clusterClient, logger)
	if err != nil {
		logger.V(logs.LogInfo).Error(err, "Failed to verify presence of kyverno deployment")
		return err
	}

	if !present {
		err = deployKyvernoInWorklaodCluster(ctx, clusterClient,
			clusterSummary.Spec.ClusterFeatureSpec.KyvernoConfiguration.Replicas, logger)
		if err != nil {
			return err
		}
	}

	if !ready {
		return fmt.Errorf("kyverno deployment is not ready yet")
	}

	clusterRestConfig, err := getKubernetesRestConfig(ctx, logger, c, clusterNamespace, clusterName)
	if err != nil {
		return err
	}

	currentPolicies := make(map[string]bool, 0)
	if clusterSummary.Spec.ClusterFeatureSpec.KyvernoConfiguration != nil {
		var confgiMaps []corev1.ConfigMap
		confgiMaps, err = collectConfigMaps(ctx, c, clusterSummary.Spec.ClusterFeatureSpec.KyvernoConfiguration.PolicyRefs, logger)
		if err != nil {
			return err
		}

		var deployed []string
		deployed, err = deployConfigMaps(ctx, confgiMaps, clusterSummary, clusterClient, clusterRestConfig, logger)
		if err != nil {
			return err
		}

		for _, k := range deployed {
			currentPolicies[k] = true
		}
	}

	err = undeployStaleResources(ctx, clusterRestConfig, clusterClient, clusterSummary,
		getDeployedGroupVersionKinds(clusterSummary, configv1alpha1.FeatureKyverno), currentPolicies)
	if err != nil {
		return err
	}

	return nil
}

func unDeployKyverno(ctx context.Context, c client.Client,
	clusterNamespace, clusterName, applicant, _ string,
	logger logr.Logger) error {

	// Get ClusterSummary that requested this
	clusterSummary := &configv1alpha1.ClusterSummary{}
	if err := c.Get(ctx, types.NamespacedName{Name: applicant}, clusterSummary); err != nil {
		return err
	}

	// Get CAPI Cluster
	cluster := &clusterv1.Cluster{}
	if err := c.Get(ctx, types.NamespacedName{Namespace: clusterNamespace, Name: clusterName}, cluster); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info(fmt.Sprintf("Cluster %s/%s not found. Nothing to cleanup", clusterNamespace, clusterName))
			return nil
		}
		return err
	}

	clusterClient, err := getKubernetesClient(ctx, logger, c, clusterNamespace, clusterName)
	if err != nil {
		return err
	}

	clusterRestConfig, err := getKubernetesRestConfig(ctx, logger, c, clusterNamespace, clusterName)
	if err != nil {
		return err
	}

	err = undeployStaleResources(ctx, clusterRestConfig, clusterClient, clusterSummary,
		getDeployedGroupVersionKinds(clusterSummary, configv1alpha1.FeatureKyverno), map[string]bool{})
	if err != nil {
		return err
	}

	return nil
}

// kyvernoHash returns the hash of all the Kyverno referenced configmaps.
func kyvernoHash(ctx context.Context, c client.Client, clusterSummaryScope *scope.ClusterSummaryScope,
	logger logr.Logger) ([]byte, error) {

	h := sha256.New()
	var config string

	clusterSummary := clusterSummaryScope.ClusterSummary
	if clusterSummary.Spec.ClusterFeatureSpec.KyvernoConfiguration == nil {
		return h.Sum(nil), nil
	}
	for i := range clusterSummary.Spec.ClusterFeatureSpec.KyvernoConfiguration.PolicyRefs {
		reference := &clusterSummary.Spec.ClusterFeatureSpec.KyvernoConfiguration.PolicyRefs[i]
		configmap := &corev1.ConfigMap{}
		err := c.Get(ctx, types.NamespacedName{Namespace: reference.Namespace, Name: reference.Name}, configmap)
		if err != nil {
			if apierrors.IsNotFound(err) {
				logger.Info(fmt.Sprintf("configMap %s/%s does not exist yet",
					reference.Namespace, reference.Name))
				continue
			}
			logger.Error(err, fmt.Sprintf("failed to get configMap %s/%s",
				reference.Namespace, reference.Name))
			return nil, err
		}

		config += render.AsCode(configmap.Data)
	}

	h.Write([]byte(config))
	return h.Sum(nil), nil
}

func getKyvernoRefs(clusterSummaryScope *scope.ClusterSummaryScope) []corev1.ObjectReference {
	if clusterSummaryScope.ClusterSummary.Spec.ClusterFeatureSpec.KyvernoConfiguration != nil {
		return clusterSummaryScope.ClusterSummary.Spec.ClusterFeatureSpec.KyvernoConfiguration.PolicyRefs
	}
	return nil
}

// isKyvernoReady checks whether kyverno deployment is present and ready
func isKyvernoReady(ctx context.Context, c client.Client, logger logr.Logger) (present, ready bool, err error) {
	logger = logger.WithValues("kyvernonamespace", kyverno.Namespace, "kyvernodeployment", kyverno.Deployment)
	present = false
	ready = false
	depl := &appsv1.Deployment{}
	err = c.Get(ctx, types.NamespacedName{Namespace: kyverno.Namespace, Name: kyverno.Deployment}, depl)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(logs.LogDebug).Info("Kyverno deployment not found")
			err = nil
			return
		}
		return
	}

	present = true

	if depl.Status.ReadyReplicas != *depl.Spec.Replicas {
		logger.V(logs.LogDebug).Info("Not all replicas are ready for Kyverno deployment")
		return
	}

	ready = true
	return
}

func changeReplicas(content string, r uint) string {
	replicas := "replicas: 1"

	index := strings.Index(content, replicas)
	if index == -1 {
		panic(fmt.Errorf("did not find proper replicas set"))
	}

	newReplicas := fmt.Sprintf("replicas: %d", r)
	content = strings.ReplaceAll(content, replicas, newReplicas)
	return content
}

func deployKyvernoInWorklaodCluster(ctx context.Context, c client.Client, replicas uint, logger logr.Logger) error {
	kyvernoYAML := changeReplicas(string(kyverno.KyvernoYAML), replicas)
	return deployDoc(ctx, c, []byte(kyvernoYAML), logger)
}

func deployKyvernoPolicy(ctx context.Context, config *rest.Config, c client.Client,
	configMap *corev1.ConfigMap, clusterSummary *configv1alpha1.ClusterSummary,
	currentKyvernos map[string]bool,
	logger logr.Logger) error {

	addedPolicies, err := deployContentOfConfigMap(ctx, config, c, configMap, clusterSummary, logger)
	if err != nil {
		return err
	}

	for _, p := range addedPolicies {
		currentKyvernos[p] = true
	}

	return nil
}
