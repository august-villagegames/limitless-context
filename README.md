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
go run ./cmd/tester run
```

### Configuration & Logging

- The CLI reads `config.yaml` from the working directory when present. Flags such as `--config`, `--log-level`, and `--log-format` override file values.
- Defaults place capture artifacts under `./runs` and cache assets under `./cache`.
- Use `tester run --plan-only` to print the resolved configuration without mutating the filesystem. All commands emit structured logs via Go's `slog` package.

Each CLI subcommand currently reports roadmap status while capture features evolve, but `tester run` now exercises offline stubs for the core capture subsystems so downstream phases have tangible artifacts to build upon.

### Run Manifests & Layout

- `tester run` now provisions timestamped directories under the configured `runs_dir`, creates per-subsystem folders (`video/`, `events/`, `screenshots/`, `asr/`, `ocr/`, `bundles/`, `report/`), and streams capture summaries into `capture.log`.
- A `manifest.json` file captures schema version, run identifier, host metadata, and which capture subsystems are enabled for downstream processing.
- Run manifests now persist lifecycle metadata (start/end timestamps and termination cause) while the CLI prints a matching summary after each run.
- Manifests are stored with relative paths for portability so that bundles can be moved between machines without rewriting metadata.

### Capture Subsystems (Phase 2 enhancements)

- **Event tap** – Generates deterministic keyboard, mouse, window, and clipboard samples at fine/coarse intervals, applies email/custom regex redaction, and writes both JSONL and bucketed summaries under `events/`.
- **Screenshot scheduler** – Captures throttled PNG frames (ScreenCaptureKit on macOS, CoreGraphics fallback otherwise) and companion JSON metadata under `screenshots/`, respecting configurable intervals and per-minute limits.
- **Video recorder** – Emits a synthetic segment file in the configured format/rotation, recording capture window metadata for future playback coordination under `video/`.
- **ASR agent** – Detects meeting window titles, checks Whisper availability, writes VTT transcripts when available, and records guidance/status JSON under `asr/` when the binary is missing.
- **OCR worker** – Reads captured screenshots, applies privacy redaction, emits `index.json` summaries plus status metadata under `ocr/` while tolerating missing Tesseract installations.
- **Privacy controls** – Allow-list enforcement trims events to approved apps/URLs and reports filtered counts for downstream auditing.
- **Coordinator** – Shared controller now coordinates pause/resume/kill so future interactive controls can manage subsystem lifecycles.
- The CLI reports each subsystem's output paths and counts so that later phases (bundling, reporting) can rely on deterministic fixtures during offline development.

### Phase 2.5 – Real Capture Scaffolding

- **Platform probing** – The orchestrator now inspects Screen Recording, Accessibility, and Microphone permissions along with optional ScreenCaptureKit/AVFoundation availability. Results are surfaced per subsystem in CLI summaries and persisted to run manifests for downstream tooling.
- **Controller diagnostics** – Pause/resume/stop signals are tracked across goroutines, logged into `capture.log`, and written to the manifest timeline so partial runs are explainable.
- **Concurrency** – Video, screenshots, events, ASR, and OCR execute concurrently under a shared controller context while respecting pause/stop signals. OCR waits for screenshot output to maintain deterministic fixtures.
- **Dependency gating** – Whisper/Tesseract detection produces guidance when binaries are missing while still generating status artifacts for offline QA.

### macOS permission prompts

- First-run captures on macOS will surface Screen Recording and Accessibility prompts; grant access via **System Settings → Privacy & Security**. ScreenCaptureKit requires Screen Recording permission before the scheduler can emit PNG frames; use `tccutil reset ScreenCapture` or `tccutil reset Accessibility` for repeatable smoke tests.
- Environment overrides help local testing: set `LIMITLESS_SCREEN_RECORDING=denied` or `prompt`, `LIMITLESS_ACCESSIBILITY=granted`, `LIMITLESS_MICROPHONE=granted`, and `LIMITLESS_VIDEO_BACKEND=avfoundation|stub` to simulate different hosts.
- The CLI reports friendly guidance when permissions are missing; `capture.log` records each controller transition so operators can correlate prompts with subsystem outcomes.

Phase 2 capture enhancements and optional subsystems are now complete; the roadmap advances to Phase 3 to build the bundling pipeline.

