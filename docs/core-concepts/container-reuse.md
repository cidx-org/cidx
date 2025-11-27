# Container Reuse & Caching

CIDX optimizes local development by reusing Docker containers across runs, preserving caches and significantly improving performance.

## How It Works

### Container Lifecycle

Instead of creating a new container for each run, CIDX:

1. **Checks** if a container with name `cidx_<toolname>` already exists
2. **Reuses** the existing container if found (preserves filesystem and cache)
3. **Creates** a new container only if none exists
4. **Keeps** containers running after execution (not deleted)

### Container Naming

Containers use **fixed names** without timestamps:

- Format: `cidx_<toolname>`
- Examples: `cidx_trivy`, `cidx_gitleaks`, `cidx_prettier`

This allows consistent reuse across multiple CI runs.

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

```bash
docker ps -a --filter "name=cidx_"
```

Example output:

```
NAMES           STATUS                      CREATED
cidx_gitleaks   Exited (0) 2 minutes ago    2025-11-18 14:07:28
cidx_trivy      Exited (0) 2 minutes ago    2025-11-18 14:07:18
cidx_prettier   Exited (1) 5 minutes ago    2025-11-18 14:02:15
```

### Clean All CIDX Containers

```bash
# Remove all CIDX containers (will be recreated on next run)
docker rm -f $(docker ps -aq --filter "name=cidx_")
```

### Clean Specific Container

```bash
# Force recreate trivy container on next run
docker rm -f cidx_trivy
```

## Verbose Logging

Run with `--verbose` to see container reuse in action:

```bash
cidx --verbose run security
```

Output:

```
time="..." level=debug msg="♻ Reusing container cidx_trivy (preserves cache)"
time="..." level=debug msg="Starting container: cidx_trivy"
```

## Container Labels

All CIDX containers are tagged with labels for easy filtering:

```yaml
labels:
  managed-by: "cidx"
  cidx.container: "trivy"
  cidx.phase: "security"
```

Query by label:

```bash
docker ps -a --filter "label=managed-by=cidx"
docker ps -a --filter "label=cidx.phase=security"
```

## When Containers Are Recreated

Containers are **recreated** (not reused) when:

1. **Manually removed** with `docker rm`
2. **Configuration changes** (future: detect config changes)
3. **Image updates** (future: detect image changes)

Currently, containers are reused even if configuration changes. This is intentional for development speed, but may be refined in future versions.

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
# Clean specific container container
docker rm -f cidx_trivy

# Clean all CIDX containers
docker rm -f $(docker ps -aq --filter "name=cidx_")

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
   ├─ List containers with name=cidx_<container>
   ├─ If found → Return existing container ID
   └─ If not found → createContainer()

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

- [ ] **Config change detection**: Recreate container if container config changed
- [ ] **Image update detection**: Recreate if Docker image updated
- [ ] **`--clean` flag**: Force clean start (delete containers before run)
- [ ] **`--no-cache` flag**: Skip cache, force fresh operations
- [ ] **Named volumes**: Use Docker volumes for even better cache management
- [ ] **Cache size reporting**: Show cache usage per container
