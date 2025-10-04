# Offline LLM Bundle Capture Project

Planning artifacts for the macOS offline capture pipeline. The repository now includes an initial Go module, CLI scaffold, configuration loader, logging setup, and a run manifest/filesystem initializer ahead of Phase 1 implementation. Review the documents below for detailed scope and design.

- `docs/SPEC.md` – Product goals, functional requirements, acceptance criteria.
- `docs/DESIGN.md` – Architecture, capture subsystems, data flow.
- `docs/ROADMAP.md` – Phase-based roadmap and milestones.
- `docs/TASK_LOG.md` – Chronological log of project planning tasks.

### Getting Started

```bash
make bootstrap   # go mod tidy + vendor
make build       # compile all packages
go run ./cmd/tester version
go run ./cmd/tester run --plan-only
```

### Configuration & Logging

- The CLI reads `config.yaml` from the working directory when present. Flags such as `--config`, `--log-level`, and `--log-format` override file values.
- Defaults place capture artifacts under `./runs` and cache assets under `./cache`.
- Use `tester run --plan-only` to print the resolved configuration without mutating the filesystem. All commands emit structured logs via Go's `slog` package.

Each CLI subcommand currently reports roadmap status while capture features evolve, but `tester run` now exercises offline stubs for the core capture subsystems so downstream phases have tangible artifacts to build upon.

### Run Manifests & Layout

- `tester run` now provisions timestamped directories under the configured `runs_dir`, creates per-subsystem folders (`video/`, `events/`, `screenshots/`, `asr/`, `ocr/`, `bundles/`, `report/`), and streams capture summaries into `capture.log`.
- A `manifest.json` file captures schema version, run identifier, host metadata, and which capture subsystems are enabled for downstream processing.
- Manifests are stored with relative paths for portability so that bundles can be moved between machines without rewriting metadata.

### Capture Subsystems (Phase 1 stubs)

- **Event tap** – Generates deterministic keyboard, mouse, window, and clipboard samples at fine/coarse intervals, applies email/custom regex redaction, and writes both JSONL and bucketed summaries under `events/`.
- **Screenshot scheduler** – Produces throttled placeholder screenshots (timestamped text markers) based on configurable intervals and per-minute limits under `screenshots/`.
- **Video recorder** – Emits a synthetic segment file in the configured format/rotation, recording capture window metadata for future playback coordination under `video/`.
- The CLI reports each subsystem's output paths and counts so that later phases (OCR, ASR, bundling) can rely on deterministic fixtures during offline development.

Phase 1 capture foundations are now complete; the roadmap advances to Phase 2 for enhanced capture and optional subsystems.

