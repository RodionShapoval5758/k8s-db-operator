package controller_test

import (
	"context"
	"errors"

	"controller/internal/provisioner"
)

// fakeProvisioner implements controller.Provisioner. Each method errors
// loudly by default rather than returning a zero value, so a test that
// expects zero calls fails clearly if the reconciler calls it anyway.
type fakeProvisioner struct {
	createFunc func(ctx context.Context, name, engine string, sizeGB int) (provisioner.Database, error)
	getFunc    func(ctx context.Context, id string) (provisioner.Database, error)
	deleteFunc func(ctx context.Context, id string) error

	createCalls int
	deleteCalls int
}

func (f *fakeProvisioner) Create(ctx context.Context, name, engine string, sizeGB int) (provisioner.Database, error) {
	f.createCalls++
	if f.createFunc == nil {
		return provisioner.Database{}, errors.New("fakeProvisioner: unexpected Create call")
	}
	return f.createFunc(ctx, name, engine, sizeGB)
}

func (f *fakeProvisioner) Get(ctx context.Context, id string) (provisioner.Database, error) {
	if f.getFunc == nil {
		return provisioner.Database{}, errors.New("fakeProvisioner: unexpected Get call")
	}
	return f.getFunc(ctx, id)
}

func (f *fakeProvisioner) Delete(ctx context.Context, id string) error {
	f.deleteCalls++
	if f.deleteFunc == nil {
		return errors.New("fakeProvisioner: unexpected Delete call")
	}
	return f.deleteFunc(ctx, id)
}
