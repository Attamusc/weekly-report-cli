# Copilot Integration Research

*Researched: 2026-06-01 — based on live API tests against `api.githubcopilot.com` and `models.github.ai`, source inspection of `actions/ai-inference`, and first-party docs.*

---

## TL;DR

There are two viable GitHub-hosted paths beyond the Models marketplace endpoint. **The Copilot API** (`api.githubcopilot.com/chat/completions`) is an OpenAI-compatible endpoint that is a near-zero-change drop-in for the current code — same request format, same `Bearer` auth, richer model catalog, and likely a different (higher-capacity) quota pool than `models.github.ai`. The API works and responds in ~1.3 s per call (live-tested). The catch: it is **not officially documented as a public third-party API**, requires a Copilot subscription–bound token (fine-grained PAT with "Copilot Requests" permission), and **GITHUB_TOKEN does not work**, making CI awkward.

**The current Models endpoint is probably not the actual problem.** With a Copilot Enterprise–scoped token, `models.github.ai` exposes `x-ratelimit-limit-requests: 20000/min` — that's not a rate-limit problem. The 8× 429s at 5-parallel are likely caused by the wrong token type (e.g., a classic PAT or a token without a Copilot seat), not the endpoint itself.

**Recommended path**: Fix the token first — use a fine-grained PAT with `models: read` scope and a Copilot subscription on the underlying account. If the 429s persist, swap the `BaseURL` to `https://api.githubcopilot.com` and add "Copilot Requests" permission to the PAT. That's the lowest-risk, lowest-complexity change.

---

## Option A: `copilot` CLI (subprocess)

### Install & status
- Package: `npm install -g @github/copilot` (current: `@github/copilot@1.0.57`)
- Also installable via Homebrew cask (`copilot-cli`)
- Status: Public Preview (not GA, but actively maintained — 716 releases as of research date)

### Non-interactive mode
**Yes, fully supported.**

```bash
copilot -p "Return ONLY this JSON: {\"ok\": true}" -s --no-ask-user
```

- `-p` / `--prompt`: execute a single prompt and exit (non-interactive)
- `-s` / `--silent`: suppress session metadata; stdout is *only* the model response
- `--no-ask-user`: disable the `ask_user` tool so the CLI never blocks waiting for input
- By default, **no tools** are allowed (no shell, no file write, no fetch), so you get pure inference

Live test confirms this works: the command outputs `{"ok": true}` on stdout. The process does linger slightly before exiting — plan for `waitpid` or a timeout wrapper.

**Output format**: Raw text on stdout. If you ask for JSON, you get JSON. No framing.

### Auth
```
COPILOT_GITHUB_TOKEN > GH_TOKEN > GITHUB_TOKEN (env, in precedence order)
```

Supported token types (per `copilot login --help`):
> "fine-grained personal access tokens (v2 PATs) with the 'Copilot Requests' permission, OAuth tokens from the GitHub Copilot CLI app, and OAuth tokens from the GitHub CLI (gh) app. **Classic personal access tokens (ghp_) are not supported.**"

**GITHUB_TOKEN (Actions auto-provisioned `ghs_` token) is NOT listed and does NOT work** — it lacks user-level Copilot seat association.

### Rate limits / quota
Not documented publicly. The CLI uses the Copilot inference backend, which is separate from the `models.github.ai` pool. No rate-limit headers are exposed. Assumed to draw from Copilot seat quota.

### Verdict for our use case
**Poor fit for batch inference.** For 50 calls per run, 50 subprocess invocations means ~50× startup overhead and serial execution (or very careful parallelism). The `actions/ai-inference` action itself uses this approach for its `provider: copilot` mode and notes the subprocess pattern. For a library making direct HTTP calls, this is the wrong abstraction layer.

---

## Option B: Copilot completions API / SDK (embedded HTTP call)

### Endpoint

```
POST https://api.githubcopilot.com/chat/completions
```

**Live-tested and confirmed working.** Request/response format is identical to OpenAI chat completions (the same format our `ghmodels.go` already uses).

```bash
curl -s \
  -H "Authorization: Bearer $(gh auth token)" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o-mini",
    "messages": [
      {"role": "system", "content": "Return only JSON"},
      {"role": "user", "content": "Return {\"ok\":true}"}
    ],
    "response_format": {"type": "json_object"}
  }' \
  "https://api.githubcopilot.com/chat/completions"
```

Response (trimmed):
```json
{
  "choices": [{"message": {"content": "{\"ok\":true}", "role": "assistant"}}],
  "model": "gpt-4o-mini-2024-07-18",
  "usage": {"total_tokens": 26},
  "copilot_usage": { ... }
}
```

Latency: ~1.3 s round-trip (live, same as `models.github.ai`).

### Model catalog

Rich — `GET https://api.githubcopilot.com/models` returns (as of research date):
- **OpenAI**: gpt-4o, gpt-4o-mini, gpt-4.1, gpt-5.2, gpt-5.4, gpt-5.5, gpt-5-mini
- **Anthropic**: claude-opus-4.x, claude-sonnet-4.5/4.6, claude-haiku-4.5
- **Google**: gemini-2.5-pro, gemini-3.1-pro-preview, gemini-3.5-flash
- **Azure OpenAI**: text-embedding-3-small, legacy gpt-3.5/gpt-4 variants

The `gpt-4o-mini` identifier maps to `gpt-4o-mini-2024-07-18` (confirmed in response `model` field).

### Auth

Same `Bearer` token as `gh auth` OAuth flow. Fine-grained PAT with "Copilot Requests" permission also works. No `Copilot-Integration-Id` header required (tested without it — returns 200).

**GITHUB_TOKEN (ghs_ type) is NOT supported** — same constraint as the CLI.

### Rate limits

The response headers from `api.githubcopilot.com` **do not include any `x-ratelimit-*` headers**. Rate limits are undocumented. Assumed to draw from the same Copilot seat quota as the editor extensions and CLI.

### Public API status

**Explicitly unverified — this is the critical caveat.** The endpoint exists, works, and has been discoverable from open-source projects (the copilot CLI itself uses it internally), but GitHub has **not published official public documentation** for third-party programmatic use of `api.githubcopilot.com`. 

What IS documented:
- The Models REST API (`models.github.ai`) — fully documented at `docs.github.com/en/rest/models/inference`
- The Copilot REST API for **management** (seat assignment, usage metrics) — fully documented
- The copilot CLI automation — documented at `docs.github.com/en/copilot/how-tos/copilot-cli/automate-copilot-cli`

What is NOT documented:
- Direct HTTP access to `api.githubcopilot.com/chat/completions` from third-party tools
- Rate limits, SLA, ToS for this endpoint

**Use at your own risk.** The endpoint could change or add additional auth requirements without notice.

### Go SDK

No official Go SDK. The current `GHModelsClient` in `internal/ai/ghmodels.go` is already a thin HTTP wrapper using the OpenAI-compatible format — switching `BaseURL` to `https://api.githubcopilot.com` would be the entire change.

### Verdict for our use case
**Best technical fit** if auth can be solved. Drop-in swap of `BaseURL`. Richer model catalog. Likely higher-capacity quota pool. Main risk: undocumented public API surface.

---

## Option C: `actions/ai-inference` (workflow-side, Models endpoint)

The official GitHub Action at `actions/ai-inference@v1` supports two providers:

| Provider | Endpoint | Auth |
|---|---|---|
| `github-models` (default) | `https://models.github.ai/inference` | `GITHUB_TOKEN` with `models: read` |
| `copilot` | copilot CLI subprocess | `COPILOT_GITHUB_TOKEN` (fine-grained PAT) |

**The Models provider uses `permissions: models: read`**, which makes `GITHUB_TOKEN` usable — no secrets needed:

```yaml
jobs:
  inference:
    permissions:
      models: read
    steps:
      - uses: actions/ai-inference@v1
        with:
          prompt: 'Hello!'
```

**This is workflow orchestration, not a drop-in for our Go CLI.** The action returns a single string output. Our tool needs to run 5–50 inference calls internally, so we'd have to restructure to call this action from within the workflow — fundamentally different architecture.

A minor note: `actions/ai-inference` also supports the `copilot` provider (via subprocess), using `COPILOT_GITHUB_TOKEN: ${{ secrets.COPILOT_PAT }}`.

---

## Option D: Stay on Models endpoint, dial concurrency to 1

The `models.github.ai` rate limit headers observed live (Copilot Enterprise token):
```
x-ratelimit-limit-requests: 20000
x-ratelimit-limit-tokens: 2000000
x-ratelimit-renewalperiod-requests: 60  (per 60 seconds)
x-ratelimit-renewalperiod-tokens: 60
```

This is 20,000 requests/minute — **333 requests/second**. At that limit, 50 calls even at full concurrency should never 429.

**Hypothesis**: The 429s in the issue description are caused by a token without Copilot Enterprise access. Free-tier and personal-plan Copilot accounts have much lower limits (documentation for exact free-tier limits is behind a 404 as of research date, but historically it's been 15–50 req/min for GPT-4 class models). The fix may be as simple as using a token from a Copilot Enterprise seat.

**Concurrency=1 reference data**: At 1 req/s, 50 calls = 50 seconds. With the batch approach already implemented in `SummarizeBatch` (25 items per call), it's 2 API calls total for 50 items. That's well within any tier's limits.

---

## GitHub Actions Story

| Approach | Works with GITHUB_TOKEN? | Extra setup | Notes |
|---|---|---|---|
| Models endpoint (`models.github.ai`) | ✅ Yes, `permissions: models: read` | None | Best for Actions |
| Copilot API direct (`api.githubcopilot.com`) | ❌ No | `COPILOT_GITHUB_TOKEN` secret (fine-grained PAT with "Copilot Requests" perm) | No official docs |
| copilot CLI subprocess | ❌ No | Install step + `COPILOT_GITHUB_TOKEN` secret | 50× subprocess overhead |
| `actions/ai-inference@v1` (Models provider) | ✅ Yes | None | Workflow-level only |
| `actions/ai-inference@v1` (Copilot provider) | ❌ No | Install step + `COPILOT_GITHUB_TOKEN` secret | Workflow-level only |

**Key insight**: For the CI use case, **Models endpoint with GITHUB_TOKEN is already the simplest path**. The token just needs the `models: read` permission (via workflow `permissions:` block) — no secrets, no extra setup. If rate limits are the real problem, that should be debugged before switching endpoints.

**For org/enterprise users**: If the org has Copilot Business or Enterprise, the rate limits on `models.github.ai` (with a Copilot-scoped token) are very generous. The org can provision a fine-grained PAT from a service account with a Copilot seat.

---

## Recommendation

**Step 1 (do first)**: Verify what token type is being used in CI. Add a step that prints:
```bash
curl -s -D - -o /dev/null -H "Authorization: Bearer $GITHUB_TOKEN" \
  -X POST -d '{"model":"openai/gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}' \
  https://models.github.ai/inference/chat/completions | grep x-ratelimit
```
If you see `x-ratelimit-limit-requests: 15` or similar, the token is free-tier. If you see 20000, the problem was elsewhere.

**Step 2 (if rate limits are confirmed low)**: Switch to a fine-grained PAT from a Copilot Enterprise–seeded account, stored as a workflow secret. Keep using `models.github.ai`. The endpoint is fully documented, rate limits are transparent, and GITHUB_TOKEN works with `models: read`.

**Step 3 (if you want the Copilot API for richer models/quota)**: Change `GHModelsClient.BaseURL` from `https://models.github.ai` to `https://api.githubcopilot.com` and update the PAT to include "Copilot Requests" permission. The current code works as-is (same request format, same Bearer auth). Accept the caveat that this endpoint is undocumented and could change.

**What NOT to do**: Don't use the `copilot` CLI subprocess for batch inference. Subprocess overhead, serial execution, and process lifecycle management make it a poor fit for 50 structured inference calls.

**Anthropic direct API**: Viable if you want fully documented, high-volume, pay-per-token access. Auth complexity is 5/5 (per-environment key provisioning, billing setup). Recommend only if GitHub-hosted options provably can't meet the volume.

---

## Sources

| Resource | URL | Notes |
|---|---|---|
| copilot CLI login help | `copilot login --help` (local) | Token type requirements |
| copilot CLI env vars | `copilot help environment` (local) | Auth env var precedence |
| `actions/ai-inference` README | `github.com/actions/ai-inference` | Provider comparison, permissions, workflow examples |
| `actions/ai-inference` copilot.ts | `github.com/actions/ai-inference/src/copilot.ts` | How `-p -s --no-ask-user` is used |
| `actions/ai-inference` main.ts | `github.com/actions/ai-inference/src/main.ts` | `provider: copilot` vs `provider: github-models` logic |
| GitHub Copilot CLI automation docs | `docs.github.com/en/copilot/how-tos/copilot-cli/automate-copilot-cli/automate-with-actions` | GITHUB_TOKEN vs COPILOT_GITHUB_TOKEN |
| `api.githubcopilot.com/chat/completions` | Live test | Confirmed working, no public docs |
| `api.githubcopilot.com/models` | Live test | Model catalog |
| `models.github.ai/inference` rate limits | Live response headers | 20K req/min with Copilot Enterprise token |
| npm `@github/copilot` | `npmjs.com/package/@github/copilot` | v1.0.57, install command for Actions |

---

## A/B run results

Run date: 2026-06-01. Command: `highlights --users Attamusc --since-days 7 --ai-top 5 --ai-concurrency 5 --verbose`. Source data: 5 survivors from recent GitHub activity.

| Metric | gpt-4o (default) | gpt-4o-mini | claude-haiku-4.5 (Copilot) |
|---|---|---|---|
| Wall clock | 14.6s | 6.8s | 6.3s |
| 429 rate-limited (retries) | 12 | 0 | 0 |
| Fallbacks (parse errors) | 5/5 | 5/5 | 5/5 |
| AI map step latency | ~5.2s | ~1.9s | ~1.9s |
| Endpoint | `models.github.ai` | `models.github.ai` | `api.githubcopilot.com` ✅ |

**Notes:**
- The `gpt-4o` default (gpt-4o-mini mapping) hit 12 rate-limit retries, adding ~8s of wall clock vs the other two.
- All three runs produced 5/5 fallbacks (title echoes in the output) due to a pre-existing parsing bug: the `SummarizeHighlight` prompt for these items returns a JSON array instead of a single object, which the current parser rejects. This is not caused by the endpoint change.
- The Haiku run confirmed the new Copilot code path works end-to-end: HTTP 200 from `api.githubcopilot.com`, correct path `/chat/completions`, `Copilot-Integration-Id: vscode-chat` header sent, OpenAI-shaped response parsed correctly.
- Run 3 (Haiku) **did not crash**; all 5 items got responses, fallbacks were parse-shape issues identical to the other models.

**Summary quality:** All three produced title-echo output due to the pre-existing fallback issue — not a meaningful quality comparison for this specific dataset. Qualitatively, Haiku response latency matched gpt-4o-mini (~1.9s map step) and both were significantly faster than the rate-limited gpt-4o default run.

**Recommendation:** The Copilot endpoint is viable. Haiku 4.5 shows no latency or reliability disadvantage vs gpt-4o-mini. Rate limiting was a gpt-4o-only problem in this run. A meaningful quality comparison requires fixing the `SummarizeHighlight` parser first (see pre-existing issue with array responses).
