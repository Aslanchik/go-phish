import { useEffect, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { ReportView } from '@/components/ReportView'
import { ProgressStream } from '@/components/ProgressStream'

const TERMINAL = new Set(['complete', 'failed'])

export function InvestigationPage() {
  const { id } = useParams<{ id: string }>()
  const [terminal, setTerminal] = useState<boolean | null>(null)
  const [refreshKey, setRefreshKey] = useState(0)

  useEffect(() => {
    if (!id) return
    fetch(`/api/v1/investigations/${id}`)
      .then((r) => {
        if (!r.ok) throw new Error(`${r.status}`)
        return r.json() as Promise<{ status: string }>
      })
      .then((data) => setTerminal(TERMINAL.has(data.status)))
      .catch(() => setTerminal(true))
  }, [id])

  if (!id) {
    return <p className="text-sm text-destructive">Invalid investigation ID.</p>
  }

  return (
    <div className="mx-auto max-w-3xl px-4 py-10 flex flex-col gap-6">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold">Investigation Report</h1>
        <Link to="/" className="text-sm text-muted-foreground hover:underline">
          ← All investigations
        </Link>
      </div>

      {terminal === null ? (
        <p className="text-sm text-muted-foreground">Loading…</p>
      ) : (
        <>
          {!terminal && (
            <ProgressStream
              id={id}
              onTerminal={() => {
                setTerminal(true)
                setRefreshKey((k) => k + 1)
              }}
            />
          )}
          <ReportView id={id} key={refreshKey} />
        </>
      )}
    </div>
  )
}
