# What leaves the container

Most CIDX phases read the workspace and report: a linter, a scanner and a test
runner all finish with their output on your screen and nothing left behind. A
**build** is the exception. Its output outlives the container that made it, and
runs somewhere else — another preset, a Docker image, a laptop, a production
host.

That crossing is where this tool's hardest-to-diagnose failure lives, and the
reason it is hard has nothing to do with how rare it is. It is that the error
message names the wrong thing.

## The symptom

```console
$ ls -l bin/cidx
-rwxrwxr-x 1 you you 36430595 bin/cidx

$ ./bin/cidx --version
sh: ./bin/cidx: not found
```

The file is there. It is executable. Its architecture is right. And the shell
says it cannot find it.

What is missing is not the file — it is the **loader the file asks for**. A
dynamically linked executable does not contain its C library; it contains the
path of the interpreter that will load one at startup. When that interpreter is
absent, the kernel fails the `execve` with `ENOENT`, and the shell reports
`ENOENT` the only way it knows how: "not found", naming the thing you typed.

Nobody debugs this quickly. You check the path, then the permissions, then the
mount, then the architecture, and every one of them is correct.

## The diagnosis, in one command

`file` answers it without running anything:

| what `file` says                          | where it runs                             |
| ----------------------------------------- | ----------------------------------------- |
| `statically linked`                       | anywhere, including `scratch`             |
| `interpreter /lib/ld-musl-x86_64.so.1`    | musl only — Alpine and derivatives        |
| `interpreter /lib64/ld-linux-x86-64.so.2` | glibc only — Debian, Ubuntu, Fedora, RHEL |

If the answer is not `statically linked`, the artifact has a requirement, and
the image that will run it has to satisfy it.

## Why a build image decides more than its size

There are two widely used C libraries on Linux — **glibc** (Debian, Ubuntu,
Fedora, RHEL) and **musl** (Alpine) — and they are not binary-compatible. A
compiler links against whichever one the image it runs in ships.

So the image chosen for a build preset is not only a question of size or of how
many CVEs it carries. **It decides where the artifact is allowed to run**, and
nothing in the resulting config records that decision.

Both directions have already caught this catalogue:

- **A glibc artifact meeting a musl image** (issue #195). `cargo-build` builds on
  Debian; `probatum` runs on Alpine. A binary handed from one to the other does
  not start, with the message above.
- **A musl artifact meeting a glibc host** (issue #419). `go-build` builds on
  Alpine, and with cgo enabled — the default — produced a musl-linked binary.
  `cidx run build` therefore handed users an executable that would not start on
  their own Debian or Ubuntu. `release.yml` had set `CGO_ENABLED=0` since #281;
  the catalogue had not caught up.

The second one survived unnoticed for a simple reason: nothing inside this
repository crosses that boundary. Every other `go build` here runs its output on
the machine that built it, where the libc necessarily matches. Only someone
running `cidx run build` on their own project ever saw the broken artifact.

## The rule

Only presets that produce an executable leaving their container are concerned,
which is a smaller set than it first appears:

| what the container produces                 | who decides the libc                        |
| ------------------------------------------- | ------------------------------------------- |
| nothing executable — lint, scan, test       | nobody; take the smallest image, it is free |
| an executable CIDX itself ships             | CIDX: static, so the question disappears    |
| an executable belonging to **your** project | you, because only you know where it runs    |

The third row is why `cargo-build` is left alone. A Rust binary heading for a
Debian host wants glibc; one heading for an Alpine image wants musl. Neither is
better in the abstract, the answer depends on a deployment CIDX has no view of —
and choosing would be CIDX deciding something about your project (guardrail 1).
So the default is what `cargo build --release` gives you on your own machine, no
surprises, and the constraint is written in the preset description.

Overriding it is three lines in your own `cidx.toml`:

```toml
[containers.cargo-build]
command = "sh -c 'rustup target add x86_64-unknown-linux-musl && cargo build --release --target x86_64-unknown-linux-musl'"
```

The `rustup target add` is not optional: `rust:<version>-slim` ships
`x86_64-unknown-linux-gnu` and nothing else, so passing `--target` alone fails on
a missing `std`. Nothing else is needed — a pure-Rust binary links musl
self-contained, with no `musl-gcc` in the image. The result is `static-pie
linked`, which runs everywhere, Alpine included.

## Presets that do not chain

`cargo-build` produces a glibc executable; `probatum` executes artifacts inside
an Alpine image. **They cannot hand off to each other** without the override
above. It is the only such pair in the catalogue today, and both descriptions
now say so — the producer as well as the consumer, since the consumer is the
tool you are not using at the moment you make the mistake.

Measured, rather than assumed. The same trivial Rust program, built both ways in
`rust:1.97.1-slim` and handed to `probatum`:

```console
✗ cargo-build par défaut (glibc)      sh: ./demo-glibc: not found
✓ cargo-build avec la surcharge musl
```

Go projects are unaffected: `go-build` pins `CGO_ENABLED=0`, so its output is
self-contained and `probatum` runs it without complaint.

## What keeps this from coming back

`TestEveryBuildPresetSaysWhatItsArtifactNeeds`, in `pkg/presets`. Every
build-phase preset states what its artifact needs elsewhere — an archive with no
loader at all, an executable carrying its libc, or one bound to the image's — and
a build preset that says nothing **fails**. There is no default, because a
default would be a silent answer to the question that costs the afternoon.

A claim the guard cannot verify fails too: a preset declaring itself
self-contained with a toolchain the guard knows no mechanism for is an unchecked
claim, which is worse than one that was never made.
