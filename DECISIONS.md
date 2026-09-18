a. The API can create a database and then lose the response. How does your controller avoid creating a duplicate on the next reconcile? What risk remains that you did not eliminate?

- Before every POST, the controller writes state=Creating to the CR.
Seeing that state again on the next reconcile means the previous attempt's 
outcome is unknown, and a database may have been created.
The risk is an untracked database with no id in the CR — the only way to even
find it is by manually matching its name in the provisioner's debug endpoint,
a test-only path a real API wouldn't offer.


b. What would you change about this external API to make your job easier? How would you argue for it to the team that owns it and has its own backlog?

- As the primary dedup solution, I would suggest adding an idempotency key to the POST 
request so that each database instance is associated with a unique key, and whenever the client asks to create a database with an existing idempotency key, it would just return the existing one.
I would tell the team this is a major problem for them too, not just for me: 
leaked databases have no owner or timestamp, so they can't even tell which ones are safe
to clean up, and every consumer of this API hits the same risk. Such a simple fix - one
dedup map keyed by the idempotency token - would save a lot of work for everyone going 
forward.


c. Deleting the external database can keep failing. Do you block deletion of the custom resource indefinitely, or give up at some point and let it go? Justify the choice you made.

- I give up after 5 minutes past deletionTimestamp rather than blocking forever -
logging the leaked id and removing the finalizer so the CR isn't stuck in 
Terminating indefinitely. At a 10% transient failure rate and a 10-second retry 
interval, that's ~30 attempts, so the deadline only triggers when something is persistently broken, not just flaky.

d. Someone edits sizeGB after the database exists. The API has no resize operation. What does your controller do, and what does the user see?

- The controller ignores the edit - a ManagedDatabase reaches Ready,
Reconcile returns before ever looking at spec.sizeGB again,
and there's no resize call to make since the provisioning API has none. 
The user sees something mildly misleading: kubectl gets SIZE column reads
straight from spec, so it updates immediately next to STATE: Ready,
looking like the resize succeeded when the real database is untouched -
nothing in status currently surfaces that drift, which was left out
deliberately.

e. What did you deliberately leave out, and what would you do next?
- No test for the concurrency/conflict-mutex claim
- CRD schema polish: no enum, no field descriptions, no shortNames, no immutability rules
- Drift ignored on engine too, not just sizeGB