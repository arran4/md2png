Fixes #86

# Housekeeping
Confirmed #64 is closed by #115 and #116 is superseded.

# Audit Results
- #67: syntax highlighting for fenced code blocks (Deferred)
- #68: SVG/vector output (Deferred)
- #69: configurable semantic themes / YAML/JSON (Deferred)
- #70: emphasis and common GFM inline/list semantics (Deferred)
- #73: table sizing/alignment/narrow-width behaviour (Deferred)
- #74: font backend modernization and fallback stacks (Deferred)
- #80: md2view live reload and viewport controls (Deferred)
- #86: deterministic renderer regression and fuzz safety net (Implemented)
- #91: structured rendering diagnostics (Deferred)
- #93: Markdown footnotes (Deferred)
- #94: transparent PNG/GIF backgrounds (Deferred)
- #95: explicit image sizing/alignment (Deferred)
- #96: batch rendering (Deferred)
- #97: safe raw-HTML policy (Deferred)
- #99: separate layout from raster painting (Deferred)
- #114: remove `gosubc_overlay` once upstream blockers are resolved (Blocked on arran4/go-subcommand#465 and arran4/go-subcommand#466)

# Changes for #86
Added deterministic visual regression tests in `regression_test.go` checking basic renderer features:
- Headings and paragraphs
- Nested lists
- Inline styling
- Code blocks
- Blockquotes and HRs
- Tables
- Local images
- Footnotes
- Unsupported nodes
- Dark theme

# How to update golden files
To re-generate golden files, developers should run:
```
UPDATE_GOLDEN=1 go test ./...
```
This explicit local action prevents CI from silently overwriting them.

# Fuzz tests
Added `FuzzRenderer` with various seeding Markdown items.
- It is constrained with `MaxHeight: 1000` and `Width: 200` to prevent enormous allocations.
- It operates with the network disabled (AllowLocal=false, AllowRemote=false).
- Tested bounds checks using short test durations, passing cleanly.

# Testing and Validation
- `go fmt ./...`: Ran cleanly
- `go test ./...`: Passed
- `go vet ./...`: Clean
- Checked generated markdown (`go run ./cmd/md2png --in README.md --out output.png`) and it succeeded.
- Re-run of `go generate ./...` and `git diff --exit-code` matched with a clean tree (or no changes).
