import { useEffect, useState } from 'react'
import { ChevronDown } from 'lucide-react'
import { cn } from '@/lib/utils'
import type { Claim, Hypothesis, InvestigationDetail, Synthesis } from '@/types'

function evidenceItems(evidence: string[] | string | null | undefined): string[] {
  if (!evidence) return []
  const items = Array.isArray(evidence) ? evidence : [evidence]
  return items.map((s) => {
    const stripped = s.replace(/^\w+:\s*/, '')
    return stripped.charAt(0).toUpperCase() + stripped.slice(1)
  })
}

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

const HERO_BORDER: Record<VerdictSignal, string> = {
  'threat-high':   '1.5px solid oklch(0.577 0.245 27)',
  'threat-medium': '1.5px solid oklch(0.65 0.18 70)',
  'threat-low':    '1px solid oklch(0.577 0.245 27 / 0.4)',
  clean:           '1.5px solid oklch(0.627 0.194 149)',
  uncertain:       '1px solid oklch(0.922 0 0)',
}

// ---- Verdict hero ----

function VerdictHero({ claim }: { claim: Claim }) {
  const signal = verdictSignal(claim)

  return (
    <div
      className="rounded-lg px-6 py-5 flex flex-col gap-3"
      style={{ border: HERO_BORDER[signal] }}
    >
      <div className="flex items-start justify-between gap-6">
        <p
          className="text-[1.125rem] font-semibold leading-snug uppercase tracking-wide"
          style={{ maxWidth: '55ch', textWrap: 'balance' } as React.CSSProperties}
        >
          {claim.value}
        </p>
        <ConfidencePill confidence={claim.confidence} />
      </div>
      {evidenceItems(claim.evidence).length > 0 && (
        <div className="flex flex-col gap-1.5 mt-0.5">
          {evidenceItems(claim.evidence).map((point, i) => (
            <p key={i} className="text-[0.8125rem] leading-relaxed text-muted-foreground max-w-[95ch]">
              {point}
            </p>
          ))}
        </div>
      )}
    </div>
  )
}

function FailedHero({ error }: { error?: string }) {
  return (
    <div
      className="rounded-lg px-6 py-5 flex flex-col gap-2"
      style={{ border: '1.5px solid oklch(0.577 0.245 27)' }}
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
        <p className="text-sm text-muted-foreground max-w-[95ch]">{error}</p>
      )}
    </div>
  )
}

// ---- Confidence pill ----

function ConfidencePill({ confidence }: { confidence: Claim['confidence'] }) {
  if (confidence === 'high') {
    return (
      <span
        className="shrink-0 text-[10px] font-medium tracking-widest uppercase rounded-full px-2.5 py-1 leading-none"
        style={{ backgroundColor: 'oklch(0.627 0.194 149 / 0.15)', color: 'oklch(0.40 0.16 149)' }}
      >
        High
      </span>
    )
  }
  if (confidence === 'medium') {
    return (
      <span
        className="shrink-0 text-[10px] font-medium tracking-widest uppercase rounded-full px-2.5 py-1 leading-none"
        style={{ backgroundColor: 'oklch(0.769 0.188 70 / 0.18)', color: 'oklch(0.46 0.14 70)' }}
      >
        Med
      </span>
    )
  }
  return (
    <span
      className="shrink-0 text-[10px] font-medium tracking-widest uppercase rounded-full px-2.5 py-1 leading-none"
      style={{ backgroundColor: 'oklch(0.145 0 0 / 0.07)', color: 'oklch(0.556 0 0)' }}
    >
      Low
    </span>
  )
}

function safeConfidence(s: string): Claim['confidence'] {
  if (s === 'high' || s === 'medium' || s === 'low') return s
  return 'low'
}

// ---- Screenshot row ----
// Positioned between verdict and the findings accordion. Same visual language
// as accordion rows but lives outside the accordion (separate card).

function ScreenshotRow({ id }: { id: string }) {
  const [open, setOpen] = useState(false)
  const [failed, setFailed] = useState(false)

  return (
    <div className="rounded-lg border border-border overflow-hidden">
      <button
        type="button"
        className="w-full flex items-center gap-3 px-4 py-3 text-left hover:bg-muted/40 transition-colors duration-100"
        onClick={() => setOpen((o) => !o)}
        aria-expanded={open}
      >
        <span className="w-[152px] shrink-0 text-[11px] font-medium uppercase tracking-wider text-muted-foreground">
          Screenshot
        </span>
        <span className="flex-1 min-w-0 text-sm text-muted-foreground">
          Page capture at time of investigation
        </span>
        <ChevronDown
          className={cn(
            'w-3.5 h-3.5 shrink-0 text-muted-foreground transition-transform duration-150',
            open && 'rotate-180',
          )}
        />
      </button>
      {open && (
        <div className="border-t border-border p-4 bg-muted/20">
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
      )}
    </div>
  )
}

// ---- Unified investigation accordion ----
// Primary claims occupy the top section. Hypothesis and enrichment notes live
// below a "Supporting context" divider — same row anatomy, lower visual weight.

type ClaimKey = 'brand_impersonated' | 'kit_identification' | 'exfil_target' | 'infrastructure_notes'
type SuppKey = 'hypothesis' | 'enrichment'
type AnyKey = ClaimKey | SuppKey

const CLAIM_ORDER: { key: ClaimKey; label: string }[] = [
  { key: 'brand_impersonated', label: 'Brand Impersonated' },
  { key: 'kit_identification', label: 'Kit Identification' },
  { key: 'exfil_target', label: 'Exfiltration Target' },
  { key: 'infrastructure_notes', label: 'Infrastructure Notes' },
]

interface InvestigationAccordionProps {
  synthesis?: Synthesis
  hypothesis?: Hypothesis
  enrichmentSummary?: string
  initialOpen?: AnyKey[]
}

function InvestigationAccordion({
  synthesis,
  hypothesis,
  enrichmentSummary,
  initialOpen,
}: InvestigationAccordionProps) {
  const [open, setOpen] = useState<Set<AnyKey>>(
    new Set(initialOpen ?? (synthesis ? ['brand_impersonated'] : ['hypothesis'])),
  )

  function toggle(key: AnyKey) {
    setOpen((prev) => {
      const next = new Set(prev)
      next.has(key) ? next.delete(key) : next.add(key)
      return next
    })
  }

  const hasSupporting = Boolean(hypothesis || enrichmentSummary)

  return (
    <div className="rounded-lg border border-border overflow-hidden">
      {/* Primary claims */}
      {synthesis &&
        CLAIM_ORDER.map(({ key, label }, i) => {
          const claim = synthesis[key]
          const isOpen = open.has(key)
          const isLast = i === CLAIM_ORDER.length - 1

          return (
            <div key={key} className={cn((!isLast || hasSupporting) && 'border-b border-border')}>
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
                  {evidenceItems(claim.evidence).length > 0 ? (
                    <div className="flex flex-col gap-1.5">
                      {evidenceItems(claim.evidence).map((point, j) => (
                        <p key={j} className="text-sm leading-relaxed text-muted-foreground max-w-[95ch]">
                          {point}
                        </p>
                      ))}
                    </div>
                  ) : (
                    <p className="text-sm text-muted-foreground">No evidence recorded.</p>
                  )}
                </div>
              )}
            </div>
          )
        })}

      {/* Supporting context: hypothesis + enrichment */}
      {hasSupporting && (
        <>
          {synthesis && (
            <div className="px-4 py-2 bg-muted/30 border-b border-border">
              <span className="text-[10px] font-medium uppercase tracking-widest text-muted-foreground/50">
                Supporting context
              </span>
            </div>
          )}

          {hypothesis && (() => {
            const isOpen = open.has('hypothesis')
            const teaser = [hypothesis.brand, hypothesis.targeted_action].filter(Boolean).join(' · ')
            return (
              <div className={cn(enrichmentSummary && 'border-b border-border')}>
                <button
                  type="button"
                  className="w-full flex items-center gap-3 px-4 py-3 text-left hover:bg-muted/40 transition-colors duration-100"
                  onClick={() => toggle('hypothesis')}
                  aria-expanded={isOpen}
                >
                  <span className="w-[152px] shrink-0 text-[11px] font-medium uppercase tracking-wider text-muted-foreground">
                    Hypothesis
                  </span>
                  <span className="flex-1 min-w-0 text-sm text-foreground/70 truncate">
                    {teaser || 'No hypothesis recorded'}
                  </span>
                  <ConfidencePill confidence={safeConfidence(hypothesis.confidence)} />
                  <ChevronDown
                    className={cn(
                      'w-3.5 h-3.5 shrink-0 ml-1 text-muted-foreground transition-transform duration-150',
                      isOpen && 'rotate-180',
                    )}
                  />
                </button>
                {isOpen && (
                  <div className="px-4 pt-3 pb-4 border-t border-border bg-muted/20">
                    <div className="grid gap-x-8 gap-y-2.5" style={{ gridTemplateColumns: 'auto 1fr' }}>
                      <span className="text-[11px] font-medium uppercase tracking-wider text-muted-foreground self-center">Brand</span>
                      <span className="text-sm font-medium text-foreground">{hypothesis.brand || 'Unknown'}</span>
                      <span className="text-[11px] font-medium uppercase tracking-wider text-muted-foreground self-center">Action</span>
                      <span className="font-mono text-xs text-foreground">{hypothesis.targeted_action || 'Unknown'}</span>
                      <span className="text-[11px] font-medium uppercase tracking-wider text-muted-foreground self-center">Confidence</span>
                      <div className="flex"><ConfidencePill confidence={safeConfidence(hypothesis.confidence)} /></div>
                    </div>
                    {hypothesis.reasoning && (
                      <div className="flex flex-col gap-1.5 pt-3 mt-3 border-t border-border">
                        <span className="text-[11px] font-medium uppercase tracking-wider text-muted-foreground">Reasoning</span>
                        <p className="text-sm text-muted-foreground leading-relaxed max-w-[95ch]">{hypothesis.reasoning}</p>
                      </div>
                    )}
                  </div>
                )}
              </div>
            )
          })()}

          {enrichmentSummary && (() => {
            const isOpen = open.has('enrichment')
            const teaser = enrichmentSummary.length > 80 ? enrichmentSummary.slice(0, 80) + '…' : enrichmentSummary
            return (
              <div>
                <button
                  type="button"
                  className="w-full flex items-center gap-3 px-4 py-3 text-left hover:bg-muted/40 transition-colors duration-100"
                  onClick={() => toggle('enrichment')}
                  aria-expanded={isOpen}
                >
                  <span className="w-[152px] shrink-0 text-[11px] font-medium uppercase tracking-wider text-muted-foreground">
                    Enrichment
                  </span>
                  <span className="flex-1 min-w-0 text-sm text-foreground/70 truncate">{teaser}</span>
                  <ChevronDown
                    className={cn(
                      'w-3.5 h-3.5 shrink-0 ml-1 text-muted-foreground transition-transform duration-150',
                      isOpen && 'rotate-180',
                    )}
                  />
                </button>
                {isOpen && (
                  <div className="px-4 pt-3 pb-4 border-t border-border bg-muted/20">
                    <p className="text-sm text-muted-foreground leading-relaxed max-w-[95ch] whitespace-pre-wrap">
                      {enrichmentSummary}
                    </p>
                  </div>
                )}
              </div>
            )
          })()}
        </>
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
  const hasDetail = hasSynthesis || Boolean(inv.hypothesis) || Boolean(inv.enrichment_summary)

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

      {/* Screenshot — contextual anchor between verdict and findings */}
      <ScreenshotRow id={id} />

      {/* Unified findings accordion */}
      {hasDetail && (
        <InvestigationAccordion
          synthesis={inv.synthesis}
          hypothesis={inv.hypothesis}
          enrichmentSummary={inv.enrichment_summary}
          initialOpen={hasSynthesis ? ['brand_impersonated'] : ['hypothesis']}
        />
      )}

      {/* Pending: nothing to show yet */}
      {!hasDetail && inv.status !== 'failed' && (
        <div className="rounded-lg border border-border p-4">
          <p className="text-sm text-muted-foreground">Waiting for synthesis…</p>
        </div>
      )}
    </div>
  )
}
