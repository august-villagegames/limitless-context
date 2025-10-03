# Offline Capture App - Technical Design

## Tech Stack
- **Language**: Go 1.21+ (static binary, strong concurrency, good macOS integration).
- **Build tooling**: `make`, Go modules, minimal third-party deps.
- **Capture APIs**:
  - Video: AVFoundation via vendored `github.com/blacktop/go-macscreenrec`; package lives in the repo for offline builds.
  - Events: Quartz Event Taps (CGEventTap) for keyboard/mouse/window focus plus Accessibility Notifications for app/window metadata.
  - Screenshots: CGWindowListCreateImage combined with event triggers.
  - ASR (optional): Local Whisper.cpp bindings (invoked via CLI if binary present).
  - OCR: Tesseract via command-line if installed; fallback is disabled.

## High-Level Architecture
```
/cmd/tester            # Entry CLI
/internal/
  capture/             # Subsystems for video, events, screenshots, asr
  redact/              # Masking pipelines for events/text/image OCR
  cluster/             # Sessionization, task clustering, metadata tagging
  bundle/              # Context assembly, token accounting, exporter
  import/              # JSON validation, checksum verification
  report/              # HTML generation, metrics aggregation
/pkg/
  config               # YAML parsing, defaults, validation
  logging              # Structured logging (zerolog or zap-lite)
  tokenizer            # Embedded BPE tokenizer w/ offline vocab
  utils                # Shared helpers (filesystem, time, errors)
```

## Runtime Flow
1. `tester run` loads config, initializes capture subsystems concurrently.
2. Coordinator manages run state (Running, Paused, Stopping) with context cancellation and channels.
3. Each subsystem writes to `runs/{stamp}/...` while reporting status events to shared event bus.
4. Redaction module filters event payloads and OCR/ASR text before persistence.
5. On stop or duration expiry, coordinator flushes buffers, finalizes indexes, and writes run manifest.
6. `tester bundle` loads manifest, events, OCR, ASR to build task clusters and bundles, writing metrics.
7. `tester process` ingests manual outputs, validates, and produces report assets.
8. `tester report` opens the `report.html` using `open` command (local).

## Capture Subsystems

## Optional Dependency Strategy
- Whisper.cpp and Tesseract are treated as optional binaries.
- Detection occurs at runtime via `exec.LookPath`; when missing, the subsystem logs guidance and marks the capability inactive.
- Demo runs and automated tests simulate transcripts/OCR with fixtures when binaries are absent.
- Bundle metadata records which optional subsystems were active for traceability.

- **VideoRecorder**: wraps AVFoundation session to record screen 1080p at configured fps, chunked writes to allow incremental flush. Emits heartbeat events for runtime metrics.
- **EventTap**: attaches to CGEventTap and Accessibility APIs, normalizes events to schema, deduplicates repeated states, tags granularity (2s/5s) per scheduled ticker.
- **Screenshotter**: listens to event bus triggers (app switch, url change, etc.), enforces throttle window, captures PNG, stores metadata JSON.
- **ASRAgent**: monitors focused window titles for meeting heuristics, records mic/system audio to temp WAV, invokes Whisper binary when auto-detected, otherwise logs guidance and skips transcription.
- **OCRWorker**: optional; queued for selected screenshots; runs Tesseract CLI when discovered locally, otherwise leaves placeholders and guidance.

## Redaction Pipeline
1. Pre-validate allow-list: discard events from non-whitelisted apps/URLs.
2. Apply regex-based masking for emails, 16-digit numbers, JWT-like patterns.
3. Strip password fields and sensitive window titles.
4. Log summary metrics and failures without storing raw sensitive values.

## Clustering & Bundling
- Sessionization: sort events, segment on idle gap >120s.
- Task clustering heuristics:
  - Group by dominant app/url/file combination within session slice.
  - Promote clusters containing build start/end, modal opens, form submits.
  - Merge adjacent clusters if duration <45s to avoid fragmentation.
- Context assembly:
  - `events` section as bullet JSON lines with event_id references.
  - `optional_ocr` and `optional_asr` sections with timestamped entries.
  - Token accounting via local BPE over prompt template + context to ensure budgets.
- Metrics: store counts, durations, checksums (SHA256) per bundle.

## Import Validation
- Use JSON schema definitions (Go structs + validation) for fast checks.
- Evidence resolver ensures `event:evt_x` or `shot:mm:ss` references exist in run metadata.
- Size/character scan rejects oversized or malformed outputs.
- Failed tasks recorded in `import/report.json` for later use in HTML.

## Report Generation
- Report builder uses Go templates + embedded CSS/JS for timeline interactions offline.
- Aggregates metrics per mode and calculates weighted scores.
- Includes privacy summary by reading redaction logs.
- Links to artifacts via relative paths within run directory.

## Command Interface
- Primary CLI uses Cobra or stdlib flag parsing to expose subcommands aligning with make targets.
- Signal handling (SIGINT/SIGTERM) triggers graceful stop.
- `tester status` (optional) prints live state from run manifest.

## Data Layout Example
```
runs/20240512_0930/
  manifest.json
  capture.log
  video/
    mode_video/recording.mp4
  events/
    mode_hybrid/events_2s.jsonl
    mode_hybrid/events_5s.jsonl
    mode_events_only/events_2s.jsonl
    mode_events_only/events_5s.jsonl
  screenshots/
    hybrid/0001.png
    hybrid/0001.json
  asr/
    hybrid/meeting123.vtt
  ocr/
    0001.txt
  bundles/
    README_bundles.md
    task_001/
      prompt.txt
      context.md
      metrics.json
      output.json (manual)
    day_summary/
      prompt.txt
      context_index.json
      output.json
  report/
    report.html
    report.json
```

## Error Handling Strategy
- Subsystems operate with supervisor pattern; failure logs include error codes.
- Non-critical failures (e.g., OCR missing) degrade gracefully and noted in manifest.
- Critical failures (video capture crash) stop only that subsystem, continue others, flag in report.

## Logging & Telemetry
- Use structured JSON logs to maintain audit trail.
- Provide CLI flag `--log-level` for verbosity.
- Summaries written to manifest for quick status.

## Security & Privacy Considerations
- All binaries and dependencies vendored; no runtime downloads.
- Configurable allow/block lists and mask patterns; default-deny approach.
- Teardown command ensures secure deletion (optional secure wipe).

## Testing Strategy
- Unit tests for clustering, redaction, tokenizer counts, validation logic.
- Integration tests using recorded fixture data (demo_3min) for bundle/process/report pipelines.
- CLI smoke tests via `make demo` target to ensure command flow.

## Open Questions
- Validate and iterate on meeting detection heuristics (initial list: window titles containing "Zoom", "Meet", "Teams", "Webex").

