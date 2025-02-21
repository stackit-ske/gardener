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

package care_test

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	resourcesv1alpha1 "github.com/gardener/gardener/pkg/apis/resources/v1alpha1"
	. "github.com/gardener/gardener/pkg/gardenlet/controller/seed/care"
)

var _ = Describe("Add", func() {
	var (
		reconciler      *Reconciler
		seed            *gardencorev1beta1.Seed
		managedResource *resourcesv1alpha1.ManagedResource
	)

	BeforeEach(func() {
		reconciler = &Reconciler{
			SeedName: "seed",
		}
		seed = &gardencorev1beta1.Seed{}
		managedResource = &resourcesv1alpha1.ManagedResource{ObjectMeta: metav1.ObjectMeta{Namespace: "garden"}}
	})

	Describe("#SeedPredicate", func() {
		var p predicate.Predicate

		BeforeEach(func() {
			p = reconciler.SeedPredicate()
		})

		Describe("#Create", func() {
			It("should return true", func() {
				Expect(p.Create(event.CreateEvent{})).To(BeTrue())
			})
		})

		Describe("#Update", func() {
			It("should return false because new object is no seed", func() {
				Expect(p.Update(event.UpdateEvent{})).To(BeFalse())
			})

			It("should return false because old object is no seed", func() {
				Expect(p.Update(event.UpdateEvent{ObjectNew: seed})).To(BeFalse())
			})

			It("should return false because Bootstrapped condition does not exist", func() {
				Expect(p.Update(event.UpdateEvent{ObjectOld: seed, ObjectNew: seed})).To(BeFalse())
			})

			It("should return false because Bootstrapped condition was true before", func() {
				seed.Status.Conditions = []gardencorev1beta1.Condition{{Type: gardencorev1beta1.SeedBootstrapped, Status: gardencorev1beta1.ConditionTrue}}
				Expect(p.Update(event.UpdateEvent{ObjectOld: seed, ObjectNew: seed})).To(BeFalse())
			})

			It("should return false because Bootstrapped condition is no longer true", func() {
				seed.Status.Conditions = []gardencorev1beta1.Condition{{Type: gardencorev1beta1.SeedBootstrapped, Status: gardencorev1beta1.ConditionFalse}}
				oldSeed := seed.DeepCopy()
				oldSeed.Status.Conditions[0].Status = gardencorev1beta1.ConditionTrue
				Expect(p.Update(event.UpdateEvent{ObjectOld: oldSeed, ObjectNew: seed})).To(BeFalse())
			})

			It("should return false because Bootstrapped condition does no longer exist", func() {
				oldSeed := seed.DeepCopy()
				oldSeed.Status.Conditions = []gardencorev1beta1.Condition{{Type: gardencorev1beta1.SeedBootstrapped, Status: gardencorev1beta1.ConditionTrue}}
				Expect(p.Update(event.UpdateEvent{ObjectOld: oldSeed, ObjectNew: seed})).To(BeFalse())
			})

			It("should return true because Bootstrapped condition did not exist before", func() {
				seed.Status.Conditions = []gardencorev1beta1.Condition{{Type: gardencorev1beta1.SeedBootstrapped, Status: gardencorev1beta1.ConditionTrue}}
				oldSeed := seed.DeepCopy()
				oldSeed.Status.Conditions = nil
				Expect(p.Update(event.UpdateEvent{ObjectOld: oldSeed, ObjectNew: seed})).To(BeTrue())
			})

			It("should return true because Bootstrapped condition was not true before", func() {
				seed.Status.Conditions = []gardencorev1beta1.Condition{{Type: gardencorev1beta1.SeedBootstrapped, Status: gardencorev1beta1.ConditionTrue}}
				oldSeed := seed.DeepCopy()
				oldSeed.Status.Conditions[0].Status = gardencorev1beta1.ConditionFalse
				Expect(p.Update(event.UpdateEvent{ObjectOld: oldSeed, ObjectNew: seed})).To(BeTrue())
			})
		})

		Describe("#Delete", func() {
			It("should return false", func() {
				Expect(p.Delete(event.DeleteEvent{})).To(BeFalse())
			})
		})

		Describe("#Generic", func() {
			It("should return false", func() {
				Expect(p.Generic(event.GenericEvent{})).To(BeFalse())
			})
		})
	})

	Describe("#IsSystemComponent", func() {
		var p predicate.Predicate

		BeforeEach(func() {
			p = reconciler.IsSystemComponent()
		})

		It("should return false because the label is not present", func() {
			Expect(p.Create(event.CreateEvent{Object: managedResource})).To(BeFalse())
			Expect(p.Update(event.UpdateEvent{ObjectNew: managedResource})).To(BeFalse())
			Expect(p.Delete(event.DeleteEvent{Object: managedResource})).To(BeFalse())
			Expect(p.Generic(event.GenericEvent{Object: managedResource})).To(BeFalse())
		})

		It("should return true because the label is present", func() {
			managedResource.Labels = map[string]string{"gardener.cloud/role": "seed-system-component"}

			Expect(p.Create(event.CreateEvent{Object: managedResource})).To(BeTrue())
			Expect(p.Update(event.UpdateEvent{ObjectNew: managedResource})).To(BeTrue())
			Expect(p.Delete(event.DeleteEvent{Object: managedResource})).To(BeTrue())
			Expect(p.Generic(event.GenericEvent{Object: managedResource})).To(BeTrue())
		})
	})

	Describe("#MapManagedResourceToSeed", func() {
		It("should return a request with the seed name", func() {
			Expect(reconciler.MapManagedResourceToSeed(context.TODO(), logr.Discard(), nil, nil)).To(ConsistOf(reconcile.Request{NamespacedName: types.NamespacedName{Name: reconciler.SeedName}}))
		})
	})
})
