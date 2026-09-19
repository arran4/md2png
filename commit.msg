feat: add structured diagnostics and deliberate safe raw HTML rendering policy

Adds structured rendering diagnostics and a deliberate safe raw-HTML policy
to the md2png renderer, enabling programmatic access to degradation info
without embedding visual warnings into output files.

Includes a `--strict` mode in the CLI that returns a non-zero exit code
for degraded renders, mapping internally to fine-grained `DiagnosticPolicy`
configurations.

Raw HTML is now securely handled: ordinary content is extracted without
markup, scripts/styles are entirely ignored, and `<br>` translates to newlines.

Existing golden image testdata/golden/unsupported_nodes.png was regenerated using UPDATE_GOLDEN=1 go test ./... because visual text is no longer natively injected into the canvas.

Fixes #91
Fixes #97
