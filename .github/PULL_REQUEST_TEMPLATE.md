## What this changes and why

<!-- The "why" matters more than the "what" here — see CONTRIBUTING.md's
     commit message conventions. Link the issue/draft-PR discussion this
     follows, if there was one. -->

## How it was verified

<!-- go test output, gosec/local CI run, or a "confirmed live on the VM/
     cluster" note (see docs/roadmap.md for what that looks like in this
     project) — whichever applies. No live cluster in CI, so anything
     that needs one is on you to state explicitly. -->

## Checklist

- [ ] `go build ./...` and `GOOS=linux go build ./...` both pass
- [ ] `gofmt -l .` prints nothing
- [ ] `go vet ./...` and `go test ./...` pass
- [ ] New behavior has a test (see CONTRIBUTING.md's "Testing expectations")
- [ ] Commits are signed off (`git commit -s`) — see [`DCO.md`](../DCO.md)
- [ ] Docs updated if this changes behavior a reader would rely on
      (`README.md`/`README.etudiants.md`, `docs/architecture.md`,
      `docs/roadmap.md`, ...)

## Scope

<!-- One exporter, one bug fix, one flag — not a grab-bag (see
     CONTRIBUTING.md). If this PR grew beyond its original scope, say so
     and why. -->
