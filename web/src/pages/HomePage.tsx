import { SubmitForm } from '@/components/SubmitForm'
import { InvestigationList } from '@/components/InvestigationList'

export function HomePage() {
  return (
    <div className="mx-auto max-w-3xl px-4 py-10 flex flex-col gap-8">
      <div className="flex flex-col gap-4">
        <div className="flex flex-col gap-1">
          <h1 className="text-2xl font-semibold">go-phish</h1>
          <p className="text-sm text-muted-foreground">
            Investigate a suspicious URL: brand, kit, exfiltration target, verdict.
          </p>
        </div>
        <SubmitForm />
      </div>
      <div className="flex flex-col gap-3">
        <h2 className="text-lg font-medium">Recent Investigations</h2>
        <InvestigationList />
      </div>
    </div>
  )
}
