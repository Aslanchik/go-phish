# go-phish

A CLI tool that takes a suspicious URL and produces a structured phishing investigation report: brand impersonated, targeted action, and confidence-rated verdict.

## How it works

1. **Fetch** — loads the URL in a sandboxed headless Chromium container; captures rendered DOM, screenshot, network log, JS files, and forms
2. **Hypothesize** — sends the screenshot and a DOM summary to Claude; forces a structured `record_hypothesis` tool call
3. **Store** — writes all artifacts and the hypothesis to Postgres
4. **Report** — prints the result to stdout

Container egress is restricted to the target IP only via an in-process HTTP CONNECT proxy. The browser runs `--cap-drop ALL --read-only --no-new-privileges`.

## Prerequisites

- Go 1.26+
- Docker (Desktop or Engine)
- PostgreSQL (or use the included Compose file)
- An Anthropic API key

## Setup

**1. Start Postgres**

```sh
docker compose up -d
```

**2. Set environment variables**

Copy the example and fill in your API key:

```sh
cp .env.example .env
# edit .env — set ANTHROPIC_API_KEY
```

The two required variables:

| Variable | Example |
|---|---|
| `DATABASE_URL` | `postgres://gophish:gophish@localhost:5432/gophish?sslmode=disable` |
| `ANTHROPIC_API_KEY` | `sk-ant-...` |

**3. Build the fetcher Docker image**

```sh
docker build -t go-phish-fetcher:latest docker/fetcher/
```

**4. Build the CLI**

```sh
go build -o gophish ./cmd/gophish
```

## Usage

```sh
export $(cat .env | xargs)

./gophish <url>
```

Example:

```
$ ./gophish https://suspicious-login.example.com

=== Phishing Investigation Report ===

Investigation ID:    3f2a1b4c-...
Timestamp:           2026-05-04T12:00:00Z
URL:                 https://suspicious-login.example.com
Final URL:           https://suspicious-login.example.com/login

--- Hypothesis ---

Brand:               PayPal
Targeted action:     credential_theft
Confidence:          high
Reasoning:           Login form posts credentials to an unrelated domain; visual design closely matches PayPal's sign-in page.
```

Exit code is 0 on success, 1 on any failure. Errors print to stderr.

```sh
./gophish              # exit 1: prints usage
./gophish not-a-url    # exit 1: URL must start with http:// or https://
./gophish --help       # prints usage
```

## Database

Migrations run automatically at startup. Investigations are stored in the `investigations` table; ground-truth labels go in `eval_labels`.

To reset a stuck investigation (e.g. after a killed process):

```sql
UPDATE investigations
SET status = 'failed', error_message = 'interrupted'
WHERE status NOT IN ('complete', 'failed');
```

## Project layout

```
cmd/gophish/          CLI entry point and pipeline wiring
internal/fetcher/     container orchestrator and egress proxy
internal/hypothesis/  DOM summary extraction and LLM call
internal/db/          Postgres connection, migrations, CRUD
internal/report/      report formatter
docker/fetcher/       standalone Rod/Chromium fetcher binary and Dockerfile
specs/                requirements, design, and task tracking
```

## Safety

- The headless browser runs in Docker with no host network or filesystem access
- Egress from the container is restricted to the target URL's resolved IPs
- Forms are never auto-submitted
