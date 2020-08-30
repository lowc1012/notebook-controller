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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Notebook is a specification for a Notebook resource
type Notebook struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NotebookSpec   `json:"spec"`
	Status NotebookStatus `json:"status"`
}

// NotebookSpec is the spec for a Notebook resource
type NotebookSpec struct {
	StatefulsetName string               `json:"statefulsetName"`
	Replicas        *int32               `json:"replicas"`
	Template        NotebookTemplateSpec `json:"template,omitempty"`
}

type NotebookTemplateSpec struct {
	Spec corev1.PodSpec `json:"spec,omitempty"`
}

// NotebookStatus is the status for a Notebook resource
type NotebookStatus struct {
	ReadyReplicas int32 `json:"readyReplicas"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// NotebookList is a list of Notebook resources
type NotebookList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items 			[]Notebook `json:"items"`
}
