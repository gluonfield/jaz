# Restart continuation: evidence and repair design

2026-10-25. Investigation only, based on v193 `f297e93b`. Lifecycle implementation awaits independent design review.

## Finding

The installer incident crossed **two recovery boundaries**. Jaz resumed an acknowledged task, then restarted during the new continuation before its acknowledgement. The continuation replaced the original confirmed turn. The next startup correctly refused to replay that unresolved delivery, but reported that the agent had never confirmed the task.

This is a task-versus-delivery identity problem. A normal restart of confirmed work still continues once. Removing the unconfirmed-delivery guard would violate the established safety contract.

## Concrete incident evidence

Thread: `20261025T142120-0a2546c4` (`installer-appearance`), Claude session `b23b4145-8cc3-4c7a-be44-df7eaaa63f42`. The supplied Codex screenshot establishes the displayed error; it does not identify its thread. The installer case independently reproduces the same symptom on Claude.

| UTC | Evidence |
| --- | --- |
| 14:30:42.478 | Native assistant describes writing headless before/after previews. Same timestamp in Jaz's persisted event. |
| 14:31:20.974–.975 | Native assistant reports the AppKit black-background investigation; persisted in Jaz. These match the reported progress entries; current physical SQLite event sequences are 260 and 285. |
| 14:31:30.118 | Desktop requests shutdown of the development backend. |
| 14:31:35–38 | Packaged backend starts, then is stopped again. |
| 14:33:09.973 | Another packaged backend starts. Installer restoration metadata is saved at 14:33:11–12. |
| 14:33:12.498 | **Native history records Jaz's `Continue from where you left off.`** Its subsequent `api-request` record is timestamped 14:33:12.542. |
| 14:33:14.472 | Desktop requests another shutdown. |
| 14:33:15.372 | Jaz persists `cancelled` with **`server_shutdown`**, rather than a user stop. |
| 14:33:23.812–24.316 | Next packaged backend starts and becomes ready, matching the reported error time. |
| 14:43:31 / 14:48:01 | A later explicit task follow-up starts and completes. The current installer thread is idle; its incident turn JSON has been cleared. |

The native continuation record proves the original acknowledged task reached recovery. It does not supply a generic, uniquely correlated receipt API for automated recovery.

Limits: the incident's original turn JSON and backend stderr were not retained. Desktop `main.log` records process transitions, not backend confirmation-write diagnostics. The exact 14:33:24 error timestamp comes from the supplied incident observation. No claim of an original confirmation-write failure is supported.

Installed app metadata reports `0.0.193`, Go 1.26.9, revision `f297e93b23a2151f7a2ce1dbe5731bc933cff278`, with `vcs.modified=true`. The checkout/base is the exact tagged source; the dirty build flag prevents claiming pristine binary/source equality. The currently running app is a development process, so it is separate from the packaged incident process.

## Code path

1. `acp/recovery.go:23–26` refuses a current prompt marked `sending`.
2. Confirmed work takes the continuation path at `recovery.go:31–69`.
3. `acp/send.go:228–241` builds a **new** turn with a new `StartedAt` and prepared `TurnPrompt`.
4. `claimTurn` calls `StartSessionTurn`; `sqlite/queries/threaddb/turns.sql:1` replaces the entire turn JSON.
5. Transport records the continuation as `sending` before writing. First primary output or its prompt response would confirm it.
6. Shutdown preserves that current turn. The next startup sees the unresolved **continuation**, but has no typed record of its acknowledged origin. `FailInterruptedTurn` uses the task-unconfirmed error and terminal completion path.

An independent possible cause remains confirmation-write failure: `prompt_delivery.go:63–66` suppresses further confirmation attempts after the first attempt, including a failed one. That is documented fail-closed behavior, and is not demonstrated as the installer cause.

## Reproduction and safety controls

Go **1.26.3**, fake ACP subprocesses, real transport and production SQLite/recovery/completion. No provider launches, credentials, live models, app restart or production-state edits.

| Boundary at second shutdown | Result on next startup, both agent policies |
| --- | --- |
| Continuation prepared, transport never attempted | Sends the prepared continuation once; settles idle. |
| Continuation `sending`, before transport write | No replay; task-unconfirmed error. |
| Continuation read by fake, before output | No replay; same error. |
| Continuation already appended an external-effect witness, before output | No replay; exactly one effect; same error. |
| Continuation durably confirmed by primary output | Continues once; settles idle. |

All ten diagnostic cases reproduce those results. Each begins with an original durably confirmed task; the diagnostic verifies that recovery changes its start timestamp and replaces its prompt with the continuation. Two recovery sweeps verify once-only dispatch.

**Negative control:** an isolated Go overlay removes the unresolved-delivery guard. Both acted cases then append `effect\neffect\n`; both fail at the duplicate-effect assertion. This models a permitted transport interleaving, not measured provider duplicate rates. An earlier control build failed on its now-unused import; the corrected control compiles and fails on the intended behavioral assertion.

Another **132 leaf cases pass** on unmodified production code: original prepared/sending/confirmed boundaries, write failures, queued claims, bot/worker recovery, confirmation generation guards, Stopped tasks, obsolete wake/result retirement and shutdown during recovery. Initial exploratory runs used the shell's Go 1.27.3; all reported acceptance evidence was rerun explicitly with Go 1.26.3.

At the bounded 14:58:38 read-only production snapshot: seven running turns were confirmed, three were sending, and six Codex threads retained this error. None of those three current sending turns had primary message/thought/tool events since its current start. These observations support the distinction between old transcript progress and acknowledgement of the current delivery; they do not classify every historical error.

## Minimal safe repair proposed for review

Preserve **acknowledged task origin** separately from the **current delivery attempt**, while retaining the existing delivery gate.

1. Add one optional typed origin to the durable turn, provisionally `ContinuationOf` (the original acknowledged turn's start timestamp). Set it only when recovering explicitly confirmed work; carry it through later attempts. Missing legacy evidence stays unknown. Use the existing current `StartedAt` for stale-write protection.
2. Pass that origin through internal `sendOptions` and the existing atomic turn-start write. Keep current `Prompt.State` as the delivery state of the actual request. Never infer origin from prompt text, old assistant output, a provider name, or string matching `Continue`.
3. When a continuation is sending/unconfirmed, retain the no-replay result path and open commitment, but report the accurate reason: **“Jaz restarted before the agent confirmed the continuation. Earlier task progress is saved.”** Distinguish this from an original task never confirmed. Preserve this reason through parent delivery; keep it terminal for automatic startup recovery so it cannot loop.
4. Leave queue claims, original unsent payload delivery, compaction, retries, routing, native steering and provider restore semantics intact. Every recovery/admission path continues checking Stopped ownership first. Deliberately stopped work stays dormant until explicit reactivation with fresh instructions.

This bounded repair preserves the known task lineage and corrects the false first-delivery diagnosis. **It does not make an unresolved continuation safe to replay.** A promise of automatic continuation across arbitrary repeated restarts requires a provider-owned, uniquely correlated durable acceptance/continuation contract, exposed through a capability at the protocol boundary. Current Jaz restore responses do not expose such a receipt. This investigation proposes no provider patch or history-matching fallback.

Reject designs that copy `confirmed` onto an unacknowledged continuation, bypass the gate because any older output exists, replay an original payload, or treat a generic continuation as an idempotent control operation. The acted control demonstrates why the last assumption is unsafe.

## Review and implementation gates

The independent reviewer must decide whether the bounded lineage/reason correction satisfies the immediate repair scope, or whether additional automatic liveness requires a separate provider-capability design. The safety contract prevents claiming that the latter is solved by a local marker change.

After design acceptance: promote the repeated-restart cases into normal regression coverage with the approved outcomes; keep an effect-before-output assertion, prepared delivery, once-only recovery, failed confirmation, stale-generation, native-steer, queued-action and dormant-Stopped coverage. Run proportionate backend checks and race checks. Apply `/Users/wins/.jaz/skills/thermo-nuclear-code-quality-review/SKILL.md` to the implementation and resolve findings before handoff. Before release, measure the mandated native parity dimensions; this investigation performs no live parity probes.

## Evidence files

All paths below are under this worktree's gitignored `runs/restart-agent-continuation/`:

- `production.json`: bounded read-only transaction, current markers and incident event timestamps.
- `desktop-restart-timeline.log`, `native-incident-excerpts.json`, `installed-version.txt`: selected local evidence, excluding auth data.
- `contracts-go126.log`, `repeated-restart-go126.log`, `unsafe-control-go126.log`: accepted contract, diagnosis and expected-negative-control results.
- `continuation_boundary_test.go` / `overlay.json`: executable diagnostic against unchanged production packages. `unsafe-overlay.json` and its two replacement files define the negative control. They are experiments, not inactive product tests.

Commands from `backend/`, with `TMPDIR` set to the worktree's `runs/restart-agent-continuation/tmp`:

```sh
GOTOOLCHAIN=go1.26.3 go test -overlay ../runs/restart-agent-continuation/overlay.json ./internal/acp -run '^TestContinuationRestartDiagnostic$' -count=1 -v
GOTOOLCHAIN=go1.26.3 go test -overlay ../runs/restart-agent-continuation/unsafe-overlay.json ./internal/acp -run '^TestContinuationRestartDiagnostic/(codex|claude)/acted$' -count=1 -v
```

The second command must fail on duplicate effects. No lifecycle implementation, integration, release or live repair has been made.
