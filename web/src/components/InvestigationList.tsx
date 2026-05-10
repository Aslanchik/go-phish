import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { Badge } from '@/components/ui/badge'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import type { Investigation } from '@/types'

function statusLabel(inv: Investigation): string {
  if (inv.verdict) return inv.verdict
  return inv.status
}

function StatusBadge({ inv }: { inv: Investigation }) {
  const variant =
    inv.status === 'failed'
      ? 'destructive'
      : inv.status === 'complete'
        ? 'default'
        : 'secondary'
  return <Badge variant={variant}>{statusLabel(inv)}</Badge>
}

function formatDate(iso: string) {
  return new Date(iso).toLocaleString()
}

export function InvestigationList() {
  const [investigations, setInvestigations] = useState<Investigation[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    fetch('/api/v1/investigations')
      .then((res) => {
        if (!res.ok) throw new Error(`${res.status}`)
        return res.json()
      })
      .then((data: Investigation[]) => {
        setInvestigations(data)
        setLoading(false)
      })
      .catch(() => {
        setError('Failed to load investigations')
        setLoading(false)
      })
  }, [])

  if (loading) return <p className="text-sm text-muted-foreground">Loading…</p>

  if (error) return <p className="text-sm text-destructive">{error}</p>

  if (investigations.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">No investigations yet.</p>
    )
  }

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>URL</TableHead>
          <TableHead>Status</TableHead>
          <TableHead>Started</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {investigations.map((inv) => (
          <TableRow key={inv.id}>
            <TableCell className="max-w-sm truncate font-mono text-xs">
              <Link
                to={`/investigations/${inv.id}`}
                className="hover:underline"
              >
                {inv.url}
              </Link>
            </TableCell>
            <TableCell>
              <StatusBadge inv={inv} />
            </TableCell>
            <TableCell className="text-muted-foreground text-sm">
              {formatDate(inv.created_at)}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}
