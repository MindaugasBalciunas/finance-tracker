---
name: ft-release
description: The finance-tracker release flow — pre-release gate, version bump in finance_tracker/config.yaml, commit message format, then STOP — the owner pushes to main themselves; after their push, watching the GitHub Actions run by commit SHA and verifying the ghcr.io image tag. Load when a change is built and verified and should ship, or when asked to release, bump the version, commit or push.
---

# Releasing

The owner's rules:
- **Never `git push` yourself.** The owner pushes manually (since 2026-10-16). Every push to `main` triggers CI and an image build, so pushing is theirs to decide.
- Once a change is built and verified green, **prepare it fully** — version bump and local commit — without asking again for each step, then stop and tell the owner it is ready to push. Never prepare red or unverified code.
- Push only when the owner explicitly says so in the current conversation, for that change.

## 1. Gate

- `make test` (repo root) green.
- UI change: live check in Firefox at 390px + desktop (`ft-live-verify`).
- Schema change: backup round-trip (`ft-migration`).
- Security pass (`ft-security` → "Release security pass"): `govulncheck`, `npm audit`.
- `git status` shows only intended files; nothing from `finance_tracker/backend/` (local data), `.env`, `*.db`, screenshots or scratch files.

## 2. Version

`finance_tracker/config.yaml` → `version: "X.Y.Z"` is the release version (CI tags the image from it; the server gets it via `-X main.version`). `fix` → patch, `feat` → minor, breaking data/rollback change → discuss first. `web/package.json`'s version is not used for releases.

## 3. Commit

One commit per release, subject in the existing style:

```
fix(v2.30.5): <what the owner now sees, as a plain sentence>

<Before → after in user terms: what was wrong, with a concrete example
(amounts, screen names), what happens now, and anything deliberately left
as it was. Wrap at ~72 columns.>

Co-Authored-By: <attribution line from the session's system reminder>
```

`feat(vX.Y.0): …` for features; a short bullet list in the body is fine for multi-part features. Stage files by name, not `git add -A`.

## 4. Hand over, then watch CI by SHA after the owner pushes

Stop here and report: the commit SHA, the version, what changed, and `git push origin main` as the command for the owner to run (or `! git push origin main` in this prompt). Do not push.

Once the owner says they pushed (or asked you to in this conversation), watch the run:

```bash
SHA=$(git rev-parse HEAD)
# the run can take a few seconds to register — poll until it exists, never use --limit 1
gh run list --json databaseId,headSha --jq ".[] | select(.headSha==\"$SHA\") | .databaseId"
gh run watch <id> --exit-status
```

Watching "the latest run" once reported a previous run's green for a broken release. The workflow (`.github/workflows/build.yaml`) runs go vet + race tests, the real-nginx gate test, tsc + vitest + build, then builds the arm64 image.

## 5. Verify the image exists

```bash
V=X.Y.Z
TOKEN=$(curl -s "https://ghcr.io/token?scope=repository:mindaugasbalciunas/finance-tracker:pull" | jq -r .token)
curl -s -o /dev/null -w '%{http_code}\n' -H "Authorization: Bearer $TOKEN" \
  -H 'Accept: application/vnd.oci.image.index.v1+json, application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json, application/vnd.docker.distribution.manifest.list.v2+json' \
  https://ghcr.io/v2/mindaugasbalciunas/finance-tracker/manifests/$V   # expect 200
```

## 6. Hand-off

Tell the owner the version is ready to update in Home Assistant, what changed in a sentence or two, and anything they should check or do afterwards (e.g. reconnect a bank, a migration that runs on restart).

If CI fails: read the failing step's log (`gh run view <id> --log-failed`), fix forward with a new patch version — don't force-push main.
