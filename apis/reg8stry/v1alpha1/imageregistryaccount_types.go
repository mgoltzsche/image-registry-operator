/*
Copyright 2021 Max Goltzsche.

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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ImageRegistryAccountSpec defines the desired state of ImageRegistryAccount
type ImageRegistryAccountSpec struct {
	// Password bcrypt hashed password
	Password string `json:"password"`
	// Labels to match against authorization rules
	Labels map[string][]string `json:"labels,omitempty"`
	// TTL time to live for the account
	TTL *metav1.Duration `json:"ttl,omitempty"`
}

// ImageRegistryAccountStatus defines the observed state of ImageRegistryAccount
type ImageRegistryAccountStatus struct {
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// ImageRegistryAccount is the Schema for the imageregistryaccounts API
type ImageRegistryAccount struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ImageRegistryAccountSpec   `json:"spec,omitempty"`
	Status ImageRegistryAccountStatus `json:"status,omitempty"`
}

func (a *ImageRegistryAccount) Expired() bool {
	if a.Spec.TTL == nil {
		return false
	}
	return time.Now().After(a.CreationTimestamp.Add(a.Spec.TTL.Duration))
}

//+kubebuilder:object:root=true

// ImageRegistryAccountList contains a list of ImageRegistryAccount
type ImageRegistryAccountList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ImageRegistryAccount `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ImageRegistryAccount{}, &ImageRegistryAccountList{})
}
