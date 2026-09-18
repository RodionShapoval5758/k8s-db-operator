# CRD known gaps

The `ManagedDatabase` CRD (`config/crd.yaml`) is intentionally minimal — sized for
a controller that only does create → poll → ready → delete. Everything below is
deferred by choice, not missed, and is not required for a minimally working
version.

## 1. No `status.conditions` / `message` / `reason`

`status.state` alone cannot distinguish "healthily retrying a flaky provisioner
call" from "wedged forever" — both look identical in `kubectl describe`. Adding
`Conditions []metav1.Condition` would fix this but makes `ManagedDatabaseStatus`
reference-typed, so the hand-written `api/v1alpha1/zz_deepcopy.go` would need a
real `DeepCopyInto` for it instead of shallow struct assignment — this is exactly
the trigger `docs/adr/0001-manual-scaffolding-over-kubebuilder.md` flags for
revisiting the hand-written deep-copy. A `status.message string` field is the
cheaper alternative that keeps `Status` reference-free.

## 2. No `status.observedGeneration`

Needed to express "spec says 50GB, the real database is still 20GB" once someone
edits `sizeGB` after creation (the provisioner has no resize endpoint).

## 3. No home for a create-in-flight claim

`status.databaseID` can only be written *after* `POST /databases` returns, but
~15% of successful creates lose their response. After a restart, an empty
`databaseID` is ambiguous: either no create happened, or one did and the id is
already lost. Candidates: `status.createAttemptedAt` / `status.createAttempts`,
or a `demo.example.com/*` annotation (annotations aren't schema-pruned). Also
open: whether the only surviving copy of a non-reconstructable external id
belongs in `status` at all, given `GET /databases` is `501` and there is no
lookup by name.

## 4. `status.state` is an unconstrained string

The provisioner emits `READY`; the README's expected `kubectl get` output shows
`Ready`. The CR will also need states the provisioner doesn't have (e.g.
`Deleting`). An `enum` would pin the state machine in the schema instead of
leaving it implicit in controller code.

## 5. No `description:` on any field

`kubectl explain manageddatabase.spec.sizeGB` currently returns nothing useful.

## 6. `endpoint` isn't in any printer column

The task requires showing "how to connect" once ready. Adding `endpoint` with
`priority: 1` would surface it under `kubectl get -o wide` without disturbing the
README's exact expected column layout.

## 7. Deferred decisions

- `shortNames: [mdb]`.
- Whether to constrain `engine` with an `enum` — the provisioner validates
  nothing and accepts any non-empty string.
- Whether `sizeGB`/`engine` should become immutable via
  `x-kubernetes-validations` (CEL) instead of allowing edits and reporting drift.

## Standing rule

Any status field the Go code writes but `crd.yaml` doesn't declare is **silently
pruned** by the API server. `api/v1alpha1/types.go` and `config/crd.yaml` must
always change together.
