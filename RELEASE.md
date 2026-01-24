# Release Process

This document describes the release process for cursortab.nvim.

## Pre-release Checklist

1. Ensure all tests pass:

   ```bash
   cd server && go test ./...
   ```

2. Test basic completion flow with at least one provider.

## Versioning

Follow semantic versioning.

**Beta:** `v0.MINOR.PATCH-beta` (e.g., `v0.1.0-beta`, `v0.2.0-beta`)

- Breaking changes increment MINOR (marked with `!` in commits)
- Bug fixes and features increment PATCH

**Stable:** `vMAJOR.MINOR.PATCH` (e.g., `v1.0.0`, `v1.1.0`)

- First stable release starts at `v1.0.0`
- Breaking changes increment MAJOR (marked with `!` in commits)

## Creating a Release

1. Update version references if any exist in code.

2. Create and push a git tag:

   ```bash
   git tag -a v0.1.0-beta -m "v0.1.0-beta"
   git push origin v0.1.0-beta
   ```
