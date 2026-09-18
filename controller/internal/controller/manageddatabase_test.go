package controller_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	demov1alpha1 "controller/api/v1alpha1"
	"controller/internal/controller"
	"controller/internal/provisioner"
)

func TestManagedDatabaseSuite(t *testing.T) {
	suite.Run(t, new(ManagedDatabaseSuite))
}

type ManagedDatabaseSuite struct {
	suite.Suite
	provisioner *fakeProvisioner
}

func (s *ManagedDatabaseSuite) SetupTest() {
	s.provisioner = &fakeProvisioner{}
}

func (s *ManagedDatabaseSuite) reconcilerFor(objs ...client.Object) (*controller.ManagedDatabaseReconciler, client.Client) {
	scheme := runtime.NewScheme()
	s.Require().NoError(demov1alpha1.AddToScheme(scheme))

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&demov1alpha1.ManagedDatabase{}).
		WithObjects(objs...).
		Build()

	return &controller.ManagedDatabaseReconciler{
		Client:      c,
		Scheme:      scheme,
		Provisioner: s.provisioner,
	}, c
}

func reconcileRequest(name string) ctrl.Request {
	return ctrl.Request{NamespacedName: client.ObjectKey{Namespace: "default", Name: name}}
}

func (s *ManagedDatabaseSuite) TestAddsFinalizerBeforeAnyCreate() {
	db := &demov1alpha1.ManagedDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "orders", Namespace: "default"},
		Spec:       demov1alpha1.ManagedDatabaseSpec{Engine: "postgres", SizeGB: 20},
	}
	r, c := s.reconcilerFor(db)

	_, err := r.Reconcile(context.Background(), reconcileRequest("orders"))
	s.Require().NoError(err)

	var got demov1alpha1.ManagedDatabase
	s.Require().NoError(c.Get(context.Background(), client.ObjectKeyFromObject(db), &got))
	s.True(controllerutil.ContainsFinalizer(&got, demov1alpha1.FinalizerName))
	s.Equal(0, s.provisioner.createCalls, "must not create before the finalizer is durably persisted")
}

func (s *ManagedDatabaseSuite) TestAmbiguousCreateOrphansWithoutRetry() {
	db := &demov1alpha1.ManagedDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "orders",
			Namespace:  "default",
			Finalizers: []string{demov1alpha1.FinalizerName},
		},
		Spec: demov1alpha1.ManagedDatabaseSpec{Engine: "postgres", SizeGB: 20},
	}
	r, c := s.reconcilerFor(db)
	s.provisioner.createFunc = func(ctx context.Context, name, engine string, sizeGB int) (provisioner.Database, error) {
		return provisioner.Database{}, errors.New("connection reset by peer")
	}

	_, err := r.Reconcile(context.Background(), reconcileRequest("orders"))
	s.Require().NoError(err, "an ambiguous create must not trigger an automatic retry")

	var got demov1alpha1.ManagedDatabase
	s.Require().NoError(c.Get(context.Background(), client.ObjectKeyFromObject(db), &got))
	s.Equal(demov1alpha1.StateOrphaned, got.Status.State)
	s.NotEmpty(got.Status.Message)
	s.Equal(1, s.provisioner.createCalls)
}

func (s *ManagedDatabaseSuite) TestReentrantCreatingNeverRepeatsThePost() {
	db := &demov1alpha1.ManagedDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "orders",
			Namespace:  "default",
			Finalizers: []string{demov1alpha1.FinalizerName},
		},
		Spec:   demov1alpha1.ManagedDatabaseSpec{Engine: "postgres", SizeGB: 20},
		Status: demov1alpha1.ManagedDatabaseStatus{State: demov1alpha1.StateCreating},
	}
	r, c := s.reconcilerFor(db)

	_, err := r.Reconcile(context.Background(), reconcileRequest("orders"))
	s.Require().NoError(err)

	s.Equal(0, s.provisioner.createCalls, "a crash-recovered Creating state must never re-POST")

	var got demov1alpha1.ManagedDatabase
	s.Require().NoError(c.Get(context.Background(), client.ObjectKeyFromObject(db), &got))
	s.Equal(demov1alpha1.StateOrphaned, got.Status.State)
}

func (s *ManagedDatabaseSuite) TestTerminalOrphanIsNeverRecreated() {
	db := &demov1alpha1.ManagedDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "orders",
			Namespace:  "default",
			Finalizers: []string{demov1alpha1.FinalizerName},
		},
		Spec:   demov1alpha1.ManagedDatabaseSpec{Engine: "postgres", SizeGB: 20},
		Status: demov1alpha1.ManagedDatabaseStatus{State: demov1alpha1.StateOrphaned},
	}
	r, _ := s.reconcilerFor(db)

	_, err := r.Reconcile(context.Background(), reconcileRequest("orders"))
	s.Require().NoError(err)
	s.Equal(0, s.provisioner.createCalls, "a terminal Orphaned CR with no DatabaseID must not fall through to create")
}

func (s *ManagedDatabaseSuite) TestDeleteGivesUpAfterDeadline() {
	past := metav1.NewTime(time.Now().Add(-10 * time.Minute))
	db := &demov1alpha1.ManagedDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "orders",
			Namespace:         "default",
			Finalizers:        []string{demov1alpha1.FinalizerName},
			DeletionTimestamp: &past,
		},
		Spec:   demov1alpha1.ManagedDatabaseSpec{Engine: "postgres", SizeGB: 20},
		Status: demov1alpha1.ManagedDatabaseStatus{State: demov1alpha1.StateProvisioning, DatabaseID: "db-1234"},
	}
	r, c := s.reconcilerFor(db)
	s.provisioner.deleteFunc = func(ctx context.Context, id string) error {
		return errors.New("service unavailable")
	}

	_, err := r.Reconcile(context.Background(), reconcileRequest("orders"))
	s.Require().NoError(err)

	var got demov1alpha1.ManagedDatabase
	getErr := c.Get(context.Background(), client.ObjectKeyFromObject(db), &got)
	if getErr == nil {
		s.False(controllerutil.ContainsFinalizer(&got, demov1alpha1.FinalizerName), "finalizer must be gone once the deadline has passed")
	} else {
		s.True(apierrors.IsNotFound(getErr), "object should be fully removed once its last finalizer clears")
	}
	s.GreaterOrEqual(s.provisioner.deleteCalls, 1)
}
