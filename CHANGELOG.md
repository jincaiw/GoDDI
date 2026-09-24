# Changelog

## v0.24.1 — 2026-09-24

- Update the Docker runtime image from the unsupported Alpine 3.19 branch to the supported Alpine 3.24 branch.
- Pin pnpm to the version used by the build and release workflows.
- Keep Makefile-built binaries aligned with the source release version before a tag exists.
- Build and smoke test the production container in CI; smoke test the Linux amd64 release binary before publishing it.
- Keep the existing v0.24.0 security and quality improvements and documented deployment boundaries.

## v0.24.0 — 2026-09-24

- Harden frontend dependencies and remove known vulnerable dependency paths.
- Improve frontend API behavior, validation, and lint cleanliness.
- Update release metadata and document feature and deployment validation boundaries.
