// Copyright 2022 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package managedseed

import (
	"context"
	"time"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	seedmanagementv1alpha1 "github.com/gardener/gardener/pkg/apis/seedmanagement/v1alpha1"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/gardenlet/apis/config"
	mockworkqueue "github.com/gardener/gardener/pkg/mock/client-go/util/workqueue"
	"github.com/gardener/gardener/pkg/utils/test"
)

var _ = Describe("Add", func() {
	var (
		ctx        = context.TODO()
		fakeClient client.Client
		reconciler *Reconciler
		p          predicate.Predicate

		managedSeed            *seedmanagementv1alpha1.ManagedSeed
		managedSeedShoot       *seedmanagementv1alpha1.Shoot
		shoot                  *gardencorev1beta1.Shoot
		seedNameFromSeedConfig string
	)

	BeforeEach(func() {
		fakeClient = fakeclient.NewClientBuilder().WithScheme(kubernetes.GardenScheme).Build()
		reconciler = &Reconciler{
			GardenClient:          fakeClient,
			GardenNamespaceGarden: v1beta1constants.GardenNamespace,
		}

		shoot = &gardencorev1beta1.Shoot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
		}

		managedSeedShoot = &seedmanagementv1alpha1.Shoot{
			Name: name,
		}

		seedNameFromSeedConfig = "test-seed"
	})

	Describe("#ManagedSeedPredicate", func() {
		var (
			oldManagedSeed, newManagedSeed *seedmanagementv1alpha1.ManagedSeed
		)

		BeforeEach(func() {
			p = reconciler.ManagedSeedPredicate(ctx, seedNameFromSeedConfig)

			oldManagedSeed = &seedmanagementv1alpha1.ManagedSeed{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
			}

			newManagedSeed = &seedmanagementv1alpha1.ManagedSeed{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
			}
		})

		It("should return false when ManagedSeed does not reference any shoot", func() {
			Expect(p.Create(event.CreateEvent{Object: newManagedSeed})).To(BeFalse())
			Expect(p.Update(event.UpdateEvent{ObjectOld: oldManagedSeed, ObjectNew: newManagedSeed})).To(BeFalse())
			Expect(p.Delete(event.DeleteEvent{Object: newManagedSeed})).To(BeFalse())
			Expect(p.Generic(event.GenericEvent{})).To(BeFalse())
		})

		It("should return false when shoot referenced by ManagedSeed is not present", func() {
			oldManagedSeed.Spec.Shoot = managedSeedShoot
			newManagedSeed.Spec.Shoot = managedSeedShoot
			Expect(p.Create(event.CreateEvent{Object: newManagedSeed})).To(BeFalse())
			Expect(p.Update(event.UpdateEvent{ObjectOld: oldManagedSeed, ObjectNew: newManagedSeed})).To(BeFalse())
			Expect(p.Delete(event.DeleteEvent{Object: newManagedSeed})).To(BeFalse())
			Expect(p.Generic(event.GenericEvent{})).To(BeFalse())
		})

		It("should return false when shoot referenced by ManagedSeed does not reference any seed", func() {
			oldManagedSeed.Spec.Shoot = managedSeedShoot
			newManagedSeed.Spec.Shoot = managedSeedShoot
			Expect(fakeClient.Create(ctx, shoot)).To(Succeed())
			Expect(p.Create(event.CreateEvent{Object: newManagedSeed})).To(BeFalse())
			Expect(p.Update(event.UpdateEvent{ObjectOld: oldManagedSeed, ObjectNew: newManagedSeed})).To(BeFalse())
			Expect(p.Delete(event.DeleteEvent{Object: newManagedSeed})).To(BeFalse())
			Expect(p.Generic(event.GenericEvent{})).To(BeFalse())
		})

		It("should return true when shoot referenced by ManagedSeed references a seed which is same as the seed mentioned in gardenlet configuration", func() {
			oldManagedSeed.Spec.Shoot = managedSeedShoot
			newManagedSeed.Spec.Shoot = managedSeedShoot
			shoot.Spec.SeedName = pointer.String(seedNameFromSeedConfig)
			Expect(fakeClient.Create(ctx, shoot)).To(Succeed())
			Expect(p.Create(event.CreateEvent{Object: newManagedSeed})).To(BeTrue())
			Expect(p.Update(event.UpdateEvent{ObjectOld: oldManagedSeed, ObjectNew: newManagedSeed})).To(BeTrue())
			Expect(p.Delete(event.DeleteEvent{Object: newManagedSeed})).To(BeTrue())
			Expect(p.Generic(event.GenericEvent{})).To(BeFalse())
		})

		It("should return false when shoot referenced by ManagedSeed references a seed which is not same as the seed mentioned in gardenlet configuration", func() {
			oldManagedSeed.Spec.Shoot = managedSeedShoot
			newManagedSeed.Spec.Shoot = managedSeedShoot
			shoot.Spec.SeedName = pointer.String("test")
			Expect(fakeClient.Create(ctx, shoot)).To(Succeed())
			Expect(p.Create(event.CreateEvent{Object: newManagedSeed})).To(BeFalse())
			Expect(p.Update(event.UpdateEvent{ObjectOld: oldManagedSeed, ObjectNew: newManagedSeed})).To(BeFalse())
			Expect(p.Delete(event.DeleteEvent{Object: newManagedSeed})).To(BeFalse())
			Expect(p.Generic(event.GenericEvent{})).To(BeFalse())
		})

		It("should return false when shoot referenced by ManagedSeed has seed name in status field which is not same as the seed mentioned in gardenlet configuration", func() {
			oldManagedSeed.Spec.Shoot = managedSeedShoot
			newManagedSeed.Spec.Shoot = managedSeedShoot
			shoot.Spec.SeedName = pointer.String("test")
			shoot.Status.SeedName = pointer.String("other-seed")
			Expect(fakeClient.Create(ctx, shoot)).To(Succeed())
			Expect(p.Create(event.CreateEvent{Object: newManagedSeed})).To(BeFalse())
			Expect(p.Update(event.UpdateEvent{ObjectOld: oldManagedSeed, ObjectNew: newManagedSeed})).To(BeFalse())
			Expect(p.Delete(event.DeleteEvent{Object: newManagedSeed})).To(BeFalse())
			Expect(p.Generic(event.GenericEvent{})).To(BeFalse())
		})

		It("should return true when shoot referenced by ManagedSeed has seed name in status field which is same as the seed mentioned in gardenlet configuration", func() {
			oldManagedSeed.Spec.Shoot = managedSeedShoot
			newManagedSeed.Spec.Shoot = managedSeedShoot
			shoot.Spec.SeedName = pointer.String("test")
			shoot.Status.SeedName = pointer.String(seedNameFromSeedConfig)
			Expect(fakeClient.Create(ctx, shoot)).To(Succeed())
			Expect(p.Create(event.CreateEvent{Object: newManagedSeed})).To(BeTrue())
			Expect(p.Update(event.UpdateEvent{ObjectOld: oldManagedSeed, ObjectNew: newManagedSeed})).To(BeTrue())
			Expect(p.Delete(event.DeleteEvent{Object: newManagedSeed})).To(BeTrue())
			Expect(p.Generic(event.GenericEvent{})).To(BeFalse())
		})
	})

	Describe("#SeedOfManagedSeedPredicate", func() {
		var (
			oldSeed, newSeed *gardencorev1beta1.Seed
		)

		BeforeEach(func() {
			p = reconciler.SeedOfManagedSeedPredicate(ctx, seedNameFromSeedConfig)

			oldSeed = &gardencorev1beta1.Seed{
				ObjectMeta: metav1.ObjectMeta{
					Name: name,
				},
			}

			newSeed = &gardencorev1beta1.Seed{
				ObjectMeta: metav1.ObjectMeta{
					Name: name,
				},
			}

			managedSeed = &seedmanagementv1alpha1.ManagedSeed{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
			}
		})

		It("should return false if the object is not seed", func() {
			Expect(p.Create(event.CreateEvent{Object: &seedmanagementv1alpha1.ManagedSeed{}})).To(BeFalse())
			Expect(p.Update(event.UpdateEvent{ObjectOld: &seedmanagementv1alpha1.ManagedSeed{}, ObjectNew: &seedmanagementv1alpha1.ManagedSeed{}})).To(BeFalse())
			Expect(p.Delete(event.DeleteEvent{Object: &seedmanagementv1alpha1.ManagedSeed{}})).To(BeFalse())
			Expect(p.Generic(event.GenericEvent{})).To(BeFalse())
		})

		It("should return false when ManagedSeed is not present", func() {
			Expect(p.Create(event.CreateEvent{Object: newSeed})).To(BeFalse())
			Expect(p.Update(event.UpdateEvent{ObjectOld: oldSeed, ObjectNew: newSeed})).To(BeFalse())
			Expect(p.Delete(event.DeleteEvent{Object: newSeed})).To(BeFalse())
			Expect(p.Generic(event.GenericEvent{})).To(BeFalse())
		})

		It("should return false when ManagedSeed does not reference any shoot", func() {
			Expect(fakeClient.Create(ctx, managedSeed)).To(Succeed())
			Expect(p.Create(event.CreateEvent{Object: newSeed})).To(BeFalse())
			Expect(p.Update(event.UpdateEvent{ObjectOld: oldSeed, ObjectNew: newSeed})).To(BeFalse())
			Expect(p.Delete(event.DeleteEvent{Object: newSeed})).To(BeFalse())
			Expect(p.Generic(event.GenericEvent{})).To(BeFalse())
		})

		It("should return false when shoot referenced by ManagedSeed is not present", func() {
			managedSeed.Spec.Shoot = managedSeedShoot
			Expect(fakeClient.Create(ctx, managedSeed)).To(Succeed())
			Expect(p.Create(event.CreateEvent{Object: newSeed})).To(BeFalse())
			Expect(p.Update(event.UpdateEvent{ObjectOld: oldSeed, ObjectNew: newSeed})).To(BeFalse())
			Expect(p.Delete(event.DeleteEvent{Object: newSeed})).To(BeFalse())
			Expect(p.Generic(event.GenericEvent{})).To(BeFalse())
		})

		It("should return false when shoot referenced by ManagedSeed does not reference any seed", func() {
			managedSeed.Spec.Shoot = managedSeedShoot
			Expect(fakeClient.Create(ctx, shoot)).To(Succeed())
			Expect(fakeClient.Create(ctx, managedSeed)).To(Succeed())
			Expect(p.Create(event.CreateEvent{Object: newSeed})).To(BeFalse())
			Expect(p.Update(event.UpdateEvent{ObjectOld: oldSeed, ObjectNew: newSeed})).To(BeFalse())
			Expect(p.Delete(event.DeleteEvent{Object: newSeed})).To(BeFalse())
			Expect(p.Generic(event.GenericEvent{})).To(BeFalse())
		})

		It("should return true when shoot referenced by ManagedSeed references a seed which is same as the seed mentioned in gardenlet configuration", func() {
			managedSeed.Spec.Shoot = managedSeedShoot
			shoot.Spec.SeedName = pointer.String(seedNameFromSeedConfig)
			Expect(fakeClient.Create(ctx, shoot)).To(Succeed())
			Expect(fakeClient.Create(ctx, managedSeed)).To(Succeed())
			Expect(p.Create(event.CreateEvent{Object: newSeed})).To(BeTrue())
			Expect(p.Update(event.UpdateEvent{ObjectOld: oldSeed, ObjectNew: newSeed})).To(BeTrue())
			Expect(p.Delete(event.DeleteEvent{Object: newSeed})).To(BeTrue())
			Expect(p.Generic(event.GenericEvent{})).To(BeFalse())
		})

		It("should return false when shoot referenced by ManagedSeed references a seed which is not same as the seed mentioned in gardenlet configuration", func() {
			managedSeed.Spec.Shoot = managedSeedShoot
			shoot.Spec.SeedName = pointer.String("test")
			Expect(fakeClient.Create(ctx, shoot)).To(Succeed())
			Expect(fakeClient.Create(ctx, managedSeed)).To(Succeed())
			Expect(p.Create(event.CreateEvent{Object: newSeed})).To(BeFalse())
			Expect(p.Update(event.UpdateEvent{ObjectOld: oldSeed, ObjectNew: newSeed})).To(BeFalse())
			Expect(p.Delete(event.DeleteEvent{Object: newSeed})).To(BeFalse())
			Expect(p.Generic(event.GenericEvent{})).To(BeFalse())
		})

		It("should return false when shoot referenced by ManagedSeed has seed name in status field which is not same as shoot's spec.seedName field and the seed mentioned in gardenlet configuration", func() {
			managedSeed.Spec.Shoot = managedSeedShoot
			shoot.Spec.SeedName = pointer.String("test")
			shoot.Status.SeedName = pointer.String("other-seed")
			Expect(fakeClient.Create(ctx, shoot)).To(Succeed())
			Expect(fakeClient.Create(ctx, managedSeed)).To(Succeed())
			Expect(p.Create(event.CreateEvent{Object: newSeed})).To(BeFalse())
			Expect(p.Update(event.UpdateEvent{ObjectOld: oldSeed, ObjectNew: newSeed})).To(BeFalse())
			Expect(p.Delete(event.DeleteEvent{Object: newSeed})).To(BeFalse())
			Expect(p.Generic(event.GenericEvent{})).To(BeFalse())
		})

		It("should return true when shoot referenced by ManagedSeed has seed name in status field which is not same as shoot's spec.seedName field but same as the seed mentioned in gardenlet configuration", func() {
			managedSeed.Spec.Shoot = managedSeedShoot
			shoot.Spec.SeedName = pointer.String("test")
			shoot.Status.SeedName = pointer.String(seedNameFromSeedConfig)
			Expect(fakeClient.Create(ctx, shoot)).To(Succeed())
			Expect(fakeClient.Create(ctx, managedSeed)).To(Succeed())
			Expect(p.Create(event.CreateEvent{Object: newSeed})).To(BeTrue())
			Expect(p.Update(event.UpdateEvent{ObjectOld: oldSeed, ObjectNew: newSeed})).To(BeTrue())
			Expect(p.Delete(event.DeleteEvent{Object: newSeed})).To(BeTrue())
			Expect(p.Generic(event.GenericEvent{})).To(BeFalse())
		})
	})

	Describe("#EnqueueWithJitterDelay", func() {
		var (
			hdlr           handler.EventHandler
			queue          *mockworkqueue.MockRateLimitingInterface
			obj            *seedmanagementv1alpha1.ManagedSeed
			req            reconcile.Request
			cfg            config.GardenletConfiguration
			randomDuration = 10 * time.Millisecond
		)

		BeforeEach(func() {
			cfg = config.GardenletConfiguration{
				Controllers: &config.GardenletControllerConfiguration{
					ManagedSeed: &config.ManagedSeedControllerConfiguration{
						SyncJitterPeriod: &metav1.Duration{Duration: 50 * time.Millisecond},
					},
				},
			}

			hdlr = (&Reconciler{Config: cfg}).EnqueueWithJitterDelay()
			queue = mockworkqueue.NewMockRateLimitingInterface(gomock.NewController(GinkgoT()))
			obj = &seedmanagementv1alpha1.ManagedSeed{ObjectMeta: metav1.ObjectMeta{Name: "managedseed", Namespace: "namespace"}}
			req = reconcile.Request{NamespacedName: types.NamespacedName{Name: obj.Name, Namespace: obj.Namespace}}

			DeferCleanup(func() {
				test.WithVar(&RandomDurationWithMetaDuration, func(_ *metav1.Duration) time.Duration { return randomDuration })
			})
		})

		It("should enqueue the object without delay for Create events when deletion timestamp is set", func() {
			queue.EXPECT().Add(req)

			now := metav1.Now()
			obj.SetDeletionTimestamp(&now)
			hdlr.Create(event.CreateEvent{Object: obj}, queue)
		})

		It("should enqueue the object without delay for Create events when generation is set to 1", func() {
			queue.EXPECT().Add(req)

			obj.Generation = 1
			hdlr.Create(event.CreateEvent{Object: obj}, queue)
		})

		It("should enqueue the object without delay for Create events when generation changed and jitterudpates is set to false", func() {
			queue.EXPECT().Add(req)

			cfg.Controllers.ManagedSeed.JitterUpdates = pointer.Bool(false)
			obj.Generation = 2
			obj.Status.ObservedGeneration = 1
			hdlr = (&Reconciler{Config: cfg}).EnqueueWithJitterDelay()
			hdlr.Create(event.CreateEvent{Object: obj}, queue)
		})

		It("should enqueue the object with random delay for Create events when generation changed and  jitterUpdates is set to true", func() {
			queue.EXPECT().AddAfter(req, randomDuration)

			cfg.Controllers.ManagedSeed.JitterUpdates = pointer.Bool(true)
			obj.Generation = 2
			obj.Status.ObservedGeneration = 1
			hdlr = (&Reconciler{Config: cfg}).EnqueueWithJitterDelay()
			hdlr.Create(event.CreateEvent{Object: obj}, queue)
		})

		It("should enqueue the object with random delay for Create events when there is no change in generation", func() {
			queue.EXPECT().AddAfter(req, randomDuration)

			cfg.Controllers.ManagedSeed.JitterUpdates = pointer.Bool(false)
			obj.Generation = 2
			obj.Status.ObservedGeneration = 2
			hdlr = (&Reconciler{Config: cfg}).EnqueueWithJitterDelay()
			hdlr.Create(event.CreateEvent{Object: obj}, queue)
		})

		It("should not enqueue the object for Update events when generation and observedGeneration are equal", func() {
			obj.Generation = 1
			obj.Status.ObservedGeneration = 1
			hdlr.Update(event.UpdateEvent{ObjectNew: obj, ObjectOld: obj}, queue)
		})

		It("should enqueue the object for Update events when deletion timestamp is set", func() {
			queue.EXPECT().Add(req)

			obj.Generation = 2
			obj.Status.ObservedGeneration = 1
			now := metav1.Now()
			obj.SetDeletionTimestamp(&now)
			hdlr.Update(event.UpdateEvent{ObjectNew: obj, ObjectOld: obj}, queue)
		})

		It("should enqueue the object for Update events when generation is 1", func() {
			queue.EXPECT().Add(req)

			obj.Generation = 1
			obj.Status.ObservedGeneration = 0
			hdlr.Update(event.UpdateEvent{ObjectNew: obj, ObjectOld: obj}, queue)
		})

		It("should enqueue the object for Update events when jitterUpdates is set to false", func() {
			queue.EXPECT().Add(req)

			cfg.Controllers.ManagedSeed.JitterUpdates = pointer.Bool(false)
			obj.Generation = 2
			obj.Status.ObservedGeneration = 1
			hdlr = (&Reconciler{Config: cfg}).EnqueueWithJitterDelay()
			hdlr.Update(event.UpdateEvent{ObjectNew: obj, ObjectOld: obj}, queue)
		})

		It("should enqueue the object with random delay for Update events when jitterUpdates is set to true", func() {
			queue.EXPECT().AddAfter(req, randomDuration)

			cfg.Controllers.ManagedSeed.JitterUpdates = pointer.Bool(true)
			obj.Generation = 2
			obj.Status.ObservedGeneration = 1
			hdlr = (&Reconciler{Config: cfg}).EnqueueWithJitterDelay()
			hdlr.Update(event.UpdateEvent{ObjectNew: obj, ObjectOld: obj}, queue)
		})

		It("should enqueue the object for Delete events", func() {
			queue.EXPECT().Add(req)

			hdlr.Delete(event.DeleteEvent{Object: obj}, queue)
		})

		It("should not enqueue the object for Generic events", func() {
			hdlr.Generic(event.GenericEvent{Object: obj}, queue)
		})
	})
})
