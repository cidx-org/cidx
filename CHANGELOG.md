# Changelog

All notable changes to this project will be documented in this file.

## v0.3.1 (2025-11-24)

### Fix

- expand environment variables in tool config

## v0.3.0 (2025-11-24)

### Feat

- split CI and Release workflows

## v0.2.0 (2025-11-24)

### Feat

- add dynamic actions system with release-create action
- use git binary for commit/push to ensure pre-commit hooks execution
- add commit-push-watch action with gh CLI auth support

### Fix

- reset CHANGELOG to v0.1.0 for clean version bump
- add git safe.directory config to release action
- remove files-only flag and reset to 0.1.0
- reset version to 0.1.0 and use files-only for bump
- add changelog flag to commitizen bump
- add --no-verify flag to commitizen bump command
- correct commitizen command in release-create action
- expand WORKSPACE variable in action volumes
- ignore untracked files in HasChanges check
- **lint**: use tagged switch for job conclusion checks

## v0.1.0 (2024-01-01)

### Features

- Initial release
