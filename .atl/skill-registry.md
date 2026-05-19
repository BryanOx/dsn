# Skill Registry

## Project: DSN (Deterministic Settlement Network)

Generated: 2026-05-14

---

## SDD Phases

| Skill | Trigger | Description |
|-------|---------|-------------|
| sdd-init | "sdd init", "iniciar sdd", "openspec init" | Initialize SDD context in project |
| sdd-explore | "/sdd-explore <topic>" | Explore and investigate ideas before committing |
| sdd-propose | "/sdd-new <change-name>" (auto) | Create change proposal with intent, scope, approach |
| sdd-spec | "/sdd-spec <change>" (auto) | Write specifications with requirements and scenarios |
| sdd-design | "/sdd-design <change>" (auto) | Create technical design with architecture decisions |
| sdd-tasks | "/sdd-tasks <change>" (auto) | Break down change into implementation task checklist |
| sdd-apply | "/sdd-apply <change>" (auto) | Implement tasks from change, write actual code |
| sdd-verify | "/sdd-verify <change>" (auto) | Validate implementation against specs and design |
| sdd-archive | "/sdd-archive <change>" (auto) | Sync delta specs to main specs and archive change |
| sdd-onboard | "/sdd-onboard" (auto) | Guided end-to-end SDD workflow walkthrough |

---

## Project Skills

| Skill | Trigger | Description |
|-------|---------|-------------|
| go-testing | Go tests, teatest, test coverage | Go testing patterns including Bubbletea TUI testing |
| branch-pr | Creating PR, opening PR, preparing changes for review | PR creation workflow following issue-first enforcement |
| issue-creation | Creating GitHub issue, reporting bug, requesting feature | Issue creation workflow for Agent Teams Lite |
| judgment-day | "judgment day", "review adversarial", "dual review", "juzgar" | Parallel adversarial review protocol |
| skill-creator | Creating new skill, adding agent instructions | Creates new AI agent skills |

---

## Compact Rules

### Go Testing (Project Standard)
- Use table-driven tests for multiple test cases
- Follow standard Go naming conventions (`*_test.go`)
- Use `testing.T` for assertions
- Consider golden file testing for complex outputs

### Branch/PR Workflow
- Every PR MUST link an approved issue
- Every PR MUST have exactly one `type:*` label
- Branch names must match: `^(feat|fix|chore|docs|style|refactor|perf|test|build|ci|revert)\/[a-z0-9._-]+$`

### SDD Phases
- All SDD phases follow: explore → propose → spec → design → tasks → apply → verify → archive
- Each phase has explicit read/write rules to engram or openspec
- Artifacts stored in: `sdd/{change-name}/{phase}`

---

## Notes

- This project uses **Go** as the primary language
- Tech stack: RocksDB/PebbleDB, libp2p, Ed25519/BLAKE3/BLS, Protobuf, WASM VM
- Strict TDD mode is enabled (test-first approach)
- Persistence: **engram** mode (default)