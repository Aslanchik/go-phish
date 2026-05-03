---
name: pr
description: Run tests and vetting, then create a GitHub pull request. Use when the user wants to open a PR, submit changes for review, or push a branch.
disable-model-invocation: true
allowed-tools: Bash(git *), Bash(go vet *), Bash(go test *), Bash(go build *), Bash(gh *)
---

## Context

- Current branch: !`git branch --show-current`
- Commits to be included: !`git log main...HEAD --oneline 2>/dev/null || git log --oneline -5 2>/dev/null || echo "(no commits yet)"`
- Git status: !`git status --short`
- Diff stat: !`git diff --stat main...HEAD 2>/dev/null || echo "(no diff)"`

## Your task

Follow these steps in order. Stop and report if any step fails.

### 1. Vet

```bash
go vet ./...
```

Fix any errors before continuing.

### 2. Test

```bash
go test ./...
```

All tests must pass before continuing.

### 3. Build check

```bash
go build ./...
```

Must compile cleanly.

### 4. Stage and commit any uncommitted changes

If `git status` shows uncommitted changes, stage and commit them now using a conventional commit message.

### 5. Push the branch

```bash
git push -u origin $(git branch --show-current)
```

### 6. Create the pull request

Use the context above to write the PR title and body. The body should read like a work summary:

```
## What was done
- <one bullet per logical change, grouped by area>

## Files and packages touched
- <package>: <what changed>

## Dependencies added
- <module>: <why it was added> (or "None")

## Testing
- Vet: passed
- Tests: passed (N tests)
- Build: clean
```

Then run:
```bash
gh pr create --title "<title>" --body "<body>"
```

Return the PR URL when done.

### 7. Switch back to main

```bash
git checkout main
```
