# Agent Notes

Observations from live runs. Updated after each investigation that reveals something worth noting.

---

## 2026-05-09 — First end-to-end run (Phase 3)

**URL:** `https://ledgr-access.wixstudio.com/learn`
**Tool calls:** 8 across multiple turns
**Outcome:** Correctly identified as Ledger impersonation / crypto phishing

### Tool call ordering

The model called tools in a sensible order without guidance: WHOIS and cert transparency first (registration-age signal), then URLhaus and urlscan for reputation, then analyze_js on the page scripts. It did not need to be prompted to prioritise WHOIS — the system prompt mentions registration age as "highest-signal" which it followed.

### What the model recovered from

- `urlscan_lookup` returned `{"error": "URLSCAN_API_KEY not set"}` — the model noted the missing key in its summary and continued without it rather than stalling.
- The egress proxy blocked all Wix CDN/analytics domains (`static.parastorage.com`, `frog.wix.com`, etc.) — this produced a partially rendered page with some JS blocked, but the model still extracted enough signal from the DOM metadata to identify the brand correctly.

### Failure modes not yet seen

- Iteration cap hit (all 8 calls completed within 10-turn default)
- analyze_js truncation (page JS was under 50 000 chars)
- WHOIS timeout

### Open questions

- Does tool call order matter for accuracy? WHOIS-first seems natural but hasn't been varied.
- The model produced a confident verdict from metadata alone (Ledger brand in page title/description). Would it be as confident on a page that renders correctly with all CDN assets loaded?

---

## 2026-05-09 — Phase 4 smoke test (synthesis)

**URL:** `https://ledgr-access.wixstudio.com/learn`
**Investigation ID:** `ebbad703-2fba-47b3-9639-fe92e65b9d60`
**Tool calls (Phase 3):** 9 across 4 turns
**Outcome:** Correctly synthesised as `phishing` / `high` confidence

### Verdict accuracy

Phase 4 produced a `phishing` / `high` verdict consistent with the Phase 2 hypothesis. The synthesis model cited specific signals across all three input blocks — page metadata (Phase 1 artifacts), hypothesis (Phase 2), and tool call trace (Phase 3). All five claims carried non-empty evidence strings.

### Tool ordering vs. synthesis quality

The enrichment phase ran 9 calls (2 turns of infra tools, then analyze_js, then one final turn). Synthesis consumed the full trace without summarisation and correctly attributed claims to their source tools (e.g., "whois_lookup: registrant privacy via Domains By Proxy LLC"). This validates the "full trace, not summary" design decision.

### Evidence quality observations

- **Brand** and **Verdict** evidence was high-quality — specific strings pulled from analyze_js output and page metadata.
- **Kit identification** was correctly marked `medium` confidence (no active harvesting logic present, staging state). The model did not hallucinate a named kit.
- **Exfil target** was marked `medium` because no active exfil endpoint was found; the model correctly attributed the uncertainty and noted what was missing rather than inventing an endpoint.
- **Infrastructure** was detailed and accurate — correctly noted Wix free subdomain, parent domain WHOIS date, and failed API calls (cert transparency 502, urlscan 400, urlhaus 401) without treating failures as verdicts.

### Synthesis vs. Phase 2 divergence

Phase 2 and Phase 4 agreed on brand and verdict. Phase 4 added the "multi-brand kit" observation (Uphold reference alongside Ledger) that Phase 2 missed — this is exactly the value of the enrichment trace in synthesis. The Phase 2 hypothesis reference section in the report makes this divergence visible.

### Failure modes observed

- All three API errors (cert_transparency 502, urlscan 400, urlhaus 401) were correctly represented as "inconclusive" rather than causing synthesis to fail.
- Wix CDN blocking by the egress proxy produced zero JS files loaded — synthesis handled this gracefully, noting "0 external JS files" in its kit assessment.

### Open questions (Phase 4)

- How does synthesis behave when Phase 3 enrichment is skipped (zero tool calls)? The "empty trace → `[]`" path exists but hasn't been tested with a real API call.
- Per-claim confidence calibration: high-confidence verdicts on staging/incomplete phishing pages may be overconfident — the page had no active harvesting logic yet the verdict was `high`. Worth tracking in the eval harness.
