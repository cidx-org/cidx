# Execution events

`cidx run` can publish what it is doing on [NATS](https://nats.io), so a caller
follows a run through structured events instead of parsing its text output.
CIDX only **emits**; whoever launched the run consumes.

Publication is optional and best-effort. Without an address, `cidx run` behaves
exactly as before: same output, same exit codes, no connection attempted.

## Enabling it

| Setting                        | Flag             | Environment variable | Default    |
| ------------------------------ | ---------------- | -------------------- | ---------- |
| NATS server address            | `--nats-url`     | `CIDX_NATS_URL`      | none (off) |
| Subject prefix                 | `--nats-subject` | `CIDX_NATS_SUBJECT`  | `cidx.run` |
| Run identifier                 | `--run-id`       | `CIDX_RUN_ID`        | random     |
| Authentication token (secret)  | —                | `CIDX_NATS_TOKEN`    | none       |

```bash
CIDX_NATS_URL=nats://broker.internal:4222 CIDX_RUN_ID=build-1842 cidx run ci
```

The token has no flag on purpose: a flag ends up in process listings and shell
history. User and password may also be given in the URL
(`nats://user:password@host:4222`).

The run identifier is the one the caller gives; when it gives none, CIDX
generates one and logs `📡 Events: publishing run <id>` (never the address).

## Failure never changes the run

If the server cannot be reached, or a publication is refused, the run goes on
and its exit code is unchanged. One warning is printed, once
(`NATS events disabled: …`), and publication stops for the rest of the run.
Connection credentials are never written to the output or to an event: they are
removed from the warning text, and no event has a field for them.

## Subjects

An event is published on `<prefix>.<type>`, with the default prefix
`cidx.run`:

| Subject                          | When                                 |
| -------------------------------- | ------------------------------------ |
| `cidx.run.run.started`           | `cidx run <target>` begins           |
| `cidx.run.run.finished`          | it ends, passed or failed            |
| `cidx.run.phase.started`         | a phase begins                       |
| `cidx.run.phase.finished`        | a phase ends                         |
| `cidx.run.container.started`     | a container is about to run          |
| `cidx.run.container.finished`    | a container ends                     |

Subscribe to `cidx.run.>` for everything, or to `cidx.run.run.finished` for the
outcome alone. Several runs share the subjects; tell them apart with `run_id`.

## Event format

Each message is one JSON object, UTF-8, with `format` set to `cidx.event/v1`.
The version changes only when an existing field changes meaning or disappears;
new optional fields may appear without a new version.

| Field         | Type    | Present on          | Meaning                                                              |
| ------------- | ------- | ------------------- | -------------------------------------------------------------------- |
| `format`      | string  | all                 | `cidx.event/v1`                                                      |
| `type`        | string  | all                 | `run.started`, `run.finished`, `phase.started`, … (see above)        |
| `run_id`      | string  | all                 | Identifier shared by every event of the run                          |
| `time`        | string  | all                 | RFC 3339 UTC time of the event                                       |
| `target`      | string  | `run.*`             | What was asked: a pipeline, a phase, a container or `all`            |
| `phase`       | string  | `phase.*`, `container.*` | Phase name; absent on a container run on its own                |
| `container`   | string  | `container.*`       | Container (preset) name                                              |
| `image`       | string  | `container.*`       | Image the container runs                                             |
| `status`      | string  | `*.finished`        | `passed`, `failed`, or `skipped` (local safety previewed it)         |
| `exit_code`   | integer | `*.finished`        | See below                                                            |
| `duration_ms` | integer | `*.finished`        | Elapsed milliseconds                                                 |

`exit_code` on a container is the code the container exited with; it is absent
when the container never started (image pull failure, no runtime). On a phase
or a run it is `0` when passed, the code of the failing container when there is
one, `1` otherwise.

## What events never contain

Events carry names, images, codes and durations. They never carry container
output, environment variables or their values, volumes, commands or any secret.

## Examples

A run of the `ci` pipeline whose `test` phase fails (one event per line):

```json
{"format":"cidx.event/v1","type":"run.started","run_id":"build-1842","time":"2026-11-11T14:02:11.120Z","target":"ci"}
{"format":"cidx.event/v1","type":"phase.started","run_id":"build-1842","time":"2026-11-11T14:02:11.121Z","phase":"code"}
{"format":"cidx.event/v1","type":"container.started","run_id":"build-1842","time":"2026-11-11T14:02:11.121Z","phase":"code","container":"golangci-lint","image":"golangci/golangci-lint:v2"}
{"format":"cidx.event/v1","type":"container.finished","run_id":"build-1842","time":"2026-11-11T14:02:20.418Z","phase":"code","container":"golangci-lint","image":"golangci/golangci-lint:v2","status":"passed","exit_code":0,"duration_ms":9297}
{"format":"cidx.event/v1","type":"phase.finished","run_id":"build-1842","time":"2026-11-11T14:02:20.418Z","phase":"code","status":"passed","exit_code":0,"duration_ms":9297}
{"format":"cidx.event/v1","type":"phase.started","run_id":"build-1842","time":"2026-11-11T14:02:20.419Z","phase":"test"}
{"format":"cidx.event/v1","type":"container.started","run_id":"build-1842","time":"2026-11-11T14:02:20.419Z","phase":"test","container":"go-test","image":"golang:1.26"}
{"format":"cidx.event/v1","type":"container.finished","run_id":"build-1842","time":"2026-11-11T14:03:02.907Z","phase":"test","container":"go-test","image":"golang:1.26","status":"failed","exit_code":1,"duration_ms":42488}
{"format":"cidx.event/v1","type":"phase.finished","run_id":"build-1842","time":"2026-11-11T14:03:02.907Z","phase":"test","status":"failed","exit_code":1,"duration_ms":42488}
{"format":"cidx.event/v1","type":"run.finished","run_id":"build-1842","time":"2026-11-11T14:03:02.907Z","target":"ci","status":"failed","exit_code":1,"duration_ms":51787}
```

Phases and containers after the failure produce no event: they did not run.
When containers of a phase run in parallel (`--parallel`), their events may
interleave; `phase` and `container` identify each.

## Verifying

The contract is specified in
[`features/executor/execution_events.feature`](../../features/executor/execution_events.feature)
and tested with a small in-process NATS server (`internal/natstest`), so no
broker is needed to run the suite.
