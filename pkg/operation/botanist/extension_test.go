// Copyright 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package botanist_test

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	gomegatypes "github.com/onsi/gomega/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	kubernetesfake "github.com/gardener/gardener/pkg/client/kubernetes/fake"
	extensionpkg "github.com/gardener/gardener/pkg/component/extensions/extension"
	mockextension "github.com/gardener/gardener/pkg/component/extensions/extension/mock"
	mockclient "github.com/gardener/gardener/pkg/mock/controller-runtime/client"
	"github.com/gardener/gardener/pkg/operation"
	. "github.com/gardener/gardener/pkg/operation/botanist"
	shootpkg "github.com/gardener/gardener/pkg/operation/shoot"
)

var _ = Describe("Extensions", func() {
	var (
		ctrl         *gomock.Controller
		extension    *mockextension.MockInterface
		gardenClient *mockclient.MockClient
		botanist     *Botanist

		ctx        = context.TODO()
		fakeErr    = fmt.Errorf("fake")
		shootState = &gardencorev1beta1.ShootState{}
		namespace  = "shoot--name--space"
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		extension = mockextension.NewMockInterface(ctrl)
		gardenClient = mockclient.NewMockClient(ctrl)
		botanist = &Botanist{Operation: &operation.Operation{
			GardenClient:  gardenClient,
			SeedClientSet: kubernetesfake.NewClientSet(),
			Shoot: &shootpkg.Shoot{
				Components: &shootpkg.Components{
					Extensions: &shootpkg.Extensions{
						Extension: extension,
					},
				},
				SeedNamespace: namespace,
			},
		}}
		botanist.Shoot.SetShootState(shootState)
		botanist.Shoot.SetInfo(&gardencorev1beta1.Shoot{})
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	Describe("#DefaultExtension", func() {
		var (
			lifecycle      *gardencorev1beta1.ControllerResourceLifecycle
			extensionKind  = extensionsv1alpha1.ExtensionResource
			providerConfig = runtime.RawExtension{
				Raw: []byte("key: value"),
			}

			fooExtensionType         = "foo"
			fooReconciliationTimeout = metav1.Duration{Duration: 5 * time.Minute}
			fooRegistration          = gardencorev1beta1.ControllerRegistration{
				Spec: gardencorev1beta1.ControllerRegistrationSpec{
					Resources: []gardencorev1beta1.ControllerResource{
						{
							Kind:             extensionKind,
							Type:             fooExtensionType,
							ReconcileTimeout: &fooReconciliationTimeout,
						},
					},
				},
			}
			fooExtension = gardencorev1beta1.Extension{
				Type:           fooExtensionType,
				ProviderConfig: &providerConfig,
			}

			barExtensionType = "bar"
			barRegistration  = gardencorev1beta1.ControllerRegistration{
				Spec: gardencorev1beta1.ControllerRegistrationSpec{
					Resources: []gardencorev1beta1.ControllerResource{
						{
							Kind:            extensionKind,
							Type:            barExtensionType,
							GloballyEnabled: pointer.Bool(true),
						},
					},
				},
			}
			barRegistrationSupportedForWorkerless = gardencorev1beta1.ControllerRegistration{
				Spec: gardencorev1beta1.ControllerRegistrationSpec{
					Resources: []gardencorev1beta1.ControllerResource{
						{
							Kind:                extensionKind,
							Type:                barExtensionType,
							GloballyEnabled:     pointer.Bool(true),
							WorkerlessSupported: pointer.Bool(true),
						},
					},
				},
			}
			barExtension = gardencorev1beta1.Extension{
				Type:           barExtensionType,
				ProviderConfig: &providerConfig,
			}
			barExtensionDisabled = gardencorev1beta1.Extension{
				Type:           barExtensionType,
				ProviderConfig: &providerConfig,
				Disabled:       pointer.Bool(true),
			}
		)

		It("should return the error because listing failed", func() {
			gardenClient.EXPECT().List(ctx, gomock.AssignableToTypeOf(&gardencorev1beta1.ControllerRegistrationList{})).Return(fakeErr)

			ext, err := botanist.DefaultExtension(ctx)
			Expect(ext).To(BeNil())
			Expect(err).To(MatchError(fakeErr))
		})

		DescribeTable("#DefaultExtension",
			func(registrations []gardencorev1beta1.ControllerRegistration, extensions []gardencorev1beta1.Extension, workerless bool, conditionMatcher gomegatypes.GomegaMatcher) {
				botanist.Shoot.GetInfo().Spec.Extensions = extensions
				botanist.Shoot.IsWorkerless = workerless

				gardenClient.EXPECT().List(ctx, gomock.AssignableToTypeOf(&gardencorev1beta1.ControllerRegistrationList{})).DoAndReturn(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) error {
					(&gardencorev1beta1.ControllerRegistrationList{Items: registrations}).DeepCopyInto(list.(*gardencorev1beta1.ControllerRegistrationList))
					return nil
				})

				ext, err := botanist.DefaultExtension(ctx)
				Expect(err).To(BeNil())
				Expect(ext.Extensions()).To(conditionMatcher)
			},

			Entry(
				"No extensions",
				nil,
				nil,
				false,
				BeEmpty(),
			),
			Entry(
				"Extension w/o registration",
				nil,
				[]gardencorev1beta1.Extension{{Type: fooExtensionType}},
				false,
				BeEmpty(),
			),
			Entry(
				"Extensions w/ registration",
				[]gardencorev1beta1.ControllerRegistration{fooRegistration},
				[]gardencorev1beta1.Extension{fooExtension},
				false,
				HaveKeyWithValue(
					Equal(fooExtensionType),
					MatchAllFields(
						Fields{
							"Extension": MatchFields(IgnoreExtras, Fields{
								"Spec": MatchFields(IgnoreExtras, Fields{
									"DefaultSpec": MatchAllFields(Fields{
										"Type":           Equal(fooExtensionType),
										"ProviderConfig": PointTo(Equal(providerConfig)),
									}),
								}),
							}),
							"Timeout":   Equal(fooReconciliationTimeout.Duration),
							"Lifecycle": Equal(lifecycle),
						},
					),
				),
			),
			Entry(
				"Registration w/o extension",
				[]gardencorev1beta1.ControllerRegistration{fooRegistration},
				nil,
				false,
				BeEmpty(),
			),
			Entry(
				"Globally enabled extension registration, w/o extension",
				[]gardencorev1beta1.ControllerRegistration{barRegistration},
				nil,
				false,
				HaveKeyWithValue(
					Equal(barExtensionType),
					MatchAllFields(
						Fields{
							"Extension": MatchFields(IgnoreExtras, Fields{
								"Spec": MatchAllFields(Fields{
									"DefaultSpec": MatchAllFields(Fields{
										"Type":           Equal(barExtensionType),
										"ProviderConfig": BeNil(),
									}),
								}),
							}),
							"Timeout":   Equal(extensionpkg.DefaultTimeout),
							"Lifecycle": Equal(lifecycle),
						},
					),
				),
			),
			Entry(
				"Globally enabled extension registration but explicitly disabled",
				[]gardencorev1beta1.ControllerRegistration{barRegistration},
				[]gardencorev1beta1.Extension{barExtensionDisabled},
				false,
				BeEmpty(),
			),
			Entry(
				"Multiple registration but a globally one is explicitly disabled",
				[]gardencorev1beta1.ControllerRegistration{fooRegistration, barRegistration},
				[]gardencorev1beta1.Extension{fooExtension, barExtensionDisabled},
				false,
				SatisfyAll(
					HaveLen(1),
					HaveKeyWithValue(
						Equal(fooExtensionType),
						MatchAllFields(
							Fields{
								"Extension": MatchFields(IgnoreExtras, Fields{
									"Spec": MatchFields(IgnoreExtras, Fields{
										"DefaultSpec": MatchAllFields(Fields{
											"Type":           Equal(fooExtensionType),
											"ProviderConfig": PointTo(Equal(providerConfig)),
										}),
									}),
								}),
								"Timeout":   Equal(fooReconciliationTimeout.Duration),
								"Lifecycle": Equal(lifecycle),
							},
						),
					),
				),
			),
			Entry(
				"Multiple registrations, w/ one extension",
				[]gardencorev1beta1.ControllerRegistration{
					fooRegistration,
					barRegistration,
					{
						Spec: gardencorev1beta1.ControllerRegistrationSpec{
							Resources: []gardencorev1beta1.ControllerResource{
								{
									Kind: "kind",
									Type: "type",
								},
							},
						},
					},
				},
				[]gardencorev1beta1.Extension{barExtension},
				false,
				HaveKeyWithValue(
					Equal(barExtensionType),
					MatchAllFields(
						Fields{
							"Extension": MatchFields(IgnoreExtras, Fields{
								"Spec": MatchAllFields(Fields{
									"DefaultSpec": MatchAllFields(Fields{
										"Type":           Equal(barExtensionType),
										"ProviderConfig": PointTo(Equal(providerConfig)),
									}),
								}),
							}),
							"Timeout":   Equal(extensionpkg.DefaultTimeout),
							"Lifecycle": Equal(lifecycle),
						},
					),
				),
			),
			Entry(
				"Globally enabled extension supported for workerless",
				[]gardencorev1beta1.ControllerRegistration{
					barRegistrationSupportedForWorkerless,
				},
				[]gardencorev1beta1.Extension{},
				true,
				HaveKeyWithValue(
					Equal(barExtensionType),
					MatchFields(IgnoreExtras, Fields{
						"Extension": MatchFields(IgnoreExtras, Fields{
							"Spec": MatchFields(IgnoreExtras, Fields{
								"DefaultSpec": MatchFields(IgnoreExtras, Fields{
									"Type": Equal(barExtensionType),
								}),
							}),
						}),
					},
					),
				),
			),
			Entry(
				"Globally enabled extension not supported for workerless",
				[]gardencorev1beta1.ControllerRegistration{
					barRegistration,
				},
				[]gardencorev1beta1.Extension{},
				true,
				BeEmpty(),
			),
		)
	})

	Describe("#DeployExtensions", func() {
		Context("deploy after kube-apiserver", func() {
			It("should deploy successfully", func() {
				extension.EXPECT().DeployAfterKubeAPIServer(ctx)
				Expect(botanist.DeployExtensionsAfterKubeAPIServer(ctx)).To(Succeed())
			})

			It("should return the error during deployment", func() {
				extension.EXPECT().DeployAfterKubeAPIServer(ctx).Return(fakeErr)
				Expect(botanist.DeployExtensionsAfterKubeAPIServer(ctx)).To(MatchError(fakeErr))
			})
		})

		Context("deploy before kube-apiserver", func() {
			It("should deploy successfully", func() {
				extension.EXPECT().DeployBeforeKubeAPIServer(ctx)
				Expect(botanist.DeployExtensionsBeforeKubeAPIServer(ctx)).To(Succeed())
			})

			It("should return the error during deployment", func() {
				extension.EXPECT().DeployBeforeKubeAPIServer(ctx).Return(fakeErr)
				Expect(botanist.DeployExtensionsBeforeKubeAPIServer(ctx)).To(MatchError(fakeErr))
			})
		})

		Context("restore", func() {
			BeforeEach(func() {
				botanist.Shoot.SetInfo(&gardencorev1beta1.Shoot{
					Status: gardencorev1beta1.ShootStatus{
						LastOperation: &gardencorev1beta1.LastOperation{
							Type: gardencorev1beta1.LastOperationTypeRestore,
						},
					},
				})
			})

			Context("after kube-apiserver", func() {
				It("should restore successfully", func() {
					extension.EXPECT().RestoreAfterKubeAPIServer(ctx, shootState)
					Expect(botanist.DeployExtensionsAfterKubeAPIServer(ctx)).To(Succeed())
				})

				It("should return the error during restoration", func() {
					extension.EXPECT().RestoreAfterKubeAPIServer(ctx, shootState).Return(fakeErr)
					Expect(botanist.DeployExtensionsAfterKubeAPIServer(ctx)).To(MatchError(fakeErr))
				})
			})

			Context("before kube-apiserver", func() {
				It("should restore successfully", func() {
					extension.EXPECT().RestoreBeforeKubeAPIServer(ctx, shootState)
					Expect(botanist.DeployExtensionsBeforeKubeAPIServer(ctx)).To(Succeed())
				})

				It("should return the error during restoration", func() {
					extension.EXPECT().RestoreBeforeKubeAPIServer(ctx, shootState).Return(fakeErr)
					Expect(botanist.DeployExtensionsBeforeKubeAPIServer(ctx)).To(MatchError(fakeErr))
				})
			})
		})
	})
})
