# v0.7 Evidence Fidelity Acceptance Criteria

Slice 4 is complete when all of the following hold:

- dynamic HTML noise alone does not produce a finding;
- a same-size HTML structural change is detected as `html_structure_changed`;
- header-only behavior is detected without exposing raw response-header values;
- rotating request-ID noise is ignored;
- behavior reproduced by a random-name control is rejected;
- existing JSON path semantics still identify stable response changes while dynamic JSON fields remain excluded;
- `go test ./...`, `go vet ./...`, `go test -race ./...`, Linux build, and Windows cross-build all pass.
