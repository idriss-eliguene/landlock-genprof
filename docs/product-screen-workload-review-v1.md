# Workload Security Review Screen v1

## Purpose

This is the first product screen the future UI should implement.

It is not a dashboard. It is a focused review workspace for one workload and
one `SecurityProfileProposal`.

The screen should help a reviewer answer five questions quickly:

1. What workload is this?
2. What was observed?
3. What is being recommended?
4. How confident is the system?
5. What exact artifacts would be applied?

## Primary user

Platform security engineer reviewing a proposal before enforcement.

## Data source

The screen is sourced primarily from:

- `SecurityProfileProposal`
- `SecurityRecommendation`
- `TrainingHistory` summary when available

## Screen structure

### A. Header band

Content:

- Workload name
- Namespace
- Container
- Binary
- Proposal generation timestamp
- Review status placeholder: draft for v1

Visual treatment:

- Large workload title
- Compact metadata row beneath it
- Confidence badge on the right

### B. Recommendation summary rail

Content:

- Overall confidence
- Number of training runs
- Number of domains with generated artifacts
- Primary next action: review artifacts

Visual treatment:

- Horizontal metrics row
- Confidence shown as a strong bar with a percentage label
- No decorative charts

### C. Domain cards

One card per domain:

- Filesystem
- Network
- Syscalls
- Hardening

Each card shows:

- Domain name
- Availability state
- Required item count
- Backend artifact mapping
- Rationale summary
- Confidence cue

Example card copy:

- Filesystem: observed stable file access patterns mapped to PodLock profile
- Network: observed connect/bind behavior mapped to NetworkPolicy
- Syscalls: observed syscall set mapped to SPO SeccompProfile
- Hardening: observed capability/runtime signals mapped to securityContext

### D. Evidence panel

Purpose:

Explain why the recommendation exists without forcing the user into raw YAML
immediately.

Content:

- Short narrative summary of observed behavior
- Training history note
- Warning callouts for low-confidence or sparse observations

This is where the product should say things like:

- “Observed over 4 training runs”
- “Syscall set still low-confidence: review before enforcement”
- “No network activity observed in this run”

### E. Artifact review tabs

Tabs:

- PodLock
- NetworkPolicy
- Patched Manifest
- SPO SeccompProfile

Each tab contains:

- YAML viewer with line wrapping
- Copy action
- Export action
- Optional download action

The Patched Manifest tab has one extra visual assertion:

- highlight the PodLock label when present

## Interaction model

### Default landing

The screen should open on the summary view with the `Patched Manifest` tab
preselected when available, because it best represents the final combined
enforcement intent.

### Reviewer path

1. Confirm workload identity
2. Check overall confidence and training run count
3. Scan domain cards for coverage and risk
4. Open patched manifest and verify label/securityContext
5. Inspect specialized artifacts only if needed

### Empty-state behavior

If a domain has no artifact:

- show the card as “not generated”
- explain why in plain language
- do not show an empty tab with meaningless content

## Content rules

- Prefer short, explicit, operational wording.
- Avoid “AI-generated” phrasing.
- Avoid vague confidence labels with no evidence.
- Every backend mention must name the concrete Kubernetes mechanism.

## Visual design direction

### Tone

- calm
- serious
- technical
- review-oriented

### Layout

- Wide desktop-first review surface
- Strong left-to-right scan path
- Tight spacing for metadata, larger spacing between major review blocks

### Component language

- Evidence chips
- Confidence bars
- Domain cards
- YAML tabs
- Caution callouts
- Command snippets for follow-up actions

### Colors

- Base: graphite, charcoal, slate
- Positive: muted green
- Caution: amber
- Neutral structure: steel blue
- Avoid bright neon or overly playful accents

### Typography

- Headings: precise editorial sans
- Metadata and artifacts: strong monospace support
- Numbers and confidence metrics must be highly legible

## Mobile behavior

The MVP is desktop-first, but the layout should still collapse predictably:

- header stacks vertically
- domain cards become a vertical list
- artifact tabs become a segmented control or accordion
- YAML viewer remains horizontally scrollable

## v1 non-goals

- Multi-workload fleet dashboard
- Trend analytics page
- Approval workflow controls beyond placeholder status
- Full diffing between proposal versions

## Success criteria

- A reviewer understands the recommendation in under 30 seconds.
- A reviewer can identify the main enforcement artifact in one glance.
- A reviewer can verify the PodLock label in the patched manifest quickly.
- A reviewer does not need to inspect raw CRD structure unless they want to.