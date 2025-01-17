package v1alpha1

import (
	recon "github.com/matrixorigin/matrixone-operator/runtime/pkg/reconciler"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DNSetSpec struct {
	DNSetBasic `json:",inline"`

	// +optional
	Overlay *Overlay `json:"overlay,omitempty"`
}

type DNSetBasic struct {
	PodSet `json:",inline"`

	// CacheVolume is the desired local cache volume for DNSet,
	// node storage will be used if not specified
	// +optional
	CacheVolume *Volume `json:"cacheVolume,omitempty"`
}

// TODO: figure out what status should be exposed
type DNSetStatus struct {
	ConditionalStatus `json:",inline"`
}

type DNSetDeps struct {
	LogSetRef `json:",inline"`
}

// +kubebuilder:object:root=true
// +kubebuilder:printcolumn:name="Image",type="string",JSONPath=".spec.image"
// +kubebuilder:printcolumn:name="Replicas",type="integer",JSONPath=".spec.replicas"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// A DNSet is a resource that represents a set of MO's DN instances
// +kubebuilder:subresource:status
type DNSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DNSetSpec   `json:"spec,omitempty"`
	Deps   DNSetDeps   `json:"deps,omitempty"`
	Status DNSetStatus `json:"status,omitempty"`
}

func (d *DNSet) GetDependencies() []recon.Dependency {
	var deps []recon.Dependency
	if d.Deps.LogSet != nil {
		deps = append(deps, &recon.ObjectDependency[*LogSet]{
			ObjectRef: d.Deps.LogSet,
			ReadyFunc: func(l *LogSet) bool {
				return recon.IsReady(&l.Status)
			},
		})
	}
	return deps
}

func (d *DNSet) SetCondition(condition metav1.Condition) {
	d.Status.SetCondition(condition)
}

func (d *DNSet) GetConditions() []metav1.Condition {
	return d.Status.GetConditions()
}

//+kubebuilder:object:root=true

// DNSetList contains a list of DNSet
type DNSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DNSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DNSet{}, &DNSetList{})
}
