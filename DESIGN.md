---
name: go-phish
description: Phishing investigation tool for security analysts
colors:
  signal-threat: "oklch(0.577 0.245 27)"
  signal-nominal: "oklch(0.627 0.194 149)"
  signal-outbound: "oklch(0.623 0.214 260)"
  signal-caution: "oklch(0.769 0.188 70)"
  ink-deep: "oklch(0.145 0 0)"
  ink-ui: "oklch(0.205 0 0)"
  ink-muted: "oklch(0.556 0 0)"
  surface-base: "oklch(1 0 0)"
  surface-muted: "oklch(0.97 0 0)"
  border-quiet: "oklch(0.922 0 0)"
  focus-ring: "oklch(0.708 0 0)"
typography:
  headline:
    fontFamily: "'Geist Variable', sans-serif"
    fontSize: "1.5rem"
    fontWeight: 600
    lineHeight: 1.2
  title:
    fontFamily: "'Geist Variable', sans-serif"
    fontSize: "1.125rem"
    fontWeight: 500
    lineHeight: 1.3
  body:
    fontFamily: "'Geist Variable', sans-serif"
    fontSize: "0.875rem"
    fontWeight: 400
    lineHeight: 1.5
  label:
    fontFamily: "'Geist Variable', sans-serif"
    fontSize: "0.75rem"
    fontWeight: 500
    lineHeight: 1.4
    letterSpacing: "0.02em"
  mono:
    fontFamily: "ui-monospace, 'SF Mono', Menlo, monospace"
    fontSize: "0.75rem"
    fontWeight: 400
    lineHeight: 1.6
rounded:
  sm: "6px"
  md: "8px"
  lg: "10px"
  pill: "9999px"
spacing:
  sm: "8px"
  md: "16px"
  lg: "24px"
components:
  button-primary:
    backgroundColor: "{colors.ink-ui}"
    textColor: "{colors.surface-base}"
    rounded: "{rounded.lg}"
    padding: "0 10px"
    height: "32px"
  button-primary-hover:
    backgroundColor: "oklch(0.205 0 0 / 0.8)"
    textColor: "{colors.surface-base}"
    rounded: "{rounded.lg}"
    padding: "0 10px"
    height: "32px"
  button-outline:
    backgroundColor: "{colors.surface-base}"
    textColor: "{colors.ink-deep}"
    rounded: "{rounded.lg}"
    padding: "0 10px"
    height: "32px"
  button-ghost:
    backgroundColor: "transparent"
    textColor: "{colors.ink-deep}"
    rounded: "{rounded.lg}"
    padding: "0 10px"
    height: "32px"
  badge-high:
    backgroundColor: "{colors.ink-ui}"
    textColor: "{colors.surface-base}"
    rounded: "{rounded.pill}"
    padding: "2px 8px"
    height: "20px"
  badge-medium:
    backgroundColor: "{colors.surface-muted}"
    textColor: "{colors.ink-deep}"
    rounded: "{rounded.pill}"
    padding: "2px 8px"
    height: "20px"
  badge-low:
    backgroundColor: "oklch(0.577 0.245 27 / 0.1)"
    textColor: "{colors.signal-threat}"
    rounded: "{rounded.pill}"
    padding: "2px 8px"
    height: "20px"
  input-default:
    backgroundColor: "transparent"
    textColor: "{colors.ink-deep}"
    rounded: "{rounded.lg}"
    padding: "4px 10px"
    height: "32px"
---

# Design System: go-phish

## 1. Overview

**Creative North Star: "The Intelligence Brief"**

The design language of go-phish is the intelligence brief: a document that conveys authority through density and structure, not through visual polish. Every surface is a flat page. Presence comes from the weight of the information on it, not from the interface's self-decoration. When the analyst opens a report, they should feel the weight of evidence, not the warmth of a product.

The system is almost entirely achromatic. Geist Variable carries the hierarchy through weight and scale contrast. Color appears only when it means something: threat signals are vermilion, safe signals are green, outbound tool calls are blue, caution states are amber. The four signals form a directional vocabulary — if you see color, the system is telling you something. If you don't see color, the system is working normally and nothing requires your attention.

This rejects three named failure modes: the SaaS dashboard aesthetic (gradient cards, hero metrics, identical icon-heading-text grids), the security-tool cliché (green-on-black terminal cosplay, red-alert banners on every state, Shodan-style raw data walls), and the clinical blank (a purely white form with no character or presence). The Intelligence Brief has presence because a serious document has presence.

**Key Characteristics:**
- Achromatic by default; color is always signal, never decoration
- Hierarchy through type weight and scale, not cards or chrome
- Flat surfaces — borders and tonal contrast convey depth, no shadows
- Monospace for machine-produced data only (IDs, tool calls, raw artifacts)
- WCAG AA throughout: 4.5:1 text, 3:1 UI components

## 2. Colors: The Signal Palette

The palette is a controlled grayscale field interrupted by four named signal colors. The signals are not decorative — they form a closed directional vocabulary. Every chromatic element in the interface is one of these four.

### Primary (Signal)

- **Alert Vermilion** (oklch(0.577 0.245 27)): Threat and failure states. Low-confidence claims (`badge-low`), failed investigations, API errors, invalid input. The only warm color in the system. Its warmth is the warning.
- **Clear Green** (oklch(0.627 0.194 149)): Nominal and complete states. Successful investigation completions, inbound tool results (`←`), confirmed-clean signals. Used sparingly; its rarity is the point.
- **Dispatch Blue** (oklch(0.623 0.214 260)): Outbound direction in the live stream (`→` tool calls). Directional, not evaluative — it means "sent", not "good".
- **Standby Amber** (oklch(0.769 0.188 70)): Caution and transitional states. Reconnecting stream, borderline confidence states. Not danger, not safe — in-between.

### Neutral

- **Ink Deep** (oklch(0.145 0 0)): Primary prose text, body copy, evidence text. Near-black; not absolute black.
- **Ink UI** (oklch(0.205 0 0)): Primary interactive fills — buttons, high-confidence badges. Slightly lighter than Ink Deep to prevent interactive elements from disappearing into body text.
- **Ink Muted** (oklch(0.556 0 0)): Secondary text, metadata, timestamps, field labels, placeholder text.
- **Document White** (oklch(1 0 0)): Page background. The document rests on a white page.
- **Fog Surface** (oklch(0.97 0 0)): Muted container backgrounds — code blocks, live stream panel, empty states.
- **Ruled Line** (oklch(0.922 0 0)): All borders and dividers. Every container boundary is this color.
- **Focus Ring** (oklch(0.708 0 0)): Interactive focus rings (`ring-3 ring-ring/50`). Mid-gray; visible but unobtrusive.

### Named Rules

**The Four Signals Rule.** The only chromatic colors in the system are Alert Vermilion, Clear Green, Dispatch Blue, and Standby Amber. These are not decorative options to reach for when something feels plain. If a new state needs visual distinction, use typography weight or spacing — not a fifth color. Any addition must replace one of the four, not extend the list.

**The No Tint Rule.** Neutral surfaces are pure achromatic OKLCH (chroma = 0). Do not add hue-tinted backgrounds, branded fills, or gradient overlays to section containers, cards, or page regions. The page is a document. Documents don't glow.

## 3. Typography

**Body/UI Font:** Geist Variable (single family for headings, body, and labels)
**Mono Font:** ui-monospace, SF Mono, Menlo, monospace

**Character:** Geist Variable is clean and technical without being cold. A single variable family eliminates font-switching noise; hierarchy comes from weight and scale contrast alone. Monospace enters only where the data is machine-produced — investigation IDs, tool call arguments, raw artifact content. When you see monospace, you're reading something the system generated, not something the analyst wrote.

### Hierarchy

- **Headline** (600, 1.5rem / 24px, line-height 1.2): Page titles. One per view. Never reused within a view for sub-sections.
- **Title** (500, 1.125rem / 18px, line-height 1.3): Section headings within a view (e.g. "Synthesis", "Recent Investigations").
- **Body Strong** (600, 0.875rem / 14px, line-height 1.5): Claim values, primary finding text, any prose that must carry semantic weight at body scale.
- **Body** (400, 0.875rem / 14px, line-height 1.5): Evidence text, descriptions, general prose. Max line length 65ch.
- **Label** (500, 0.75rem / 12px, line-height 1.4, tracking +0.02em): Field labels, claim type labels, badge text, metadata keys.
- **Mono** (400, 0.75rem / 12px, line-height 1.6): Investigation IDs, URLs, tool call inputs/outputs, JSON artifact content.

### Named Rules

**The Mono Discipline Rule.** Monospace is reserved for machine-produced data only. Never use it on prose labels, navigation items, section headings, or descriptive text. Only on values the system generated, received, or captured from an external source.

**The One Headline Rule.** Every view has exactly one Headline (600/1.5rem). If content feels under-emphasized, escalate to Body Strong or Title — never to a custom intermediate size. An in-between size signals a broken scale, not emphasis.

## 4. Elevation

This system is flat. All depth is conveyed through background tonal contrast (Document White vs Fog Surface) and Ruled Line borders. There are no box-shadows on resting surfaces.

The only depth-like effect is the interactive focus ring: `box-shadow: 0 0 0 3px oklch(0.708 0 0 / 0.5)` on focused inputs and buttons. This is a state indicator, not a layering affordance.

### Named Rules

**The Flat Document Rule.** No `box-shadow` values on any resting surface — not on claim blocks, containers, cards, inputs, or nav. If a surface needs to feel distinct, use `border: 1px solid oklch(0.922 0 0)` or a Fog Surface background fill. The system is a document, not a 3D stage.

## 5. Components

### Buttons

Compact and functional. Rounded at `--radius-lg` (10px). Heights are prescribed; buttons are never taller than the density requires.

- **Shape:** Gently rounded (10px radius)
- **Primary:** Ink UI fill (oklch(0.205 0 0)), Document White text, `h-8` (32px), `px-2.5` padding, 500 weight, 14px
- **Hover:** Primary fades to 80% opacity. No scale or shadow.
- **Focus:** `border-ring` shift + `ring-3 ring-ring/50` (3px ring at 50% opacity of Focus Ring)
- **Outline:** 1px Ruled Line border, transparent background, Ink Deep text. Hover: Fog Surface fill.
- **Ghost:** No border, no fill. Hover: Fog Surface fill, Ink Deep text.

### Confidence Badges

The primary place signal colors appear in the report. Pill-shaped labels communicating claim certainty.

- **Shape:** Fully pill (border-radius 9999px), `h-5` (20px), `px-2` (8px horizontal padding), 500 weight, 12px
- **High:** Ink UI fill, Document White text
- **Medium:** Fog Surface fill, Ink Deep text
- **Low:** Alert Vermilion at 10% opacity fill, Alert Vermilion text
- **Rule:** Confidence badges are the only regular-use surface where signal colors appear in the report view. Every other colored element is in the live stream.

### Claim Blocks

The signature component. Each synthesis claim is a bordered container: label + confidence badge on top, Body Strong value below, Body evidence beneath.

- **Shape:** `border-radius: 10px`, `border: 1px solid oklch(0.922 0 0)`
- **Background:** Transparent (sits on Document White)
- **Padding:** 16px all sides
- **Layout:** flex-column, 8px gap between rows; label-badge row uses space-between alignment
- **Evidence text:** Visible by default, never hidden. Body/400/14px, Ink Muted.

### Inputs / Fields

Single-line URL entry. Transparent background so the field reads as a rule on the page rather than a filled box.

- **Style:** Transparent bg, 1px Ruled Line border, 10px radius, `h-8` (32px)
- **Focus:** Border shifts to Focus Ring + `ring-3 ring-ring/50`
- **Error:** Border shifts to Alert Vermilion + `ring-3 oklch(0.577 0.245 27 / 0.2)`
- **Disabled:** 50% opacity, no pointer events

### Live Stream (ProgressStream)

The signature real-time component. A scrolling log panel with a header bar and mono-type entry rows — the most visually distinctive surface in the product.

- **Container:** 10px radius, 1px Ruled Line border, Fog Surface at 30% opacity background
- **Header:** Bottom-ruled, uppercase Label text (500/12px, tracking-wider), Ink Muted. Reconnecting state shows Standby Amber text on the right.
- **Entry rows:** ui-monospace/12px, 2px vertical padding, flex-row with 64px fixed timestamp column
- **Outbound arrow** (`→`): Dispatch Blue. Means "tool invoked."
- **Inbound arrow** (`←`): Clear Green. Means "result received."
- **Phase transitions:** Body Strong (600/14px), Ink Deep, uppercase phase name
- **Complete row:** Clear Green, font-semibold, `✓ Complete — verdict: [value]`
- **Failed row:** Alert Vermilion, font-semibold, `✗ Failed — [reason]`

### Investigation List

Table-style list of past investigations. Minimal; three columns — URL, verdict/status, timestamp.

- **URL column:** `font-mono text-sm`, truncated with `truncate`, Ink Deep
- **Verdict column:** Confidence badge if complete; plain Label text (Ink Muted) for in-progress statuses
- **Timestamp:** Label/12px, Ink Muted
- **Row hover:** Fog Surface background fill, no border change
- **Failed rows:** No special background tint — the failed verdict badge (Alert Vermilion) is the only visual signal

## 6. Do's and Don'ts

### Do:

- **Do** reserve the four signal colors for their exact roles. Alert Vermilion = threat/failure. Clear Green = complete/nominal. Dispatch Blue = outbound. Standby Amber = caution/transitional. A new state that doesn't fit one of these four should be expressed through typography, not a fifth color.
- **Do** show evidence text by default on every claim block. Never hide it behind a toggle at rest. Completeness is a design principle, not a UX tradeoff.
- **Do** use monospace exclusively for machine-produced data: investigation IDs, submitted URLs, tool call inputs and outputs, raw artifact content.
- **Do** keep all surfaces flat. Tonal contrast (`bg-muted`) and Ruled Line borders are the depth vocabulary.
- **Do** conform to WCAG AA: 4.5:1 for all text, 3:1 for UI component boundaries. Alert Vermilion text over its 10% opacity fill exceeds 4.5:1. Verify any new signal color placement before shipping.
- **Do** respect `prefers-reduced-motion` for the live stream's scroll-to-bottom behavior and any phase-transition animations.

### Don't:

- **Don't** use SaaS dashboard patterns: gradient cards, hero metric tiles, identical icon-heading-text card grids, or the Vercel/Linear copycat aesthetic. This tool processes forensic evidence. It is not a product landing page.
- **Don't** use security-tool clichés: green-on-black terminal cosplay, red-alert banners on non-failure states, Shodan-style raw data walls. Alert Vermilion appears on low-confidence claims and failures only — not as ambient tension.
- **Don't** produce a clinical blank form. Presence comes from weight contrast, monospace registers, and precise spacing. If a surface feels empty, fix the typography hierarchy before reaching for color or decoration.
- **Don't** add a sixth signal color. The four-signal vocabulary is closed. Expanding it dilutes every entry.
- **Don't** add `box-shadow` to resting surfaces. No card floats, no ambient glows, no elevated containers.
- **Don't** use gradient text (`background-clip: text`), glassmorphism, or side-stripe borders (`border-left > 1px` as a colored accent). These are banned system-wide.
- **Don't** add dark mode. It is explicitly out of scope for v1. The dark mode tokens in shadcn's default CSS are present but unused by the product.
- **Don't** use `border-left` greater than 1px as a colored accent stripe on claim blocks, list items, or callouts. If something needs distinction, use a full border or a Fog Surface fill.
