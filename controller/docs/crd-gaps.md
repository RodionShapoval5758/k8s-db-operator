# CRD known gaps

The `ManagedDatabase` CRD (`config/crd.yaml`) is intentionally minimal — sized for
a controller that only does create → poll → ready → delete. Everything below is
deferred by choice, not missed, and is not required for a minimally working
version.

**Resolved in the minimal-controller pass:** the create-in-flight ambiguity
(requirement 3) is now handled without any CRD change — a `Creating` status
state is written durably *before* the POST, and any ambiguous outcome (a
crash, a lost response, a transport error) is fail-closed into a terminal
`Orphaned` state rather than retried. `status.message` was added to carry the
explanation, visible via `kubectl describe` and `kubectl get -o yaml`.

## 1. No `status.conditions`

`status.message` is a one-shot string, not a history — it gets overwritten on
every transition, so `kubectl describe` shows only the *current* reason, not
how the CR got there. Adding `Conditions []metav1.Condition` would fix this
but makes `ManagedDatabaseStatus` reference-typed, so the hand-written
`api/v1alpha1/zz_deepcopy.go` would need a real `DeepCopyInto` for it instead
of shallow struct assignment — this is exactly the trigger
`docs/adr/0001-manual-scaffolding-over-kubebuilder.md` flags for revisiting
the hand-written deep-copy.

## 2. No `status.observedGeneration`

Needed to express "spec says 50GB, the real database is still 20GB" once
someone edits `sizeGB` after creation. Today the controller silently ignores
such edits (see `README.md`), which is the intended v1 behavior — but nothing
surfaces that ignoring in `status`.

## 3. `status.state` is an unconstrained string

The controller's own state machine (`Pending`/`Creating`/`Provisioning`/
`Ready`/`Failed`/`Orphaned`, in `api/v1alpha1/types.go`) has no `enum` in the
schema, so a typo or a stray manual edit is not rejected at admission time.
An `enum` would pin it down.

## 4. No `description:` on any field

`kubectl explain manageddatabase.spec.sizeGB` currently returns nothing
useful.

## 5. Deferred decisions

- `shortNames: [mdb]`.
- Whether to constrain `engine` with an `enum` — the provisioner validates
  nothing and accepts any non-empty string.
- Whether `sizeGB`/`engine` should become immutable via
  `x-kubernetes-validations` (CEL) instead of silently ignoring edits.

## Standing rule

Any status field the Go code writes but `crd.yaml` doesn't declare is **silently
pruned** by the API server. `api/v1alpha1/types.go` and `config/crd.yaml` must
always change together.
