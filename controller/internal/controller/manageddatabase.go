package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlpkg "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	demov1alpha1 "controller/api/v1alpha1"
	"controller/internal/provisioner"
)

const (
	pollInterval            = 10 * time.Second
	deletionDeadline        = 5 * time.Minute
	maxConcurrentReconciles = 5
)

type Provisioner interface {
	Create(ctx context.Context, name, engine string, sizeGB int) (provisioner.Database, error)
	Get(ctx context.Context, id string) (provisioner.Database, error)
	Delete(ctx context.Context, id string) error
}

type ManagedDatabaseReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	Provisioner Provisioner
}

func (r *ManagedDatabaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var db demov1alpha1.ManagedDatabase
	if err := r.Get(ctx, req.NamespacedName, &db); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if !db.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &db)
	}

	if !controllerutil.ContainsFinalizer(&db, demov1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(&db, demov1alpha1.FinalizerName)
		if err := r.Update(ctx, &db); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	switch db.Status.State {
	case demov1alpha1.StateReady, demov1alpha1.StateFailed, demov1alpha1.StateOrphaned:
		return ctrl.Result{}, nil
	}

	if db.Status.DatabaseID == "" {
		return r.reconcileCreate(ctx, &db)
	}
	return r.reconcilePoll(ctx, &db)
}

func (r *ManagedDatabaseReconciler) reconcileCreate(ctx context.Context, db *demov1alpha1.ManagedDatabase) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	if db.Status.State == demov1alpha1.StateCreating {
		return ctrl.Result{}, r.setTerminal(ctx, db, demov1alpha1.StateOrphaned,
			"a create request may have been sent before a restart; its outcome is unknown, so it was not retried")
	}

	db.Status.State = demov1alpha1.StateCreating
	db.Status.Message = ""
	if err := r.Status().Update(ctx, db); err != nil {
		return ctrl.Result{}, err
	}

	created, err := r.Provisioner.Create(ctx, db.Name, db.Spec.Engine, db.Spec.SizeGB)
	switch {
	case err == nil:
		return ctrl.Result{}, r.commitDatabaseID(ctx, db, created)

	case errors.Is(err, provisioner.ErrUnavailable):
		log.Info("create rejected with 503 before processing, will retry", "name", db.Name)
		db.Status.State = demov1alpha1.StatePending
		if uerr := r.Status().Update(ctx, db); uerr != nil {
			return ctrl.Result{}, uerr
		}
		return ctrl.Result{}, err

	default:
		log.Error(err, "create outcome unknown, marking orphaned", "name", db.Name)
		return ctrl.Result{}, r.setTerminal(ctx, db, demov1alpha1.StateOrphaned,
			fmt.Sprintf("create request failed ambiguously and was not retried: %v", err))
	}
}

func (r *ManagedDatabaseReconciler) commitDatabaseID(ctx context.Context, db *demov1alpha1.ManagedDatabase, created provisioner.Database) error {
	key := client.ObjectKeyFromObject(db)
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		var latest demov1alpha1.ManagedDatabase
		if err := r.Get(ctx, key, &latest); err != nil {
			return err
		}
		latest.Status.DatabaseID = created.ID
		latest.Status.State = demov1alpha1.StateProvisioning
		latest.Status.Message = ""
		return r.Status().Update(ctx, &latest)
	})
}

func (r *ManagedDatabaseReconciler) reconcilePoll(ctx context.Context, db *demov1alpha1.ManagedDatabase) (ctrl.Result, error) {
	found, err := r.Provisioner.Get(ctx, db.Status.DatabaseID)
	if err != nil {
		if errors.Is(err, provisioner.ErrNotFound) {
			return ctrl.Result{}, r.setTerminal(ctx, db, demov1alpha1.StateOrphaned,
				fmt.Sprintf("external database %s is gone (deleted, or the provisioner restarted)", db.Status.DatabaseID))
		}
		return ctrl.Result{}, err
	}

	newState, newEndpoint := mapExternalState(found)

	if newState != db.Status.State || newEndpoint != db.Status.Endpoint {
		db.Status.State = newState
		db.Status.Endpoint = newEndpoint
		if err := r.Status().Update(ctx, db); err != nil {
			return ctrl.Result{}, err
		}
	}

	if newState == demov1alpha1.StateProvisioning {
		return ctrl.Result{RequeueAfter: pollInterval}, nil
	}
	return ctrl.Result{}, nil
}

func mapExternalState(found provisioner.Database) (state, endpoint string) {
	switch found.State {
	case provisioner.StateReady:
		return demov1alpha1.StateReady, found.Endpoint
	case provisioner.StateFailed:
		return demov1alpha1.StateFailed, ""
	default:
		return demov1alpha1.StateProvisioning, ""
	}
}

func (r *ManagedDatabaseReconciler) reconcileDelete(ctx context.Context, db *demov1alpha1.ManagedDatabase) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	if !controllerutil.ContainsFinalizer(db, demov1alpha1.FinalizerName) {
		return ctrl.Result{}, nil
	}

	if db.Status.DatabaseID != "" {
		if err := r.Provisioner.Delete(ctx, db.Status.DatabaseID); err != nil {
			if time.Since(db.DeletionTimestamp.Time) > deletionDeadline {
				log.Error(err, "giving up on external delete after deadline, database is leaked",
					"databaseID", db.Status.DatabaseID)
				return ctrl.Result{}, r.removeFinalizer(ctx, db)
			}
			return ctrl.Result{RequeueAfter: pollInterval}, nil
		}
	} else if db.Status.State == demov1alpha1.StateOrphaned {
		log.Info("deleting CR whose external database is unreachable", "message", db.Status.Message)
	}

	return ctrl.Result{}, r.removeFinalizer(ctx, db)
}

func (r *ManagedDatabaseReconciler) removeFinalizer(ctx context.Context, db *demov1alpha1.ManagedDatabase) error {
	controllerutil.RemoveFinalizer(db, demov1alpha1.FinalizerName)
	return r.Update(ctx, db)
}

func (r *ManagedDatabaseReconciler) setTerminal(ctx context.Context, db *demov1alpha1.ManagedDatabase, state, message string) error {
	db.Status.State = state
	db.Status.Message = message
	return r.Status().Update(ctx, db)
}

func (r *ManagedDatabaseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&demov1alpha1.ManagedDatabase{}).
		WithOptions(ctrlpkg.Options{MaxConcurrentReconciles: maxConcurrentReconciles}).
		Complete(r)
}
