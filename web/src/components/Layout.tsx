import React, { useEffect, useState } from 'react'
import { Link, Outlet, useLocation } from 'react-router-dom'
import { Pin } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import type { Investigation } from '@/types'

const SIDEBAR_W = 272
const RAIL_W = 79

function useReducedMotion(): boolean {
  const [reduced, setReduced] = useState(
    () =>
      typeof window !== 'undefined' &&
      window.matchMedia('(prefers-reduced-motion: reduce)').matches,
  )
  useEffect(() => {
    const mq = window.matchMedia('(prefers-reduced-motion: reduce)')
    const handler = (e: MediaQueryListEvent) => setReduced(e.matches)
    mq.addEventListener('change', handler)
    return () => mq.removeEventListener('change', handler)
  }, [])
  return reduced
}

function badgeVariant(inv: Investigation): 'default' | 'secondary' | 'destructive' | 'outline' {
  if (inv.status === 'complete') return 'default'
  if (inv.status === 'failed') return 'destructive'
  return 'outline'
}

function displayHost(raw: string): string {
  try {
    const u = new URL(raw)
    const path = u.pathname === '/' ? '' : u.pathname
    return u.host + path
  } catch {
    return raw
  }
}

function invLabel(inv: Investigation): string {
  return inv.verdict ?? inv.status
}

function shortTime(iso: string): string {
  return new Date(iso).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

interface ItemProps {
  inv: Investigation
  active: boolean
  expanded: boolean
  noMotion: boolean
}

function SidebarItem({ inv, active, expanded, noMotion }: ItemProps) {
  const textTransition = noMotion
    ? 'none'
    : expanded
      ? 'opacity 120ms 80ms linear'
      : 'opacity 60ms linear'

  return (
    <Link
      to={`/investigations/${inv.id}`}
      title={inv.url}
      className={cn(
        'flex items-center min-h-[52px] px-4 border-b transition-colors duration-100',
        expanded ? 'border-border' : 'border-transparent',
        active ? 'bg-accent' : 'hover:bg-accent/60',
      )}
    >
      <div
        className="flex-1 min-w-0 flex flex-col gap-1.5"
        style={{ opacity: expanded ? 1 : 0, transition: textTransition }}
      >
        <div className="flex items-center justify-between gap-2">
          <span className="font-mono text-[11px] text-foreground leading-tight truncate min-w-0">
            {displayHost(inv.url)}
          </span>
          <Badge variant={badgeVariant(inv)} className="uppercase shrink-0 text-[9px] leading-none tracking-wider">
            {invLabel(inv)}
          </Badge>
        </div>
        <span className="text-[10px] text-muted-foreground tabular-nums">
          {shortTime(inv.created_at)}
        </span>
      </div>
    </Link>
  )
}

export function Layout() {
  const location = useLocation()
  const noMotion = useReducedMotion()

  const isHome = location.pathname === '/'
  const activeId = location.pathname.match(/^\/investigations\/(.+)$/)?.[1] ?? null

  const [investigations, setInvestigations] = useState<Investigation[]>([])
  const [hovering, setHovering] = useState(false)
  const [pinned, setPinned] = useState(false)

  const expanded = isHome || hovering || pinned

  useEffect(() => {
    let live = true
    const load = () =>
      fetch('/api/v1/investigations')
        .then((r) => (r.ok ? r.json() : []))
        .then((data: Investigation[]) => { if (live) setInvestigations(data) })
        .catch(() => {})
    load()
    const tid = setInterval(load, 5000)
    return () => { live = false; clearInterval(tid) }
  }, [location.pathname])

  const sidebarWidth = expanded ? SIDEBAR_W : RAIL_W
  const duration = noMotion ? 0 : expanded ? 200 : 180
  const fadeIn = noMotion ? 'none' : 'opacity 140ms 60ms linear'
  const fadeOut = noMotion ? 'none' : 'opacity 60ms linear'

  return (
    <div className="flex h-screen overflow-hidden bg-background">
      <aside
        className="shrink-0 border-r border-border bg-muted overflow-hidden"
        style={{ width: sidebarWidth, transition: `width ${duration}ms cubic-bezier(0.16, 1, 0.3, 1)` }}
        onMouseEnter={() => { if (!isHome) setHovering(true) }}
        onMouseLeave={() => setHovering(false)}
        aria-label="Investigations"
      >
        <div className="flex flex-col" style={{ width: SIDEBAR_W, height: '100%' }}>
          {/* Header */}
          <div className="h-[78px] border-b border-border flex items-center justify-between shrink-0">
            <Link
              to="/"
              className="w-[78px] h-full shrink-0 flex items-center justify-center text-muted-foreground hover:text-foreground transition-colors duration-100"
              aria-label="go-phish home"
            >
              <img src="/logo.png" alt="go-phish" className="w-[64px] h-[66px] scale-110 select-none" draggable={false} />
            </Link>
            <button
              onClick={() => setPinned((p) => !p)}
              className={cn(
                'w-8 h-8 mr-1.5 shrink-0 flex items-center justify-center rounded cursor-pointer transition-colors duration-100',
                pinned
                  ? 'text-foreground bg-accent'
                  : 'text-muted-foreground hover:bg-accent hover:text-foreground',
              )}
              style={{ opacity: expanded ? 1 : 0, transition: expanded ? fadeIn : fadeOut }}
              title={pinned ? 'Unpin sidebar' : 'Keep sidebar open'}
              aria-label={pinned ? 'Unpin sidebar' : 'Keep sidebar open'}
              tabIndex={expanded ? 0 : -1}
            >
              <Pin className="w-3.5 h-3.5" />
            </button>
          </div>

          {/* Section label */}
          <div
            className="shrink-0 pt-4 pb-1"
            style={{
              paddingLeft: 16,
              paddingRight: 16,
              opacity: expanded ? 1 : 0,
              transition: expanded ? fadeIn : fadeOut,
            }}
          >
            <span className="text-[10px] font-medium uppercase tracking-widest text-muted-foreground">
              Recent investigations
            </span>
          </div>

          {/* List */}
          <div className="flex-1 overflow-y-auto">
            {investigations.length === 0 ? (
              <p
                className="text-xs text-muted-foreground"
                style={{
                  paddingLeft: 16,
                  paddingRight: 16,
                  paddingTop: 8,
                  opacity: expanded ? 1 : 0,
                  transition: expanded ? fadeIn : fadeOut,
                }}
              >
                No investigations yet.
              </p>
            ) : (
              investigations.map((inv) => (
                <SidebarItem
                  key={inv.id}
                  inv={inv}
                  active={inv.id === activeId}
                  expanded={expanded}
                  noMotion={noMotion}
                />
              ))
            )}
          </div>
        </div>
      </aside>

      <main className="flex-1 overflow-y-auto overflow-x-hidden flex flex-col" style={{ scrollbarGutter: 'stable' } as React.CSSProperties}>
        <Outlet />
      </main>
    </div>
  )
}
