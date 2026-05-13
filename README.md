<p align="center">
  <img src="web/public/logo-full.png" alt="go-phish" width="320" />
</p>

<p align="center">A phishing investigation tool powered by Claude. Submit a suspicious URL and get a structured, confidence-rated report — brand impersonated, kit mechanics, credential exfiltration target, and infrastructure notes.</p>

---

## How it works

Investigations run as a four-phase agent pipeline:

1. **Fetch** — loads the URL in a sandboxed headless Chromium container; captures rendered DOM, screenshot, network log, JS files, and forms
2. **Hypothesize** — sends the screenshot and DOM summary to Claude; produces a structured initial hypothesis (brand, targeted action, confidence)
3. **Enrich** — Claude runs an agentic loop over tools: WHOIS, certificate transparency, URLhaus, urlscan.io, and JS analysis; tool calls are streamed live to the UI
4. **Synthesize** — Claude produces a final report with per-claim confidence levels (brand, kit, exfil target, verdict)

Progress streams to the browser in real time via SSE. Results are stored in Postgres and accessible any time from the investigation list.

Container egress is restricted to the target IP only via an in-process HTTP CONNECT proxy. The browser runs `--cap-drop ALL --read-only --no-new-privileges`.

## Prerequisites

- Docker (Desktop or Engine)
- An Anthropic API key

## Setup

### Docker — recommended

Builds the React frontend, Go server, and fetcher image; Postgres runs as a container. No local Go or Node install needed.

**1. Set your API key**

```sh
cp .env.example .env
# edit .env — set ANTHROPIC_API_KEY=sk-ant-...
```

**2. Build images**

```sh
docker compose build server   # multi-stage: React build → Go binary
docker compose build fetcher  # sandboxed Chromium fetcher (build once, reuse)
```

**3. Start everything**

```sh
docker compose up -d
```

Open [http://localhost:8080](http://localhost:8080).

---

### Local development

Requires Go 1.22+, Node.js 20+, Docker, and a running Postgres instance.

**1. Start Postgres**

```sh
docker compose up -d postgres
```

**2. Set environment variables**

```sh
cp .env.example .env
# edit .env — set ANTHROPIC_API_KEY and DATABASE_URL
```

| Variable | Example |
|---|---|
| `DATABASE_URL` | `postgres://gophish:gophish@localhost:5432/gophish?sslmode=disable` |
| `ANTHROPIC_API_KEY` | `sk-ant-...` |

**3. Build the fetcher image**

```sh
docker build -t go-phish-fetcher:latest docker/fetcher/
```

**4. Build the frontend**

```sh
cd web && npm install && npm run build && cd ..
```

**5. Start the server**

```sh
go run ./cmd/server
```

Open [http://localhost:8080](http://localhost:8080).

## Usage

Paste any `http://` or `https://` URL into the form and click **Investigate**. The pipeline runs in the background; tool calls and phase transitions stream live in the progress panel. When the investigation completes, the full report is shown — including per-claim confidence levels for brand identification, kit fingerprint, exfiltration target, and verdict.

Past investigations are listed on the home page and can be reopened at any time.

## Database

Migrations run automatically at server startup. The main tables are `investigations` (all pipeline artifacts and results) and `eval_labels` (ground-truth labels for future eval harness work).

To reset a stuck investigation after a killed process:

```sql
UPDATE investigations
SET status = 'failed', error_message = 'interrupted'
WHERE status NOT IN ('complete', 'failed');
```

## Project layout

```
cmd/server/           HTTP server entry point
internal/api/         REST handlers, SSE broker, routing
internal/pipeline/    four-phase pipeline orchestrator
internal/fetcher/     container orchestrator and egress proxy
internal/hypothesis/  Phase 2 — DOM summary and hypothesis generation
internal/agent/       Phase 3 — enrichment agent loop
internal/tools/       MCP tool server (whois, crt.sh, urlscan, urlhaus, JS analysis)
internal/synthesis/   Phase 4 — final report synthesis
internal/db/          Postgres connection, migrations, CRUD
internal/report/      report formatter
web/                  React + Tailwind frontend
docker/fetcher/       standalone Rod/Chromium fetcher binary and Dockerfile
specs/                requirements, design, and task tracking
```

## Safety

- The headless browser runs in Docker with no host network or filesystem access
- Egress from the container is restricted to the target URL's resolved IPs
- Forms are never auto-submitted
