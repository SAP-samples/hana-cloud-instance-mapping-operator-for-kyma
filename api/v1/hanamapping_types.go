/*
Copyright 2024.

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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// HANAMappingSpec defines the desired state of HANAMapping
type HANAMappingSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// +required
	BTPOperatorConfigmap NamespacedName `json:"btpOperatorConfigmap"`
	// +required
	AdminAPIAccessSecret NamespacedName `json:"adminAPIAccessSecret"`
	// +required
	Mapping Mapping `json:"mapping"`
}

// HANAMappingStatus defines the observed state of HANAMapping
type HANAMappingStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// +required
	Conditions []metav1.Condition `json:"conditions"`
	// +optional
	MappingID *MappingID `json:"mappingID,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Service Instance ID",type="string",JSONPath=`.spec.mapping.serviceInstanceID`,description="Service Instance ID"
//+kubebuilder:printcolumn:name="Target Namespace",type="string",JSONPath=`.spec.mapping.targetNamespace`,description="Target Namespace"
//+kubebuilder:printcolumn:name="Ready",type="string",JSONPath=`.status.conditions[?(@.type=='Ready')].status`,description="Ready"

// HANAMapping is the Schema for the hanamappings API
type HANAMapping struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HANAMappingSpec   `json:"spec,omitempty"`
	Status HANAMappingStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// HANAMappingList contains a list of HANAMapping
type HANAMappingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HANAMapping `json:"items"`
}

type NamespacedName struct {
	// +required
	Namespace string `json:"namespace"`
	// +required
	Name string `json:"name"`
}

type Mapping struct {
	// +required
	ServiceInstanceID string `json:"serviceInstanceID"`
	// +optional
	TargetNamespace string `json:"targetNamespace,omitempty"`
}

type MappingID struct {
	// +required
	ServiceInstanceID string `json:"serviceInstanceID"`
	// +required
	PrimaryID string `json:"primaryID"`
	// +required
	SecondaryID string `json:"secondaryID"`
}

func init() {
	SchemeBuilder.Register(&HANAMapping{}, &HANAMappingList{})
}
