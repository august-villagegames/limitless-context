# Roadmap & Milestones

## Phase Tracking Rules
- Each phase kicked off in Codex must be logged in `docs/TASK_LOG.md` with start date and current status.
- Milestones below are represented as checkboxes; mark them `[x]` upon completion and note evidence in the task log.
- Do not advance to the next phase until all prior milestones are checked or explicitly waived with documented rationale.

## Phase 0 – Planning (Day 0)
- [x] Finalize product spec, design, and success criteria documents.
- [x] Align on technology stack and dependency strategy.
- [x] Set up repository layout, Go module, and basic tooling.

## Phase 1 – Capture Foundations (Days 1-3)
- [x] Implement config loader, logging, CLI scaffolding.
- [x] Build run manifest structure and filesystem layout.
- [x] Implement event tap capture with dual granularity and redaction filters.
- [x] Integrate screenshot triggers and throttling.
- [x] Add basic video recorder stub (recording to disk) with pause/resume.

## Phase 2 – Enhanced Capture & Optional Subsystems (Days 4-6)
- [x] Add ASR meeting detection and Whisper integration (optional dependency gate).
- [x] Integrate OCR worker with optional Tesseract dependency.
- [x] Harden privacy controls and allow-list enforcement.
- [x] Implement pause/resume/kill coordination across subsystems.

## Phase 2.5 – Real Capture Integrations (Days 6-7)
- [x] Implement configurable run duration and controller loop.
    - Story 2.5.1: Extend configuration schema with `capture.duration_minutes`, drive controller deadlines from config, and stream countdown/stop events into `capture.log`.
    - Story 2.5.2: Update CLI status output and manifest to record actual start/end timestamps and termination causes.
- [x] Replace video stub with ScreenCaptureKit plus AVFoundation fallback.
    - Story 2.5.3: Capture primary display using ScreenCaptureKit (`SCShareableContent` + `SCStream`) on macOS 12.3+; write H.264 MP4 segments honoring `chunk_seconds`.
    - Story 2.5.4: Detect ScreenCaptureKit availability and fallback to `AVCaptureScreenInput` when required; preflight `CGPreflightDisplayCaptureAccess()` and request permission via `CGRequestScreenCaptureAccess()`.
- [x] Replace screenshot stub with CoreGraphics/ScreenCaptureKit capture.
    - Story 2.5.5: Sample frames from ScreenCaptureKit stream or `CGWindowListCreateImage` on older macOS releases; emit PNG plus JSON metadata containing trigger reason and window title.
    - Story 2.5.6: Share the Screen Recording permission checks with video to avoid duplicate prompts and document mitigation when access is denied.
- [x] Replace synthetic event tap with Quartz event tap and accessibility prompts.
    - Story 2.5.7: Implement live `CGEventTapCreate` capture for keyboard, mouse, and window focus events; normalize payloads for privacy filtering.
    - Story 2.5.8: Use `AXIsProcessTrustedWithOptions` with `kAXTrustedCheckOptionPrompt` to request accessibility trust, and surface actionable CLI guidance when permission is missing.
- [x] Wire pause/resume/stop signals across concurrent subsystems.
    - Story 2.5.9: Run video, screenshots, events, ASR, and OCR in goroutines governed by shared context; propagate controller state transitions to each worker.
    - Story 2.5.10: Persist controller state changes in `capture.log` and manifest so downstream tools understand partial runs or early exits.
- [x] Harden ASR/OCR dependency gating and permissions.
    - Story 2.5.11: Detect Whisper and Tesseract binaries before launch, request microphone permission via `AVAudioSession`/`AVCaptureDevice`, and skip gracefully when unavailable.
    - [x] Story 2.5.12: Record subsystem availability in manifest with human-readable status to support QA triage.
- [x] Update documentation and acceptance tests for macOS permissions.
    - Story 2.5.13: Expand README and SPEC with first-run Screen Recording, Accessibility, and Microphone walkthroughs plus `tccutil reset` guidance for repeatable tests.
    - Story 2.5.14: Add a smoke test plan that verifies permission prompts appear exactly once and that denied permissions produce friendly CLI errors.
Success Criteria: On a clean macOS 12+ user account, a single `go run ./cmd/tester run` session after granting prompts records five minutes of activity with real MP4/PNG/JSON artifacts and manifest status entries reflecting granted or denied permissions.


## Phase 3 – Bundling Pipeline (Days 7-8)
- [ ] Implement sessionization and task clustering.
- [ ] Build tokenizer module and token accounting utilities.
- [ ] Generate bundle directories, prompts, context trimming, metrics.
- [ ] Produce README_bundles.md instructions.

## Phase 4 – Import & Reporting (Days 9-10)
- [ ] Implement strict JSON validation and evidence resolution.
- [ ] Aggregate metrics and privacy scan results.
- [ ] Render HTML report with comparison tables, timeline, and recommendations.
- [ ] Add CLI `process` and `report` commands.

## Phase 5 – Tooling & Sample Assets (Day 11)
- [ ] Create `demo_3min` sample run with synthetic data.
- [ ] Author README with manual workflow and teardown instructions.
- [ ] Implement `make clean` and verification scripts for acceptance tests.

## Phase 6 – QA & Hardening (Day 12)
- [ ] Run acceptance test matrix.
- [ ] Polish logging, error handling, and config validation.
- [ ] Finalize documentation and task log updates.

## Success Criteria Checklist
- [ ] All acceptance tests pass from clean checkout.
- [ ] Offline only: no network dependency required at build or runtime.
- [ ] Privacy mask tests ensure zero email matches in sample run.
- [ ] HTML report renders key metrics and highlights failures gracefully.
- [ ] README enables new user to complete manual workflow within 1 hour.

