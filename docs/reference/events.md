# Run events

`cidx run` can publish what it is doing on NATS, so a caller follows a run
without parsing its text output: which phase and container is running, how each
ended, and how long it took.

Publishing is observation. It never changes the output of the run, its exit
code, or whether it starts.

## Turning it on

| Option           | Environment         | Default           | Meaning                                    |
| ---------------- | ------------------- | ----------------- | ------------------------------------------ |
| `--nats-url`     | `CIDX_NATS_URL`     | none              | NATS server, e.g. `nats://localhost:4222`  |
| `--nats-subject` | `CIDX_NATS_SUBJECT` | `cidx.run.events` | Subject every event is published on        |
| `--run-id`       | `CIDX_RUN_ID`       | generated         | Identifier shared by every event of a run  |

```bash
CIDX_NATS_URL=nats://localhost:4222 CIDX_RUN_ID=build-1842 cidx run ci
```

Without a NATS address nothing changes: same output, same exit codes, no
connection attempted. `--dry-run` publishes nothing either, since nothing runs.

The URL may carry credentials (`nats://user:pass@host:4222`). They are used to
connect and written nowhere: not in the output, not in the events.

## When the bus is down

If the server cannot be reached, or refuses a publication, the run continues and
its exit code is unchanged. One warning on stderr says so, once:

```text
Warning: NATS unavailable, run events are not published
```

The warning names the failure, never the address.

## Events

One JSON document per message, all on the same subject. Fields are only ever
added within a version; the `format` field changes on a breaking change.

| Field         | Present on          | Meaning                                                              |
| ------------- | ------------------- | -------------------------------------------------------------------- |
| `format`      | all                 | `cidx.run-event/v1`                                                  |
| `type`        | all                 | `run.started`, `run.finished`, `phase.started`, `phase.finished`, `container.started`, `container.finished` |
| `run_id`      | all                 | Shared by every event of the run (`--run-id`, or generated)          |
| `time`        | all                 | RFC 3339, UTC                                                        |
| `target`      | `run.*`             | What was passed to `cidx run`                                        |
| `phase`       | `phase.*`, `container.*` | Phase name; absent on a container run directly as the target     |
| `container`   | `container.*`       | Container name                                                       |
| `image`       | `container.*`       | Resolved image reference                                             |
| `status`      | `*.finished`        | `passed`, `failed`, or `skipped` (local safety previewed the container) |
| `exit_code`   | `*.finished`        | 0 when passed; the container's own code when it exited non-zero. Absent when the step failed without one (image not pulled) |
| `duration_ms` | `*.finished`        | Wall-clock duration in milliseconds                                  |

Events never contain container output, commands, environment values, error
text or connection details.

## Example

`cidx run security` with one container, `trivy`, that fails with exit code 1:

```json
{"format":"cidx.run-event/v1","type":"run.started","run_id":"build-1842","time":"2026-11-11T10:00:00.000Z","target":"security"}
{"format":"cidx.run-event/v1","type":"phase.started","run_id":"build-1842","time":"2026-11-11T10:00:00.001Z","phase":"security"}
{"format":"cidx.run-event/v1","type":"container.started","run_id":"build-1842","time":"2026-11-11T10:00:00.002Z","phase":"security","container":"trivy","image":"aquasec/trivy:0.58.0"}
{"format":"cidx.run-event/v1","type":"container.finished","run_id":"build-1842","time":"2026-11-11T10:00:12.410Z","phase":"security","container":"trivy","image":"aquasec/trivy:0.58.0","status":"failed","exit_code":1,"duration_ms":12408}
{"format":"cidx.run-event/v1","type":"phase.finished","run_id":"build-1842","time":"2026-11-11T10:00:12.411Z","phase":"security","status":"failed","exit_code":1,"duration_ms":12410}
{"format":"cidx.run-event/v1","type":"run.finished","run_id":"build-1842","time":"2026-11-11T10:00:12.411Z","target":"security","status":"failed","exit_code":1,"duration_ms":12411}
```

Events of containers running in parallel (`--parallel`) interleave; use
`phase` and `container` to tell them apart.
