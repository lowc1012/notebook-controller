/*
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
	v1alpha1 "github.com/lowc1012/notebook-controller/pkg/apis/notebookcontroller/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// NotebookLister helps list Notebooks.
// All objects returned here must be treated as read-only.
type NotebookLister interface {
	// List lists all Notebooks in the indexer.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha1.Notebook, err error)
	// Notebooks returns an object that can list and get Notebooks.
	Notebooks(namespace string) NotebookNamespaceLister
	NotebookListerExpansion
}

// notebookLister implements the NotebookLister interface.
type notebookLister struct {
	indexer cache.Indexer
}

// NewNotebookLister returns a new NotebookLister.
func NewNotebookLister(indexer cache.Indexer) NotebookLister {
	return &notebookLister{indexer: indexer}
}

// List lists all Notebooks in the indexer.
func (s *notebookLister) List(selector labels.Selector) (ret []*v1alpha1.Notebook, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.Notebook))
	})
	return ret, err
}

// Notebooks returns an object that can list and get Notebooks.
func (s *notebookLister) Notebooks(namespace string) NotebookNamespaceLister {
	return notebookNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// NotebookNamespaceLister helps list and get Notebooks.
// All objects returned here must be treated as read-only.
type NotebookNamespaceLister interface {
	// List lists all Notebooks in the indexer for a given namespace.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha1.Notebook, err error)
	// Get retrieves the Notebook from the indexer for a given namespace and name.
	// Objects returned here must be treated as read-only.
	Get(name string) (*v1alpha1.Notebook, error)
	NotebookNamespaceListerExpansion
}

// notebookNamespaceLister implements the NotebookNamespaceLister
// interface.
type notebookNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all Notebooks in the indexer for a given namespace.
func (s notebookNamespaceLister) List(selector labels.Selector) (ret []*v1alpha1.Notebook, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.Notebook))
	})
	return ret, err
}

// Get retrieves the Notebook from the indexer for a given namespace and name.
func (s notebookNamespaceLister) Get(name string) (*v1alpha1.Notebook, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1alpha1.Resource("notebook"), name)
	}
	return obj.(*v1alpha1.Notebook), nil
}
