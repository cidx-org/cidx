# Container Reuse & Caching

CIDX optimizes local development by reusing Docker containers across runs, preserving caches and significantly improving performance.

## How It Works

### Container Lifecycle

Instead of creating a new container for each run, CIDX:

1. **Checks** — among containers labelled `managed-by=cidx` — whether one already owns this project's container name
2. **Reuses** the existing container if its config hash still matches (preserves filesystem and cache)
3. **Recreates** it when the config changed, **creates** a new one when none exists
4. **Keeps** containers around after execution (not deleted)

### Container Naming

Containers use **fixed, project-scoped names** without timestamps:

- Format: `cidx_<project>_<toolname>`
- `<project>` is the sanitized workspace basename plus a short hash of its absolute path
- Examples: `cidx_myrepo-3f5a9c21_trivy`, `cidx_myrepo-3f5a9c21_golangci-lint`

The name is stable for a given directory, so reuse works across runs. The path
hash keeps two repositories on the same host — including two checkouts of the
same repo, or two different repos sharing a basename — from fighting over one
container and destroying each other's cache (issue #197).

Containers created by cidx versions before this change use the unscoped
`cidx_<toolname>` form. They carry the same `managed-by=cidx` label, so
`cidx cleanup` still collects them; they are simply never reused again.

## Performance Benefits

### Before (Create/Delete Strategy)

```
First run:  ~15 seconds (download + execute)
Second run: ~15 seconds (download + execute)
Third run:  ~15 seconds (download + execute)
```

### After (Container Reuse)

```
First run:     ~15 seconds (download + execute)
Second run:    ~2.4 seconds (cache reused ⚡)
Third run:     ~2.4 seconds (cache reused ⚡)
→ 6x faster for subsequent runs!
```

## Cache Preservation

Containers preserve their internal filesystem between runs, which means:

### Trivy

- **Vulnerability DB** (~75 MB) downloaded once
- Subsequent scans use cached DB
- Only updates when DB is outdated

### Gitleaks

- Git repository state preserved
- Faster subsequent scans

### Prettier

- Node modules cache (if applicable)
- Formatted files cache

## Container Management

### List CIDX Containers

Select on the ownership label, not on the name:

```bash
docker ps -a --filter "label=managed-by=cidx"
```

Example output:

```
NAMES                            STATUS                      CREATED
cidx_myrepo-3f5a9c21_gitleaks    Exited (0) 2 minutes ago    2025-11-18 14:07:28
cidx_myrepo-3f5a9c21_trivy       Exited (0) 2 minutes ago    2025-11-18 14:07:18
cidx_myrepo-3f5a9c21_prettier    Exited (1) 5 minutes ago    2025-11-18 14:02:15
```

### Clean All CIDX Containers

```bash
# Remove stopped CIDX containers (recreated on the next run)
cidx cleanup

# Preview first
cidx cleanup --dry-run
```

### Clean Specific Container

```bash
# Force recreate one container on the next run
docker rm -f cidx_myrepo-3f5a9c21_trivy

# Or force recreate everything for one run
CIDX_NO_REUSE=1 cidx run security
```

## Verbose Logging

Run with `--verbose` to see container reuse in action:

```bash
cidx --verbose run security
```

Output:

```
time="..." level=debug msg="♻ Reusing container cidx_myrepo-3f5a9c21_trivy (preserves cache, config hash 7c8ba14f6a4f3495)"
time="..." level=debug msg="Starting container: cidx_myrepo-3f5a9c21_trivy"
```

## Container Labels

All CIDX containers are tagged with labels. `managed-by=cidx` is the ownership
signal: lookup, reconciliation and `cidx cleanup` all select on it, never on the
container name.

```yaml
labels:
  managed-by: "cidx"
  cidx.tool: "trivy"
  cidx.phase: "security"
  cidx.image: "dhi.io/trivy:0.68"
  cidx.version: "2.1.0"
  cidx.config_hash: "7c8ba14f6a4f3495"
```

Query by label:

```bash
docker ps -a --filter "label=managed-by=cidx"
docker ps -a --filter "label=cidx.phase=security"
```

## When Containers Are Recreated

Containers are **recreated** (not reused) when:

1. **Manually removed** with `docker rm`
2. **The config changed** — cidx stores a `cidx.config_hash` label over the
   behavior-affecting fields (image, command, workdir, entrypoint, volumes, env)
   and recreates the container when it no longer matches
3. **The container predates the hash label** — it can't be proven current
4. **`CIDX_NO_REUSE` is set** — escape hatch that forces a recreate every run

If the container name is held by a container that carries `managed-by=cidx`,
cidx reclaims it (removes and recreates). If it is held by a container cidx does
**not** own, cidx refuses to touch it and tells you so — it never force-removes
someone else's container.

## Best Practices

### Development Workflow

1. **First run**: Expect normal timing (downloads, setup)
2. **Subsequent runs**: Enjoy fast iterations ⚡
3. **Clean periodically**: Remove containers if caches grow too large

### CI/CD Pipelines

For production CI/CD, consider:

- Using `--clean` flag (future feature) to ensure fresh state
- Implementing cache volume mounts for even better performance
- Separating dev and CI container namespaces

### Troubleshooting

If you experience issues with cached data:

```bash
# Clean one specific container
docker rm -f cidx_myrepo-3f5a9c21_trivy

# Clean all CIDX containers
cidx cleanup

# Next run will create fresh containers
cidx run security
```

## Technical Details

### Implementation

- **Location**: `pkg/executor/docker.go`
- **Function**: `getOrCreateContainer()`
- **Strategy**: Check existence → Reuse or Create

### Container Lifecycle Code Flow

```
1. getOrCreateContainer()
   ├─ List containers with label=managed-by=cidx, match the name exactly
   ├─ If found and config hash matches → Return existing container ID
   ├─ If found and stale → Remove, then createContainer()
   └─ If not found → createContainer()
      └─ On a name conflict → reclaim if ours, error otherwise

2. ContainerStart()
   └─ Starts the container (fresh or reused)

3. StreamLogs() & Wait()
   └─ Container exits naturally

4. Container remains in "Exited" state
   └─ Available for next run
```

### Why Not Remove Containers?

Removing containers would:

- ❌ Delete cached data (DB, packages, build artifacts)
- ❌ Require re-downloading on every run
- ❌ Waste bandwidth and time
- ❌ Slow down local development

Keeping containers:

- ✅ Preserves caches
- ✅ Speeds up iterations
- ✅ Reduces network usage
- ✅ Better developer experience

## Future Enhancements

Planned improvements:

- [ ] **`--clean` flag**: Force clean start (delete containers before run)
- [ ] **`--no-cache` flag**: Skip cache, force fresh operations
- [ ] **Named volumes**: Use Docker volumes for even better cache management
- [ ] **Cache size reporting**: Show cache usage per container
