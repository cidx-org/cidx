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

| key           | type            | what it does                                                            |
| ------------- | --------------- | ----------------------------------------------------------------------- |
| `preset`      | string          | Make this container an alias of a catalogue preset, under its own name. |
| `image`       | string          | Replace the image. Pin it by digest if you care what runs.              |
| `command`     | string          | Replace the command the container runs.                                 |
| `entrypoint`  | list of strings | Replace the image entrypoint.                                           |
| `workdir`     | string          | Working directory inside the container.                                 |
| `volumes`     | list of strings | Bind mounts, `host:container[:opts]`. Order matters to Docker.          |
| `env`         | table           | Environment variables for the container.                                |
| `phase`       | string          | Move the container to another phase.                                    |
| `privileged`  | bool            | Run as root, skipping user mapping. Needed by a few images, rarely.     |
| `ephemeral`   | bool            | Never reuse this container — see below.                                 |
| `pull_policy` | string          | `always`, `if-not-present` or `never`.                                  |
| `dns`         | list of strings | DNS servers (IP addresses) for the container. Default: the runtime's.   |
| `timeout`     | duration string | e.g. `"10m"`. Empty means the default 30m.                              |

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

## `dns`

```toml
[containers.cargo-build]
dns = ["1.1.1.1", "9.9.9.9"]
```

For a host whose containers cannot reach the resolver they inherit — the bridge
network cannot see the host's, and a tool dies on `Temporary failure in name
resolution` while the same image resolves with `docker run --dns 1.1.1.1` (#543).
Each entry must be an IP address, not a hostname; anything else is refused when
`cidx.toml` is read, before a container is pulled. The list is passed to the
container as `HostConfig.DNS`, in order, and absent by default, which leaves the
runtime's own resolver in place.

The servers are part of what a container is reused for: adding, changing or
reordering them recreates it, and a container that sets none keeps the container
it already has. The setting is per container: a project that needs it on several repeats it..."`) does not inherit the DNS of the
preset it names.

Limits: this is the Docker API's field, so it follows Docker's own rules (it does
not apply to a container on the `host` network). Podman receives it through its
Docker-compatible API; that path is not covered by a test here.

### DNS for a whole host: the operator's side

There is no `CIDX_DNS` variable, deliberately: a setting that reaches every
container at once changes tools that never had a problem, and it would live
outside the repository where nobody reads it. Two places hold the setting
instead, and which one depends on whose problem it is:

- **The project's, per container** — `dns = [...]` in `cidx.toml`, above. It is
  committed, it is visible in review, and it travels with the repository to
  every runner. Right when the project knows its tool needs a resolver.
- **The host's, for every container on it** — Docker's own daemon setting, in
  `/etc/docker/daemon.json`: `{ "dns": ["1.1.1.1", "9.9.9.9"] }`, then restart
  the daemon. Right when one self-hosted runner cannot resolve names and the
  repository should not carry that runner's network. It needs root on the host,
  and it is what the `dns` key exists to avoid when you do not have it.

A container's `dns` wins over the daemon's, as `docker run --dns` does.
