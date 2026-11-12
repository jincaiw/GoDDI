# Offline pull response candidates

`Build` and `BuildControl` are pure offline encoders. Every input, including `Head`, is a caller-supplied observation; successful serialization is not authentication, evidence of role or authority, a verified history, or permission to send a response. The caller is responsible for authorization, a consistent trusted read view and trusted anchor. **Never infer DIVERGED or SNAPSHOT_REQUIRED from a database error, timeout or cancellation.**

`BuildControl` accepts four control dispositions. Each requires explicit, locally verifiable evidence supplied by the caller; the encoder itself never reads storage and never classifies an error:

| Disposition | Required evidence | Meaning of the evidence |
|---|---|---|
| `EPOCH_REBASE_REQUIRED` | `After.Epoch != Head.Epoch`, no `Next` | An unverified cross-epoch request position. Does not establish a verified handover. |
| `SCHEMA_INCOMPATIBLE` | `Next`: the actual v1 event at `After.Seq+1`, same epoch, chained to `After`, with a valid digest; client schema list excludes v1 | The next event is real but unreadable by the client. Event validation does not prove storage provenance. |
| `SNAPSHOT_REQUIRED` | `Earliest`: the oldest event still retained in the leader's log, same epoch, `Earliest.Seq > After.Seq+1` and `<= Head.Seq`, valid digest | A gap a batch cannot bridge; the follower needs a snapshot. |
| `DIVERGED` | `Conflict`: the leader's event at `After`'s exact (epoch, seq), valid digest, whose hash differs from `After.Hash` | The follower's claimed digest disagrees with the leader's chain at that position. |

All four candidates contain no events and no `required_bytes`; their `observed_head` is telemetry. Every candidate applies bounded candidate/event counts and protobuf envelope budgets. Inputs must not be concurrently mutated. The encoder clones evidence before verifying it and never mutates caller-owned data.

Supplying well-formed evidence is **not** independent role/epoch/fencing authority, proof of an authoritative tail, or permission to install a snapshot. No production RPC, read path, protected write, snapshot install or authority decision is wired to these encoders here; those remain closed pending independent review. In particular, a caller that hit a storage error has no evidence to supply and cannot construct a valid `SNAPSHOT_REQUIRED` or `DIVERGED` candidate at all.
