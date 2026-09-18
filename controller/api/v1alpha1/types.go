package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	GroupVersion  = schema.GroupVersion{Group: "demo.example.com", Version: "v1alpha1"}
	SchemeBuilder = runtime.NewSchemeBuilder(func(s *runtime.Scheme) error {
		metav1.AddToGroupVersion(s, GroupVersion)
		s.AddKnownTypes(GroupVersion, &ManagedDatabase{}, &ManagedDatabaseList{})
		return nil
	})
	AddToScheme = SchemeBuilder.AddToScheme
)

const FinalizerName = "demo.example.com/database-cleanup"

const (
	StatePending      = "Pending"      // not yet acted on
	StateCreating     = "Creating"     // a create request is in flight; outcome not yet known
	StateProvisioning = "Provisioning" // external database is being provisioned
	StateReady        = "Ready"        // terminal: database is ready, endpoint populated
	StateFailed       = "Failed"       // terminal: external provisioning failed; still deletable
	StateOrphaned     = "Orphaned"     // terminal: create outcome unknown, or database unreachable; needs a human
)

type ManagedDatabaseSpec struct {
	Engine string `json:"engine"`
	SizeGB int    `json:"sizeGB"`
}

type ManagedDatabaseStatus struct {
	State      string `json:"state,omitempty"`
	DatabaseID string `json:"databaseID,omitempty"`
	Endpoint   string `json:"endpoint,omitempty"`
	Message    string `json:"message,omitempty"`
}

type ManagedDatabase struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ManagedDatabaseSpec   `json:"spec"`
	Status ManagedDatabaseStatus `json:"status,omitempty"`
}

type ManagedDatabaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ManagedDatabase `json:"items"`
}
