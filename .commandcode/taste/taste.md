# Taste

## Code review / audits
- For project audits, wants comprehensive multi-dimension coverage — explicitly asked for "performance, security, bugs, etc" (open-ended scope), and findings organized by severity with a prioritized fix order, not a single-focus review. Confidence: 0.6
- Expects audits to be evidence-based, not static reading alone: run `go build`/`go vet`/`go test`, check git history for committed secrets, and cite concrete file:line references in findings. Confidence: 0.5

## Communication style
- Gives terse, informal, low-detail briefs (lowercase, catch-all "etc") and delegates scoping/detail decisions to the agent; expects autonomous execution without follow-up questions mid-task. Confidence: 0.5
