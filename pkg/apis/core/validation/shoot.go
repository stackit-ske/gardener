// Copyright 2018 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package validation

import (
	"fmt"
	"math/big"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Masterminds/semver"
	"github.com/go-test/deep"
	"github.com/robfig/cron"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	apivalidation "k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metav1validation "k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/pointer"
	"k8s.io/utils/strings/slices"

	"github.com/gardener/gardener/pkg/apis/core"
	"github.com/gardener/gardener/pkg/apis/core/helper"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/gardener/gardener/pkg/features"
	"github.com/gardener/gardener/pkg/utils"
	gardenerutils "github.com/gardener/gardener/pkg/utils/gardener"
	"github.com/gardener/gardener/pkg/utils/timewindow"
	admissionpluginsvalidation "github.com/gardener/gardener/pkg/utils/validation/admissionplugins"
	cidrvalidation "github.com/gardener/gardener/pkg/utils/validation/cidr"
	featuresvalidation "github.com/gardener/gardener/pkg/utils/validation/features"
	versionutils "github.com/gardener/gardener/pkg/utils/version"
)

var (
	availableProxyModes = sets.New(
		string(core.ProxyModeIPTables),
		string(core.ProxyModeIPVS),
	)
	availableKubernetesDashboardAuthenticationModes = sets.New(
		core.KubernetesDashboardAuthModeToken,
	)
	availableNginxIngressExternalTrafficPolicies = sets.New(
		string(corev1.ServiceExternalTrafficPolicyTypeCluster),
		string(corev1.ServiceExternalTrafficPolicyTypeLocal),
	)
	availableShootOperations = sets.New(
		v1beta1constants.ShootOperationMaintain,
		v1beta1constants.ShootOperationRetry,
	).Union(availableShootMaintenanceOperations)
	availableShootMaintenanceOperations = sets.New(
		v1beta1constants.GardenerOperationReconcile,
		v1beta1constants.OperationRotateCAStart,
		v1beta1constants.OperationRotateCAComplete,
		v1beta1constants.ShootOperationRotateKubeconfigCredentials,
		v1beta1constants.ShootOperationRotateObservabilityCredentials,
		v1beta1constants.ShootOperationRotateSSHKeypair,
	).Union(forbiddenShootOperationsWhenHibernated)
	forbiddenShootOperationsWhenHibernated = sets.New(
		v1beta1constants.OperationRotateCredentialsStart,
		v1beta1constants.OperationRotateCredentialsComplete,
		v1beta1constants.OperationRotateETCDEncryptionKeyStart,
		v1beta1constants.OperationRotateETCDEncryptionKeyComplete,
		v1beta1constants.OperationRotateServiceAccountKeyStart,
		v1beta1constants.OperationRotateServiceAccountKeyComplete,
	)
	availableShootPurposes = sets.New(
		string(core.ShootPurposeEvaluation),
		string(core.ShootPurposeTesting),
		string(core.ShootPurposeDevelopment),
		string(core.ShootPurposeProduction),
	)
	availableWorkerCRINames = sets.New(
		string(core.CRINameContainerD),
		string(core.CRINameDocker),
	)
	availableClusterAutoscalerExpanderModes = sets.New(
		string(core.ClusterAutoscalerExpanderLeastWaste),
		string(core.ClusterAutoscalerExpanderMostPods),
		string(core.ClusterAutoscalerExpanderPriority),
		string(core.ClusterAutoscalerExpanderRandom),
	)
	availableCoreDNSAutoscalingModes = sets.New(
		string(core.CoreDNSAutoscalingModeClusterProportional),
		string(core.CoreDNSAutoscalingModeHorizontal),
	)
	availableSchedulingProfiles = sets.New(
		string(core.SchedulingProfileBalanced),
		string(core.SchedulingProfileBinPacking),
	)

	// asymmetric algorithms from https://datatracker.ietf.org/doc/html/rfc7518#section-3.1
	availableOIDCSigningAlgs = sets.New(
		"RS256",
		"RS384",
		"RS512",
		"ES256",
		"ES384",
		"ES512",
		"PS256",
		"PS384",
		"PS512",
		"none",
	)

	workerlessErrorMsg = "this field should not be set for workerless Shoot clusters"
)

// ValidateShoot validates a Shoot object.
func ValidateShoot(shoot *core.Shoot) field.ErrorList {
	allErrs := field.ErrorList{}

	allErrs = append(allErrs, apivalidation.ValidateObjectMeta(&shoot.ObjectMeta, true, apivalidation.NameIsDNSLabel, field.NewPath("metadata"))...)
	allErrs = append(allErrs, validateNameConsecutiveHyphens(shoot.Name, field.NewPath("metadata", "name"))...)
	allErrs = append(allErrs, validateShootOperation(shoot.Annotations[v1beta1constants.GardenerOperation], shoot.Annotations[v1beta1constants.GardenerMaintenanceOperation], shoot, field.NewPath("metadata", "annotations"))...)
	allErrs = append(allErrs, ValidateShootSpec(shoot.ObjectMeta, &shoot.Spec, field.NewPath("spec"), false)...)
	allErrs = append(allErrs, ValidateShootHAConfig(shoot)...)

	return allErrs
}

// ValidateShootUpdate validates a Shoot object before an update.
func ValidateShootUpdate(newShoot, oldShoot *core.Shoot) field.ErrorList {
	allErrs := field.ErrorList{}

	allErrs = append(allErrs, apivalidation.ValidateObjectMetaUpdate(&newShoot.ObjectMeta, &oldShoot.ObjectMeta, field.NewPath("metadata"))...)
	allErrs = append(allErrs, ValidateShootObjectMetaUpdate(newShoot.ObjectMeta, oldShoot.ObjectMeta, field.NewPath("metadata"))...)
	allErrs = append(allErrs, ValidateShootSpecUpdate(&newShoot.Spec, &oldShoot.Spec, newShoot.ObjectMeta, field.NewPath("spec"))...)
	// validate version updates only to kubernetes 1.25
	allErrs = append(allErrs, validateKubernetesVersionUpdate125(newShoot, oldShoot)...)
	allErrs = append(allErrs, ValidateShoot(newShoot)...)
	allErrs = append(allErrs, ValidateShootHAConfigUpdate(newShoot, oldShoot)...)

	return allErrs
}

// ValidateShootTemplate validates a ShootTemplate.
func ValidateShootTemplate(shootTemplate *core.ShootTemplate, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	allErrs = append(allErrs, metav1validation.ValidateLabels(shootTemplate.Labels, fldPath.Child("metadata", "labels"))...)
	allErrs = append(allErrs, apivalidation.ValidateAnnotations(shootTemplate.Annotations, fldPath.Child("metadata", "annotations"))...)
	allErrs = append(allErrs, ValidateShootSpec(shootTemplate.ObjectMeta, &shootTemplate.Spec, fldPath.Child("spec"), true)...)

	return allErrs
}

// ValidateShootTemplateUpdate validates a ShootTemplate before an update.
func ValidateShootTemplateUpdate(newShootTemplate, oldShootTemplate *core.ShootTemplate, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	allErrs = append(allErrs, ValidateShootSpecUpdate(&newShootTemplate.Spec, &oldShootTemplate.Spec, newShootTemplate.ObjectMeta, fldPath.Child("spec"))...)

	if oldShootTemplate.Spec.Networking != nil && oldShootTemplate.Spec.Networking.Nodes != nil {
		allErrs = append(allErrs, apivalidation.ValidateImmutableField(newShootTemplate.Spec.Networking.Nodes, oldShootTemplate.Spec.Networking.Nodes, fldPath.Child("spec", "networking", "nodes"))...)
	}

	return allErrs
}

// ValidateShootObjectMetaUpdate validates the object metadata of a Shoot object.
func ValidateShootObjectMetaUpdate(newMeta, oldMeta metav1.ObjectMeta, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, validateShootKubeconfigRotation(newMeta, oldMeta, fldPath)...)
	return allErrs
}

// validateShootKubeconfigRotation validates that shoot in deletion cannot rotate its kubeconfig.
func validateShootKubeconfigRotation(newMeta, oldMeta metav1.ObjectMeta, fldPath *field.Path) field.ErrorList {
	if newMeta.DeletionTimestamp == nil {
		return field.ErrorList{}
	}

	// already set operation is valid use case
	if oldOperation, oldOk := oldMeta.Annotations[v1beta1constants.GardenerOperation]; oldOk && oldOperation == v1beta1constants.ShootOperationRotateKubeconfigCredentials {
		return field.ErrorList{}
	}

	allErrs := field.ErrorList{}

	// disallow kubeconfig rotation
	if operation, ok := newMeta.Annotations[v1beta1constants.GardenerOperation]; ok && operation == v1beta1constants.ShootOperationRotateKubeconfigCredentials {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("annotations").Key(v1beta1constants.GardenerOperation), v1beta1constants.ShootOperationRotateKubeconfigCredentials, "kubeconfig rotations is not allowed for clusters in deletion"))
	}

	return allErrs
}

// ValidateShootSpec validates the specification of a Shoot object.
func ValidateShootSpec(meta metav1.ObjectMeta, spec *core.ShootSpec, fldPath *field.Path, inTemplate bool) field.ErrorList {
	var (
		allErrs    = field.ErrorList{}
		workerless = len(spec.Provider.Workers) == 0
	)

	allErrs = append(allErrs, validateProvider(spec.Provider, spec.Kubernetes, spec.Networking, workerless, fldPath.Child("provider"), inTemplate)...)
	allErrs = append(allErrs, validateAddons(spec.Addons, spec.Kubernetes, spec.Purpose, workerless, fldPath.Child("addons"))...)
	allErrs = append(allErrs, validateDNS(spec.DNS, fldPath.Child("dns"))...)
	allErrs = append(allErrs, validateExtensions(spec.Extensions, fldPath.Child("extensions"))...)
	allErrs = append(allErrs, validateResources(spec.Resources, fldPath.Child("resources"))...)
	allErrs = append(allErrs, validateKubernetes(spec.Kubernetes, spec.Networking, isDockerConfigured(spec.Provider.Workers), workerless, fldPath.Child("kubernetes"))...)
	allErrs = append(allErrs, validateNetworking(spec.Networking, workerless, fldPath.Child("networking"))...)
	allErrs = append(allErrs, validateMaintenance(spec.Maintenance, fldPath.Child("maintenance"), workerless)...)
	allErrs = append(allErrs, validateMonitoring(spec.Monitoring, fldPath.Child("monitoring"))...)
	allErrs = append(allErrs, ValidateHibernation(meta.Annotations, spec.Hibernation, fldPath.Child("hibernation"))...)

	if len(spec.Region) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("region"), "must specify a region"))
	}
	if len(spec.CloudProfileName) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("cloudProfileName"), "must specify a cloud profile"))
	}
	if spec.SecretBindingName != nil && workerless {
		allErrs = append(allErrs, field.Forbidden(fldPath.Child("secretBindingName"), workerlessErrorMsg))
	} else if len(pointer.StringDeref(spec.SecretBindingName, "")) == 0 && !workerless {
		allErrs = append(allErrs, field.Required(fldPath.Child("secretBindingName"), "must specify a name"))
	}
	if spec.SeedName != nil && len(*spec.SeedName) == 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("seedName"), spec.SeedName, "seed name must not be empty when providing the key"))
	}
	if spec.SeedSelector != nil {
		allErrs = append(allErrs, metav1validation.ValidateLabelSelector(&spec.SeedSelector.LabelSelector, metav1validation.LabelSelectorValidationOptions{AllowInvalidLabelValueInSelector: true}, fldPath.Child("seedSelector"))...)
	}
	if purpose := spec.Purpose; purpose != nil {
		allowedShootPurposes := availableShootPurposes
		if meta.Namespace == v1beta1constants.GardenNamespace || inTemplate {
			allowedShootPurposes = sets.New(append(sets.List(availableShootPurposes), string(core.ShootPurposeInfrastructure))...)
		}

		if !allowedShootPurposes.Has(string(*purpose)) {
			allErrs = append(allErrs, field.NotSupported(fldPath.Child("purpose"), *purpose, sets.List(allowedShootPurposes)))
		}
	}
	allErrs = append(allErrs, ValidateTolerations(spec.Tolerations, fldPath.Child("tolerations"))...)
	allErrs = append(allErrs, ValidateSystemComponents(spec.SystemComponents, fldPath.Child("systemComponents"), workerless)...)

	return allErrs
}

func isDockerConfigured(workers []core.Worker) bool {
	for _, worker := range workers {
		if worker.CRI == nil || worker.CRI.Name == core.CRINameDocker {
			return true
		}
	}
	return false
}

// ValidateShootSpecUpdate validates the specification of a Shoot object.
func ValidateShootSpecUpdate(newSpec, oldSpec *core.ShootSpec, newObjectMeta metav1.ObjectMeta, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if newObjectMeta.DeletionTimestamp != nil && !apiequality.Semantic.DeepEqual(newSpec, oldSpec) {
		if diff := deep.Equal(newSpec, oldSpec); diff != nil {
			return field.ErrorList{field.Forbidden(fldPath, strings.Join(diff, ","))}
		}
		return apivalidation.ValidateImmutableField(newSpec, oldSpec, fldPath)
	}

	allErrs = append(allErrs, apivalidation.ValidateImmutableField(newSpec.Region, oldSpec.Region, fldPath.Child("region"))...)
	allErrs = append(allErrs, apivalidation.ValidateImmutableField(newSpec.CloudProfileName, oldSpec.CloudProfileName, fldPath.Child("cloudProfileName"))...)
	allErrs = append(allErrs, apivalidation.ValidateImmutableField(newSpec.SecretBindingName, oldSpec.SecretBindingName, fldPath.Child("secretBindingName"))...)
	allErrs = append(allErrs, apivalidation.ValidateImmutableField(newSpec.ExposureClassName, oldSpec.ExposureClassName, fldPath.Child("exposureClassName"))...)

	allErrs = append(allErrs, validateDNSUpdate(newSpec.DNS, oldSpec.DNS, newSpec.SeedName != nil, fldPath.Child("dns"))...)
	allErrs = append(allErrs, ValidateKubernetesVersionUpdate(newSpec.Kubernetes.Version, oldSpec.Kubernetes.Version, fldPath.Child("kubernetes", "version"))...)

	allErrs = append(allErrs, validateKubeControllerManagerUpdate(newSpec.Kubernetes.KubeControllerManager, oldSpec.Kubernetes.KubeControllerManager, fldPath.Child("kubernetes", "kubeControllerManager"))...)

	if err := validateWorkerUpdate(len(newSpec.Provider.Workers) > 0, len(oldSpec.Provider.Workers) > 0, fldPath.Child("provider", "workers")); err != nil {
		allErrs = append(allErrs, err)
		return allErrs
	}

	allErrs = append(allErrs, ValidateProviderUpdate(&newSpec.Provider, &oldSpec.Provider, fldPath.Child("provider"))...)

	for i, newWorker := range newSpec.Provider.Workers {
		oldWorker := newWorker
		for _, ow := range oldSpec.Provider.Workers {
			if ow.Name == newWorker.Name {
				oldWorker = ow
				break
			}
		}
		idxPath := fldPath.Child("provider", "workers").Index(i)

		oldKubernetesVersion := oldSpec.Kubernetes.Version
		newKubernetesVersion := newSpec.Kubernetes.Version
		if oldWorker.Kubernetes != nil && oldWorker.Kubernetes.Version != nil {
			oldKubernetesVersion = *oldWorker.Kubernetes.Version
		}
		if newWorker.Kubernetes != nil && newWorker.Kubernetes.Version != nil {
			newKubernetesVersion = *newWorker.Kubernetes.Version
		}

		// worker kubernetes versions must not be downgraded and must not skip a minor
		allErrs = append(allErrs, ValidateKubernetesVersionUpdate(newKubernetesVersion, oldKubernetesVersion, idxPath.Child("kubernetes", "version"))...)
	}

	allErrs = append(allErrs, validateNetworkingUpdate(newSpec.Networking, oldSpec.Networking, fldPath.Child("networking"))...)

	return allErrs
}

func validateWorkerUpdate(newHasWorkers, oldHasWorkers bool, fldPath *field.Path) *field.Error {
	if oldHasWorkers && !newHasWorkers {
		return field.Forbidden(fldPath, "cannot switch from a Shoot with workers to a workerless Shoot")
	}
	if !oldHasWorkers && newHasWorkers {
		return field.Forbidden(fldPath, "cannot switch from a workerless Shoot to a Shoot with workers")
	}

	return nil
}

// ValidateProviderUpdate validates the specification of a Provider object.
func ValidateProviderUpdate(newProvider, oldProvider *core.Provider, fldPath *field.Path) field.ErrorList {
	return apivalidation.ValidateImmutableField(newProvider.Type, oldProvider.Type, fldPath.Child("type"))
}

// ValidateShootStatusUpdate validates the status field of a Shoot object.
func ValidateShootStatusUpdate(newStatus, oldStatus core.ShootStatus) field.ErrorList {
	var (
		allErrs = field.ErrorList{}
		fldPath = field.NewPath("status")
	)

	if len(oldStatus.UID) > 0 {
		allErrs = append(allErrs, apivalidation.ValidateImmutableField(newStatus.UID, oldStatus.UID, fldPath.Child("uid"))...)
	}
	if len(oldStatus.TechnicalID) > 0 {
		allErrs = append(allErrs, apivalidation.ValidateImmutableField(newStatus.TechnicalID, oldStatus.TechnicalID, fldPath.Child("technicalID"))...)
	}

	if oldStatus.ClusterIdentity != nil && !apiequality.Semantic.DeepEqual(oldStatus.ClusterIdentity, newStatus.ClusterIdentity) {
		allErrs = append(allErrs, apivalidation.ValidateImmutableField(newStatus.ClusterIdentity, oldStatus.ClusterIdentity, fldPath.Child("clusterIdentity"))...)
	}
	if len(newStatus.AdvertisedAddresses) > 0 {
		allErrs = append(allErrs, validateAdvertiseAddresses(newStatus.AdvertisedAddresses, fldPath.Child("advertisedAddresses"))...)
	}

	return allErrs
}

// validateAdvertiseAddresses validates kube-apiserver addresses.
func validateAdvertiseAddresses(addresses []core.ShootAdvertisedAddress, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	names := sets.New[string]()
	for i, address := range addresses {
		if address.Name == "" {
			allErrs = append(allErrs, field.Required(fldPath.Index(i).Child("name"), "field must not be empty"))
		} else if names.Has(address.Name) {
			allErrs = append(allErrs, field.Duplicate(fldPath.Index(i).Child("name"), address.Name))
		} else {
			names.Insert(address.Name)
			allErrs = append(allErrs, validateAdvertisedURL(address.URL, fldPath.Index(i).Child("url"))...)
		}
	}
	return allErrs
}

// validateAdvertisedURL validates kube-apiserver's URL.
func validateAdvertisedURL(URL string, fldPath *field.Path) field.ErrorList {
	var allErrors field.ErrorList
	const form = "; desired format: https://host[:port]"
	if u, err := url.Parse(URL); err != nil {
		allErrors = append(allErrors, field.Required(fldPath, "url must be a valid URL: "+err.Error()+form))
	} else {
		if u.Scheme != "https" {
			allErrors = append(allErrors, field.Invalid(fldPath, u.Scheme, "'https' is the only allowed URL scheme"+form))
		}
		if len(u.Host) == 0 {
			allErrors = append(allErrors, field.Invalid(fldPath, u.Host, "host must be provided"+form))
		}
		if len(u.Path) > 0 {
			allErrors = append(allErrors, field.Invalid(fldPath, u.Path, "path is not permitted in the URL"+form))
		}
		if u.User != nil {
			allErrors = append(allErrors, field.Invalid(fldPath, u.User.String(), "user information is not permitted in the URL"+form))
		}
		if len(u.Fragment) != 0 {
			allErrors = append(allErrors, field.Invalid(fldPath, u.Fragment, "fragments are not permitted in the URL"+form))
		}
		if len(u.RawQuery) != 0 {
			allErrors = append(allErrors, field.Invalid(fldPath, u.RawQuery, "query parameters are not permitted in the URL"+form))
		}
	}
	return allErrors
}

func validateAddons(addons *core.Addons, kubernetes core.Kubernetes, purpose *core.ShootPurpose, workerless bool, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if workerless && addons != nil {
		allErrs = append(allErrs, field.Forbidden(fldPath, "addons cannot be enabled for Workerless Shoot clusters"))
		return allErrs
	}

	versionGreaterOrEqual122, _ := versionutils.CheckVersionMeetsConstraint(kubernetes.Version, ">= 1.22")
	if (helper.NginxIngressEnabled(addons) || helper.KubernetesDashboardEnabled(addons)) && versionGreaterOrEqual122 && (purpose != nil && *purpose != core.ShootPurposeEvaluation) {
		allErrs = append(allErrs, field.Forbidden(fldPath, "for Kubernetes versions >= 1.22 addons can only be enabled on shoots with .spec.purpose=evaluation"))
	}

	if helper.NginxIngressEnabled(addons) {
		if policy := addons.NginxIngress.ExternalTrafficPolicy; policy != nil {
			if !availableNginxIngressExternalTrafficPolicies.Has(string(*policy)) {
				allErrs = append(allErrs, field.NotSupported(fldPath.Child("nginxIngress", "externalTrafficPolicy"), *policy, sets.List(availableNginxIngressExternalTrafficPolicies)))
			}
		}
	}

	if helper.KubernetesDashboardEnabled(addons) {
		if authMode := addons.KubernetesDashboard.AuthenticationMode; authMode != nil {
			if !availableKubernetesDashboardAuthenticationModes.Has(*authMode) {
				allErrs = append(allErrs, field.NotSupported(fldPath.Child("kubernetesDashboard", "authenticationMode"), *authMode, sets.List(availableKubernetesDashboardAuthenticationModes)))
			}
		}
	}

	return allErrs
}

const (
	// kube-controller-manager's default value for --node-cidr-mask-size for IPv4
	defaultNodeCIDRMaskSizeV4 = 24
	// kube-controller-manager's default value for --node-cidr-mask-size for IPv6
	defaultNodeCIDRMaskSizeV6 = 64
)

// ValidateNodeCIDRMaskWithMaxPod validates if the Pod Network has enough ip addresses (configured via the NodeCIDRMask on the kube controller manager) to support the highest max pod setting on the shoot
func ValidateNodeCIDRMaskWithMaxPod(maxPod int32, nodeCIDRMaskSize int32, networking core.Networking) field.ErrorList {
	allErrs := field.ErrorList{}

	totalBitLen := int32(net.IPv4len * 8) // entire IPv4 bit length
	defaultNodeCIDRMaskSize := defaultNodeCIDRMaskSizeV4

	if core.IsIPv6SingleStack(networking.IPFamilies) {
		totalBitLen = net.IPv6len * 8 // entire IPv6 bit length
		defaultNodeCIDRMaskSize = defaultNodeCIDRMaskSizeV6
	}

	// Each Node gets assigned a subnet of the entire pod network with a mask size of nodeCIDRMaskSize,
	// calculate bit length of a single podCIDR subnet (Node.status.podCIDR).
	subnetBitLen := totalBitLen - nodeCIDRMaskSize

	// Calculate how many addresses a single podCIDR subnet contains.
	// This will overflow uint64 if nodeCIDRMaskSize <= 64 (subnetBitLen >= 64, default in IPv6), so use big.Int
	ipAdressesAvailable := &big.Int{}
	ipAdressesAvailable.Exp(big.NewInt(2), big.NewInt(int64(subnetBitLen)), nil)
	// first and last ips are reserved, subtract 2
	ipAdressesAvailable.Sub(ipAdressesAvailable, big.NewInt(2))

	if ipAdressesAvailable.Cmp(big.NewInt(int64(maxPod))) < 0 {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("kubernetes").Child("kubeControllerManager").Child("nodeCIDRMaskSize"), nodeCIDRMaskSize, fmt.Sprintf("kubelet or kube-controller-manager configuration incorrect. Please adjust the nodeCIDRMaskSize to support the highest maxPod on any worker pool. The nodeCIDRMaskSize of %d (default: %d) only supports %d IP addresses. The highest maxPod setting is %d (default: 110). Please choose a nodeCIDRMaskSize that at least supports %d IP addresses", nodeCIDRMaskSize, defaultNodeCIDRMaskSize, ipAdressesAvailable, maxPod, maxPod)))
	}

	return allErrs
}

// ValidateTotalNodeCountWithPodCIDR validates if the podCIDRs in the Pod Network can support the maximum number of nodes configured in the worker pools of the shoot
func ValidateTotalNodeCountWithPodCIDR(shoot *core.Shoot) field.ErrorList {
	allErrs := field.ErrorList{}

	nodeCIDRMaskSize := int64(defaultNodeCIDRMaskSizeV4)
	if core.IsIPv6SingleStack(shoot.Spec.Networking.IPFamilies) {
		nodeCIDRMaskSize = defaultNodeCIDRMaskSizeV6
	}
	if shoot.Spec.Kubernetes.KubeControllerManager != nil && shoot.Spec.Kubernetes.KubeControllerManager.NodeCIDRMaskSize != nil {
		nodeCIDRMaskSize = int64(*shoot.Spec.Kubernetes.KubeControllerManager.NodeCIDRMaskSize)
	}

	// calculate maximum number of total nodes
	totalNodes := int64(0)
	for _, worker := range shoot.Spec.Provider.Workers {
		totalNodes += int64(worker.Maximum)
	}

	podNetworkCIDR := core.DefaultPodNetworkCIDR
	if shoot.Spec.Networking.Pods != nil {
		podNetworkCIDR = *shoot.Spec.Networking.Pods
	}

	_, podNetwork, err := net.ParseCIDR(podNetworkCIDR)
	if err != nil {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("networking").Child("pods"), podNetworkCIDR, fmt.Sprintf("cannot parse shoot's pod network cidr : %s", podNetworkCIDR)))
		return allErrs
	}

	podCIDRMaskSize, _ := podNetwork.Mask.Size()
	if podCIDRMaskSize == 0 {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("networking").Child("pods"), podNetwork.String(), fmt.Sprintf("incorrect pod network mask : %s. Please ensure the mask is in proper form", podNetwork.String())))
		return allErrs
	}

	// Calculate how many subnets with nodeCIDRMaskSize can be allocated out of the pod network (with podCIDRMaskSize).
	// This indicates how many Nodes we can host at max from a networking perspective.
	var bitLen, maxNodeCount = &big.Int{}, &big.Int{}
	bitLen.Sub(big.NewInt(nodeCIDRMaskSize), big.NewInt(int64(podCIDRMaskSize)))
	maxNodeCount.Exp(big.NewInt(2), bitLen, nil)

	if maxNodeCount.Cmp(big.NewInt(totalNodes)) < 0 {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("provider").Child("workers"), totalNodes, fmt.Sprintf("worker configuration incorrect. The podCIDRs in `spec.networking.pod` can only support a maximum of %d nodes. The total number of worker pool nodes should be less than %d ", maxNodeCount, maxNodeCount)))
	}

	return allErrs
}

func validateKubeControllerManagerUpdate(newConfig, oldConfig *core.KubeControllerManagerConfig, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	var (
		nodeCIDRMaskNew *int32
		nodeCIDRMaskOld *int32
	)

	if newConfig != nil {
		nodeCIDRMaskNew = newConfig.NodeCIDRMaskSize
	}
	if oldConfig != nil {
		nodeCIDRMaskOld = oldConfig.NodeCIDRMaskSize
	}

	allErrs = append(allErrs, apivalidation.ValidateImmutableField(nodeCIDRMaskNew, nodeCIDRMaskOld, fldPath.Child("nodeCIDRMaskSize"))...)

	return allErrs
}

func validateDNSUpdate(new, old *core.DNS, seedGotAssigned bool, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if old != nil && new == nil {
		allErrs = append(allErrs, apivalidation.ValidateImmutableField(new, old, fldPath)...)
	}

	if new != nil && old != nil {
		if old.Domain != nil && new.Domain != old.Domain {
			allErrs = append(allErrs, apivalidation.ValidateImmutableField(new.Domain, old.Domain, fldPath.Child("domain"))...)
		}

		// allow to finalize DNS configuration during seed assignment. this is required because
		// some decisions about the DNS setup can only be taken once the target seed is clarified.
		if !seedGotAssigned {
			var (
				primaryOld = helper.FindPrimaryDNSProvider(old.Providers)
				primaryNew = helper.FindPrimaryDNSProvider(new.Providers)
			)

			if primaryOld != nil && primaryNew == nil {
				allErrs = append(allErrs, field.Forbidden(fldPath.Child("providers"), "removing a primary provider is not allowed"))
			}

			if primaryOld != nil && primaryOld.Type != nil && primaryNew != nil && primaryNew.Type == nil {
				allErrs = append(allErrs, field.Forbidden(fldPath.Child("providers"), "removing the primary provider type is not allowed"))
			}

			if primaryOld != nil && primaryOld.Type != nil && primaryNew != nil && primaryNew.Type != nil && *primaryOld.Type != *primaryNew.Type {
				allErrs = append(allErrs, field.Forbidden(fldPath.Child("providers"), "changing primary provider type is not allowed"))
			}
		}
	}

	return allErrs
}

// ValidateKubernetesVersionUpdate ensures that new version is newer than old version and does not skip one minor
func ValidateKubernetesVersionUpdate(new, old string, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if len(new) == 0 {
		allErrs = append(allErrs, field.Invalid(fldPath, new, "cannot validate kubernetes version upgrade because it is unset"))
		return allErrs
	}

	// Forbid Kubernetes version downgrade
	downgrade, err := versionutils.CompareVersions(new, "<", old)
	if err != nil {
		allErrs = append(allErrs, field.Invalid(fldPath, new, err.Error()))
	}
	if downgrade {
		allErrs = append(allErrs, field.Forbidden(fldPath, "kubernetes version downgrade is not supported"))
	}

	// Forbid Kubernetes version upgrade which skips a minor version
	oldVersion, err := semver.NewVersion(old)
	if err != nil {
		allErrs = append(allErrs, field.Invalid(fldPath, old, err.Error()))
	}
	nextMinorVersion := oldVersion.IncMinor().IncMinor()

	skippingMinorVersion, err := versionutils.CompareVersions(new, ">=", nextMinorVersion.String())
	if err != nil {
		allErrs = append(allErrs, field.Invalid(fldPath, new, err.Error()))
	}
	if skippingMinorVersion {
		allErrs = append(allErrs, field.Forbidden(fldPath, "kubernetes version upgrade cannot skip a minor version"))
	}

	return allErrs
}

func validateNetworkingUpdate(newNetworking, oldNetworking *core.Networking, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if oldNetworking != nil {
		if newNetworking == nil {
			allErrs = append(allErrs, field.Forbidden(fldPath, "networking cannot be set to nil if it's already set"))
			return allErrs
		}
	} else {
		// if the old networking is nil, we cannot validate immutability anyway, so exit early
		return allErrs
	}

	allErrs = append(allErrs, apivalidation.ValidateImmutableField(newNetworking.Type, oldNetworking.Type, fldPath.Child("type"))...)
	allErrs = append(allErrs, apivalidation.ValidateImmutableField(newNetworking.IPFamilies, oldNetworking.IPFamilies, fldPath.Child("ipFamilies"))...)
	if oldNetworking.Pods != nil {
		allErrs = append(allErrs, apivalidation.ValidateImmutableField(newNetworking.Pods, oldNetworking.Pods, fldPath.Child("pods"))...)
	}
	if oldNetworking.Services != nil {
		allErrs = append(allErrs, apivalidation.ValidateImmutableField(newNetworking.Services, oldNetworking.Services, fldPath.Child("services"))...)
	}
	if !features.DefaultFeatureGate.Enabled(features.MutableShootSpecNetworkingNodes) && oldNetworking.Nodes != nil {
		allErrs = append(allErrs, apivalidation.ValidateImmutableField(newNetworking.Nodes, oldNetworking.Nodes, fldPath.Child("nodes"))...)
	}

	return allErrs
}

// validateWorkerGroupAndControlPlaneKubernetesVersion ensures that new version is newer than old version and does not skip two minor
func validateWorkerGroupAndControlPlaneKubernetesVersion(controlPlaneVersion, workerGroupVersion string, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// worker group kubernetes version must not be higher than controlplane version
	uplift, err := versionutils.CompareVersions(workerGroupVersion, ">", controlPlaneVersion)
	if err != nil {
		allErrs = append(allErrs, field.Invalid(fldPath, controlPlaneVersion, err.Error()))
	}
	if uplift {
		allErrs = append(allErrs, field.Forbidden(fldPath, "worker group kubernetes version must not be higher than control plane version"))
	}

	// Forbid Kubernetes version upgrade which skips a minor version
	workerVersion, err := semver.NewVersion(workerGroupVersion)
	if err != nil {
		allErrs = append(allErrs, field.Invalid(fldPath, workerGroupVersion, err.Error()))
	}
	threeMinorSkewVersion := workerVersion.IncMinor().IncMinor().IncMinor()

	versionSkewViolation, err := versionutils.CompareVersions(controlPlaneVersion, ">=", threeMinorSkewVersion.String())
	if err != nil {
		allErrs = append(allErrs, field.Invalid(fldPath, controlPlaneVersion, err.Error()))
	}
	if versionSkewViolation {
		allErrs = append(allErrs, field.Forbidden(fldPath, "worker group kubernetes version must be at most two minor versions behind control plane version"))
	}

	return allErrs
}

func validateDNS(dns *core.DNS, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if dns == nil {
		return allErrs
	}

	if dns.Domain != nil {
		allErrs = append(allErrs, ValidateDNS1123Subdomain(*dns.Domain, fldPath.Child("domain"))...)
	}

	primaryDNSProvider := helper.FindPrimaryDNSProvider(dns.Providers)
	if primaryDNSProvider != nil && primaryDNSProvider.Type != nil {
		if *primaryDNSProvider.Type != core.DNSUnmanaged && dns.Domain == nil {
			allErrs = append(allErrs, field.Required(fldPath.Child("domain"), fmt.Sprintf("domain must be set when primary provider type is not set to %q", core.DNSUnmanaged)))
		}
	}

	var (
		names        = sets.New[string]()
		primaryFound bool
	)
	for i, provider := range dns.Providers {
		idxPath := fldPath.Child("providers").Index(i)

		if provider.SecretName != nil && provider.Type != nil {
			providerName := gardenerutils.GenerateDNSProviderName(*provider.SecretName, *provider.Type)
			if names.Has(providerName) {
				allErrs = append(allErrs, field.Invalid(idxPath, providerName, "combination of .secretName and .type must be unique across dns providers"))
				continue
			}
			for _, err := range validation.IsDNS1123Subdomain(providerName) {
				allErrs = append(allErrs, field.Invalid(idxPath, providerName, fmt.Sprintf("combination of .secretName and .type is invalid: %q", err)))
			}
			names.Insert(providerName)
		}

		if provider.Primary != nil && *provider.Primary {
			if primaryFound {
				allErrs = append(allErrs, field.Forbidden(idxPath.Child("primary"), "multiple primary DNS providers are not supported"))
				continue
			}
			primaryFound = true
		}

		if providerType := provider.Type; providerType != nil {
			if *providerType == core.DNSUnmanaged && provider.SecretName != nil {
				allErrs = append(allErrs, field.Invalid(idxPath.Child("secretName"), provider.SecretName, fmt.Sprintf("secretName must not be set when type is %q", core.DNSUnmanaged)))
				continue
			}
		}

		if provider.SecretName != nil && provider.Type == nil {
			allErrs = append(allErrs, field.Required(idxPath.Child("type"), "type must be set when secretName is set"))
		}
	}

	return allErrs
}

func validateExtensions(extensions []core.Extension, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	for i, extension := range extensions {
		if extension.Type == "" {
			allErrs = append(allErrs, field.Required(fldPath.Index(i).Child("type"), "field must not be empty"))
		}
	}
	return allErrs
}

func validateResources(resources []core.NamedResourceReference, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	names := sets.Set[string]{}
	for i, resource := range resources {
		if resource.Name == "" {
			allErrs = append(allErrs, field.Required(fldPath.Index(i).Child("name"), "field must not be empty"))
		} else if names.Has(resource.Name) {
			allErrs = append(allErrs, field.Duplicate(fldPath.Index(i).Child("name"), resource.Name))
		} else {
			names.Insert(resource.Name)
		}
		allErrs = append(allErrs, validateCrossVersionObjectReference(resource.ResourceRef, fldPath.Index(i).Child("resourceRef"))...)
	}
	return allErrs
}

func validateKubernetes(kubernetes core.Kubernetes, networking *core.Networking, dockerConfigured, workerless bool, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if len(kubernetes.Version) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("version"), "kubernetes version must not be empty"))
		return allErrs
	}

	k8sGreaterEqual127, _ := versionutils.CheckVersionMeetsConstraint(kubernetes.Version, ">= 1.27")
	if k8sGreaterEqual127 && pointer.BoolDeref(kubernetes.EnableStaticTokenKubeconfig, false) {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("enableStaticTokenKubeconfig"), kubernetes.EnableStaticTokenKubeconfig, "for Kubernetes versions >= 1.27, enableStaticTokenKubeconfig field cannot not be set to true, please see https://github.com/gardener/gardener/blob/master/docs/usage/shoot_access.md#static-token-kubeconfig"))
	}

	allErrs = append(allErrs, ValidateKubeAPIServer(kubernetes.KubeAPIServer, kubernetes.Version, false, fldPath.Child("kubeAPIServer"))...)
	allErrs = append(allErrs, ValidateKubeControllerManager(kubernetes.KubeControllerManager, networking, kubernetes.Version, workerless, fldPath.Child("kubeControllerManager"))...)

	if workerless {
		allErrs = append(allErrs, validateKubernetesForWorkerlessShoot(kubernetes, fldPath)...)
	} else {
		allErrs = append(allErrs, validateKubeScheduler(kubernetes.KubeScheduler, kubernetes.Version, fldPath.Child("kubeScheduler"))...)
		allErrs = append(allErrs, validateKubeProxy(kubernetes.KubeProxy, kubernetes.Version, fldPath.Child("kubeProxy"))...)

		if kubernetes.Kubelet != nil {
			allErrs = append(allErrs, ValidateKubeletConfig(*kubernetes.Kubelet, kubernetes.Version, dockerConfigured, fldPath.Child("kubelet"))...)
		}

		if clusterAutoscaler := kubernetes.ClusterAutoscaler; clusterAutoscaler != nil {
			allErrs = append(allErrs, ValidateClusterAutoscaler(*clusterAutoscaler, kubernetes.Version, fldPath.Child("clusterAutoscaler"))...)
		}

		if verticalPodAutoscaler := kubernetes.VerticalPodAutoscaler; verticalPodAutoscaler != nil {
			allErrs = append(allErrs, ValidateVerticalPodAutoscaler(*verticalPodAutoscaler, fldPath.Child("verticalPodAutoscaler"))...)
		}

		k8sGreaterEqual125, _ := versionutils.CheckVersionMeetsConstraint(kubernetes.Version, ">= 1.25")
		if k8sGreaterEqual125 && kubernetes.AllowPrivilegedContainers != nil {
			allErrs = append(allErrs, field.Forbidden(fldPath.Child("allowPrivilegedContainers"), "for Kubernetes versions >= 1.25, allowPrivilegedContainers field should not be set, please see https://github.com/gardener/gardener/blob/master/docs/usage/pod-security.md#speckubernetesallowprivilegedcontainers-in-the-shoot-spec"))
		}
	}

	return allErrs
}

func validateKubernetesForWorkerlessShoot(kubernetes core.Kubernetes, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if kubernetes.KubeScheduler != nil {
		allErrs = append(allErrs, field.Forbidden(fldPath.Child("kubeScheduler"), workerlessErrorMsg))
	}

	if kubernetes.KubeProxy != nil {
		allErrs = append(allErrs, field.Forbidden(fldPath.Child("kubeProxy"), workerlessErrorMsg))
	}

	if kubernetes.Kubelet != nil {
		allErrs = append(allErrs, field.Forbidden(fldPath.Child("kubelet"), workerlessErrorMsg))
	}

	if kubernetes.ClusterAutoscaler != nil {
		allErrs = append(allErrs, field.Forbidden(fldPath.Child("clusterAutoScaler"), workerlessErrorMsg))
	}

	if kubernetes.VerticalPodAutoscaler != nil {
		allErrs = append(allErrs, field.Forbidden(fldPath.Child("verticalPodAutoScaler"), workerlessErrorMsg))
	}

	if kubernetes.AllowPrivilegedContainers != nil {
		allErrs = append(allErrs, field.Forbidden(fldPath.Child("allowPrivilegedContainers"), workerlessErrorMsg))
	}

	return allErrs
}

func fieldNilOrEmptyString(field *string) bool {
	return field == nil || len(*field) == 0
}

func validateNetworking(networking *core.Networking, workerless bool, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if workerless {
		// Nothing to be validated here, exit
		if networking == nil {
			return allErrs
		}

		if networking.Type != nil {
			allErrs = append(allErrs, field.Forbidden(fldPath.Child("type"), workerlessErrorMsg))
		}
		if networking.ProviderConfig != nil {
			allErrs = append(allErrs, field.Forbidden(fldPath.Child("providerConfig"), workerlessErrorMsg))
		}
		if networking.Pods != nil {
			allErrs = append(allErrs, field.Forbidden(fldPath.Child("pods"), workerlessErrorMsg))
		}
		if networking.Nodes != nil {
			allErrs = append(allErrs, field.Forbidden(fldPath.Child("nodes"), workerlessErrorMsg))
		}
	} else {
		if networking == nil {
			allErrs = append(allErrs, field.Required(fldPath, "networking should not be nil for a Shoot with workers"))
			return allErrs
		}

		if len(pointer.StringDeref(networking.Type, "")) == 0 {
			allErrs = append(allErrs, field.Required(fldPath.Child("type"), "networking type must be provided"))
		}
	}

	if errs := ValidateIPFamilies(networking.IPFamilies, fldPath.Child("ipFamilies")); len(errs) > 0 {
		// further validation doesn't make any sense, because we don't know which IP family to check for in the CIDR fields
		return append(allErrs, errs...)
	}

	primaryIPFamily := helper.DeterminePrimaryIPFamily(networking.IPFamilies)

	if networking.Nodes != nil {
		path := fldPath.Child("nodes")
		cidr := cidrvalidation.NewCIDR(*networking.Nodes, path)

		allErrs = append(allErrs, cidr.ValidateParse()...)
		allErrs = append(allErrs, cidr.ValidateIPFamily(string(primaryIPFamily))...)
		allErrs = append(allErrs, cidrvalidation.ValidateCIDRIsCanonical(path, cidr.GetCIDR())...)
	}

	if networking.Pods != nil {
		path := fldPath.Child("pods")
		cidr := cidrvalidation.NewCIDR(*networking.Pods, path)

		allErrs = append(allErrs, cidr.ValidateParse()...)
		allErrs = append(allErrs, cidr.ValidateIPFamily(string(primaryIPFamily))...)
		allErrs = append(allErrs, cidrvalidation.ValidateCIDRIsCanonical(path, cidr.GetCIDR())...)
	}

	if networking.Services != nil {
		path := fldPath.Child("services")
		cidr := cidrvalidation.NewCIDR(*networking.Services, path)

		allErrs = append(allErrs, cidr.ValidateParse()...)
		allErrs = append(allErrs, cidr.ValidateIPFamily(string(primaryIPFamily))...)
		allErrs = append(allErrs, cidrvalidation.ValidateCIDRIsCanonical(path, cidr.GetCIDR())...)
	}

	return allErrs
}

// ValidateWatchCacheSizes validates the given WatchCacheSizes fields.
func ValidateWatchCacheSizes(sizes *core.WatchCacheSizes, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if sizes != nil {
		if defaultSize := sizes.Default; defaultSize != nil {
			allErrs = append(allErrs, apivalidation.ValidateNonnegativeField(int64(*defaultSize), fldPath.Child("default"))...)
		}

		for idx, resourceWatchCacheSize := range sizes.Resources {
			idxPath := fldPath.Child("resources").Index(idx)
			if len(resourceWatchCacheSize.Resource) == 0 {
				allErrs = append(allErrs, field.Required(idxPath.Child("resource"), "must not be empty"))
			}
			allErrs = append(allErrs, apivalidation.ValidateNonnegativeField(int64(resourceWatchCacheSize.CacheSize), idxPath.Child("size"))...)
		}
	}
	return allErrs
}

// ValidateKubeAPIServerLogging validates the given KubeAPIServer Logging fields.
func ValidateKubeAPIServerLogging(loggingConfig *core.KubeAPIServerLogging, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if loggingConfig != nil {
		if verbosity := loggingConfig.Verbosity; verbosity != nil {
			allErrs = append(allErrs, apivalidation.ValidateNonnegativeField(int64(*verbosity), fldPath.Child("verbosity"))...)
		}
		if httpAccessVerbosity := loggingConfig.HTTPAccessVerbosity; httpAccessVerbosity != nil {
			allErrs = append(allErrs, apivalidation.ValidateNonnegativeField(int64(*httpAccessVerbosity), fldPath.Child("httpAccessVerbosity"))...)
		}
	}
	return allErrs
}

// ValidateClusterAutoscaler validates the given ClusterAutoscaler fields.
func ValidateClusterAutoscaler(autoScaler core.ClusterAutoscaler, k8sVersion string, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if threshold := autoScaler.ScaleDownUtilizationThreshold; threshold != nil {
		if *threshold < 0.0 {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("scaleDownUtilizationThreshold"), *threshold, "can not be negative"))
		}
		if *threshold > 1.0 {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("scaleDownUtilizationThreshold"), *threshold, "can not be greater than 1.0"))
		}
	}
	if maxNodeProvisionTime := autoScaler.MaxNodeProvisionTime; maxNodeProvisionTime != nil && maxNodeProvisionTime.Duration < 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("maxNodeProvisionTime"), *maxNodeProvisionTime, "can not be negative"))
	}
	if maxGracefulTerminationSeconds := autoScaler.MaxGracefulTerminationSeconds; maxGracefulTerminationSeconds != nil && *maxGracefulTerminationSeconds < 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("maxGracefulTerminationSeconds"), *maxGracefulTerminationSeconds, "can not be negative"))
	}

	if expander := autoScaler.Expander; expander != nil && !availableClusterAutoscalerExpanderModes.Has(string(*expander)) {
		allErrs = append(allErrs, field.NotSupported(fldPath.Child("expander"), *expander, sets.List(availableClusterAutoscalerExpanderModes)))
	}

	if ignoreTaints := autoScaler.IgnoreTaints; ignoreTaints != nil {
		allErrs = append(allErrs, validateClusterAutoscalerIgnoreTaints(ignoreTaints, fldPath.Child("ignoreTaints"))...)
	}

	return allErrs
}

// ValidateVerticalPodAutoscaler validates the given VerticalPodAutoscaler fields.
func ValidateVerticalPodAutoscaler(autoScaler core.VerticalPodAutoscaler, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if threshold := autoScaler.EvictAfterOOMThreshold; threshold != nil && threshold.Duration < 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("evictAfterOOMThreshold"), *threshold, "can not be negative"))
	}
	if interval := autoScaler.UpdaterInterval; interval != nil && interval.Duration < 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("updaterInterval"), *interval, "can not be negative"))
	}
	if interval := autoScaler.RecommenderInterval; interval != nil && interval.Duration < 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("recommenderInterval"), *interval, "can not be negative"))
	}

	return allErrs
}

func validateKubernetesVersionUpdate125(new, old *core.Shoot) field.ErrorList {
	allErrs := field.ErrorList{}

	newShootVersionGreaterEqual125, err := versionutils.CheckVersionMeetsConstraint(new.Spec.Kubernetes.Version, ">= 1.25")
	if err != nil {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "kubernetes", "version"), new.Spec.Kubernetes.Version, "Invalid new kubernetes version"))
	}
	oldShootVersionLess125, err := versionutils.CheckVersionMeetsConstraint(old.Spec.Kubernetes.Version, "< 1.25")
	if err != nil {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "kubernetes", "version"), old.Spec.Kubernetes.Version, "Invalid old kubernetes version"))
	}

	// since we have disabled the policy/v1beta1/podsecuritypolicies API for workerless shoots, we only need to check for disabled PSPs for regular shoots.
	if !helper.IsWorkerless(new) && newShootVersionGreaterEqual125 && oldShootVersionLess125 {
		pspDisabledInNewSpec := isPSPDisabled(new.Spec.Kubernetes.KubeAPIServer)
		pspDisabledInOldSpec := isPSPDisabled(old.Spec.Kubernetes.KubeAPIServer)
		if !pspDisabledInNewSpec || !pspDisabledInOldSpec {
			allErrs = append(allErrs, field.Forbidden(field.NewPath("spec", "kubernetes", "version"), `admission plugin "PodSecurityPolicy" should be disabled for Kubernetes versions >=1.25, please check https://github.com/gardener/gardener/blob/master/docs/usage/pod-security.md#migrating-from-podsecuritypolicys-to-podsecurity-admission-controller`))
		} else if shootReconciliationSuccessful, msg := shootReconciliationSuccessful(old); !shootReconciliationSuccessful {
			allErrs = append(allErrs, field.Forbidden(field.NewPath("spec", "kubernetes", "version"), fmt.Sprintf("Shoot should have been reconciled successfully before upgrading to v1.25; error: %s", msg)))
		}
	}
	return allErrs
}

func isPSPDisabled(kubeAPIServerConfig *core.KubeAPIServerConfig) bool {
	if kubeAPIServerConfig != nil {
		for _, plugin := range kubeAPIServerConfig.AdmissionPlugins {
			if plugin.Name == "PodSecurityPolicy" && pointer.BoolDeref(plugin.Disabled, false) {
				return true
			}
		}
	}
	return false
}

// ValidateKubeAPIServer validates KubeAPIServerConfig.
func ValidateKubeAPIServer(kubeAPIServer *core.KubeAPIServerConfig, version string, kubeconfigAllowedForAdmissionPlugins bool, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if kubeAPIServer == nil {
		return allErrs
	}

	if oidc := kubeAPIServer.OIDCConfig; oidc != nil {
		oidcPath := fldPath.Child("oidcConfig")

		if fieldNilOrEmptyString(oidc.ClientID) {
			if oidc.ClientID != nil {
				allErrs = append(allErrs, field.Invalid(oidcPath.Child("clientID"), oidc.ClientID, "clientID cannot be empty when key is provided"))
			}
			if !fieldNilOrEmptyString(oidc.IssuerURL) {
				allErrs = append(allErrs, field.Invalid(oidcPath.Child("clientID"), oidc.ClientID, "clientID must be set when issuerURL is provided"))
			}
		}

		if fieldNilOrEmptyString(oidc.IssuerURL) {
			if oidc.IssuerURL != nil {
				allErrs = append(allErrs, field.Invalid(oidcPath.Child("issuerURL"), oidc.IssuerURL, "issuerURL cannot be empty when key is provided"))
			}
			if !fieldNilOrEmptyString(oidc.ClientID) {
				allErrs = append(allErrs, field.Invalid(oidcPath.Child("issuerURL"), oidc.IssuerURL, "issuerURL must be set when clientID is provided"))
			}
		} else {
			issuer, err := url.Parse(*oidc.IssuerURL)
			if err != nil || (issuer != nil && len(issuer.Host) == 0) {
				allErrs = append(allErrs, field.Invalid(oidcPath.Child("issuerURL"), oidc.IssuerURL, "must be a valid URL and have https scheme"))
			}
			if issuer != nil && issuer.Scheme != "https" {
				allErrs = append(allErrs, field.Invalid(oidcPath.Child("issuerURL"), oidc.IssuerURL, "must have https scheme"))
			}
		}

		if oidc.CABundle != nil {
			if _, err := utils.DecodeCertificate([]byte(*oidc.CABundle)); err != nil {
				allErrs = append(allErrs, field.Invalid(oidcPath.Child("caBundle"), *oidc.CABundle, "caBundle is not a valid PEM-encoded certificate"))
			}
		}
		if oidc.GroupsClaim != nil && len(*oidc.GroupsClaim) == 0 {
			allErrs = append(allErrs, field.Invalid(oidcPath.Child("groupsClaim"), *oidc.GroupsClaim, "groupsClaim cannot be empty when key is provided"))
		}
		if oidc.GroupsPrefix != nil && len(*oidc.GroupsPrefix) == 0 {
			allErrs = append(allErrs, field.Invalid(oidcPath.Child("groupsPrefix"), *oidc.GroupsPrefix, "groupsPrefix cannot be empty when key is provided"))
		}
		for i, alg := range oidc.SigningAlgs {
			if !availableOIDCSigningAlgs.Has(alg) {
				allErrs = append(allErrs, field.NotSupported(oidcPath.Child("signingAlgs").Index(i), alg, sets.List(availableOIDCSigningAlgs)))
			}
		}
		if oidc.UsernameClaim != nil && len(*oidc.UsernameClaim) == 0 {
			allErrs = append(allErrs, field.Invalid(oidcPath.Child("usernameClaim"), *oidc.UsernameClaim, "usernameClaim cannot be empty when key is provided"))
		}
		if oidc.UsernamePrefix != nil && len(*oidc.UsernamePrefix) == 0 {
			allErrs = append(allErrs, field.Invalid(oidcPath.Child("usernamePrefix"), *oidc.UsernamePrefix, "usernamePrefix cannot be empty when key is provided"))
		}
	}

	allErrs = append(allErrs, admissionpluginsvalidation.ValidateAdmissionPlugins(kubeAPIServer.AdmissionPlugins, version, kubeconfigAllowedForAdmissionPlugins, fldPath.Child("admissionPlugins"))...)

	if auditConfig := kubeAPIServer.AuditConfig; auditConfig != nil {
		auditPath := fldPath.Child("auditConfig")
		if auditPolicy := auditConfig.AuditPolicy; auditPolicy != nil && auditConfig.AuditPolicy.ConfigMapRef != nil {
			allErrs = append(allErrs, validateAuditPolicyConfigMapReference(auditPolicy.ConfigMapRef, auditPath.Child("auditPolicy", "configMapRef"))...)
		}
	}

	allErrs = append(allErrs, ValidateWatchCacheSizes(kubeAPIServer.WatchCacheSizes, fldPath.Child("watchCacheSizes"))...)

	allErrs = append(allErrs, ValidateKubeAPIServerLogging(kubeAPIServer.Logging, fldPath.Child("logging"))...)

	if defaultNotReadyTolerationSeconds := kubeAPIServer.DefaultNotReadyTolerationSeconds; defaultNotReadyTolerationSeconds != nil {
		allErrs = append(allErrs, apivalidation.ValidateNonnegativeField(int64(*defaultNotReadyTolerationSeconds), fldPath.Child("defaultNotReadyTolerationSeconds"))...)
	}
	if defaultUnreachableTolerationSeconds := kubeAPIServer.DefaultUnreachableTolerationSeconds; defaultUnreachableTolerationSeconds != nil {
		allErrs = append(allErrs, apivalidation.ValidateNonnegativeField(int64(*defaultUnreachableTolerationSeconds), fldPath.Child("defaultUnreachableTolerationSeconds"))...)
	}

	if kubeAPIServer.Requests != nil {
		const maxMaxNonMutatingRequestsInflight = 800
		if v := kubeAPIServer.Requests.MaxNonMutatingInflight; v != nil {
			path := fldPath.Child("requests", "maxNonMutatingInflight")

			allErrs = append(allErrs, apivalidation.ValidateNonnegativeField(int64(*v), path)...)
			if *v > maxMaxNonMutatingRequestsInflight {
				allErrs = append(allErrs, field.Invalid(path, *v, fmt.Sprintf("cannot set higher than %d", maxMaxNonMutatingRequestsInflight)))
			}
		}

		const maxMaxMutatingRequestsInflight = 400
		if v := kubeAPIServer.Requests.MaxMutatingInflight; v != nil {
			path := fldPath.Child("requests", "maxMutatingInflight")

			allErrs = append(allErrs, apivalidation.ValidateNonnegativeField(int64(*v), path)...)
			if *v > maxMaxMutatingRequestsInflight {
				allErrs = append(allErrs, field.Invalid(path, *v, fmt.Sprintf("cannot set higher than %d", maxMaxMutatingRequestsInflight)))
			}
		}
	}

	if kubeAPIServer.ServiceAccountConfig != nil {
		if kubeAPIServer.ServiceAccountConfig.MaxTokenExpiration != nil {
			if kubeAPIServer.ServiceAccountConfig.MaxTokenExpiration.Duration < 0 {
				allErrs = append(allErrs, field.Invalid(fldPath.Child("serviceAccountConfig", "maxTokenExpiration"), *kubeAPIServer.ServiceAccountConfig.MaxTokenExpiration, "can not be negative"))
			}

			if duration := kubeAPIServer.ServiceAccountConfig.MaxTokenExpiration.Duration; duration > 0 && duration < 720*time.Hour {
				allErrs = append(allErrs, field.Forbidden(fldPath.Child("serviceAccountConfig", "maxTokenExpiration"), "must be at least 720h (30d)"))
			}

			if duration := kubeAPIServer.ServiceAccountConfig.MaxTokenExpiration.Duration; duration > 2160*time.Hour {
				allErrs = append(allErrs, field.Forbidden(fldPath.Child("serviceAccountConfig", "maxTokenExpiration"), "must be at most 2160h (90d)"))
			}
		}

		geqKubernetes122, _ := versionutils.CheckVersionMeetsConstraint(version, ">= 1.22")
		if kubeAPIServer.ServiceAccountConfig.AcceptedIssuers != nil && !geqKubernetes122 {
			allErrs = append(allErrs, field.Forbidden(fldPath.Child("serviceAccountConfig", "acceptedIssuers"), "this field is only available in Kubernetes v1.22+"))
		}
	}

	if kubeAPIServer.EventTTL != nil {
		if kubeAPIServer.EventTTL.Duration < 0 {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("eventTTL"), *kubeAPIServer.EventTTL, "can not be negative"))
		}
		if kubeAPIServer.EventTTL.Duration > time.Hour*24*7 {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("eventTTL"), *kubeAPIServer.EventTTL, "can not be longer than 7d"))
		}
	}

	allErrs = append(allErrs, featuresvalidation.ValidateFeatureGates(kubeAPIServer.FeatureGates, version, fldPath.Child("featureGates"))...)

	return allErrs
}

// ValidateKubeControllerManager validates KubeControllerManagerConfig.
func ValidateKubeControllerManager(kcm *core.KubeControllerManagerConfig, networking *core.Networking, version string, workerless bool, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if kcm == nil {
		return nil
	}

	if !workerless {
		if maskSize := kcm.NodeCIDRMaskSize; maskSize != nil && networking != nil {
			if core.IsIPv4SingleStack(networking.IPFamilies) {
				if *maskSize < 16 || *maskSize > 28 {
					allErrs = append(allErrs, field.Invalid(fldPath.Child("nodeCIDRMaskSize"), *maskSize, "nodeCIDRMaskSize must be between 16 and 28"))
				}
			}
		}

		if podEvictionTimeout := kcm.PodEvictionTimeout; podEvictionTimeout != nil && podEvictionTimeout.Duration <= 0 {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("podEvictionTimeout"), podEvictionTimeout.Duration, "podEvictionTimeout must be larger than 0"))
		}

		if nodeMonitorGracePeriod := kcm.NodeMonitorGracePeriod; nodeMonitorGracePeriod != nil && nodeMonitorGracePeriod.Duration <= 0 {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("nodeMonitorGracePeriod"), nodeMonitorGracePeriod.Duration, "nodeMonitorGracePeriod must be larger than 0"))
		}

		if hpa := kcm.HorizontalPodAutoscalerConfig; hpa != nil {
			hpaPath := fldPath.Child("horizontalPodAutoscaler")

			if hpa.SyncPeriod != nil && hpa.SyncPeriod.Duration < 1*time.Second {
				allErrs = append(allErrs, field.Invalid(hpaPath.Child("syncPeriod"), *hpa.SyncPeriod, "syncPeriod must not be less than a second"))
			}
			if hpa.Tolerance != nil && *hpa.Tolerance <= 0 {
				allErrs = append(allErrs, field.Invalid(hpaPath.Child("tolerance"), *hpa.Tolerance, "tolerance of must be greater than 0"))
			}
			if hpa.DownscaleStabilization != nil && hpa.DownscaleStabilization.Duration < 1*time.Second {
				allErrs = append(allErrs, field.Invalid(hpaPath.Child("downscaleStabilization"), *hpa.DownscaleStabilization, "downScale stabilization must not be less than a second"))
			}
			if hpa.InitialReadinessDelay != nil && hpa.InitialReadinessDelay.Duration <= 0 {
				allErrs = append(allErrs, field.Invalid(hpaPath.Child("initialReadinessDelay"), *hpa.InitialReadinessDelay, "initial readiness delay must be greater than 0"))
			}
			if hpa.CPUInitializationPeriod != nil && hpa.CPUInitializationPeriod.Duration < 1*time.Second {
				allErrs = append(allErrs, field.Invalid(hpaPath.Child("cpuInitializationPeriod"), *hpa.CPUInitializationPeriod, "cpu initialization period must not be less than a second"))
			}
		}
	} else {
		if kcm.NodeCIDRMaskSize != nil {
			allErrs = append(allErrs, field.Forbidden(fldPath.Child("nodeCIDRMaskSize"), workerlessErrorMsg))
		}
		if kcm.HorizontalPodAutoscalerConfig != nil {
			allErrs = append(allErrs, field.Forbidden(fldPath.Child("horizontalPodAutoscaler"), workerlessErrorMsg))
		}
		if kcm.PodEvictionTimeout != nil {
			allErrs = append(allErrs, field.Forbidden(fldPath.Child("podEvictionTimeout"), workerlessErrorMsg))
		}
		if kcm.NodeMonitorGracePeriod != nil {
			allErrs = append(allErrs, field.Forbidden(fldPath.Child("nodeMonitorGracePeriod"), workerlessErrorMsg))
		}
	}

	allErrs = append(allErrs, featuresvalidation.ValidateFeatureGates(kcm.FeatureGates, version, fldPath.Child("featureGates"))...)

	return allErrs
}

func validateKubeScheduler(ks *core.KubeSchedulerConfig, version string, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if ks != nil {
		profile := ks.Profile
		if profile != nil {
			if !availableSchedulingProfiles.Has(string(*profile)) {
				allErrs = append(allErrs, field.NotSupported(fldPath.Child("profile"), *profile, sets.List(availableSchedulingProfiles)))
			}
		}

		allErrs = append(allErrs, featuresvalidation.ValidateFeatureGates(ks.FeatureGates, version, fldPath.Child("featureGates"))...)
	}

	return allErrs
}

func validateKubeProxy(kp *core.KubeProxyConfig, version string, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if kp != nil {
		if kp.Mode == nil {
			allErrs = append(allErrs, field.Required(fldPath.Child("mode"), "must be set when .spec.kubernetes.kubeProxy is set"))
		} else if mode := *kp.Mode; !availableProxyModes.Has(string(mode)) {
			allErrs = append(allErrs, field.NotSupported(fldPath.Child("mode"), mode, sets.List(availableProxyModes)))
		}
		allErrs = append(allErrs, featuresvalidation.ValidateFeatureGates(kp.FeatureGates, version, fldPath.Child("featureGates"))...)
	}
	return allErrs
}

func validateMonitoring(monitoring *core.Monitoring, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if monitoring != nil && monitoring.Alerting != nil {
		allErrs = append(allErrs, validateAlerting(monitoring.Alerting, fldPath.Child("alerting"))...)
	}
	return allErrs
}

func validateAlerting(alerting *core.Alerting, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	emails := make(map[string]struct{})
	for i, email := range alerting.EmailReceivers {
		if !utils.TestEmail(email) {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("emailReceivers").Index(i), email, "must provide a valid email"))
		}

		if _, duplicate := emails[email]; duplicate {
			allErrs = append(allErrs, field.Duplicate(fldPath.Child("emailReceivers").Index(i), email))
		} else {
			emails[email] = struct{}{}
		}
	}
	return allErrs
}

func validateMaintenance(maintenance *core.Maintenance, fldPath *field.Path, workerless bool) field.ErrorList {
	allErrs := field.ErrorList{}

	if maintenance == nil {
		return allErrs
	}

	if maintenance.AutoUpdate != nil {
		if workerless && maintenance.AutoUpdate.MachineImageVersion != nil {
			allErrs = append(allErrs, field.Forbidden(fldPath.Child("autoUpdate", "machineImageVersion"), workerlessErrorMsg))
		}
	}

	if maintenance.TimeWindow != nil {
		maintenanceTimeWindow, err := timewindow.ParseMaintenanceTimeWindow(maintenance.TimeWindow.Begin, maintenance.TimeWindow.End)
		if err != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("timeWindow", "begin/end"), maintenance.TimeWindow, err.Error()))
		} else {
			duration := maintenanceTimeWindow.Duration()
			if duration > core.MaintenanceTimeWindowDurationMaximum {
				allErrs = append(allErrs, field.Invalid(fldPath.Child("timeWindow"), duration, fmt.Sprintf("time window must not be greater than %s", core.MaintenanceTimeWindowDurationMaximum)))
				return allErrs
			}
			if duration < core.MaintenanceTimeWindowDurationMinimum {
				allErrs = append(allErrs, field.Invalid(fldPath.Child("timeWindow"), duration, fmt.Sprintf("time window must not be smaller than %s", core.MaintenanceTimeWindowDurationMinimum)))
				return allErrs
			}
		}
	}

	return allErrs
}

func validateProvider(provider core.Provider, kubernetes core.Kubernetes, networking *core.Networking, workerless bool, fldPath *field.Path, inTemplate bool) field.ErrorList {
	var (
		allErrs = field.ErrorList{}
		maxPod  int32
	)

	if len(provider.Type) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("type"), "must specify a provider type"))
	}

	if workerless {
		if provider.InfrastructureConfig != nil {
			allErrs = append(allErrs, field.Forbidden(fldPath.Child("infrastructureConfig"), workerlessErrorMsg))
		}
		if provider.ControlPlaneConfig != nil {
			allErrs = append(allErrs, field.Forbidden(fldPath.Child("controlPlaneConfig"), workerlessErrorMsg))
		}
		if provider.WorkersSettings != nil {
			allErrs = append(allErrs, field.Forbidden(fldPath.Child("workersSettings"), workerlessErrorMsg))
		}
	} else {
		if kubernetes.Kubelet != nil && kubernetes.Kubelet.MaxPods != nil {
			maxPod = *kubernetes.Kubelet.MaxPods
		}

		for i, worker := range provider.Workers {
			allErrs = append(allErrs, ValidateWorker(worker, kubernetes, fldPath.Child("workers").Index(i), inTemplate)...)

			if worker.Kubernetes != nil && worker.Kubernetes.Kubelet != nil && worker.Kubernetes.Kubelet.MaxPods != nil && *worker.Kubernetes.Kubelet.MaxPods > maxPod {
				maxPod = *worker.Kubernetes.Kubelet.MaxPods
			}
		}

		allErrs = append(allErrs, ValidateWorkers(provider.Workers, fldPath.Child("workers"))...)
		allErrs = append(allErrs, ValidateSystemComponentWorkers(provider.Workers, kubernetes.Version, fldPath.Child("workers"))...)
	}

	if kubernetes.KubeControllerManager != nil && kubernetes.KubeControllerManager.NodeCIDRMaskSize != nil && networking != nil {
		if maxPod == 0 {
			// default maxPod setting on kubelet
			maxPod = 110
		}
		allErrs = append(allErrs, ValidateNodeCIDRMaskWithMaxPod(maxPod, *kubernetes.KubeControllerManager.NodeCIDRMaskSize, *networking)...)
	}

	return allErrs
}

const (
	// maxWorkerNameLength is a constant for the maximum length for worker name.
	maxWorkerNameLength = 15

	// maxVolumeNameLength is a constant for the maximum length for data volume name.
	maxVolumeNameLength = 15
)

// ValidateWorker validates the worker object.
func ValidateWorker(worker core.Worker, kubernetes core.Kubernetes, fldPath *field.Path, inTemplate bool) field.ErrorList {
	kubernetesVersion := kubernetes.Version
	allErrs := field.ErrorList{}

	allErrs = append(allErrs, validateDNS1123Label(worker.Name, fldPath.Child("name"))...)
	if len(worker.Name) > maxWorkerNameLength {
		allErrs = append(allErrs, field.TooLong(fldPath.Child("name"), worker.Name, maxWorkerNameLength))
	}
	if len(worker.Machine.Type) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("machine", "type"), "must specify a machine type"))
	}
	if worker.Machine.Image != nil {
		if len(worker.Machine.Image.Name) == 0 {
			allErrs = append(allErrs, field.Required(fldPath.Child("machine", "image", "name"), "must specify a machine image name"))
		}
		if !inTemplate && len(worker.Machine.Image.Version) == 0 {
			allErrs = append(allErrs, field.Required(fldPath.Child("machine", "image", "version"), "must specify a machine image version"))
		}
	}
	if worker.Minimum < 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("minimum"), worker.Minimum, "minimum value must not be negative"))
	}
	if worker.Maximum < 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("maximum"), worker.Maximum, "maximum value must not be negative"))
	}
	if worker.Maximum < worker.Minimum {
		allErrs = append(allErrs, field.Forbidden(fldPath.Child("maximum"), "maximum value must not be less than minimum value"))
	}

	allErrs = append(allErrs, ValidatePositiveIntOrPercent(worker.MaxSurge, fldPath.Child("maxSurge"))...)
	allErrs = append(allErrs, ValidatePositiveIntOrPercent(worker.MaxUnavailable, fldPath.Child("maxUnavailable"))...)
	allErrs = append(allErrs, IsNotMoreThan100Percent(worker.MaxUnavailable, fldPath.Child("maxUnavailable"))...)

	if (worker.MaxUnavailable == nil || getIntOrPercentValue(*worker.MaxUnavailable) == 0) && (worker.MaxSurge != nil && getIntOrPercentValue(*worker.MaxSurge) == 0) {
		// Both MaxSurge and MaxUnavailable cannot be zero.
		allErrs = append(allErrs, field.Invalid(fldPath.Child("maxUnavailable"), worker.MaxUnavailable, "may not be 0 when `maxSurge` is 0"))
	}

	allErrs = append(allErrs, metav1validation.ValidateLabels(worker.Labels, fldPath.Child("labels"))...)
	allErrs = append(allErrs, apivalidation.ValidateAnnotations(worker.Annotations, fldPath.Child("annotations"))...)
	if len(worker.Taints) > 0 {
		allErrs = append(allErrs, validateTaints(worker.Taints, fldPath.Child("taints"))...)
	}
	if worker.Kubernetes != nil {
		if worker.Kubernetes.Version != nil {
			workerGroupKubernetesVersion := *worker.Kubernetes.Version
			allErrs = append(allErrs, validateWorkerGroupAndControlPlaneKubernetesVersion(kubernetesVersion, workerGroupKubernetesVersion, fldPath.Child("kubernetes", "version"))...)
			kubernetesVersion = workerGroupKubernetesVersion
		}

		if worker.Kubernetes.Kubelet != nil {
			allErrs = append(allErrs, ValidateKubeletConfig(*worker.Kubernetes.Kubelet, kubernetesVersion, isDockerConfigured([]core.Worker{worker}), fldPath.Child("kubernetes", "kubelet"))...)
		} else if kubernetes.Kubelet != nil {
			allErrs = append(allErrs, ValidateKubeletConfig(*kubernetes.Kubelet, kubernetesVersion, isDockerConfigured([]core.Worker{worker}), fldPath.Child("kubernetes", "kubelet"))...)
		}
	}

	if worker.CABundle != nil {
		if _, err := utils.DecodeCertificate([]byte(*worker.CABundle)); err != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("caBundle"), *(worker.CABundle), "caBundle is not a valid PEM-encoded certificate"))
		}
	}

	volumeSizeRegex, _ := regexp.Compile(`^(\d)+Gi$`)

	if worker.Volume != nil {
		if !volumeSizeRegex.MatchString(worker.Volume.VolumeSize) {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("volume", "size"), worker.Volume.VolumeSize, fmt.Sprintf("volume size must match the regex %s", volumeSizeRegex)))
		}
	}

	if worker.DataVolumes != nil {
		volumeNames := make(map[string]int)
		if len(worker.DataVolumes) > 0 && worker.Volume == nil {
			allErrs = append(allErrs, field.Required(fldPath.Child("volume"), "a worker volume must be defined if data volumes are defined"))
		}
		for idx, volume := range worker.DataVolumes {
			idxPath := fldPath.Child("dataVolumes").Index(idx)
			if len(volume.Name) == 0 {
				allErrs = append(allErrs, field.Required(idxPath.Child("name"), "must specify a name"))
			} else {
				allErrs = append(allErrs, validateDNS1123Label(volume.Name, idxPath.Child("name"))...)
			}
			if len(volume.Name) > maxVolumeNameLength {
				allErrs = append(allErrs, field.TooLong(idxPath.Child("name"), volume.Name, maxVolumeNameLength))
			}
			if _, keyExist := volumeNames[volume.Name]; keyExist {
				volumeNames[volume.Name]++
				allErrs = append(allErrs, field.Duplicate(idxPath.Child("name"), volume.Name))
			} else {
				volumeNames[volume.Name] = 1
			}
			if !volumeSizeRegex.MatchString(volume.VolumeSize) {
				allErrs = append(allErrs, field.Invalid(idxPath.Child("size"), volume.VolumeSize, fmt.Sprintf("data volume size must match the regex %s", volumeSizeRegex)))
			}
		}
	}

	if worker.KubeletDataVolumeName != nil {
		found := false
		for _, volume := range worker.DataVolumes {
			if volume.Name == *worker.KubeletDataVolumeName {
				found = true
			}
		}
		if !found {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("kubeletDataVolumeName"), worker.KubeletDataVolumeName, fmt.Sprintf("KubeletDataVolumeName refers to unrecognized data volume %s", *worker.KubeletDataVolumeName)))
		}
	}

	if worker.CRI != nil {
		allErrs = append(allErrs, ValidateCRI(worker.CRI, kubernetesVersion, fldPath.Child("cri"))...)
	}

	if worker.Machine.Architecture != nil {
		allErrs = append(allErrs, ValidateArchitecture(worker.Machine.Architecture, fldPath.Child("machine", "architecture"))...)
	}

	return allErrs
}

// PodPIDsLimitMinimum is a constant for the minimum value for the podPIDsLimit field.
const PodPIDsLimitMinimum int64 = 100

// ValidateKubeletConfig validates the KubeletConfig object.
func ValidateKubeletConfig(kubeletConfig core.KubeletConfig, version string, dockerConfigured bool, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if kubeletConfig.MaxPods != nil {
		allErrs = append(allErrs, apivalidation.ValidateNonnegativeField(int64(*kubeletConfig.MaxPods), fldPath.Child("maxPods"))...)
	}
	if value := kubeletConfig.PodPIDsLimit; value != nil {
		if *value < PodPIDsLimitMinimum {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("podPIDsLimit"), *value, fmt.Sprintf("podPIDsLimit value must be at least %d", PodPIDsLimitMinimum)))
		}
	}
	if kubeletConfig.ImagePullProgressDeadline != nil {
		if !dockerConfigured {
			allErrs = append(allErrs, field.Forbidden(fldPath.Child("imagePullProgressDeadline"), "can only be configured when a worker pool is configured with 'docker'. This setting has no effect for other container runtimes."))
		}
		allErrs = append(allErrs, ValidatePositiveDuration(kubeletConfig.ImagePullProgressDeadline, fldPath.Child("imagePullProgressDeadline"))...)
	}
	if kubeletConfig.EnforceNodeAllocatable != nil {
		allErrs = append(allErrs, validateKubeletConfigEnforceNodeAllocatable(kubeletConfig.EnforceNodeAllocatable, fldPath.Child("enforceNodeAllocatable"))...)
	}
	if kubeletConfig.EvictionPressureTransitionPeriod != nil {
		allErrs = append(allErrs, ValidatePositiveDuration(kubeletConfig.EvictionPressureTransitionPeriod, fldPath.Child("evictionPressureTransitionPeriod"))...)
	}
	if kubeletConfig.EvictionMaxPodGracePeriod != nil {
		allErrs = append(allErrs, apivalidation.ValidateNonnegativeField(int64(*kubeletConfig.EvictionMaxPodGracePeriod), fldPath.Child("evictionMaxPodGracePeriod"))...)
	}
	if kubeletConfig.EvictionHard != nil {
		allErrs = append(allErrs, validateKubeletConfigEviction(kubeletConfig.EvictionHard, fldPath.Child("evictionHard"))...)
	}
	if kubeletConfig.EvictionSoft != nil {
		allErrs = append(allErrs, validateKubeletConfigEviction(kubeletConfig.EvictionSoft, fldPath.Child("evictionSoft"))...)
	}
	if kubeletConfig.EvictionMinimumReclaim != nil {
		allErrs = append(allErrs, validateKubeletConfigEvictionMinimumReclaim(kubeletConfig.EvictionMinimumReclaim, fldPath.Child("evictionMinimumReclaim"))...)
	}
	if kubeletConfig.EvictionSoftGracePeriod != nil {
		allErrs = append(allErrs, validateKubeletConfigEvictionSoftGracePeriod(kubeletConfig.EvictionSoftGracePeriod, fldPath.Child("evictionSoftGracePeriod"))...)
	}
	if kubeletConfig.KubeReserved != nil {
		allErrs = append(allErrs, validateKubeletConfigReserved(kubeletConfig.KubeReserved, fldPath.Child("kubeReserved"))...)
	}
	if kubeletConfig.SystemReserved != nil {
		allErrs = append(allErrs, validateKubeletConfigReserved(kubeletConfig.SystemReserved, fldPath.Child("systemReserved"))...)
	}
	if v := kubeletConfig.ImageGCHighThresholdPercent; v != nil && (*v < 0 || *v > 100) {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("imageGCHighThresholdPercent"), *v, "value must be in [0,100]"))
	}
	if v := kubeletConfig.ImageGCLowThresholdPercent; v != nil && (*v < 0 || *v > 100) {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("imageGCLowThresholdPercent"), *v, "value must be in [0,100]"))
	}
	if kubeletConfig.ImageGCHighThresholdPercent != nil && kubeletConfig.ImageGCLowThresholdPercent != nil && *kubeletConfig.ImageGCLowThresholdPercent >= *kubeletConfig.ImageGCHighThresholdPercent {
		allErrs = append(allErrs, field.Forbidden(fldPath.Child("imageGCLowThresholdPercent"), "imageGCLowThresholdPercent must be less than imageGCHighThresholdPercent"))
	}
	allErrs = append(allErrs, featuresvalidation.ValidateFeatureGates(kubeletConfig.FeatureGates, version, fldPath.Child("featureGates"))...)
	if v := kubeletConfig.RegistryPullQPS; v != nil {
		allErrs = append(allErrs, apivalidation.ValidateNonnegativeField(int64(*v), fldPath.Child("registryPullQPS"))...)
	}
	if v := kubeletConfig.RegistryBurst; v != nil {
		allErrs = append(allErrs, apivalidation.ValidateNonnegativeField(int64(*v), fldPath.Child("registryBurst"))...)
	}
	if v := kubeletConfig.SeccompDefault; v != nil {
		if k8sVersionIsLessThan125, _ := versionutils.CompareVersions(version, "<", "1.25"); k8sVersionIsLessThan125 {
			allErrs = append(allErrs, field.Forbidden(fldPath.Child("seccompDefault"), "seccomp defaulting is not available for kubernetes versions < 1.25"))
		}
		if featureGateEnabled, ok := kubeletConfig.FeatureGates["SeccompDefault"]; ok && !featureGateEnabled && *v {
			allErrs = append(allErrs, field.Forbidden(fldPath.Child("seccompDefault"), "seccomp defaulting is not available when kubelet's 'SeccompDefault' feature gate is disabled"))
		}
	}
	if v := kubeletConfig.ContainerLogMaxSize; v != nil && dockerConfigured {
		allErrs = append(allErrs, field.Forbidden(fldPath.Child("containerLogMaxSize"), "can only be configured with containerd runtime. This setting has no effect for docker container runtime."))
	}
	if v := kubeletConfig.ContainerLogMaxFiles; v != nil {
		if dockerConfigured {
			allErrs = append(allErrs, field.Forbidden(fldPath.Child("containerLogMaxFiles"), "can only be configured with containerd runtime. This setting has no effect for docker container runtime."))
		}
		if *v < 2 {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("containerLogMaxFiles"), *v, "value must be >= 2."))
		}
	}
	if v := kubeletConfig.StreamingConnectionIdleTimeout; v != nil {
		if v.Duration < time.Second*30 || time.Hour*4 < v.Duration {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("streamingConnectionIdleTimeout"), *v, "value must be between 30s and 4h"))
		}
	}

	if v := kubeletConfig.MemorySwap; v != nil {
		if k8sVersionIsLessThan122, _ := versionutils.CompareVersions(version, "<", "1.22"); k8sVersionIsLessThan122 {
			allErrs = append(allErrs, field.Forbidden(fldPath.Child("memorySwap"), "configuring swap behaviour is not available for kubernetes versions < 1.22"))
		}

		if pointer.BoolDeref(kubeletConfig.FailSwapOn, false) {
			allErrs = append(allErrs, field.Forbidden(fldPath.Child("memorySwap"), "configuring swap behaviour is not available when the kubelet is configured with 'FailSwapOn=true'"))
		}

		if featureGateEnabled, ok := kubeletConfig.FeatureGates["NodeSwap"]; !ok || (!featureGateEnabled && v.SwapBehavior != nil) {
			allErrs = append(allErrs, field.Forbidden(fldPath.Child("memorySwap"), "configuring swap behaviour is not available when kubelet's 'NodeSwap' feature gate is not set"))
		}
	}

	return allErrs
}

const enforceNodeAllocatableNone = "none"

var validEnforceNodeAllocatable = sets.NewString(enforceNodeAllocatableNone, "pods", "system-reserved", "kube-reserved")

func validateKubeletConfigEnforceNodeAllocatable(enforceNodeAllocatable []string, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if sets.NewString(enforceNodeAllocatable...).Has(enforceNodeAllocatableNone) && len(enforceNodeAllocatable) > 1 {
		return append(allErrs, field.Invalid(fldPath, enforceNodeAllocatable, "If none is specified, no additional options must be set"))
	}

	hasSeen := sets.NewString()
	for i, entry := range enforceNodeAllocatable {
		if hasSeen.Has(entry) {
			allErrs = append(allErrs, field.Duplicate(fldPath.Index(i), entry))
		}

		hasSeen.Insert(entry)

		if !validEnforceNodeAllocatable.Has(entry) {
			allErrs = append(allErrs, field.NotSupported(fldPath.Index(i), entry, validEnforceNodeAllocatable.List()))
		}
	}

	return allErrs
}

func validateKubeletConfigEviction(eviction *core.KubeletConfigEviction, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, ValidateResourceQuantityOrPercent(eviction.MemoryAvailable, fldPath, "memoryAvailable")...)
	allErrs = append(allErrs, ValidateResourceQuantityOrPercent(eviction.ImageFSAvailable, fldPath, "imagefsAvailable")...)
	allErrs = append(allErrs, ValidateResourceQuantityOrPercent(eviction.ImageFSInodesFree, fldPath, "imagefsInodesFree")...)
	allErrs = append(allErrs, ValidateResourceQuantityOrPercent(eviction.NodeFSAvailable, fldPath, "nodefsAvailable")...)
	allErrs = append(allErrs, ValidateResourceQuantityOrPercent(eviction.ImageFSInodesFree, fldPath, "imagefsInodesFree")...)
	return allErrs
}

func validateKubeletConfigEvictionMinimumReclaim(eviction *core.KubeletConfigEvictionMinimumReclaim, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if eviction.MemoryAvailable != nil {
		allErrs = append(allErrs, ValidateResourceQuantityValue("memoryAvailable", *eviction.MemoryAvailable, fldPath.Child("memoryAvailable"))...)
	}
	if eviction.ImageFSAvailable != nil {
		allErrs = append(allErrs, ValidateResourceQuantityValue("imagefsAvailable", *eviction.ImageFSAvailable, fldPath.Child("imagefsAvailable"))...)
	}
	if eviction.ImageFSInodesFree != nil {
		allErrs = append(allErrs, ValidateResourceQuantityValue("imagefsInodesFree", *eviction.ImageFSInodesFree, fldPath.Child("imagefsInodesFree"))...)
	}
	if eviction.NodeFSAvailable != nil {
		allErrs = append(allErrs, ValidateResourceQuantityValue("nodefsAvailable", *eviction.NodeFSAvailable, fldPath.Child("nodefsAvailable"))...)
	}
	if eviction.ImageFSInodesFree != nil {
		allErrs = append(allErrs, ValidateResourceQuantityValue("imagefsInodesFree", *eviction.ImageFSInodesFree, fldPath.Child("imagefsInodesFree"))...)
	}
	return allErrs
}

func validateKubeletConfigEvictionSoftGracePeriod(eviction *core.KubeletConfigEvictionSoftGracePeriod, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, ValidatePositiveDuration(eviction.MemoryAvailable, fldPath.Child("memoryAvailable"))...)
	allErrs = append(allErrs, ValidatePositiveDuration(eviction.ImageFSAvailable, fldPath.Child("imagefsAvailable"))...)
	allErrs = append(allErrs, ValidatePositiveDuration(eviction.ImageFSInodesFree, fldPath.Child("imagefsInodesFree"))...)
	allErrs = append(allErrs, ValidatePositiveDuration(eviction.NodeFSAvailable, fldPath.Child("nodefsAvailable"))...)
	allErrs = append(allErrs, ValidatePositiveDuration(eviction.ImageFSInodesFree, fldPath.Child("imagefsInodesFree"))...)
	return allErrs
}

func validateKubeletConfigReserved(reserved *core.KubeletConfigReserved, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if reserved.CPU != nil {
		allErrs = append(allErrs, ValidateResourceQuantityValue("cpu", *reserved.CPU, fldPath.Child("cpu"))...)
	}
	if reserved.Memory != nil {
		allErrs = append(allErrs, ValidateResourceQuantityValue("memory", *reserved.Memory, fldPath.Child("memory"))...)
	}
	if reserved.EphemeralStorage != nil {
		allErrs = append(allErrs, ValidateResourceQuantityValue("ephemeralStorage", *reserved.EphemeralStorage, fldPath.Child("ephemeralStorage"))...)
	}
	if reserved.PID != nil {
		allErrs = append(allErrs, ValidateResourceQuantityValue("pid", *reserved.PID, fldPath.Child("pid"))...)
	}
	return allErrs
}

var reservedTaintKeys = sets.New(v1beta1constants.TaintNodeCriticalComponentsNotReady)

func validateClusterAutoscalerIgnoreTaints(ignoredTaints []string, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	taintKeySet := make(map[string]struct{})

	for i, taint := range ignoredTaints {
		idxPath := fldPath.Index(i)

		// validate the taint key
		allErrs = append(allErrs, metav1validation.ValidateLabelName(taint, idxPath)...)

		// deny reserved taint keys
		if reservedTaintKeys.Has(taint) {
			allErrs = append(allErrs, field.Forbidden(idxPath, "taint key is reserved by gardener"))
		}

		// validate if taint key is duplicate
		if _, ok := taintKeySet[taint]; ok {
			allErrs = append(allErrs, field.Duplicate(idxPath, taint))
			continue
		}
		taintKeySet[taint] = struct{}{}
	}
	return allErrs
}

// https://github.com/kubernetes/kubernetes/blob/ee9079f8ec39914ff8975b5390749771b9303ea4/pkg/apis/core/validation/validation.go#L4057-L4089
func validateTaints(taints []corev1.Taint, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	uniqueTaints := map[corev1.TaintEffect]sets.Set[string]{}

	for i, taint := range taints {
		idxPath := fldPath.Index(i)
		// validate the taint key
		allErrs = append(allErrs, metav1validation.ValidateLabelName(taint.Key, idxPath.Child("key"))...)

		// deny reserved taint keys
		if reservedTaintKeys.Has(taint.Key) {
			allErrs = append(allErrs, field.Forbidden(idxPath.Child("key"), "taint key is reserved by gardener"))
		}

		// validate the taint value
		if errs := validation.IsValidLabelValue(taint.Value); len(errs) != 0 {
			allErrs = append(allErrs, field.Invalid(idxPath.Child("value"), taint.Value, strings.Join(errs, ";")))
		}
		// validate the taint effect
		allErrs = append(allErrs, validateTaintEffect(&taint.Effect, false, idxPath.Child("effect"))...)

		// validate if taint is unique by <key, effect>
		if len(uniqueTaints[taint.Effect]) > 0 && uniqueTaints[taint.Effect].Has(taint.Key) {
			duplicatedError := field.Duplicate(idxPath, taint)
			duplicatedError.Detail = "taints must be unique by key and effect pair"
			allErrs = append(allErrs, duplicatedError)
			continue
		}

		// add taint to existingTaints for uniqueness check
		if len(uniqueTaints[taint.Effect]) == 0 {
			uniqueTaints[taint.Effect] = sets.Set[string]{}
		}
		uniqueTaints[taint.Effect].Insert(taint.Key)
	}
	return allErrs
}

// https://github.com/kubernetes/kubernetes/blob/ee9079f8ec39914ff8975b5390749771b9303ea4/pkg/apis/core/validation/validation.go#L2774-L2795
func validateTaintEffect(effect *corev1.TaintEffect, allowEmpty bool, fldPath *field.Path) field.ErrorList {
	if !allowEmpty && len(*effect) == 0 {
		return field.ErrorList{field.Required(fldPath, "")}
	}

	allErrors := field.ErrorList{}
	switch *effect {
	case corev1.TaintEffectNoSchedule, corev1.TaintEffectPreferNoSchedule, corev1.TaintEffectNoExecute:
	default:
		validValues := []string{
			string(corev1.TaintEffectNoSchedule),
			string(corev1.TaintEffectPreferNoSchedule),
			string(corev1.TaintEffectNoExecute),
		}
		allErrors = append(allErrors, field.NotSupported(fldPath, *effect, validValues))
	}
	return allErrors
}

// ValidateWorkers validates worker objects.
func ValidateWorkers(workers []core.Worker, fldPath *field.Path) field.ErrorList {
	var (
		allErrs     = field.ErrorList{}
		workerNames = sets.New[string]()
	)

	for i, worker := range workers {
		if workerNames.Has(worker.Name) {
			allErrs = append(allErrs, field.Duplicate(fldPath.Index(i).Child("name"), worker.Name))
		}
		workerNames.Insert(worker.Name)
	}

	return allErrs
}

// ValidateSystemComponentWorkers validates workers specified to run system components.
func ValidateSystemComponentWorkers(workers []core.Worker, kubernetesVersion string, fldPath *field.Path) field.ErrorList {
	var (
		allErrs                                   = field.ErrorList{}
		atLeastOnePoolWithAllowedSystemComponents = false

		workerPoolsWithSufficientWorkers   = make(map[string]struct{})
		workerPoolsWithInsufficientWorkers = make(map[string]int)
	)

	for i, worker := range workers {
		// check if system component worker pool is configured
		if !helper.SystemComponentsAllowed(&worker) {
			continue
		}

		if worker.Minimum == 0 || worker.Maximum == 0 {
			continue
		}
		atLeastOnePoolWithAllowedSystemComponents = true

		// Check if the maximum worker count is greater than or equal to the number of specified zones.
		// It ensures that the cluster has at least one worker per zone in order to schedule required system components with TopologySpreadConstraints.
		// This check is done per distinct worker pool concerning their zone setup,
		// e.g. 'worker[x].zones: {1,2,3}' is the same as 'worker[y].zones: {3,2,1}', so the constraint is only considered once for both worker groups.
		zonesSet := sets.New(worker.Zones...)

		var (
			hasSufficientWorkers = false
			workerPoolKey        = strings.Join(sets.List(zonesSet), "--")
		)

		if int(worker.Maximum) >= len(worker.Zones) {
			hasSufficientWorkers = true
		}

		if hasSufficientWorkers {
			workerPoolsWithSufficientWorkers[workerPoolKey] = struct{}{}
			delete(workerPoolsWithInsufficientWorkers, workerPoolKey)
		} else {
			if _, b := workerPoolsWithSufficientWorkers[workerPoolKey]; !b {
				workerPoolsWithInsufficientWorkers[workerPoolKey] = i
			}
		}
	}

	// TODO(timuthy): Remove this check as soon as v1.27 is the least supported Kubernetes version in Gardener.
	k8sGreaterEqual127, _ := versionutils.CheckVersionMeetsConstraint(kubernetesVersion, ">= 1.27")
	if k8sGreaterEqual127 {
		for _, i := range workerPoolsWithInsufficientWorkers {
			allErrs = append(allErrs, field.Forbidden(fldPath.Index(i).Child("maximum"), "maximum node count should be greater than or equal to the number of zones specified for this pool"))
		}
	}

	if !atLeastOnePoolWithAllowedSystemComponents {
		allErrs = append(allErrs, field.Forbidden(fldPath, "at least one active worker pool with allowSystemComponents=true needed"))
	}

	return allErrs
}

// ValidateHibernation validates a Hibernation object.
func ValidateHibernation(annotations map[string]string, hibernation *core.Hibernation, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if hibernation == nil {
		return allErrs
	}

	if maintenanceOp := annotations[v1beta1constants.GardenerMaintenanceOperation]; forbiddenShootOperationsWhenHibernated.Has(maintenanceOp) && pointer.BoolDeref(hibernation.Enabled, false) {
		allErrs = append(allErrs, field.Forbidden(fldPath.Child("enabled"), fmt.Sprintf("shoot cannot be hibernated when %s=%s annotation is set", v1beta1constants.GardenerMaintenanceOperation, maintenanceOp)))
	}

	allErrs = append(allErrs, ValidateHibernationSchedules(hibernation.Schedules, fldPath.Child("schedules"))...)

	return allErrs
}

// ValidateHibernationSchedules validates a list of hibernation schedules.
func ValidateHibernationSchedules(schedules []core.HibernationSchedule, fldPath *field.Path) field.ErrorList {
	var (
		allErrs = field.ErrorList{}
		seen    = sets.New[string]()
	)

	for i, schedule := range schedules {
		allErrs = append(allErrs, ValidateHibernationSchedule(seen, &schedule, fldPath.Index(i))...)
	}

	return allErrs
}

// ValidateHibernationCronSpec validates a cron specification of a hibernation schedule.
func ValidateHibernationCronSpec(seenSpecs sets.Set[string], spec string, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	_, err := cron.ParseStandard(spec)
	switch {
	case err != nil:
		allErrs = append(allErrs, field.Invalid(fldPath, spec, fmt.Sprintf("not a valid cron spec: %v", err)))
	case seenSpecs.Has(spec):
		allErrs = append(allErrs, field.Duplicate(fldPath, spec))
	default:
		seenSpecs.Insert(spec)
	}

	return allErrs
}

// ValidateHibernationScheduleLocation validates that the location of a HibernationSchedule is correct.
func ValidateHibernationScheduleLocation(location string, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if _, err := time.LoadLocation(location); err != nil {
		allErrs = append(allErrs, field.Invalid(fldPath, location, fmt.Sprintf("not a valid location: %v", err)))
	}

	return allErrs
}

// ValidateHibernationSchedule validates the correctness of a HibernationSchedule.
// It checks whether the set start and end time are valid cron specs.
func ValidateHibernationSchedule(seenSpecs sets.Set[string], schedule *core.HibernationSchedule, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if schedule.Start == nil && schedule.End == nil {
		allErrs = append(allErrs, field.Required(fldPath.Child("start/end"), "either start or end has to be provided"))
	}
	if schedule.Start != nil {
		allErrs = append(allErrs, ValidateHibernationCronSpec(seenSpecs, *schedule.Start, fldPath.Child("start"))...)
	}
	if schedule.End != nil {
		allErrs = append(allErrs, ValidateHibernationCronSpec(seenSpecs, *schedule.End, fldPath.Child("end"))...)
	}
	if schedule.Location != nil {
		allErrs = append(allErrs, ValidateHibernationScheduleLocation(*schedule.Location, fldPath.Child("location"))...)
	}

	return allErrs
}

// ValidatePositiveDuration validates that a duration is positive.
func ValidatePositiveDuration(duration *metav1.Duration, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if duration == nil {
		return allErrs
	}
	if duration.Seconds() < 0 {
		allErrs = append(allErrs, field.Invalid(fldPath, duration.Duration.String(), "must be non-negative"))
	}
	return allErrs
}

// ValidateResourceQuantityOrPercent checks if a value can be parsed to either a resource.quantity, a positive int or percent.
func ValidateResourceQuantityOrPercent(valuePtr *string, fldPath *field.Path, key string) field.ErrorList {
	allErrs := field.ErrorList{}

	if valuePtr == nil {
		return allErrs
	}
	value := *valuePtr
	// check for resource quantity
	if quantity, err := resource.ParseQuantity(value); err == nil {
		if len(ValidateResourceQuantityValue(key, quantity, fldPath)) == 0 {
			return allErrs
		}
	}

	if validation.IsValidPercent(value) != nil {
		allErrs = append(allErrs, field.Invalid(fldPath.Child(key), value, "field must be either a valid resource quantity (e.g 200Mi) or a percentage (e.g '5%')"))
		return allErrs
	}

	percentValue, _ := strconv.Atoi(value[:len(value)-1])
	if percentValue > 100 || percentValue < 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child(key), value, "must not be greater than 100% and not smaller than 0%"))
	}
	return allErrs
}

// ValidatePositiveIntOrPercent validates a int or string object and ensures it is positive.
func ValidatePositiveIntOrPercent(intOrPercent *intstr.IntOrString, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if intOrPercent == nil {
		return allErrs
	}

	if intOrPercent.Type == intstr.String {
		if validation.IsValidPercent(intOrPercent.StrVal) != nil {
			allErrs = append(allErrs, field.Invalid(fldPath, intOrPercent, "must be an integer or percentage (e.g '5%')"))
		}
	} else if intOrPercent.Type == intstr.Int {
		allErrs = append(allErrs, apivalidation.ValidateNonnegativeField(int64(intOrPercent.IntValue()), fldPath)...)
	}

	return allErrs
}

// IsNotMoreThan100Percent validates an int or string object and ensures it is not more than 100%.
func IsNotMoreThan100Percent(intOrStringValue *intstr.IntOrString, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if intOrStringValue == nil {
		return allErrs
	}

	value, isPercent := getPercentValue(*intOrStringValue)
	if !isPercent || value <= 100 {
		return nil
	}
	allErrs = append(allErrs, field.Invalid(fldPath, intOrStringValue, "must not be greater than 100%"))

	return allErrs
}

// ValidateCRI validates container runtime interface name and its container runtimes
func ValidateCRI(CRI *core.CRI, kubernetesVersion string, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	k8sVersionIs123OrGreater, _ := versionutils.CompareVersions(kubernetesVersion, ">=", "1.23")

	if k8sVersionIs123OrGreater && CRI.Name == core.CRINameDocker {
		allErrs = append(allErrs, field.Forbidden(fldPath.Child("name"), "'docker' is only allowed for kubernetes versions < 1.23"))
	}

	if !availableWorkerCRINames.Has(string(CRI.Name)) {
		allErrs = append(allErrs, field.NotSupported(fldPath.Child("name"), CRI.Name, sets.List(availableWorkerCRINames)))
	}

	if CRI.ContainerRuntimes != nil {
		allErrs = append(allErrs, ValidateContainerRuntimes(CRI.ContainerRuntimes, fldPath.Child("containerruntimes"))...)
	}

	return allErrs
}

// ValidateContainerRuntimes validates the given container runtimes
func ValidateContainerRuntimes(containerRuntime []core.ContainerRuntime, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	crSet := make(map[string]bool)

	for i, cr := range containerRuntime {
		if len(cr.Type) == 0 {
			allErrs = append(allErrs, field.Required(fldPath.Index(i).Child("type"), "must specify a container runtime type"))
		}
		if crSet[cr.Type] {
			allErrs = append(allErrs, field.Duplicate(fldPath.Index(i).Child("type"), fmt.Sprintf("must specify different type, %s already exist", cr.Type)))
		}
		crSet[cr.Type] = true
	}

	return allErrs
}

// ValidateArchitecture validates the CPU architecure of the machines in this worker pool.
func ValidateArchitecture(arch *string, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if !slices.Contains(v1beta1constants.ValidArchitectures, *arch) {
		allErrs = append(allErrs, field.NotSupported(fldPath, *arch, v1beta1constants.ValidArchitectures))
	}

	return allErrs
}

// ValidateSystemComponents validates the given system components.
func ValidateSystemComponents(systemComponents *core.SystemComponents, fldPath *field.Path, workerless bool) field.ErrorList {
	allErrs := field.ErrorList{}

	if systemComponents == nil {
		return allErrs
	} else if workerless {
		allErrs = append(allErrs, field.Forbidden(fldPath, workerlessErrorMsg))
		return allErrs
	}

	allErrs = append(allErrs, validateCoreDNS(systemComponents.CoreDNS, fldPath.Child("coreDNS"))...)

	return allErrs
}

// validateCoreDNS validates the given Core DNS settings.
func validateCoreDNS(coreDNS *core.CoreDNS, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if coreDNS == nil {
		return allErrs
	}

	if coreDNS.Autoscaling != nil && !availableCoreDNSAutoscalingModes.Has(string(coreDNS.Autoscaling.Mode)) {
		allErrs = append(allErrs, field.NotSupported(fldPath.Child("autoscaling").Child("mode"), coreDNS.Autoscaling.Mode, sets.List(availableCoreDNSAutoscalingModes)))
	}
	if coreDNS.Rewriting != nil {
		allErrs = append(allErrs, ValidateCoreDNSRewritingCommonSuffixes(coreDNS.Rewriting.CommonSuffixes, fldPath.Child("rewriting"))...)
	}

	return allErrs
}

// ValidateCoreDNSRewritingCommonSuffixes validates the given common suffixes used for DNS rewriting.
func ValidateCoreDNSRewritingCommonSuffixes(commonSuffixes []string, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if len(commonSuffixes) == 0 {
		return allErrs
	}

	suffixes := map[string]struct{}{}
	for i, s := range commonSuffixes {
		if strings.Count(s, ".") < 1 || (s[0] == '.' && strings.Count(s, ".") < 2) {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("commonSuffixes").Index(i), s, "must contain at least one non-leading dot ('.')"))
		}
		s = strings.TrimPrefix(s, ".")
		if _, found := suffixes[s]; found {
			allErrs = append(allErrs, field.Duplicate(fldPath.Child("commonSuffixes").Index(i), s))
		} else {
			suffixes[s] = struct{}{}
		}
	}

	return allErrs
}

func validateShootOperation(operation, maintenanceOperation string, shoot *core.Shoot, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if operation == "" && maintenanceOperation == "" {
		return allErrs
	}

	fldPathOp := fldPath.Key(v1beta1constants.GardenerOperation)
	fldPathMaintOp := fldPath.Key(v1beta1constants.GardenerMaintenanceOperation)

	if operation == maintenanceOperation {
		allErrs = append(allErrs, field.Forbidden(fldPath, fmt.Sprintf("annotations %s and %s must not be equal", fldPathOp, fldPathMaintOp)))
	}

	if operation != "" {
		if !availableShootOperations.Has(operation) {
			allErrs = append(allErrs, field.NotSupported(fldPathOp, operation, sets.List(availableShootOperations)))
		}
		if helper.HibernationIsEnabled(shoot) && forbiddenShootOperationsWhenHibernated.Has(operation) {
			allErrs = append(allErrs, field.Forbidden(fldPathOp, "operation is not permitted when shoot is hibernated"))
		}
	}

	if maintenanceOperation != "" {
		if !availableShootMaintenanceOperations.Has(maintenanceOperation) {
			allErrs = append(allErrs, field.NotSupported(fldPathMaintOp, maintenanceOperation, sets.List(availableShootMaintenanceOperations)))
		}
		if helper.HibernationIsEnabled(shoot) && forbiddenShootOperationsWhenHibernated.Has(maintenanceOperation) {
			allErrs = append(allErrs, field.Forbidden(fldPathMaintOp, "operation is not permitted when shoot is hibernated"))
		}
	}

	allErrs = append(allErrs, validateShootOperationContext(operation, shoot, fldPathOp)...)
	allErrs = append(allErrs, validateShootOperationContext(maintenanceOperation, shoot, fldPathMaintOp)...)

	return allErrs
}

func validateShootOperationContext(operation string, shoot *core.Shoot, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	switch operation {
	case v1beta1constants.OperationRotateCredentialsStart:
		if !isShootReadyForRotationStart(shoot.Status.LastOperation) {
			allErrs = append(allErrs, field.Forbidden(fldPath, "cannot start rotation of all credentials if shoot was not yet created successfully or is not ready for reconciliation"))
		}
		if phase := helper.GetShootCARotationPhase(shoot.Status.Credentials); len(phase) > 0 && phase != core.RotationCompleted {
			allErrs = append(allErrs, field.Forbidden(fldPath, "cannot start rotation of all credentials if .status.credentials.rotation.certificateAuthorities.phase is not 'Completed'"))
		}
		if phase := helper.GetShootServiceAccountKeyRotationPhase(shoot.Status.Credentials); len(phase) > 0 && phase != core.RotationCompleted {
			allErrs = append(allErrs, field.Forbidden(fldPath, "cannot start rotation of all credentials if .status.credentials.rotation.serviceAccountKey.phase is not 'Completed'"))
		}
		if phase := helper.GetShootETCDEncryptionKeyRotationPhase(shoot.Status.Credentials); len(phase) > 0 && phase != core.RotationCompleted {
			allErrs = append(allErrs, field.Forbidden(fldPath, "cannot start rotation of all credentials if .status.credentials.rotation.etcdEncryptionKey.phase is not 'Completed'"))
		}
	case v1beta1constants.OperationRotateCredentialsComplete:
		if helper.GetShootCARotationPhase(shoot.Status.Credentials) != core.RotationPrepared {
			allErrs = append(allErrs, field.Forbidden(fldPath, "cannot complete rotation of all credentials if .status.credentials.rotation.certificateAuthorities.phase is not 'Prepared'"))
		}
		if helper.GetShootServiceAccountKeyRotationPhase(shoot.Status.Credentials) != core.RotationPrepared {
			allErrs = append(allErrs, field.Forbidden(fldPath, "cannot complete rotation of all credentials if .status.credentials.rotation.serviceAccountKey.phase is not 'Prepared'"))
		}
		if helper.GetShootETCDEncryptionKeyRotationPhase(shoot.Status.Credentials) != core.RotationPrepared {
			allErrs = append(allErrs, field.Forbidden(fldPath, "cannot complete rotation of all credentials if .status.credentials.rotation.etcdEncryptionKey.phase is not 'Prepared'"))
		}

	case v1beta1constants.OperationRotateCAStart:
		if !isShootReadyForRotationStart(shoot.Status.LastOperation) {
			allErrs = append(allErrs, field.Forbidden(fldPath, "cannot start CA rotation if shoot was not yet created successfully or is not ready for reconciliation"))
		}
		if phase := helper.GetShootCARotationPhase(shoot.Status.Credentials); len(phase) > 0 && phase != core.RotationCompleted {
			allErrs = append(allErrs, field.Forbidden(fldPath, "cannot start CA rotation if .status.credentials.rotation.certificateAuthorities.phase is not 'Completed'"))
		}
	case v1beta1constants.OperationRotateCAComplete:
		if helper.GetShootCARotationPhase(shoot.Status.Credentials) != core.RotationPrepared {
			allErrs = append(allErrs, field.Forbidden(fldPath, "cannot complete CA rotation if .status.credentials.rotation.certificateAuthorities.phase is not 'Prepared'"))
		}

	case v1beta1constants.OperationRotateServiceAccountKeyStart:
		if !isShootReadyForRotationStart(shoot.Status.LastOperation) {
			allErrs = append(allErrs, field.Forbidden(fldPath, "cannot start service account key rotation if shoot was not yet created successfully or is not ready for reconciliation"))
		}
		if phase := helper.GetShootServiceAccountKeyRotationPhase(shoot.Status.Credentials); len(phase) > 0 && phase != core.RotationCompleted {
			allErrs = append(allErrs, field.Forbidden(fldPath, "cannot start service account key rotation if .status.credentials.rotation.serviceAccountKey.phase is not 'Completed'"))
		}
	case v1beta1constants.OperationRotateServiceAccountKeyComplete:
		if helper.GetShootServiceAccountKeyRotationPhase(shoot.Status.Credentials) != core.RotationPrepared {
			allErrs = append(allErrs, field.Forbidden(fldPath, "cannot complete service account key rotation if .status.credentials.rotation.serviceAccountKey.phase is not 'Prepared'"))
		}

	case v1beta1constants.OperationRotateETCDEncryptionKeyStart:
		if !isShootReadyForRotationStart(shoot.Status.LastOperation) {
			allErrs = append(allErrs, field.Forbidden(fldPath, "cannot start ETCD encryption key rotation if shoot was not yet created successfully or is not ready for reconciliation"))
		}
		if phase := helper.GetShootETCDEncryptionKeyRotationPhase(shoot.Status.Credentials); len(phase) > 0 && phase != core.RotationCompleted {
			allErrs = append(allErrs, field.Forbidden(fldPath, "cannot start ETCD encryption key rotation if .status.credentials.rotation.etcdEncryptionKey.phase is not 'Completed'"))
		}
	case v1beta1constants.OperationRotateETCDEncryptionKeyComplete:
		if helper.GetShootETCDEncryptionKeyRotationPhase(shoot.Status.Credentials) != core.RotationPrepared {
			allErrs = append(allErrs, field.Forbidden(fldPath, "cannot complete ETCD encryption key rotation if .status.credentials.rotation.etcdEncryptionKey.phase is not 'Prepared'"))
		}
	}
	return allErrs
}

// ValidateShootHAConfig enforces that both annotation and HA spec are not set together.
func ValidateShootHAConfig(shoot *core.Shoot) field.ErrorList {
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, validateHAShootControlPlaneConfigurationValue(shoot)...)
	return allErrs
}

// ValidateShootHAConfigUpdate validates the HA shoot control plane configuration.
func ValidateShootHAConfigUpdate(newShoot, oldShoot *core.Shoot) field.ErrorList {
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, validateShootHAControlPlaneSpecUpdate(newShoot, oldShoot, field.NewPath("spec.controlPlane"))...)
	return allErrs
}

func validateHAShootControlPlaneConfigurationValue(shoot *core.Shoot) field.ErrorList {
	allErrs := field.ErrorList{}
	if shoot.Spec.ControlPlane != nil && shoot.Spec.ControlPlane.HighAvailability != nil {
		allErrs = append(allErrs, ValidateFailureToleranceTypeValue(shoot.Spec.ControlPlane.HighAvailability.FailureTolerance.Type, field.NewPath("spec", "controlPlane", "highAvailability", "failureTolerance", "type"))...)
	}
	return allErrs
}

func validateShootHAControlPlaneSpecUpdate(newShoot, oldShoot *core.Shoot, fldPath *field.Path) field.ErrorList {
	var (
		allErrs          = field.ErrorList{}
		shootIsScheduled = newShoot.Spec.SeedName != nil

		oldVal, newVal core.FailureToleranceType
		oldValExists   bool
	)

	if oldShoot.Spec.ControlPlane != nil && oldShoot.Spec.ControlPlane.HighAvailability != nil {
		oldVal = oldShoot.Spec.ControlPlane.HighAvailability.FailureTolerance.Type
		oldValExists = true
	}

	if newShoot.Spec.ControlPlane != nil && newShoot.Spec.ControlPlane.HighAvailability != nil {
		newVal = newShoot.Spec.ControlPlane.HighAvailability.FailureTolerance.Type
		// TODO(@aaronfern): remove this validation of not allowing scale-up to HA while hibernated when https://github.com/gardener/etcd-druid/issues/589 is resolved
		if !oldValExists && isShootInHibernation(newShoot) {
			allErrs = append(allErrs, field.Forbidden(fldPath.Child("highAvailability", "failureTolerance", "type"), "Shoot is currently hibernated and cannot be scaled up to HA. Please make sure your cluster has woken up before scaling it up to HA"))
		}
	}

	if oldValExists && shootIsScheduled {
		// If the HighAvailability field is already set for the shoot then enforce that it cannot be changed.
		allErrs = append(allErrs, apivalidation.ValidateImmutableField(newVal, oldVal, fldPath.Child("highAvailability", "failureTolerance", "type"))...)
	}

	return allErrs
}

func isShootReadyForRotationStart(lastOperation *core.LastOperation) bool {
	if lastOperation == nil {
		return false
	}
	if lastOperation.Type == core.LastOperationTypeCreate && lastOperation.State == core.LastOperationStateSucceeded {
		return true
	}
	if lastOperation.Type == core.LastOperationTypeRestore && lastOperation.State == core.LastOperationStateSucceeded {
		return true
	}
	return lastOperation.Type == core.LastOperationTypeReconcile
}

func isShootInHibernation(shoot *core.Shoot) bool {
	if shoot.Spec.Hibernation != nil && shoot.Spec.Hibernation.Enabled != nil {
		return *shoot.Spec.Hibernation.Enabled || shoot.Status.IsHibernated
	}

	return shoot.Status.IsHibernated
}
