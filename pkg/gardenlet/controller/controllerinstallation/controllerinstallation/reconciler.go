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

package controllerinstallation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/component-base/featuregate"
	podsecurityadmissionapi "k8s.io/pod-security-admission/api"
	"k8s.io/utils/clock"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	v1beta1helper "github.com/gardener/gardener/pkg/apis/core/v1beta1/helper"
	resourcesv1alpha1 "github.com/gardener/gardener/pkg/apis/resources/v1alpha1"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/controllerutils"
	"github.com/gardener/gardener/pkg/features"
	"github.com/gardener/gardener/pkg/gardenlet/apis/config"
	ctrlinstutils "github.com/gardener/gardener/pkg/gardenlet/controller/controllerinstallation/utils"
	"github.com/gardener/gardener/pkg/resourcemanager/controller/garbagecollector/references"
	"github.com/gardener/gardener/pkg/utils"
	gardenerutils "github.com/gardener/gardener/pkg/utils/gardener"
	kubernetesutils "github.com/gardener/gardener/pkg/utils/kubernetes"
	"github.com/gardener/gardener/pkg/utils/managedresources"
	secretsutils "github.com/gardener/gardener/pkg/utils/secrets"
)

const finalizerName = "core.gardener.cloud/controllerinstallation"

// RequeueDurationWhenResourceDeletionStillPresent is the duration used for requeueing when owned resources are still in
// the process of being deleted when deleting a ControllerInstallation.
var RequeueDurationWhenResourceDeletionStillPresent = 30 * time.Second

// Reconciler reconciles ControllerInstallations and deploys them into the seed cluster.
type Reconciler struct {
	GardenClient          client.Client
	GardenConfig          *rest.Config
	SeedClientSet         kubernetes.Interface
	Config                config.GardenletConfiguration
	Clock                 clock.Clock
	Identity              *gardencorev1beta1.Gardener
	GardenClusterIdentity string
}

// Reconcile reconciles ControllerInstallations and deploys them into the seed cluster.
func (r *Reconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	log := logf.FromContext(ctx)

	gardenCtx, cancel := controllerutils.GetMainReconciliationContext(ctx, controllerutils.DefaultReconciliationTimeout)
	defer cancel()
	seedCtx, cancel := controllerutils.GetChildReconciliationContext(ctx, controllerutils.DefaultReconciliationTimeout)
	defer cancel()

	controllerInstallation := &gardencorev1beta1.ControllerInstallation{}
	if err := r.GardenClient.Get(gardenCtx, request.NamespacedName, controllerInstallation); err != nil {
		if apierrors.IsNotFound(err) {
			log.V(1).Info("Object is gone, stop reconciling")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("error retrieving object from store: %w", err)
	}

	if controllerInstallation.DeletionTimestamp != nil {
		return r.delete(gardenCtx, seedCtx, log, controllerInstallation)
	}
	return r.reconcile(gardenCtx, seedCtx, log, controllerInstallation)
}

func (r *Reconciler) reconcile(
	gardenCtx context.Context,
	seedCtx context.Context,
	log logr.Logger,
	controllerInstallation *gardencorev1beta1.ControllerInstallation,
) (
	reconcile.Result,
	error,
) {
	if !controllerutil.ContainsFinalizer(controllerInstallation, finalizerName) {
		log.Info("Adding finalizer")
		if err := controllerutils.AddFinalizers(gardenCtx, r.GardenClient, controllerInstallation, finalizerName); err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to add finalizer: %w", err)
		}
	}

	var (
		conditionValid     = v1beta1helper.GetOrInitConditionWithClock(r.Clock, controllerInstallation.Status.Conditions, gardencorev1beta1.ControllerInstallationValid)
		conditionInstalled = v1beta1helper.GetOrInitConditionWithClock(r.Clock, controllerInstallation.Status.Conditions, gardencorev1beta1.ControllerInstallationInstalled)
	)

	defer func() {
		if err := patchConditions(gardenCtx, r.GardenClient, controllerInstallation, conditionValid, conditionInstalled); err != nil {
			log.Error(err, "Failed to patch conditions")
		}
	}()

	controllerRegistration := &gardencorev1beta1.ControllerRegistration{}
	if err := r.GardenClient.Get(gardenCtx, client.ObjectKey{Name: controllerInstallation.Spec.RegistrationRef.Name}, controllerRegistration); err != nil {
		if apierrors.IsNotFound(err) {
			conditionValid = v1beta1helper.UpdatedConditionWithClock(r.Clock, conditionValid, gardencorev1beta1.ConditionFalse, "RegistrationNotFound", fmt.Sprintf("Referenced ControllerRegistration does not exist: %+v", err))
		} else {
			conditionValid = v1beta1helper.UpdatedConditionWithClock(r.Clock, conditionValid, gardencorev1beta1.ConditionUnknown, "RegistrationReadError", fmt.Sprintf("Referenced ControllerRegistration cannot be read: %+v", err))
		}
		return reconcile.Result{}, err
	}

	seed := &gardencorev1beta1.Seed{}
	if err := r.GardenClient.Get(gardenCtx, client.ObjectKey{Name: controllerInstallation.Spec.SeedRef.Name}, seed); err != nil {
		if apierrors.IsNotFound(err) {
			conditionValid = v1beta1helper.UpdatedConditionWithClock(r.Clock, conditionValid, gardencorev1beta1.ConditionFalse, "SeedNotFound", fmt.Sprintf("Referenced Seed does not exist: %+v", err))
		} else {
			conditionValid = v1beta1helper.UpdatedConditionWithClock(r.Clock, conditionValid, gardencorev1beta1.ConditionUnknown, "SeedReadError", fmt.Sprintf("Referenced Seed cannot be read: %+v", err))
		}
		return reconcile.Result{}, err
	}

	var providerConfig *runtime.RawExtension
	if deploymentRef := controllerInstallation.Spec.DeploymentRef; deploymentRef != nil {
		controllerDeployment := &gardencorev1beta1.ControllerDeployment{}
		if err := r.GardenClient.Get(gardenCtx, kubernetesutils.Key(deploymentRef.Name), controllerDeployment); err != nil {
			return reconcile.Result{}, err
		}
		providerConfig = &controllerDeployment.ProviderConfig
	}

	var helmDeployment struct {
		// chart is a Helm chart tarball.
		Chart []byte `json:"chart,omitempty"`
		// Values is a map of values for the given chart.
		Values map[string]interface{} `json:"values,omitempty"`
	}

	if err := json.Unmarshal(providerConfig.Raw, &helmDeployment); err != nil {
		conditionValid = v1beta1helper.UpdatedConditionWithClock(r.Clock, conditionValid, gardencorev1beta1.ConditionFalse, "ChartInformationInvalid", fmt.Sprintf("chart Information cannot be unmarshalled: %+v", err))
		return reconcile.Result{}, err
	}

	namespace := getNamespaceForControllerInstallation(controllerInstallation)
	if _, err := controllerutils.GetAndCreateOrMergePatch(seedCtx, r.SeedClientSet.Client(), namespace, func() error {
		metav1.SetMetaDataLabel(&namespace.ObjectMeta, v1beta1constants.GardenRole, v1beta1constants.GardenRoleExtension)
		metav1.SetMetaDataLabel(&namespace.ObjectMeta, v1beta1constants.LabelControllerRegistrationName, controllerRegistration.Name)
		metav1.SetMetaDataLabel(&namespace.ObjectMeta, resourcesv1alpha1.HighAvailabilityConfigConsider, "true")
		metav1.SetMetaDataAnnotation(&namespace.ObjectMeta, resourcesv1alpha1.HighAvailabilityConfigZones, strings.Join(seed.Spec.Provider.Zones, ","))

		if podSecurityEnforce, ok := controllerInstallation.Annotations[v1beta1constants.AnnotationPodSecurityEnforce]; ok {
			metav1.SetMetaDataLabel(&namespace.ObjectMeta, podsecurityadmissionapi.EnforceLevelLabel, podSecurityEnforce)
		} else {
			delete(namespace.Labels, podsecurityadmissionapi.EnforceLevelLabel)
		}

		return nil
	}); err != nil {
		return reconcile.Result{}, err
	}

	if seed.Status.ClusterIdentity == nil {
		return reconcile.Result{}, fmt.Errorf("cluster-identity of seed '%s' not set", seed.Name)
	}

	genericGardenKubeconfigSecretName, err := r.reconcileGenericGardenKubeconfig(seedCtx, namespace.Name)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to reconcile generic garden kubeconfig: %w", err)
	}

	gardenAccessSecret, err := r.reconcileGardenAccessSecret(seedCtx, controllerInstallation.Name, namespace.Name)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to reconcile garden access secret: %w", err)
	}

	var (
		volumeProvider  string
		volumeProviders []gardencorev1beta1.SeedVolumeProvider
	)

	if seed.Spec.Volume != nil {
		volumeProviders = seed.Spec.Volume.Providers
		if len(seed.Spec.Volume.Providers) > 0 {
			volumeProvider = seed.Spec.Volume.Providers[0].Name
		}
	}

	featureToEnabled := make(map[featuregate.Feature]bool)
	for feature := range features.DefaultFeatureGate.GetAll() {
		featureToEnabled[feature] = features.DefaultFeatureGate.Enabled(feature)
	}

	// Mix-in some standard values for garden and seed.
	gardenerValues := map[string]interface{}{
		"gardener": map[string]interface{}{
			"version": r.Identity.Version,
			"garden": map[string]interface{}{
				"clusterIdentity":             r.GardenClusterIdentity,
				"genericKubeconfigSecretName": genericGardenKubeconfigSecretName,
			},
			"seed": map[string]interface{}{
				"name":            seed.Name,
				"clusterIdentity": *seed.Status.ClusterIdentity,
				"annotations":     seed.Annotations,
				"labels":          seed.Labels,
				"provider":        seed.Spec.Provider.Type,
				"region":          seed.Spec.Provider.Region,
				"volumeProvider":  volumeProvider,
				"volumeProviders": volumeProviders,
				"ingressDomain":   &seed.Spec.Ingress.Domain,
				"protected":       v1beta1helper.TaintsHave(seed.Spec.Taints, gardencorev1beta1.SeedTaintProtected),
				"visible":         seed.Spec.Settings.Scheduling.Visible,
				"taints":          seed.Spec.Taints,
				"networks":        seed.Spec.Networks,
				"blockCIDRs":      seed.Spec.Networks.BlockCIDRs,
				"spec":            seed.Spec,
			},
			"gardenlet": map[string]interface{}{
				"featureGates": featureToEnabled,
			},
		},
	}

	release, err := r.SeedClientSet.ChartRenderer().RenderArchive(helmDeployment.Chart, controllerRegistration.Name, namespace.Name, utils.MergeMaps(helmDeployment.Values, gardenerValues))
	if err != nil {
		conditionValid = v1beta1helper.UpdatedConditionWithClock(r.Clock, conditionValid, gardencorev1beta1.ConditionFalse, "ChartCannotBeRendered", fmt.Sprintf("chart rendering process failed: %+v", err))
		return reconcile.Result{}, err
	}
	conditionValid = v1beta1helper.UpdatedConditionWithClock(r.Clock, conditionValid, gardencorev1beta1.ConditionTrue, "RegistrationValid", "chart could be rendered successfully.")
	secretData := release.AsSecretData()

	if err := injectGardenAccessSecrets(secretData, namespace.Name, genericGardenKubeconfigSecretName, gardenAccessSecret.Secret.Name, seed.Name); err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to inject garden access secrets: %w", err)
	}

	if err := managedresources.Create(
		seedCtx,
		r.SeedClientSet.Client(),
		v1beta1constants.GardenNamespace,
		controllerInstallation.Name,
		map[string]string{ctrlinstutils.LabelKeyControllerInstallationName: controllerInstallation.Name},
		false,
		v1beta1constants.SeedResourceManagerClass,
		secretData,
		nil,
		nil,
		nil,
	); err != nil {
		conditionInstalled = v1beta1helper.UpdatedConditionWithClock(r.Clock, conditionInstalled, gardencorev1beta1.ConditionFalse, "InstallationFailed", fmt.Sprintf("Creation of ManagedResource %q failed: %+v", controllerInstallation.Name, err))
		return reconcile.Result{}, err
	}

	if conditionInstalled.Status == gardencorev1beta1.ConditionUnknown {
		// initially set condition to Pending
		// care controller will update condition based on 'ResourcesApplied' condition of ManagedResource
		conditionInstalled = v1beta1helper.UpdatedConditionWithClock(r.Clock, conditionInstalled, gardencorev1beta1.ConditionFalse, "InstallationPending", fmt.Sprintf("Installation of ManagedResource %q is still pending.", controllerInstallation.Name))
	}

	return reconcile.Result{}, nil
}

func (r *Reconciler) delete(
	gardenCtx context.Context,
	seedCtx context.Context,
	log logr.Logger,
	controllerInstallation *gardencorev1beta1.ControllerInstallation,
) (
	reconcile.Result,
	error,
) {
	var (
		newConditions      = v1beta1helper.MergeConditions(controllerInstallation.Status.Conditions, v1beta1helper.InitConditionWithClock(r.Clock, gardencorev1beta1.ControllerInstallationValid), v1beta1helper.InitConditionWithClock(r.Clock, gardencorev1beta1.ControllerInstallationInstalled))
		conditionValid     = newConditions[0]
		conditionInstalled = newConditions[1]
	)

	defer func() {
		if err := patchConditions(gardenCtx, r.GardenClient, controllerInstallation, conditionValid, conditionInstalled); client.IgnoreNotFound(err) != nil {
			log.Error(err, "Failed to patch conditions")
		}
	}()

	seed := &gardencorev1beta1.Seed{}
	if err := r.GardenClient.Get(gardenCtx, client.ObjectKey{Name: controllerInstallation.Spec.SeedRef.Name}, seed); err != nil {
		if apierrors.IsNotFound(err) {
			conditionValid = v1beta1helper.UpdatedConditionWithClock(r.Clock, conditionValid, gardencorev1beta1.ConditionFalse, "SeedNotFound", fmt.Sprintf("Referenced Seed does not exist: %+v", err))
		} else {
			conditionValid = v1beta1helper.UpdatedConditionWithClock(r.Clock, conditionValid, gardencorev1beta1.ConditionUnknown, "SeedReadError", fmt.Sprintf("Referenced Seed cannot be read: %+v", err))
		}
		return reconcile.Result{}, err
	}

	mr := &resourcesv1alpha1.ManagedResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      controllerInstallation.Name,
			Namespace: v1beta1constants.GardenNamespace,
		},
	}
	if err := r.SeedClientSet.Client().Delete(seedCtx, mr); err == nil {
		log.Info("Deletion of ManagedResource is still pending", "managedResource", client.ObjectKeyFromObject(mr))

		msg := fmt.Sprintf("Deletion of ManagedResource %q is still pending.", controllerInstallation.Name)
		conditionInstalled = v1beta1helper.UpdatedConditionWithClock(r.Clock, conditionInstalled, gardencorev1beta1.ConditionFalse, "DeletionPending", msg)
		return reconcile.Result{RequeueAfter: RequeueDurationWhenResourceDeletionStillPresent}, nil
	} else if !apierrors.IsNotFound(err) {
		conditionInstalled = v1beta1helper.UpdatedConditionWithClock(r.Clock, conditionInstalled, gardencorev1beta1.ConditionFalse, "DeletionFailed", fmt.Sprintf("Deletion of ManagedResource %q failed: %+v", controllerInstallation.Name, err))
		return reconcile.Result{}, err
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      controllerInstallation.Name,
			Namespace: v1beta1constants.GardenNamespace,
		},
	}
	if err := r.SeedClientSet.Client().Delete(seedCtx, secret); client.IgnoreNotFound(err) != nil {
		conditionInstalled = v1beta1helper.UpdatedConditionWithClock(r.Clock, conditionInstalled, gardencorev1beta1.ConditionFalse, "DeletionFailed", fmt.Sprintf("Deletion of ManagedResource secret %q failed: %+v", controllerInstallation.Name, err))
	}

	namespace := getNamespaceForControllerInstallation(controllerInstallation)
	if err := r.SeedClientSet.Client().Delete(seedCtx, namespace); err == nil || apierrors.IsConflict(err) {
		log.Info("Deletion of Namespace is still pending", "namespace", client.ObjectKeyFromObject(namespace))

		msg := fmt.Sprintf("Deletion of Namespace %q is still pending.", namespace.Name)
		conditionInstalled = v1beta1helper.UpdatedConditionWithClock(r.Clock, conditionInstalled, gardencorev1beta1.ConditionFalse, "DeletionPending", msg)
		return reconcile.Result{RequeueAfter: RequeueDurationWhenResourceDeletionStillPresent}, nil
	} else if !apierrors.IsNotFound(err) {
		conditionInstalled = v1beta1helper.UpdatedConditionWithClock(r.Clock, conditionInstalled, gardencorev1beta1.ConditionFalse, "DeletionFailed", fmt.Sprintf("Deletion of Namespace %q failed: %+v", namespace.Name, err))
		return reconcile.Result{}, err
	}

	conditionInstalled = v1beta1helper.UpdatedConditionWithClock(r.Clock, conditionInstalled, gardencorev1beta1.ConditionFalse, "DeletionSuccessful", "Deletion of old resources succeeded.")

	if controllerutil.ContainsFinalizer(controllerInstallation, finalizerName) {
		log.Info("Removing finalizer")
		if err := controllerutils.RemoveFinalizers(gardenCtx, r.GardenClient, controllerInstallation, finalizerName); err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
		}
	}

	return reconcile.Result{}, nil
}

func patchConditions(ctx context.Context, c client.StatusClient, controllerInstallation *gardencorev1beta1.ControllerInstallation, conditions ...gardencorev1beta1.Condition) error {
	patch := client.StrategicMergeFrom(controllerInstallation.DeepCopy())
	controllerInstallation.Status.Conditions = v1beta1helper.MergeConditions(controllerInstallation.Status.Conditions, conditions...)
	return c.Status().Patch(ctx, controllerInstallation, patch)
}

func getNamespaceForControllerInstallation(controllerInstallation *gardencorev1beta1.ControllerInstallation) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: gardenerutils.NamespaceNameForControllerInstallation(controllerInstallation),
		},
	}
}

func (r *Reconciler) reconcileGenericGardenKubeconfig(ctx context.Context, namespace string) (string, error) {
	var (
		address *string
		caCert  []byte
	)

	if gcc := r.Config.GardenClientConnection; gcc != nil {
		address = gcc.GardenClusterAddress
		caCert = gcc.GardenClusterCACert
	}

	restConfig := gardenerutils.PrepareGardenClientRestConfig(r.GardenConfig, address, caCert)

	kubeconfig, err := clientcmd.Write(clientcmdapi.Config{
		Clusters: map[string]*clientcmdapi.Cluster{"garden": {
			Server:                   restConfig.Host,
			InsecureSkipTLSVerify:    restConfig.Insecure,
			CertificateAuthorityData: restConfig.CAData,
		}},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{"extension": {
			TokenFile: gardenerutils.PathGardenToken,
		}},
		Contexts: map[string]*clientcmdapi.Context{"garden": {
			Cluster:  "garden",
			AuthInfo: "extension",
		}},
		CurrentContext: "garden",
	})
	if err != nil {
		return "", fmt.Errorf("failed to serialize generic garden kubeconfig: %w", err)
	}

	kubeconfigSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      v1beta1constants.SecretNameGenericGardenKubeconfig,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			secretsutils.DataKeyKubeconfig: kubeconfig,
		},
	}

	utilruntime.Must(kubernetesutils.MakeUnique(kubeconfigSecret))

	return kubeconfigSecret.Name, client.IgnoreAlreadyExists(r.SeedClientSet.Client().Create(ctx, kubeconfigSecret))
}

func (r *Reconciler) reconcileGardenAccessSecret(ctx context.Context, controllerInstallationName string, namespace string) (*gardenerutils.AccessSecret, error) {
	accessSecret := gardenerutils.NewGardenAccessSecret("extension", namespace).
		WithServiceAccountName(v1beta1constants.ExtensionGardenServiceAccountPrefix + controllerInstallationName)

	return accessSecret, accessSecret.Reconcile(ctx, r.SeedClientSet.Client())
}

var (
	injectionScheme    = kubernetes.SeedScheme
	injectionAPIGroups = sets.New[string](appsv1.GroupName, batchv1.GroupName)
)

// injectGardenAccessSecrets iterates over the given rendered secret data and injects the given garden access secrets
// into all objects in the apps API group.
func injectGardenAccessSecrets(secretData map[string][]byte, namespace, genericGardenKubeconfigSecretName, gardenAccessSecretName, seedName string) error {
	return mutateObjects(secretData, func(obj *unstructured.Unstructured) error {
		// only inject into objects of selected API groups
		if !injectionAPIGroups.Has(obj.GetObjectKind().GroupVersionKind().Group) {
			return nil
		}

		// we can only inject the access secret into objects in the ControllerInstallation's namespace
		if obj.GetNamespace() != namespace {
			return nil
		}

		return mutateTypedObject(obj, func(typedObject runtime.Object) error {
			// inject garden kubeconfig
			if err := gardenerutils.InjectGenericGardenKubeconfig(
				typedObject,
				genericGardenKubeconfigSecretName,
				gardenAccessSecretName,
			); err != nil {
				return err
			}

			// inject reference annotations for generic kubeconfig
			if err := references.InjectAnnotations(typedObject); err != nil {
				return err
			}

			// inject seed name env var
			return kubernetesutils.VisitPodSpec(typedObject, func(podSpec *corev1.PodSpec) {
				kubernetesutils.VisitContainers(podSpec, func(container *corev1.Container) {
					kubernetesutils.AddEnvVar(container, corev1.EnvVar{
						Name:  v1beta1constants.EnvSeedName,
						Value: seedName,
					}, true)
				})
			})
		})
	})
}

// mutateObject iterates over the given rendered secret data and calls the given mutator for each of them. It marshals
// the objects back after mutation and updates the secret data.
func mutateObjects(secretData map[string][]byte, mutate func(obj *unstructured.Unstructured) error) error {
	for key, data := range secretData {
		buffer := &bytes.Buffer{}
		manifestReader := kubernetes.NewManifestReader(data)

		for {
			_, _ = buffer.WriteString("\n---\n")
			obj, err := manifestReader.Read()
			if err == io.EOF {
				break
			}

			if err := mutate(obj); err != nil {
				return err
			}

			// serialize unstructured back to secret data
			// Note: we have to do this for all objects, not only for mutated ones, as there could be multiple objects in one file
			objBytes, err := yaml.Marshal(obj)
			if err != nil {
				return err
			}

			if _, err := buffer.Write(objBytes); err != nil {
				return err
			}
		}

		secretData[key] = buffer.Bytes()
	}

	return nil
}

// mutateTypedObject converts the given object to a typed object, calls the mutator, and converts the object back to the
// original type.
func mutateTypedObject(obj runtime.Object, mutate func(obj runtime.Object) error) error {
	// convert to typed object for injection logic
	typedObject, err := injectionScheme.New(obj.GetObjectKind().GroupVersionKind())
	if err != nil {
		return err
	}
	if err := injectionScheme.Convert(obj, typedObject, nil); err != nil {
		return err
	}

	if err := mutate(typedObject); err != nil {
		return err
	}

	// convert back into unstructured for serialization
	return injectionScheme.Convert(typedObject, obj, nil)
}
