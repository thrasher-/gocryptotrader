---
description: Daily check for GitHub Actions failures on latest master commit (grouped per workflow).
on:
  schedule:
    - cron: "0 2 * * 1-5"
  workflow_dispatch:

permissions:
  contents: read
  actions: read
  issues: read
  pull-requests: read

tracker-id: daily-job-checker

tools:
  github:
    toolsets:
      - default
      - actions
      - issues

safe-outputs:
  create-issue:
    title-prefix: "[CI Failure] "
    labels: [ci, automation]
    max: 5
    expires: false
    close-older-issues: false

  add-comment:
    max: 20

---

# Daily Job Checker (master only, grouped per workflow)

You must only evaluate failures for the **latest commit on `master`** and group multiple failing jobs from the **same workflow** into a **single issue**.

## Step 1 — Get latest master SHA

- Get branch `master`, store:
  - `latest_master_sha`
  - `latest_master_short_sha`

Only inspect workflow runs where:
- `head_branch == "master"`
- `head_sha == latest_master_sha`

Ignore everything else.

---

## Step 2 — Fetch workflow runs for that SHA

List workflow runs for `master` filtered to `head_sha == latest_master_sha`.

Consider runs where conclusion is in:
- `failure`
- `timed_out`

(Do not treat `cancelled` as failure unless you can prove it’s breakage.)

---

## Step 3 — Extract failing jobs and group by workflow

For each failing workflow run:
1. List jobs for that run.
2. Collect jobs where `conclusion in ["failure","timed_out"]`.

Normalize a `workflow_key`:
- Prefer workflow file path if available (e.g. `.github/workflows/build.yml`)
- Otherwise workflow name.

For each failing job capture:
- `workflow_name`
- `workflow_file` (or fallback)
- `workflow_key`
- `run_id`
- `run_attempt` (default 1 if missing)
- `run_url`
- `job_name`
- `job_url` (if available)
- `head_sha` (latest_master_sha)
- `timestamp` (run updated_at or created_at)

Now group all failing jobs by `workflow_key`.

---

## Step 4 — Workflow-level dedupe (MANDATORY)

For each workflow group (one issue per workflow), compute a workflow fingerprint:

`wf_fingerprint: workflow=<workflow_key>;branch=master;sha=<latest_master_sha>`

This is intentionally workflow+sha scoped (NOT per-job), because you are grouping jobs.

### 4.1 Exact fingerprint match
Search open issues:
`repo:${{ github.repository }} is:issue is:open "wf_fingerprint: workflow=<workflow_key>;branch=master;sha=<latest_master_sha>" in:body`

If found:
- Do NOT create a new issue.
- You may optionally comment with “still failing” + latest run links, but do not spam; only comment if the issue has no reference to the latest run_id.

### 4.2 Legacy match (no fingerprint yet)
If no exact match exists, try to find an existing tracking item (open issue or PR) by searching for:
- any run URL/run id for this workflow group, OR
- workflow name + `master` + short sha.

Examples:
- `repo:${{ github.repository }} is:open (is:issue OR is:pr) "<workflow_name>" "master" "<latest_master_short_sha>"`
- `repo:${{ github.repository }} is:open (is:issue OR is:pr) "actions/runs/<run_id>" in:body`

If you find a plausible existing issue/PR:
- Do NOT create a new issue.
- Request a `comment_issue` that adds the workflow fingerprint (wf_fingerprint) so future runs dedupe.

If no match:
- Create a new issue.

---

## Step 5 — Issue content (one issue per workflow)

Title (without prefix):

`<workflow_name> failures (master @ <latest_master_short_sha>)`

Body must include:
- Summary + scope (latest master only)
- Workflow key/name
- Branch + commit
- A table or bullet list of failing jobs, including:
  - Job name
  - Conclusion
  - Run attempt
  - Run URL
  - Job URL (if available)
- Include the exact workflow fingerprint line:
  - `wf_fingerprint: workflow=...;branch=master;sha=...`

Also include a per-job mini fingerprint line for each job (optional but useful):

`job_fingerprint: workflow=<workflow_key>;run_id=<run_id>;attempt=<run_attempt>;job=<job_name>`

---

## Output format (STRICT): JSON only

You must output ONLY JSON. It may contain a mix:

### No work
```json
{"type":"noop","message":"No new workflow failures for latest master commit (or already tracked)."}
