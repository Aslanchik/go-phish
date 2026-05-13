export interface Investigation {
  id: string
  url: string
  status: 'pending' | 'running' | 'complete' | 'failed'
  created_at: string
  verdict?: string | null
}

export interface Claim {
  value: string
  confidence: 'low' | 'medium' | 'high'
  evidence: string[]
}

export interface Synthesis {
  brand_impersonated: Claim
  kit_identification: Claim
  exfil_target: Claim
  infrastructure_notes: Claim
  verdict: Claim
}

export interface Hypothesis {
  brand: string
  targeted_action: string
  confidence: string
  reasoning: string
}

export interface InvestigationDetail {
  id: string
  url: string
  created_at: string
  status: 'pending' | 'running' | 'complete' | 'failed'
  error_message?: string
  hypothesis?: Hypothesis
  enrichment_summary?: string
  synthesis?: Synthesis
}
