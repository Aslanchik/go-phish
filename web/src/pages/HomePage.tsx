import { SubmitForm } from '@/components/SubmitForm'
import { InvestigationList } from '@/components/InvestigationList'

export function HomePage() {
  return (
    <div className="mx-auto max-w-4xl px-4 py-10 flex flex-col gap-8">
      <div>
        <h1 className="text-2xl font-semibold mb-1">go-phish</h1>
        <p className="text-muted-foreground text-sm mb-4">
          Submit a suspicious URL to run a phishing investigation.
        </p>
        <SubmitForm />
      </div>
      <div>
        <h2 className="text-lg font-medium mb-3">Recent Investigations</h2>
        <InvestigationList />
      </div>
    </div>
  )
}
