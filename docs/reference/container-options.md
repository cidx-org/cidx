# Container Options

What a `[containers.<name>]` section may carry. Every key here overrides the
preset of the same name; anything else in that section is rejected, because a
key nothing reads is a setting that looks applied and is not.

```toml
[containers.trivy]
severity = "HIGH,CRITICAL"   # an option the trivy preset declares
timeout = "10m"              # a structural key, from the table below
```

## Structural keys

| key           | type            | what it does                                                        |
| ------------- | --------------- | ------------------------------------------------------------------- |
| `image`       | string          | Replace the image. Pin it by digest if you care what runs.          |
| `command`     | string          | Replace the command the container runs.                             |
| `entrypoint`  | list of strings | Replace the image entrypoint.                                       |
| `workdir`     | string          | Working directory inside the container.                             |
| `volumes`     | list of strings | Bind mounts, `host:container[:opts]`. Order matters to Docker.      |
| `env`         | table           | Environment variables for the container.                            |
| `phase`       | string          | Move the container to another phase.                                |
| `privileged`  | bool            | Run as root, skipping user mapping. Needed by a few images, rarely. |
| `ephemeral`   | bool            | Never reuse this container — see below.                             |
| `pull_policy` | string          | `always`, `if-not-present` or `never`.                              |
| `timeout`     | duration string | e.g. `"10m"`. Empty means the default 30m.                          |

A preset may declare **options of its own** on top of these — `severity` above
is one of trivy's. `cidx preset show <name>` lists them.

## `ephemeral`, and why it is not a flag

Containers are reused between runs, which is what keeps a build cache alive. It
is the wrong default for a container that writes to its own filesystem: the
second run finds the first run's output already there, and a check claiming to
assert what a command just produced quietly asserts against leftovers — a wrong
green, worse than a red.

```toml
[containers.probatum]
ephemeral = true    # never reuse; this one writes to disk
```

`CIDX_NO_REUSE` still exists and still applies to every container in the run,
which is why it is not the answer: isolating one container should not cost every
other container its cache. The declaration is also checked before the config
hash, so it holds on the run where nothing changed — the case that bites.

A preset can ship it, and `probatum` does. See
[Container Reuse](../core-concepts/container-reuse.md) for the reuse rules
themselves (issue #434).
