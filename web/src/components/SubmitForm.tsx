import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

export function SubmitForm() {
  const navigate = useNavigate()
  const [url, setUrl] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)

  function validate(value: string): string | null {
    if (!value.trim()) return 'URL is required'
    let parsed: URL
    try {
      parsed = new URL(value)
    } catch {
      return 'Must be a valid URL (e.g. https://example.com)'
    }
    if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
      return 'URL must use http or https'
    }
    if (!parsed.host) return 'URL must have a host'
    return null
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    const validationError = validate(url)
    if (validationError) {
      setError(validationError)
      return
    }
    setError(null)
    setLoading(true)
    try {
      const res = await fetch('/api/v1/investigations', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ url }),
      })
      if (!res.ok) {
        const body = await res.json().catch(() => ({}))
        setError(body.error ?? `Server error: ${res.status}`)
        return
      }
      const data = await res.json()
      navigate(`/investigations/${data.id}`)
    } catch {
      setError('Network error: is the server running?')
    } finally {
      setLoading(false)
    }
  }

  return (
    <form onSubmit={handleSubmit} noValidate className="flex flex-col gap-3">
      <label htmlFor="url-input" className="sr-only">
        Suspicious URL
      </label>
      <Input
        id="url-input"
        type="url"
        placeholder="https://suspicious-site.example.com"
        value={url}
        onChange={(e: React.ChangeEvent<HTMLInputElement>) => {
          setUrl(e.target.value)
          if (error) setError(null)
        }}
        disabled={loading}
        aria-invalid={error ? true : undefined}
        aria-describedby={error ? 'url-error' : undefined}
        className="h-10 font-mono text-sm"
      />
      {error && (
        <p id="url-error" className="text-sm text-destructive" role="alert">
          {error}
        </p>
      )}
      <div className="flex justify-end">
        <Button type="submit" disabled={loading}>
          {loading ? 'Investigating…' : 'Investigate'}
        </Button>
      </div>
    </form>
  )
}
