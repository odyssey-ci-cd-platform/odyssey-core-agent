# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Format

- Latest changes go at the top, under `[Unreleased]`, until a version is tagged.
- Group every entry under one of these headings, in this order:

  - **Added** — new features.
  - **Changed** — behavior changes to existing features.
  - **Deprecated** — soon-to-be-removed features.
  - **Removed** — removed features.
  - **Fixed** — bug fixes.
  - **Security** — vulnerability fixes.

- One entry per change: `- <summary>.` — sentence case, past tense.
- Reference the issue when one exists: `- <summary> (gh-<N>).`
- Only headings with entries appear in a release. Empty headings are omitted.
- Commits and branches keep the `gh-<N>` convention; entries summarize, not enumerate commits.

## [Unreleased]

### Added
- Event bus technology decision record: Redis Streams chosen for v1 (gh-38).
