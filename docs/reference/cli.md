# CLI Reference

## Commands

### `cidx init`

Initialize a new configuration file in the current directory.

```bash
cidx init
cidx init --format yaml  # Not yet implemented, defaults to toml
```

### `cidx run`

Execute a tool, pipeline, or phase.

```bash
cidx run <name> [flags]
```

**Arguments:**

- `<name>`: Name of a tool (e.g., `trivy`), pipeline (e.g., `ci`), or phase (e.g., `security`).

**Flags:**

- `--dry-run`: Print what would be executed without running it.

### `cidx list`

List all available tools and pipelines.

```bash
cidx list
```

### `cidx info`

Show detailed information about a specific tool.

```bash
cidx info <tool>
```

### `cidx validate`

Validate the syntax and structure of the configuration file.

```bash
cidx validate
```
