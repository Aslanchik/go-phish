import { useEffect, useState } from 'react'
import { ChevronDown } from 'lucide-react'
import { cn } from '@/lib/utils'
import type { Claim, Hypothesis, InvestigationDetail, Synthesis } from '@/types'

// ---- Verdict signal detection ----

type VerdictSignal = 'threat-high' | 'threat-medium' | 'threat-low' | 'clean' | 'uncertain'

function verdictSignal(claim: Claim): VerdictSignal {
  const v = claim.value.toLowerCase()
  const isClean =
    v.includes('no threat') ||
    v.includes('clean') ||
    v.includes('legitimate') ||
    v.includes('benign') ||
    v.includes('not phishing')
  const isThreat =
    v.includes('phishing') ||
    v.includes('malicious') ||
    v.includes('suspicious') ||
    v.includes('confirmed') ||
    v.includes('likely')

  if (isClean) return 'clean'
  if (!isThreat) return 'uncertain'
  if (claim.confidence === 'high') return 'threat-high'
  if (claim.confidence === 'medium') return 'threat-medium'
  return 'threat-low'
}

// ---- Hero color tokens ----
// Border + light tint only. Text is always ink-deep — no contrast compromise.

type HeroStyle = { bg: string; fg: string; border: string; pillBg: string; pillFg: string }

const HERO_STYLES: Record<VerdictSignal, HeroStyle> = {
  'threat-high': {
    bg: 'oklch(0.577 0.245 27 / 0.055)',
    fg: 'oklch(0.145 0 0)',
    border: '1.5px solid oklch(0.577 0.245 27)',
    pillBg: 'oklch(0.577 0.245 27 / 0.12)',
    pillFg: 'oklch(0.47 0.22 27)',
  },
  'threat-medium': {
    bg: 'oklch(0.769 0.188 70 / 0.07)',
    fg: 'oklch(0.145 0 0)',
    border: '1.5px solid oklch(0.65 0.18 70)',
    pillBg: 'oklch(0.65 0.18 70 / 0.18)',
    pillFg: 'oklch(0.48 0.15 70)',
  },
  'threat-low': {
    bg: 'oklch(0.577 0.245 27 / 0.04)',
    fg: 'oklch(0.145 0 0)',
    border: '1px solid oklch(0.577 0.245 27 / 0.4)',
    pillBg: 'oklch(0.577 0.245 27 / 0.1)',
    pillFg: 'oklch(0.577 0.245 27)',
  },
  clean: {
    bg: 'oklch(0.627 0.194 149 / 0.06)',
    fg: 'oklch(0.145 0 0)',
    border: '1.5px solid oklch(0.627 0.194 149)',
    pillBg: 'oklch(0.627 0.194 149 / 0.15)',
    pillFg: 'oklch(0.40 0.16 149)',
  },
  uncertain: {
    bg: 'oklch(0.97 0 0)',
    fg: 'oklch(0.145 0 0)',
    border: '1px solid oklch(0.922 0 0)',
    pillBg: 'oklch(0.145 0 0 / 0.07)',
    pillFg: 'oklch(0.556 0 0)',
  },
}

function heroConfidenceLabel(c: Claim['confidence']): string {
  return c === 'medium' ? 'med' : c
}

// ---- Verdict hero ----

function VerdictHero({ claim }: { claim: Claim }) {
  const signal = verdictSignal(claim)
  const s = HERO_STYLES[signal]

  return (
    <div
      className="rounded-lg px-6 py-5 flex flex-col gap-3"
      style={{ backgroundColor: s.bg, border: s.border }}
    >
      <div className="flex items-start justify-between gap-6">
        <p
          className="text-[1.125rem] font-semibold leading-snug"
          style={{ color: s.fg, maxWidth: '55ch', textWrap: 'balance' } as React.CSSProperties}
        >
          {claim.value}
        </p>
        <span
          className="shrink-0 text-[10px] font-medium tracking-widest uppercase rounded-full px-2.5 py-1 leading-none mt-0.5"
          style={{ backgroundColor: s.pillBg, color: s.pillFg }}
        >
          {heroConfidenceLabel(claim.confidence)}
        </span>
      </div>
      {claim.evidence?.length > 0 && (
        <ul className="flex flex-col gap-1 mt-0.5">
          {claim.evidence.map((point, i) => (
            <li key={i} className="flex gap-2 text-[0.8125rem] leading-relaxed text-muted-foreground">
              <span className="shrink-0 select-none mt-px">·</span>
              <span>{point}</span>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

function FailedHero({ error }: { error?: string }) {
  return (
    <div
      className="rounded-lg px-6 py-5 flex flex-col gap-2"
      style={{
        backgroundColor: 'oklch(0.577 0.245 27 / 0.055)',
        border: '1.5px solid oklch(0.577 0.245 27)',
      }}
    >
      <div className="flex items-start justify-between gap-6">
        <p className="text-[1.125rem] font-semibold text-foreground">
          Investigation failed
        </p>
        <span
          className="shrink-0 text-[10px] font-medium tracking-widest uppercase rounded-full px-2.5 py-1 leading-none mt-0.5"
          style={{ backgroundColor: 'oklch(0.577 0.245 27 / 0.12)', color: 'oklch(0.47 0.22 27)' }}
        >
          Error
        </span>
      </div>
      {error && (
        <p className="text-sm text-muted-foreground max-w-[65ch]">{error}</p>
      )}
    </div>
  )
}

// ---- Confidence pill (for claim cards) ----

function ConfidencePill({ confidence }: { confidence: Claim['confidence'] }) {
  if (confidence === 'high') {
    return (
      <span className="text-[10px] font-medium tracking-widest uppercase rounded-full px-2 py-0.5 bg-primary text-primary-foreground leading-none">
        High
      </span>
    )
  }
  if (confidence === 'medium') {
    return (
      <span
        className="text-[10px] font-medium tracking-widest uppercase rounded-full px-2 py-0.5 leading-none"
        style={{ backgroundColor: 'oklch(0.769 0.188 70 / 0.18)', color: 'oklch(0.46 0.14 70)' }}
      >
        Med
      </span>
    )
  }
  return (
    <span
      className="text-[10px] font-medium tracking-widest uppercase rounded-full px-2 py-0.5 leading-none"
      style={{ backgroundColor: 'oklch(0.577 0.245 27 / 0.1)', color: 'oklch(0.577 0.245 27)' }}
    >
      Low
    </span>
  )
}

function safeConfidence(s: string): Claim['confidence'] {
  if (s === 'high' || s === 'medium' || s === 'low') return s
  return 'low'
}

// ---- Claims accordion ----

type ClaimKey = 'brand_impersonated' | 'kit_identification' | 'exfil_target' | 'infrastructure_notes'

const CLAIM_ORDER: { key: ClaimKey; label: string }[] = [
  { key: 'brand_impersonated', label: 'Brand Impersonated' },
  { key: 'kit_identification', label: 'Kit Identification' },
  { key: 'exfil_target', label: 'Exfiltration Target' },
  { key: 'infrastructure_notes', label: 'Infrastructure Notes' },
]

function ClaimsAccordion({ synthesis }: { synthesis: Synthesis }) {
  const [open, setOpen] = useState<Set<ClaimKey>>(new Set(['brand_impersonated']))

  function toggle(key: ClaimKey) {
    setOpen((prev) => {
      const next = new Set(prev)
      next.has(key) ? next.delete(key) : next.add(key)
      return next
    })
  }

  return (
    <div className="rounded-lg border border-border overflow-hidden">
      {CLAIM_ORDER.map(({ key, label }, i) => {
        const claim = synthesis[key]
        const isOpen = open.has(key)
        const isLast = i === CLAIM_ORDER.length - 1

        return (
          <div key={key} className={cn(!isLast && 'border-b border-border')}>
            <button
              type="button"
              className="w-full flex items-center gap-3 px-4 py-3 text-left hover:bg-muted/40 transition-colors duration-100"
              onClick={() => toggle(key)}
              aria-expanded={isOpen}
            >
              <span className="w-[152px] shrink-0 text-[11px] font-medium uppercase tracking-wider text-muted-foreground">
                {label}
              </span>
              <span className="flex-1 min-w-0 text-sm font-semibold text-foreground truncate">
                {claim.value}
              </span>
              <ConfidencePill confidence={claim.confidence} />
              <ChevronDown
                className={cn(
                  'w-3.5 h-3.5 shrink-0 ml-1 text-muted-foreground transition-transform duration-150',
                  isOpen && 'rotate-180',
                )}
              />
            </button>
            {isOpen && (
              <div className="px-4 pt-3 pb-4 border-t border-border bg-muted/20">
                {claim.evidence?.length > 0 ? (
                  <ul className="flex flex-col gap-1">
                    {claim.evidence.map((point, i) => (
                      <li key={i} className="flex gap-2 text-sm leading-relaxed text-muted-foreground">
                        <span className="shrink-0 select-none mt-px">·</span>
                        <span>{point}</span>
                      </li>
                    ))}
                  </ul>
                ) : (
                  <p className="text-sm text-muted-foreground">No evidence recorded.</p>
                )}
              </div>
            )}
          </div>
        )
      })}
    </div>
  )
}

// ---- Collapse widget ----
// Generic expandable section. Teaser text shows in the header when collapsed
// so the analyst can decide whether to open without expanding.

interface CollapseWidgetProps {
  label: string
  teaser?: string
  defaultOpen?: boolean
  children: React.ReactNode
}

function CollapseWidget({ label, teaser, defaultOpen = false, children }: CollapseWidgetProps) {
  const [open, setOpen] = useState(defaultOpen)

  return (
    <div className="border border-border rounded-lg overflow-hidden">
      <button
        type="button"
        className="w-full flex items-center justify-between gap-3 px-4 py-3 text-left hover:bg-muted/50 transition-colors duration-100"
        onClick={() => setOpen((o) => !o)}
        aria-expanded={open}
      >
        <div className="flex items-baseline gap-3 min-w-0">
          <span className="text-[11px] font-medium uppercase tracking-wider text-muted-foreground shrink-0">
            {label}
          </span>
          {teaser && !open && (
            <span className="text-xs text-muted-foreground truncate">{teaser}</span>
          )}
        </div>
        <ChevronDown
          className={cn('w-3.5 h-3.5 shrink-0 text-muted-foreground transition-transform duration-150', open && 'rotate-180')}
        />
      </button>
      {open && <div className="border-t border-border">{children}</div>}
    </div>
  )
}

// ---- Hypothesis widget ----

function HypothesisWidget({ hypothesis, defaultOpen }: { hypothesis: Hypothesis; defaultOpen?: boolean }) {
  const teaser = [hypothesis.brand, hypothesis.targeted_action, hypothesis.confidence]
    .filter(Boolean)
    .join(' · ')

  return (
    <CollapseWidget label="Initial Hypothesis" teaser={teaser} defaultOpen={defaultOpen}>
      <div className="px-4 pt-4 pb-5 flex flex-col gap-4">
        <div className="grid gap-x-8 gap-y-2.5" style={{ gridTemplateColumns: 'auto 1fr' }}>
          <span className="text-[11px] font-medium uppercase tracking-wider text-muted-foreground self-center">
            Brand
          </span>
          <span className="text-sm font-medium text-foreground">
            {hypothesis.brand || 'Unknown'}
          </span>

          <span className="text-[11px] font-medium uppercase tracking-wider text-muted-foreground self-center">
            Action
          </span>
          <span className="font-mono text-xs text-foreground">
            {hypothesis.targeted_action || 'Unknown'}
          </span>

          <span className="text-[11px] font-medium uppercase tracking-wider text-muted-foreground self-center">
            Confidence
          </span>
          <ConfidencePill confidence={safeConfidence(hypothesis.confidence)} />
        </div>

        {hypothesis.reasoning && (
          <div className="flex flex-col gap-1.5 pt-1 border-t border-border">
            <span className="text-[11px] font-medium uppercase tracking-wider text-muted-foreground">
              Reasoning
            </span>
            <p className="text-sm text-muted-foreground leading-relaxed max-w-[65ch]">
              {hypothesis.reasoning}
            </p>
          </div>
        )}
      </div>
    </CollapseWidget>
  )
}

// ---- Enrichment widget ----

function EnrichmentWidget({ summary }: { summary: string }) {
  const teaser = summary.length > 72 ? summary.slice(0, 72) + '…' : summary

  return (
    <CollapseWidget label="Enrichment Notes" teaser={teaser}>
      <div className="px-4 pt-4 pb-5">
        <p className="text-sm text-muted-foreground leading-relaxed max-w-[65ch] whitespace-pre-wrap">
          {summary}
        </p>
      </div>
    </CollapseWidget>
  )
}

// ---- Screenshot widget ----

function ScreenshotPanel({ id }: { id: string }) {
  const [failed, setFailed] = useState(false)

  return (
    <div className="p-4 bg-muted/20">
      {failed ? (
        <p className="text-sm text-muted-foreground text-center py-8">Screenshot unavailable</p>
      ) : (
        <img
          src={`/api/v1/investigations/${id}/screenshot`}
          alt="Page screenshot"
          className="w-full rounded object-contain max-h-[36rem]"
          onError={() => setFailed(true)}
        />
      )}
    </div>
  )
}

// ---- Main component ----

export function ReportView({ id }: { id: string }) {
  const [inv, setInv] = useState<InvestigationDetail | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    fetch(`/api/v1/investigations/${id}`)
      .then((res) => {
        if (res.status === 404) throw new Error('Investigation not found')
        if (!res.ok) throw new Error(`${res.status}`)
        return res.json()
      })
      .then((data: InvestigationDetail) => setInv(data))
      .catch((e: Error) => setError(e.message))
  }, [id])

  if (error) {
    return <p className="text-sm text-destructive">{error}</p>
  }

  if (!inv) {
    return <p className="text-sm text-muted-foreground">Loading…</p>
  }

  const hasSynthesis = Boolean(inv.synthesis)

  return (
    <div className="flex flex-col gap-6">
      {/* Metadata */}
      <div className="flex flex-col gap-1">
        <p className="font-mono text-sm text-foreground truncate">{inv.url}</p>
        <p className="font-mono text-xs text-muted-foreground tabular-nums">
          {inv.id} · {new Date(inv.created_at).toLocaleString()}
        </p>
      </div>

      {/* Verdict hero or failed hero */}
      {inv.status === 'failed' ? (
        <FailedHero error={inv.error_message} />
      ) : inv.synthesis ? (
        <VerdictHero claim={inv.synthesis.verdict} />
      ) : null}

      {/* Claims accordion — executive summary with drill-down */}
      {inv.synthesis && <ClaimsAccordion synthesis={inv.synthesis} />}

      {/* No synthesis yet: pending state */}
      {!hasSynthesis && !inv.hypothesis && inv.status !== 'failed' && (
        <div className="rounded-lg border border-border p-4">
          <p className="text-sm text-muted-foreground">Waiting for synthesis…</p>
        </div>
      )}

      {/* Collapsible detail widgets */}
      <div className="flex flex-col gap-3">
        {inv.hypothesis && (
          <HypothesisWidget
            hypothesis={inv.hypothesis}
            defaultOpen={!hasSynthesis}
          />
        )}
        {inv.enrichment_summary && (
          <EnrichmentWidget summary={inv.enrichment_summary} />
        )}
        <CollapseWidget label="Page Screenshot">
          <ScreenshotPanel id={id} />
        </CollapseWidget>
      </div>
    </div>
  )
}
