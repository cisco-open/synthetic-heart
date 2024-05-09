// Copyright 2024 Cisco Systems, Inc. and its affiliates
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type Timeouts struct {
	Init   string `json:"init,omitempty"`
	Run    string `json:"run,omitempty"`
	Finish string `json:"finish,omitempty"`
}

// SyntheticTestSpec defines the desired state of SyntheticTest
type SyntheticTestSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Plugin              string            `json:"plugin" yaml:"plugin"`
	Node                string            `json:"node,omitempty" yaml:"node,omitempty"`
	PodLabelSelector    map[string]string `json:"podLabelSelector,omitempty" yaml:"podLabelSelector,omitempty"`
	DisplayName         string            `json:"displayName,omitempty" yaml:"displayName,omitempty"`
	Description         string            `json:"description,omitempty" yaml:"description,omitempty"`
	Importance          string            `json:"importance,omitempty" yaml:"importance,omitempty"`
	Repeat              string            `json:"repeat" yaml:"repeat"`
	DependsOn           []string          `json:"dependsOn,omitempty" yaml:"dependsOn,omitempty"`
	Timeouts            *Timeouts         `json:"timeouts,omitempty" yaml:"timeouts,omitempty"`
	PluginRestartPolicy string            `json:"pluginRestartPolicy,omitempty" yaml:"pluginRestartPolicy,omitempty"`
	LogWaitTime         string            `json:"logWaitTime,omitempty" yaml:"logWaitTime,omitempty"`
	Config              string            `json:"config,omitempty" yaml:"config,omitempty"`
}

// SyntheticTestStatus defines the observed state of SyntheticTest
type SyntheticTestStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Deployed bool   `json:"deployed,omitempty"`
	Agent    string `json:"agent,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// SyntheticTest is the Schema for the synthetictests API
type SyntheticTest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SyntheticTestSpec   `json:"spec,omitempty"`
	Status SyntheticTestStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SyntheticTestList contains a list of SyntheticTest
type SyntheticTestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SyntheticTest `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SyntheticTest{}, &SyntheticTestList{})
}
