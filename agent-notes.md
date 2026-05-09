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
