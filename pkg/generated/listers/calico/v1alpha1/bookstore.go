/*
Copyright The Kubernetes Authors.

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

// Code generated by lister-gen. DO NOT EDIT.

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/listers"
	"k8s.io/client-go/tools/cache"
	v1alpha1 "k8s.io/sample-controller/pkg/apis/calico/v1alpha1"
)

// BookstoreLister helps list Bookstores.
// All objects returned here must be treated as read-only.
type BookstoreLister interface {
	// List lists all Bookstores in the indexer.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha1.Bookstore, err error)
	// Bookstores returns an object that can list and get Bookstores.
	Bookstores(namespace string) BookstoreNamespaceLister
	BookstoreListerExpansion
}

// bookstoreLister implements the BookstoreLister interface.
type bookstoreLister struct {
	listers.ResourceIndexer[*v1alpha1.Bookstore]
}

// NewBookstoreLister returns a new BookstoreLister.
func NewBookstoreLister(indexer cache.Indexer) BookstoreLister {
	return &bookstoreLister{listers.New[*v1alpha1.Bookstore](indexer, v1alpha1.Resource("bookstore"))}
}

// Bookstores returns an object that can list and get Bookstores.
func (s *bookstoreLister) Bookstores(namespace string) BookstoreNamespaceLister {
	return bookstoreNamespaceLister{listers.NewNamespaced[*v1alpha1.Bookstore](s.ResourceIndexer, namespace)}
}

// BookstoreNamespaceLister helps list and get Bookstores.
// All objects returned here must be treated as read-only.
type BookstoreNamespaceLister interface {
	// List lists all Bookstores in the indexer for a given namespace.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha1.Bookstore, err error)
	// Get retrieves the Bookstore from the indexer for a given namespace and name.
	// Objects returned here must be treated as read-only.
	Get(name string) (*v1alpha1.Bookstore, error)
	BookstoreNamespaceListerExpansion
}

// bookstoreNamespaceLister implements the BookstoreNamespaceLister
// interface.
type bookstoreNamespaceLister struct {
	listers.ResourceIndexer[*v1alpha1.Bookstore]
}