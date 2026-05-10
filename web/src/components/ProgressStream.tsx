import { useEffect, useRef, useState } from 'react'

interface SsePayload {
  type: string
  timestamp: string
  data: Record<string, unknown>
}

interface LogEntry {
  seq: number
  type: string
  timestamp: string
  data: Record<string, unknown>
  elapsed?: number // ms, only for tool_result
}

function truncate(s: string, max = 80) {
  return s.length > max ? s.slice(0, max) + '…' : s
}

function formatTime(iso: string) {
  return new Date(iso).toLocaleTimeString()
}

function EntryRow({ entry }: { entry: LogEntry }) {
  const { type, timestamp, data, elapsed } = entry

  if (type === 'phase_transition') {
    const phase = String(data.phase ?? '').toUpperCase()
    return (
      <div className="flex items-center gap-3 py-1">
        <span className="text-xs text-muted-foreground w-16 shrink-0">
          {formatTime(timestamp)}
        </span>
        <span className="text-sm font-bold tracking-wide">{phase}</span>
      </div>
    )
  }

  if (type === 'tool_call') {
    const tool = String(data.tool ?? '')
    const input = truncate(JSON.stringify(data.input ?? {}))
    return (
      <div className="flex items-start gap-3 py-0.5">
        <span className="text-xs text-muted-foreground w-16 shrink-0">
          {formatTime(timestamp)}
        </span>
        <span className="text-xs font-mono">
          <span className="text-blue-500">→</span>{' '}
          <span className="font-medium">{tool}</span>{' '}
          <span className="text-muted-foreground">{input}</span>
        </span>
      </div>
    )
  }

  if (type === 'tool_result') {
    const tool = String(data.tool ?? '')
    const summary = truncate(String(data.summary ?? ''), 120)
    const elapsedStr = elapsed != null ? `${(elapsed / 1000).toFixed(1)}s` : ''
    return (
      <div className="flex items-start gap-3 py-0.5">
        <span className="text-xs text-muted-foreground w-16 shrink-0">
          {formatTime(timestamp)}
        </span>
        <span className="text-xs font-mono">
          <span className="text-green-500">←</span>{' '}
          <span className="font-medium">{tool}</span>{' '}
          <span className="text-muted-foreground">{summary}</span>
          {elapsedStr && (
            <span className="ml-2 text-muted-foreground/60">{elapsedStr}</span>
          )}
        </span>
      </div>
    )
  }

  if (type === 'log') {
    return (
      <div className="flex items-start gap-3 py-0.5">
        <span className="text-xs text-muted-foreground w-16 shrink-0">
          {formatTime(timestamp)}
        </span>
        <span className="text-xs text-muted-foreground">{String(data.message ?? '')}</span>
      </div>
    )
  }

  if (type === 'complete') {
    const verdict = String(data.verdict ?? '')
    return (
      <div className="flex items-center gap-3 py-1">
        <span className="text-xs text-muted-foreground w-16 shrink-0">
          {formatTime(timestamp)}
        </span>
        <span className="text-sm font-semibold text-green-600">
          ✓ Complete — verdict: {verdict}
        </span>
      </div>
    )
  }

  if (type === 'failed') {
    const reason = String(data.reason ?? 'unknown error')
    return (
      <div className="flex items-center gap-3 py-1">
        <span className="text-xs text-muted-foreground w-16 shrink-0">
          {formatTime(timestamp)}
        </span>
        <span className="text-sm font-semibold text-destructive">✗ Failed — {reason}</span>
      </div>
    )
  }

  return null
}

export function ProgressStream({
  id,
  onTerminal,
}: {
  id: string
  onTerminal: () => void
}) {
  const [entries, setEntries] = useState<LogEntry[]>([])
  const [reconnecting, setReconnecting] = useState(false)
  const toolCallTimes = useRef<Map<string, number>>(new Map())
  const onTerminalRef = useRef(onTerminal)
  onTerminalRef.current = onTerminal
  const bottomRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const es = new EventSource(`/api/v1/investigations/${id}/events`)

    es.onopen = () => setReconnecting(false)
    es.onerror = () => setReconnecting(true)

    es.onmessage = (e) => {
      const payload: SsePayload = JSON.parse(e.data)
      const seq = parseInt(e.lastEventId || '0', 10)

      const entry: LogEntry = {
        seq,
        type: payload.type,
        timestamp: payload.timestamp,
        data: payload.data,
      }

      if (payload.type === 'tool_call') {
        toolCallTimes.current.set(String(payload.data.tool ?? ''), Date.now())
      }

      if (payload.type === 'tool_result') {
        const start = toolCallTimes.current.get(String(payload.data.tool ?? ''))
        if (start != null) {
          entry.elapsed = Date.now() - start
        }
      }

      setEntries((prev) => [...prev, entry])

      if (payload.type === 'complete' || payload.type === 'failed') {
        es.close()
        onTerminalRef.current()
      }
    }

    return () => es.close()
  }, [id])

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [entries])

  return (
    <div className="rounded-lg border bg-muted/30 flex flex-col">
      <div className="px-4 py-2 border-b flex items-center justify-between">
        <span className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
          Live progress
        </span>
        {reconnecting && (
          <span className="text-xs text-amber-500">Reconnecting…</span>
        )}
      </div>
      <div className="px-4 py-3 max-h-72 overflow-y-auto">
        {entries.length === 0 ? (
          <p className="text-xs text-muted-foreground">Waiting for events…</p>
        ) : (
          entries.map((entry) => <EntryRow key={entry.seq} entry={entry} />)
        )}
        <div ref={bottomRef} />
      </div>
    </div>
  )
}
