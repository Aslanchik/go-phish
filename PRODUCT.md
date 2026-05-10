# Product

## Register

product

## Users

Solo security researcher or analyst sitting down to investigate a specific suspicious URL they received. Deliberate mode, not under triage pressure. They want completeness: every piece of evidence visible, every confidence level explicit, nothing hidden behind a toggle or a second click. They are technically sophisticated and will distrust the tool if it looks like it's performing rather than working.

## Product Purpose

go-phish takes a suspicious URL and produces a structured phishing investigation report: brand impersonated, kit mechanics, credential exfiltration destination, infrastructure overlap, and a confidence-rated verdict with per-claim evidence. The analyst submits a URL, watches the four-phase pipeline run in real time (fetch → hypothesize → enrich → synthesize), and reads the structured findings. Success looks like a report the analyst can act on and cite, not a score they have to interpret.

## Brand Personality

Sharp, Confident, Alert. A tool that knows what it is. Slight purposeful tension in the visual system: color carries signal, not decoration. When something is red, it means something is wrong. When something is amber, the tool is uncertain. The tool never hedges visually for no reason.

## Anti-references

- **SaaS dashboard generic:** No hero metrics, no gradient cards, no icon-heading-text card grids. The Vercel/Linear aesthetic doesn't belong here.
- **Security-tool cliché:** No green-on-black terminal cosplay, no Shodan-style raw data walls, no red-alert banners that trigger on everything. The tool investigates phishing — it doesn't perform being a hacker tool.
- **Clinical/sterile:** Not a blank white medical form. The tool should have visual presence and character. Utility alone is not a personality.

## Design Principles

1. **Signal over decoration.** Color appears only when it means something: threat confidence, phase status, verdict severity. A neutral page is not a failure — absence of color means absence of signal.
2. **Completeness first.** The analyst is in deliberate mode. Evidence is visible by default, not behind toggles. Dense information reads through hierarchy, not through progressive disclosure.
3. **Presence without performance.** The tool has a distinct visual character. It does not perform being dangerous or technical. It looks like a serious instrument.
4. **Hierarchy through typography.** Dense information reads clearly through scale and weight contrast. Cards are not the default affordance.
5. **Honest about uncertainty.** Exact timestamps, exact IDs, explicit confidence labels (low / medium / high) with evidence. The tool never implies more certainty than it has.

## Accessibility & Inclusion

WCAG AA: 4.5:1 text contrast, 3:1 UI component contrast. No hard AAA requirement. Reduced-motion preference should be respected for any progress animations in the live stream view.
