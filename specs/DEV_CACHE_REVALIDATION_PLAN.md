# DEV cache revalidation — conditional GET across `sailor-backend-app` and `sailor-go`

## Problem

`sailor-go`'s DEV fetch mode caches a resource to `~/.sailor/cache/{ns}-{app}-{env}-{name}.json` and, before this change, treated the existence of that file as a permanent short-circuit: the backend was never contacted again. A developer with a warm cache therefore kept running whatever the resource looked like the first time they ever started the process, with no signal that it had been redeployed since.

The naive fix — always refetch and overwrite — is wrong here, because the DEV cache file is deliberately dual-purpose. It is also the file a developer is invited to hand-edit for live reload (the SDK watches it with fsnotify). Overwriting it on every start would silently destroy local experiments.

So the fix has two halves:

- **`sailor-go`** must tell a *pristine* cache (safe to resync) apart from a *hand-edited* one (never overwrite), and revalidate only the former. **Done** — see "SDK half" below.
- **`sailor-backend-app`** should support HTTP conditional GET so that revalidation is cheap (`304 Not Modified`, no body, no server-side decrypt/marshal) instead of costing a full content fetch every startup. **Done** — see "Backend half" below.

The two halves are independent and can ship in either order. The SDK already sends `If-None-Match` and degrades gracefully: a backend that ignores the header just answers `200`, and the SDK falls back to comparing content hashes.

---

## Backend half — `sailor-backend-app` (implemented)

### 1. `cmd/sailor/handlers/getres.go`

In `(sc *SailorCore) GetResourceHandler`, `builtRes.Version` is already computed at line 80 and emitted at line 88:

```go
// TODO :: move header key to a common place
ctx.Response.Header.Set("x-resource-version", fmt.Sprintf("%d", builtRes.Version))
```

Insert the conditional-GET handling **immediately after that line** — i.e. after the version is known, but *before* the `if !resource.Encrypted` branch at line 90. Placing it there means a `304` also skips the JSON unmarshal/encode of `map[string]vault.SecretRecord` for encrypted resources, so the saving is server CPU, not just response bytes:

```go
// The deployment version doubles as the resource's ETag: it's already
// monotonic per deploy, so a client that echoes it back is asking exactly
// "is this still the deployment I have?". Answered before the content is
// built so an unchanged resource costs no marshal/decrypt work at all.
etag := fmt.Sprintf("%q", builtRes.Version)
ctx.Response.Header.Set("ETag", etag)

if string(ctx.Request.Header.Peek("If-None-Match")) == etag {
    ctx.SetStatusCode(fasthttp.StatusNotModified)
    return
}
```

Notes:

- `x-resource-version` stays exactly as it is — `pkg/core/v1.CoreAPIClient.GetResource` (line 196) treats it as **required** on a 200, so it must not be removed or renamed.
- The ETag must be quoted per RFC 7232 — `fmt.Sprintf("%q", ...)` yields `"7"`. A plain string comparison is enough; no `W/` weak-validator handling is needed since we both generate and compare the value.
- Return before any `enc.Encode(...)` / `ctx.SetBodyString(...)` so the 304 carries no body. If fasthttp has already defaulted a `Content-Type`, `ctx.Response.ResetBody()` before `SetStatusCode` keeps the response strictly clean.
- This is purely additive. A client that sends no `If-None-Match` sees byte-identical behaviour to today.

### 2. `cmd/sailor/handlers/cors.go` (browser clients only — the Go SDK is unaffected)

```go
// line 32 — let the console send the conditional header
ctx.Response.Header.Set("Access-Control-Allow-Headers", "Content-Type, Authorization, x-token, x-user, x-pass, If-None-Match")
// line 33 — let the console read the validator back
ctx.Response.Header.Set("Access-Control-Expose-Headers", "x-resource-version, x-resource-encrypted, ETag")
```

### 3. Tests — `cmd/sailor/handlers/getres_test.go`

Existing tests already assert on `x-resource-version` / `x-resource-encrypted` (lines 52-56, 85-89, …); add alongside them:

| Case | Expectation |
| --- | --- |
| No `If-None-Match` sent | Unchanged `200` + full body (regression guard for existing clients) |
| `If-None-Match: "<current version>"` | `304`, empty body, `ETag` still present |
| `If-None-Match: "<older version>"` | `200` with full body and the new `ETag` |
| Encrypted resource, matching `If-None-Match` | `304` — proves the decrypt/encode path is skipped, not merely bypassed |

### 4. Optional follow-up

`pkg/core/v1/resource.go`'s `CoreAPIClient.GetResource` (used by the backend's own CLI) could also send `If-None-Match` and handle `304`. Not required for the SDK fix — track separately if wanted.

### Verifying the backend half

```bash
go test ./cmd/sailor/handlers/...

# against a running instance
curl -i -H "x-token: $TOKEN" http://localhost:7766/api/v1/resource/<ns>/<app>/config          # 200 + ETag
curl -i -H "x-token: $TOKEN" -H 'If-None-Match: "<etag>"' \
     http://localhost:7766/api/v1/resource/<ns>/<app>/config                                  # 304, no body
```

---

## SDK half — `sailor-go` (implemented)

### Mechanism

Alongside each cache file, the SDK now writes a sidecar `{cachePath}.meta.json`:

```json
{ "version": "\"7\"", "hash": "<sha256 of the cached bytes>" }
```

`version` is the opaque validator kept verbatim (preferring `ETag`, falling back to `x-resource-version`) and echoed back verbatim in `If-None-Match`. `hash` is what proves whether the cache file is still exactly what the SDK wrote.

On each startup, for a DEV resource with an existing cache file:

1. **Hash mismatch, or no sidecar at all** → the file was hand-edited (or predates the sidecar, so there's no baseline to trust). The local copy wins and is never overwritten. The SDK still makes a best-effort conditional GET purely to report drift, logging `stale` if upstream has moved past the version the local copy branched from.
2. **Hash matches** (pristine) → revalidate with `If-None-Match`:
   - `304` → `up to date`, no disk write.
   - `200` whose content hashes to the same value → also `up to date` (this is the path a pre-conditional-GET backend takes); the sidecar's version marker is refreshed so the next run can still get a cheap `304`.
   - `200` with different content → cache file and sidecar are both rewritten, logged as `updated (vOLD → vNEW)`.
   - transport error / non-200 → fall back to the cache silently, so a dead backend never breaks a working local environment.
3. **No cache file** → cold fetch as before, now also writing the sidecar.

### Files changed

| File | Change |
| --- | --- |
| `sailor.go` | `devLoadOrFetch` rewritten; new `devFetch`, `devReportUpstreamDrift`, `devCacheMeta` + read/write helpers, `resourceVersion`, `sha256Hex`; `doGet` takes an `ifNoneMatch` argument |
| `dev_log.go` | New `up to date` / `updated` / `local edit` / `stale` log lines in the existing `devLogLine` vocabulary |
| `defaults.go` | `ConfigDevDefault` / `SecretsDevDefault` doc comments describe the new behaviour |
| `README.md` | DEV row of the fetch-methods table and the dev rows of the defaults table |
| `dev_test.go` | **New** — DEV mode previously had zero test coverage |

`FetchDefinition.Once` was deliberately left untouched; there is no opt-out knob, this applies to all DEV resources.

### Behavioural notes

- **Existing caches self-heal on first run after upgrade.** A cache file with no sidecar is treated as a local override and left alone. Delete `~/.sailor/cache/*.json` once to get a tracked, auto-syncing baseline.
- **The fsnotify live-reload path is untouched.** A hand edit still reloads in-memory immediately, and because the sidecar keeps holding the *upstream* hash, the next startup correctly still sees the file as locally edited.

### Verifying the SDK half

```bash
go test ./...          # includes the new dev_test.go
go test -race ./...
```

Manual: run a program using `ConfigDevDefault` against a real backend (cold cache) → redeploy the resource → rerun and confirm `updated` is logged and the cache content changed → hand-edit the cache file → rerun and confirm `local edit` / `stale` is logged and the file is left intact.
