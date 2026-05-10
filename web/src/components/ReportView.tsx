import { useEffect, useState } from 'react'
import { Badge } from '@/components/ui/badge'
import type { Claim, InvestigationDetail } from '@/types'

const CLAIM_LABELS: Record<string, string> = {
  brand_impersonated: 'Brand Impersonated',
  kit_identification: 'Kit Identification',
  exfil_target: 'Exfiltration Target',
  infrastructure_notes: 'Infrastructure Notes',
  verdict: 'Verdict',
}

const CLAIM_ORDER = [
  'brand_impersonated',
  'kit_identification',
  'exfil_target',
  'infrastructure_notes',
  'verdict',
] as const

function confidenceVariant(c: Claim['confidence']) {
  if (c === 'high') return 'default' as const
  if (c === 'medium') return 'secondary' as const
  return 'destructive' as const
}

function ClaimBlock({ label, claim }: { label: string; claim: Claim }) {
  return (
    <div className="rounded-lg border p-4 flex flex-col gap-2">
      <div className="flex items-center justify-between gap-3">
        <span className="text-sm font-medium text-muted-foreground">{label}</span>
        <Badge variant={confidenceVariant(claim.confidence)}>{claim.confidence}</Badge>
      </div>
      <p className="text-base font-semibold">{claim.value}</p>
      <p className="text-sm text-muted-foreground">{claim.evidence}</p>
    </div>
  )
}

function ScreenshotBlock({ id }: { id: string }) {
  const [failed, setFailed] = useState(false)

  if (failed) {
    return (
      <div className="rounded-lg border bg-muted flex items-center justify-center h-40 text-sm text-muted-foreground">
        Screenshot unavailable
      </div>
    )
  }

  return (
    <img
      src={`/api/v1/investigations/${id}/screenshot`}
      alt="Page screenshot"
      className="rounded-lg border w-full object-contain max-h-96"
      onError={() => setFailed(true)}
    />
  )
}

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

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-1">
        <p className="text-xs text-muted-foreground font-mono">{inv.id}</p>
        <p className="text-sm text-muted-foreground">
          {new Date(inv.created_at).toLocaleString()}
        </p>
        <p className="font-mono text-sm truncate">{inv.url}</p>
      </div>

      <ScreenshotBlock id={id} />

      {inv.synthesis ? (
        <div className="flex flex-col gap-3">
          <h2 className="text-base font-semibold">Synthesis</h2>
          {CLAIM_ORDER.map((key) => (
            <ClaimBlock
              key={key}
              label={CLAIM_LABELS[key]}
              claim={inv.synthesis![key]}
            />
          ))}
        </div>
      ) : (
        <div className="rounded-lg border p-4 flex flex-col gap-2">
          <p className="text-sm font-medium text-muted-foreground">
            Preliminary hypothesis — synthesis not yet available
          </p>
          {inv.hypothesis ? (
            <pre className="text-xs bg-muted rounded p-3 overflow-x-auto whitespace-pre-wrap">
              {JSON.stringify(inv.hypothesis, null, 2)}
            </pre>
          ) : (
            <p className="text-sm text-muted-foreground">No hypothesis available yet.</p>
          )}
        </div>
      )}
    </div>
  )
}
