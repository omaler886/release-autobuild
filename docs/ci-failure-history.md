# GitHub Actions failure history

Last audited: 2026-07-12 (Asia/Shanghai)

## Scope and totals

The audit paged through every Actions run exposed by the GitHub API from 2026-04-27 through 2026-07-11.

- Total runs: 320
- Successful: 70
- Failed: 244
- Cancelled: 6
- Scheduled: 299
- Manually dispatched: 21
- Workflow: `Release Autobuild`

A failed run may contain more than one failed matrix job. Failed-job counts therefore do not add up to the failed-run total.

| Failed job group | Count |
| --- | ---: |
| Momogram | 171 |
| v2rayNG | 105 |
| sing-box | 12 |
| Legacy single build job | 13 |
| Upstream branch sync | 5 |
| State commit | 1 |

## Root-cause tracking

| Root cause | Evidence and affected period | Attribution | Resolution status |
| --- | --- | --- | --- |
| Momogram keystore path was missing | Run `24981950788`; early Actions revisions | CI signing layout did not match the upstream Gradle layout | Fixed by commits `27a979c6`, `1db3b101`, and `fd4789f7` |
| Momogram keystore password or alias mismatch | Run `25910452683` | Generated or copied credentials did not match every Gradle signing location | Fixed by PR #2 / commit `f2c4fe8b` |
| Momogram native FFmpeg/dav1d outputs were missing | Repeated failures from late April through early May; errors included missing `libswscale.a` | Native dependency update and ABI-specific build assumptions drifted from upstream | Fixed incrementally by `e4dbfa22`, `90fabe73`, `27c99659`, and `327c4371` |
| Telegram returned HTTP 413 for large artifacts | Momogram and v2rayNG artifacts exceeded the standard Bot API limit | Publication failed after compilation succeeded | Superseded by explicit size checks; durable intact-file fallback added in PR #5 |
| Telegram size policy permanently rejected successful builds | Latest run `29165489752`; Momogram APKs were 61,935,583 and 62,302,014 bytes | Splitting was intentionally disabled while the only upload backend retained a 50,000,000-byte limit | PR #5 publishes the intact asset to GitHub Releases and sends its link to Telegram; real run `29176966966` tracks verification |
| Upstream branch push had no credentials | Run `26334741152` | Generated HTTPS remote did not include Actions authentication | Fixed by PR #3 / commit `ef380979` |
| GitHub Releases response was truncated | Runs `26108693436`, `27289316720`, and `28278358325`; mostly sing-box and upstream sync | Large 20-22 MB API responses raised `http.client.IncompleteRead`, which was outside the retry handler | PR #5 now retries `IncompleteRead` and timeouts; covered by a regression test |
| Upstream metadata sync blocked every build | The build job had a hard dependency on the sync job | An auxiliary metadata failure caused the full matrix to be skipped | PR #5 allows builds whenever preparation succeeds, while still reporting sync failure separately |
| State commit was rejected as non-fast-forward | Run `25963837338` | Concurrent workflow state writers raced | Current workflow-level concurrency serializes runs; no recurrence in the audited current design |
| Runner shutdown or cancellation | Isolated exit 143 / operation-cancelled run on 2026-07-07, plus six cancelled runs | Hosted-runner or externally cancelled infrastructure event | Classified as transient; rerun is appropriate |

## Current invariant

Successful compilation must not be reported as a build failure solely because a complete artifact is larger than Telegram's standard upload limit. The supported behavior is:

1. Files at or below `TELEGRAM_MAX_UPLOAD_BYTES` are uploaded directly to Telegram.
2. Larger files remain intact, are uploaded as GitHub Release assets when `TELEGRAM_OVERSIZE_MODE=github-release`, and their download URLs are sent to Telegram.
3. Segmented uploads remain rejected.
4. State is written only after the selected publication path succeeds.

## Verification gates

- `python -m unittest discover -s tests -v`
- `python -m py_compile build_release.py patches/momogram.py patches/v2rayng-no-ads.py`
- `actionlint .github/workflows/release-autobuild.yml`
- A real Momogram build must create a downloadable intact Release asset, notify Telegram, update state, and finish successfully.
- A subsequent full scheduled or manually dispatched `poll-all` run must complete without a recurring known root cause.

## Residual engineering risks

These were not the direct cause of the latest repeated failures but remain tracked:

- APK collection is broad and can include unsigned variants; add explicit variant selection and signature verification.
- Android preview SDK packages and moving `ubuntu-latest`/Action major tags reduce reproducibility.
- Multipart Telegram uploads read the full file into memory.
- Branches are unprotected while the workflow has `contents: write` and upstream metadata branches are force-updated.
- PR #1 and old Momogram fix branches are stale and should be closed or removed after the active fix is merged.
