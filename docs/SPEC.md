# Offline LLM Bundle Capture App - Product Spec

## Overview
A macOS-only capture harness that records a single offline 1-hour working session and produces structured "LLM bundles" for manual large language model evaluation. The CLI manages capture, bundle export, import of manual model outputs, and HTML report generation while never touching the network.

## Objectives
- Capture one session concurrently in three modes: Video-only, Hybrid, Events-only.
- Export per-task and day-summary bundles ready for manual copy/paste into any AI application.
- Re-ingest pasted outputs, validate them, and generate a consolidated HTML scorecard report.
- Operate entirely offline with clear privacy guardrails and redaction.

## In/Out of Scope
- In scope: macOS desktop capture, local preprocessing, offline token counting, manual summarizer workflow, HTML report.
- Out of scope: automated model calls, multi-day aggregation, web UI, cloud storage, network operations, Windows/Linux support.

## User Roles & Scenarios
1. **Evaluator** uses CLI to bootstrap, run a 1-hour capture, bundle tasks, paste prompts into a chosen LLM manually, then process outputs into a report.
2. **Developer** runs short demo capture (3 minutes) to verify pipeline and smoke acceptance tests.
3. **Privacy reviewer** inspects artifacts ensuring redaction rules hold.

## Session Capture Modes (Concurrent)
- **Video-only**: Screen record 1080p MP4 @ 5-8 fps. Optionally generates ASR tracks for meeting windows using local Whisper.
- **Hybrid**: Collect event spine at 2-5 second cadence, sparse screenshots on state changes (<=1 per 15s), optional ASR for meetings.
- **Events-only**: Capture dual granularities (2s and 5s) simultaneously, annotate event payloads with granularity tags.

## Functional Requirements
- `make bootstrap` installs local dependencies, fails fast if network is required.
- `make run` starts capture for configured duration (default 60 min) or until stopped.
  - Subcommands or signals provide `Pause`, `Resume`, `Stop` semantics.
  - Stores artifacts under `runs/{stamp}/` with subfolders for modes, screenshots, ASR, metadata.
  - macOS hosts must grant Screen Recording permission so ScreenCaptureKit can emit PNG frames; when denied the scheduler falls back to CoreGraphics capture and notes the backend in metadata.
- `make stop` stops the active capture session early, ensuring partial artifacts are flushed safely.
- `make bundle RUN=stamp` produces LLM bundles aligned to clustered tasks and day summary.
- `make process RUN=stamp` imports human-produced `output.json` files, validates structures, and generates report assets.
- `make report RUN=stamp` opens the HTML report locally.
- `make clean` removes all runs after explicit confirmation.

## Data Artifacts & Formats
- **Events**: JSON lines matching provided schema; stored per mode (2s/5s) and per session.
- **Screenshots**: PNG files with sidecar JSON metadata documents referencing capture reason, timestamps, backend (`screencapturekit` or `cgwindow`), and relative image path. Artifacts land under `runs/{stamp}/screenshots/screenshot_###.png` with matching `.json` metadata files.
- **Video**: MP4 recorded via AVFoundation or macOS capture APIs with stored fps and resolution metadata.
- **ASR**: Optional VTT/JSON segments tagged to time ranges when meeting windows detected.
- **Bundles**: Directory layout under `runs/{stamp}/bundles/` with per-task and `day_summary/` folders containing prompts, context, metrics (including token counts), README.

## Bundling Workflow
1. Sessionize events by idle gap >120s.
2. Cluster tasks using app/url/file/time proximity, promoting clusters with build or modal events.
3. For each task cluster:
   - Assemble `context.md` with sections for events, OCR, ASR.
   - Trim context to `per_task_token_budget` prioritizing events > OCR > ASR.
   - Create `prompt.txt` with strict JSON output instructions.
   - Record token estimates, character counts, artifact item counts, checksum in `metrics.json`.
4. Generate `day_summary/` bundle referencing selected task outputs.
5. Write `README_bundles.md` explaining manual steps and file relationships.

## Manual Summarization Flow
- User opens each `task_{k}` prompt/context pair, copies into preferred AI app, saves JSON reply to `task_{k}/output.json`.
- After individual tasks, user processes `day_summary` prompt with references to completed tasks, saving `output.json`.
- CLI command `make process RUN=...` validates files and builds consolidated report.

## Validation Rules (Process Phase)
- Strict JSON parsing with schema enforcement for task and summary outputs.
- Evidence references must resolve to captured events or screenshots.
- File size limit 200 KB and no control characters.
- Optional checksum verification to catch edits.
- Failures are captured per task; processing continues, marking invalid items red in report and logs.

## HTML Report
- Single self-contained file summarizing metrics:
  - Executive comparison table Mode x Metrics (Fidelity, Traceability, TokenCost, StorageCost, SetupEffort, Runtime, PrivacyExposure, Robustness) with default weights 30/20/15/10/10/10/5.
  - Token usage aggregated from `metrics.json` per task and mode.
  - Storage footprint table (MP4, PNG, JSON, VTT).
  - Timeline view linking to task outputs and evidence anchors.
  - Privacy scan results showing masked item counts/violations.
  - Recommendation summary block.

## Privacy & Redaction
- Configurable allow-list for apps and URL prefixes; default deny all others.
- Drop password fields, mask emails/16-digit numbers/JWT-like strings at capture time.
- All data written locally; no network transmission.
- Privacy scanner tallies masked items and flags violations surfaced in report.

## Config (`config/config.yaml` Defaults)
```
duration_minutes: 60
video: { enabled: true, fps: 6, resolution: "1920x1080" }
events: { cadence_secs: [2,5] }
screenshots: { throttle_secs: 15, triggers: ["app_switch","url_change","modal_open","error_toast","build_start","build_end"] }
asr: { enabled_for_meetings_only: true }
privacy:
  allowlist_apps: ["Chrome","Safari","VSCode","Terminal","Google Docs"]
  allowlist_url_prefixes: ["https://docs.google.com","https://github.com"]
  block_apps: ["Password Manager"]
  mask_patterns: ["email","cc16","jwt"]
paths: { root: "./runs" }
summarizer:
  mode: "manual"
  per_task_token_budget: 5000
  max_context_tokens: 8192
```

## Implementation Decisions
- Language: Go with minimal vendored dependencies.
- Screen recording: use vendored `github.com/blacktop/go-macscreenrec` for AVFoundation capture.
- OCR: auto-detect local Tesseract binary; disable gracefully with user guidance when absent.
- ASR: auto-detect local Whisper.cpp binary; enable only for meeting windows and note when unavailable.
- Tokenizer: embed offline BPE tokenizer with bundled vocab.

## macOS Permission Handling (Phase 2.5)
- On first launch, request Screen Recording and Accessibility permissions via standard macOS prompts; document recovery steps (`tccutil reset ScreenCapture` / `tccutil reset Accessibility`) for smoke tests.
- Detect microphone, screen recording, and accessibility status before capture; emit manifest guidance when permissions are denied so QA can triage missing assets quickly.
- Support environment overrides (`LIMITLESS_SCREEN_RECORDING`, `LIMITLESS_ACCESSIBILITY`, `LIMITLESS_MICROPHONE`, `LIMITLESS_VIDEO_BACKEND`) to simulate host conditions during offline development.

## System Constraints & Guardrails
- No outbound network calls at runtime; detect and abort if libraries attempt network access.
- Works offline with minimal dependencies (Go or Python implementation decision captured in design doc).
- Provide pause/resume/kill commands; handle subsystem failures gracefully (log gaps, continue other subsystems, surface issues in report).
- Include teardown command removing `./runs` tree safely.

## Acceptance Criteria
1. Run `make bootstrap` then `make run` for 3-minute demo; artifacts appear in `runs/demo_3min`.
2. Run `make bundle` and confirm `prompt.txt`, `context.md`, `metrics.json` exist per task.
3. Create dummy `output.json` values; `make process` generates HTML report.
4. Corrupt one task `output.json`; processing flags task in report but completes.
5. Search run folder for email regex; expect zero unmasked matches.
6. Delete `day_summary/output.json`; processing warns and still completes partial report.

## Development Criteria & Success Metrics
- Deterministic CLI behavior with reproducible paths and timestamps.
- Automated token counts using bundled tokenizer (no network).
- Accurate privacy masking (unit tests for redaction patterns, config-driven allowlists).
- Comprehensive logging for user troubleshooting.
- Success determined by meeting acceptance criteria, passing lint/tests, and manual dry run documented in sample `demo_3min` run.

## Deliverables
- CLI located under `/cmd/tester` with supporting internal packages for capture, redaction, clustering, bundling, import, and reporting.
- Config defaults and sample config file.
- HTML template and generator.
- README with quick start, manual workflow, teardown instructions.
- Sample run artifacts for `demo_3min` scenario.

