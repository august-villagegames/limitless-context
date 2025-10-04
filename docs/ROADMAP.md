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
- [ ] Add ASR meeting detection and Whisper integration (optional dependency gate).
- [ ] Integrate OCR worker with optional Tesseract dependency.
- [ ] Harden privacy controls and allow-list enforcement.
- [ ] Implement pause/resume/kill coordination across subsystems.

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

