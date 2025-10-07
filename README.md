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

### macOS native build & signing requirements

The ScreenCaptureKit integrations depend on Hardened Runtime entitlements. Building on macOS with a proper signature ensures the operating system will surface the Screen Recording and Accessibility prompts instead of failing with `AVFoundationErrorDomain -11800`.

1. **Install prerequisites.** Build on macOS with Xcode command line tools (`xcode-select --install`) so CGO can link against ScreenCaptureKit/AVFoundation. Decide which signing identity you will use, e.g. `codesign -v -s "Developer ID Application: …"`.
2. **Create the entitlements file.** The repository includes a default `entitlements.plist` requesting Screen Recording and audio input access. Adjust it if you need to omit audio capture.
3. **Build the CLI with CGO enabled.** From the repo root run:

   ```bash
   make macos-build
   ```

   The resulting `./tester` binary is emitted next to `entitlements.plist` so codesign can attach the entitlements without additional path juggling.
4. **Codesign with the Hardened Runtime and entitlements.** Replace the signing identity with your own:

   ```bash
   codesign --force --options runtime --entitlements entitlements.plist \
     --sign "Developer ID Application: YOUR TEAM" ./tester
   codesign --display --entitlements :- ./tester
   ```

   The second command prints the embedded entitlements and is an easy sanity check before distributing the binary.
5. **Trigger the macOS permission prompts.** Launch the signed binary (`./tester run`). macOS should request Screen Recording (and optionally Accessibility/Microphone). Approve the prompts in **System Settings → Privacy & Security** so future runs start capture immediately. Re-authorise after every re-sign.

   Once permission is granted, rerun `./tester run` (or allow the first run to continue) and wait for the configured duration (defaults to 60 minutes, adjustable via `capture.duration_minutes` in `config.yaml`). The CLI will report `Video: segment recorded -> …` and you will find an MP4 in `runs/<timestamp>/video/` alongside screenshots and manifests. If macOS denies permission the CLI surfaces a `macOS screen recording permission required for video capture` status so you can revisit System Settings and re-launch the signed binary.

Grant the executable Screen Recording permission after the first launch via **System Settings → Privacy & Security → Screen Recording**. Permissions must be re-authorised if the binary signature changes.

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


- **Event tap** – On macOS, installs a Quartz `CGEventTap` listener (using `CFRunLoop` + `AXIsProcessTrustedWithOptions`) to stream live keyboard, mouse, and focus changes through the redaction/privacy pipeline before persisting `events_fine.jsonl` and `events_coarse.json`. Non-mac builds fall back to deterministic fixtures for offline CI.
- **Screenshot scheduler** – Captures throttled PNG frames (ScreenCaptureKit on macOS, CoreGraphics fallback otherwise) and companion JSON metadata under `screenshots/`, respecting configurable intervals and per-minute limits.
- - **Video recorder** – Streams the primary display to H.264 MP4 segments under `video/`, preferring ScreenCaptureKit on macOS 12.3+ and falling back to AVFoundation capture on older releases while preserving `chunk_seconds` boundaries.
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

- First-run captures on macOS will surface Screen Recording and Accessibility prompts; grant access via **System Settings → Privacy & Security**. Use `tccutil reset ScreenCapture` or `tccutil reset Accessibility` for repeatable smoke tests.
- The event tap requires Accessibility trust. If the CLI reports `macOS accessibility permission required for event capture`, open **Privacy & Security → Accessibility**, unlock the panel, enable the `tester` binary, and relaunch. Toggle the checkbox off/on after signing new builds so Quartz picks up the signature change.
- Troubleshooting tips: verify the binary is codesigned, remove stale entries with `tccutil reset Accessibility com.offlinefirst.tester`, and confirm the process appears in `System Settings` after invoking `tester run` once (the prompt appears when `AXIsProcessTrustedWithOptions` executes).
- Environment overrides help local testing: set `LIMITLESS_SCREEN_RECORDING=denied` or `prompt`, `LIMITLESS_ACCESSIBILITY=granted`, `LIMITLESS_MICROPHONE=granted`, and `LIMITLESS_VIDEO_BACKEND=avfoundation|stub` to simulate different hosts.
- The CLI reports friendly guidance when permissions are missing; `capture.log` records each controller transition so operators can correlate prompts with subsystem outcomes.

Phase 2 capture enhancements and optional subsystems are now complete; the roadmap advances to Phase 3 to build the bundling pipeline.

