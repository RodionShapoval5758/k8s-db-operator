package v1alpha1

import "k8s.io/apimachinery/pkg/runtime"

func (in *ManagedDatabase) DeepCopyInto(out *ManagedDatabase) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	out.Status = in.Status
}

func (in *ManagedDatabase) DeepCopy() *ManagedDatabase {
	if in == nil {
		return nil
	}
	out := new(ManagedDatabase)
	in.DeepCopyInto(out)
	return out
}

func (in *ManagedDatabase) DeepCopyObject() runtime.Object {
	return in.DeepCopy()
}

func (in *ManagedDatabaseList) DeepCopyInto(out *ManagedDatabaseList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		items := make([]ManagedDatabase, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&items[i])
		}
		out.Items = items
	}
}

func (in *ManagedDatabaseList) DeepCopy() *ManagedDatabaseList {
	if in == nil {
		return nil
	}
	out := new(ManagedDatabaseList)
	in.DeepCopyInto(out)
	return out
}

func (in *ManagedDatabaseList) DeepCopyObject() runtime.Object {
	return in.DeepCopy()
}
