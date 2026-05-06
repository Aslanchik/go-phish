# Data Model

## Entity-Relationship Diagram

```mermaid
erDiagram
    investigations {
        uuid    id              PK
        text    url
        timestamptz created_at
        text    status
        text    error_message
        text    final_url
        text    rendered_dom
        bytea   screenshot
        jsonb   network_log
        jsonb   js_files
        jsonb   forms
        jsonb   hypothesis
        text    report
    }

    eval_labels {
        uuid    id              PK
        uuid    investigation_id FK
        timestamptz labeled_at
        text    labeled_by
        text    brand_impersonated
        text    exfil_destination
        text    kit_name
        boolean is_actually_phishing
    }

    investigations ||--o{ eval_labels : "has"
```

## Status transitions

```mermaid
stateDiagram-v2
    [*] --> pending : CreateInvestigation
    pending --> fetching : fetcher.Run started
    fetching --> hypothesizing : UpdateArtifacts
    hypothesizing --> complete : UpdateReport
    pending --> failed : any error
    fetching --> failed : any error
    hypothesizing --> failed : any error
```

## Column notes

### `investigations`

| Column | Phase | Notes |
|---|---|---|
| `status` | all | Enum-like: `pending` → `fetching` → `hypothesizing` → `complete` / `failed` |
| `network_log` | 1 | Array of `{url, method, status, content_type}` objects from Rod |
| `js_files` | 1 | Array of `{url, content}` objects for inline/external scripts |
| `forms` | 1 | Array of `{action, method, fields[]}` objects |
| `hypothesis` | 2 | `{brand, targeted_action, confidence, reasoning}` — LLM output |
| `report` | 4 | Final plain-text report stored after synthesis |

### `eval_labels`

Ground-truth labels applied by a human analyst after an investigation completes. Accumulate from the first investigation so the eval harness has data from day one.

| Column | Notes |
|---|---|
| `brand_impersonated` | Analyst-verified brand (may differ from LLM hypothesis) |
| `is_actually_phishing` | The primary eval signal |
| `exfil_destination` | Where credentials/data actually go (post-manual analysis) |
| `kit_name` | Known kit family if identified |
