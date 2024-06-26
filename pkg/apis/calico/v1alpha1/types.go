/*
Copyright 2017 The Kubernetes Authors.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Bookstore is a specification for a Bookstore resource
type Bookstore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BookstoreSpec   `json:"spec"`
	Status BookstoreStatus `json:"status"`
}

// BookstoreSpec is the spec for a Bookstore resource
type BookstoreSpec struct {
	EnvAdminUsername    string `json:"envAdminUsername"`
	EnvAdminPassword    string `json:"envAdminPassword"`
	EnvJWTSECRET        string `json:"envJWTSECRET"`
	DeploymentImageName string `json:"deploymentImageName"`
	DeploymentImageTag  string `json:"deploymentImageTag"`
	ImagePullPolicy     string `json:"imagePullPolicy"`
	DeploymentName      string `json:"deploymentName"`
	Replicas            *int32 `json:"replicas"`
	ServiceName         string `json:"serviceName"`
	ServiceType         string `json:"serviceType"`
	ContainerPort       int32  `json:"containerPort"`
	NodePort            int32  `json:"nodePort"`
	TargetPort          int32  `json:"targetPort"`
}

// BookstoreStatus is the status for a Bookstore resource
type BookstoreStatus struct {
	AvailableReplicas int32 `json:"availableReplicas"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// BookstoreList is a list of Bookstore resources
type BookstoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Bookstore `json:"items"`
}

func (bookstore *Bookstore) GetSelectorLabels() map[string]string {
	return map[string]string{
		"app":        bookstore.Name + "-app",
		"controller": bookstore.Name + "-customController1",
	}
}
