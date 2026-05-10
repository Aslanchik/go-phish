import { SubmitForm } from '@/components/SubmitForm'

export function HomePage() {
  return (
    <div className="w-full px-8 pb-16" style={{ paddingTop: '22vh' }}>
      <div className="w-full max-w-lg mx-auto flex flex-col gap-5">
        <img src="/logo.png" alt="go-phish" className="w-80 mx-auto select-none" draggable={false} />
        <h1 className="sr-only">go-phish</h1>
        <p className="text-sm text-muted-foreground leading-relaxed max-w-[52ch]">
          Investigate a suspicious URL: brand, kit, exfiltration target, verdict.
        </p>
        <SubmitForm />
      </div>
    </div>
  )
}
