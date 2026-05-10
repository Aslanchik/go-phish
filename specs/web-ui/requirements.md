# web-ui: Requirements

## Overview

The web UI adds a browser-based interface to go-phish so security analysts can submit URLs, watch investigations progress in real time, and read structured reports without touching the CLI. The Go HTTP server exposes a versioned REST API and a real-time event stream; a frontend application consumes both. This is the first user-facing surface beyond stdout, and it must not weaken any of the pipeline's existing safety constraints.

---

## WU-1: URL submission

A security analyst can submit a URL for investigation from the browser.

**Acceptance criteria:**
- A submission form accepts a single URL and triggers a new investigation
- The frontend prevents submission of an empty input
- The frontend prevents submission of a string that is not recognisable as a URL (i.e. contains no scheme and no domain-like structure); this check happens before the request is sent
- The server rejects submissions from the API as well — a non-URL payload returns a 400 response with a human-readable error message and no investigation is created
- A successful submission returns the new investigation ID and status in the API response

---

## WU-2: Real-time investigation progress

A security analyst can watch phase transitions as an investigation runs, without polling or refreshing the page.

**Acceptance criteria:**
- The server pushes status updates to the client as the investigation moves through each phase: `fetching`, `hypothesizing`, `enriching`, `synthesizing`, `complete`, `failed`
- Each event carries at minimum: investigation ID, current status, and a timestamp
- When an investigation reaches `complete` or `failed`, a terminal event is delivered and the stream closes cleanly
- If the connection is interrupted mid-investigation, reconnecting to the same investigation's event stream resumes delivery of subsequent events; events that occurred during the gap are not required to be replayed
- The real-time mechanism (SSE or WebSocket) is specified and justified in design.md with explicit alternatives considered

---

## WU-3: Full report view

A security analyst can read the complete synthesis report for an investigation in the browser.

**Acceptance criteria:**
- The report view shows all five synthesis claims: brand impersonated, kit identification, exfiltration target, infrastructure notes, verdict
- Each claim is displayed alongside its confidence level (`low`, `medium`, or `high`)
- Evidence citations are visible for each claim, not hidden behind a toggle by default
- Investigation ID and start timestamp are present on the report view
- If synthesis is absent (investigation predates this feature or failed before synthesis), the view shows the Phase 2 hypothesis output and clearly labels it as a preliminary hypothesis rather than a final report

---

## WU-4: Captured screenshot display

The phishing page screenshot captured during Phase 1 is shown inline in the report view.

**Acceptance criteria:**
- The screenshot is displayed in the report view without requiring the analyst to download a file or open a separate tab
- If no screenshot was captured (fetch failed before screenshot was taken), the report view indicates the screenshot is unavailable rather than showing a broken image

---

## WU-5: Investigation history list

A security analyst can browse all past investigations.

**Acceptance criteria:**
- The history view lists all investigations stored in the database
- Each row in the list shows at minimum: the submitted URL, the verdict (or current status if not yet complete), and the investigation start timestamp
- The list is ordered with the most recent investigation first
- Investigations with status `failed` are included in the list and are visually distinguishable from completed ones
- The history list is reachable without first submitting a URL

---

## WU-6: Navigation from history to report

A security analyst can open the full report for any past investigation from the history list.

**Acceptance criteria:**
- Clicking a past investigation in the history list navigates to that investigation's full report view
- The report view for a past investigation is directly addressable by URL (i.e. a unique path exists per investigation that can be bookmarked or shared)

---

## WU-7: API versioning

All API endpoints are versioned.

**Acceptance criteria:**
- Every server endpoint used by the frontend is reachable under the `/api/v1/` path prefix
- Requests to paths outside `/api/v1/` return a 404 rather than silently routing to an unversioned handler
- The frontend never constructs API URLs that do not include the `/api/v1/` prefix

---

## WU-8: API response safety

The server does not leak internal state or credentials in API responses.

**Acceptance criteria:**
- No API response body contains Postgres connection strings, credentials, or DSN fragments
- No API response body contains a Go stack trace or internal runtime error detail; server errors return a generic message to the client and log the full detail server-side only
- These constraints hold for both success and error responses, including 4xx and 5xx responses

---

## Safety requirements

The web UI introduces a new network-reachable surface. None of the pipeline's existing safety constraints are relaxed.

- **S-WU-1:** Submitting a URL via the web UI invokes the same containerised fetch pipeline as the CLI — no shortcut path that bypasses container isolation exists
- **S-WU-2:** The API does not expose an endpoint that triggers form submission, page interaction, or any action beyond what Phase 1 already performs (page load and artifact capture only)
- **S-WU-3:** The server does not proxy or re-fetch attacker-controlled URLs on behalf of the browser to serve the screenshot — the screenshot is served from the database or local artifact store, not fetched from the phishing domain at display time

---

## Out of scope

- Authentication and multi-user access control
- Bulk URL submission (more than one URL per form action)
- Report export in any machine-readable or document format (PDF, CSV, JSON download)
- Dark mode or theme switching
- Manual labelling of `eval_labels` through the UI
- Triggering a re-investigation of an existing URL from the UI
- Any mobile-specific layout requirements
