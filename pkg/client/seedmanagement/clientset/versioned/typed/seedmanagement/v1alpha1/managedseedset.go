/*
Copyright SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file

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

// Code generated by client-gen. DO NOT EDIT.

package v1alpha1

import (
	"context"
	"time"

	v1alpha1 "github.com/gardener/gardener/pkg/apis/seedmanagement/v1alpha1"
	scheme "github.com/gardener/gardener/pkg/client/seedmanagement/clientset/versioned/scheme"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	rest "k8s.io/client-go/rest"
)

// ManagedSeedSetsGetter has a method to return a ManagedSeedSetInterface.
// A group's client should implement this interface.
type ManagedSeedSetsGetter interface {
	ManagedSeedSets(namespace string) ManagedSeedSetInterface
}

// ManagedSeedSetInterface has methods to work with ManagedSeedSet resources.
type ManagedSeedSetInterface interface {
	Create(ctx context.Context, managedSeedSet *v1alpha1.ManagedSeedSet, opts v1.CreateOptions) (*v1alpha1.ManagedSeedSet, error)
	Update(ctx context.Context, managedSeedSet *v1alpha1.ManagedSeedSet, opts v1.UpdateOptions) (*v1alpha1.ManagedSeedSet, error)
	UpdateStatus(ctx context.Context, managedSeedSet *v1alpha1.ManagedSeedSet, opts v1.UpdateOptions) (*v1alpha1.ManagedSeedSet, error)
	Delete(ctx context.Context, name string, opts v1.DeleteOptions) error
	DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error
	Get(ctx context.Context, name string, opts v1.GetOptions) (*v1alpha1.ManagedSeedSet, error)
	List(ctx context.Context, opts v1.ListOptions) (*v1alpha1.ManagedSeedSetList, error)
	Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error)
	Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.ManagedSeedSet, err error)
	ManagedSeedSetExpansion
}

// managedSeedSets implements ManagedSeedSetInterface
type managedSeedSets struct {
	client rest.Interface
	ns     string
}

// newManagedSeedSets returns a ManagedSeedSets
func newManagedSeedSets(c *SeedmanagementV1alpha1Client, namespace string) *managedSeedSets {
	return &managedSeedSets{
		client: c.RESTClient(),
		ns:     namespace,
	}
}

// Get takes name of the managedSeedSet, and returns the corresponding managedSeedSet object, and an error if there is any.
func (c *managedSeedSets) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha1.ManagedSeedSet, err error) {
	result = &v1alpha1.ManagedSeedSet{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("managedseedsets").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do(ctx).
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of ManagedSeedSets that match those selectors.
func (c *managedSeedSets) List(ctx context.Context, opts v1.ListOptions) (result *v1alpha1.ManagedSeedSetList, err error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	result = &v1alpha1.ManagedSeedSetList{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("managedseedsets").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Do(ctx).
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested managedSeedSets.
func (c *managedSeedSets) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	opts.Watch = true
	return c.client.Get().
		Namespace(c.ns).
		Resource("managedseedsets").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Watch(ctx)
}

// Create takes the representation of a managedSeedSet and creates it.  Returns the server's representation of the managedSeedSet, and an error, if there is any.
func (c *managedSeedSets) Create(ctx context.Context, managedSeedSet *v1alpha1.ManagedSeedSet, opts v1.CreateOptions) (result *v1alpha1.ManagedSeedSet, err error) {
	result = &v1alpha1.ManagedSeedSet{}
	err = c.client.Post().
		Namespace(c.ns).
		Resource("managedseedsets").
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(managedSeedSet).
		Do(ctx).
		Into(result)
	return
}

// Update takes the representation of a managedSeedSet and updates it. Returns the server's representation of the managedSeedSet, and an error, if there is any.
func (c *managedSeedSets) Update(ctx context.Context, managedSeedSet *v1alpha1.ManagedSeedSet, opts v1.UpdateOptions) (result *v1alpha1.ManagedSeedSet, err error) {
	result = &v1alpha1.ManagedSeedSet{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("managedseedsets").
		Name(managedSeedSet.Name).
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(managedSeedSet).
		Do(ctx).
		Into(result)
	return
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *managedSeedSets) UpdateStatus(ctx context.Context, managedSeedSet *v1alpha1.ManagedSeedSet, opts v1.UpdateOptions) (result *v1alpha1.ManagedSeedSet, err error) {
	result = &v1alpha1.ManagedSeedSet{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("managedseedsets").
		Name(managedSeedSet.Name).
		SubResource("status").
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(managedSeedSet).
		Do(ctx).
		Into(result)
	return
}

// Delete takes name of the managedSeedSet and deletes it. Returns an error if one occurs.
func (c *managedSeedSets) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("managedseedsets").
		Name(name).
		Body(&opts).
		Do(ctx).
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *managedSeedSets) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	var timeout time.Duration
	if listOpts.TimeoutSeconds != nil {
		timeout = time.Duration(*listOpts.TimeoutSeconds) * time.Second
	}
	return c.client.Delete().
		Namespace(c.ns).
		Resource("managedseedsets").
		VersionedParams(&listOpts, scheme.ParameterCodec).
		Timeout(timeout).
		Body(&opts).
		Do(ctx).
		Error()
}

// Patch applies the patch and returns the patched managedSeedSet.
func (c *managedSeedSets) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.ManagedSeedSet, err error) {
	result = &v1alpha1.ManagedSeedSet{}
	err = c.client.Patch(pt).
		Namespace(c.ns).
		Resource("managedseedsets").
		Name(name).
		SubResource(subresources...).
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(data).
		Do(ctx).
		Into(result)
	return
}
