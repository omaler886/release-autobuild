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

A failed run may contain more than one failed matrix job. The 244 failed runs contain 307 failed jobs; all 307 job logs were readable and classified. Category counts overlap when one job exposed multiple root causes.

| Failed job group | Count |
| --- | ---: |
| Momogram | 171 |
| v2rayNG | 105 |
| sing-box | 12 |
| Legacy single build job | 13 |
| Upstream branch sync | 5 |
| State commit | 1 |

## Root-cause tracking

Publication-size failures dominate the history: 220 of 244 failed runs (90.2%) contained either Telegram HTTP 413 or the later 50 MB local rejection. In many of these runs compilation itself had already succeeded.

| Classified signature | Failed jobs | Failed runs | Period |
| --- | ---: | ---: | --- |
| Telegram 50 MB preflight rejection | 208 | 155 | 2026-06-01 through 2026-07-11 |
| Telegram HTTP 413 | 65 | 65 | 2026-04-29 through 2026-05-16 |
| Momogram dav1d/FFmpeg/libswscale missing | 62 | 62 | 2026-04-27 through 2026-05-15 |
| Momogram invalid keystore/password | 62 | 62 | 2026-04-29 through 2026-05-15 |
| GitHub API `IncompleteRead` | 16 | 16 | 2026-05-19 through 2026-06-27 |
| Momogram R8/Gradle Java heap exhaustion | 10 | 10 | 2026-06-29 through 2026-07-11 |
| Momogram missing keystore/path | 8 | 8 | 2026-04-27 through 2026-04-29 |
| Momogram dynamic-feature Gradle configuration | 7 | 7 | 2026-04-27 through 2026-04-29 |
| Runner disk full | 2 | 2 | 2026-04-28 and 2026-06-17 |
| Corrupt/incomplete Android NDK archive | 1 | 1 | 2026-06-12 |
| Telegram TLS EOF | 1 | 1 | 2026-05-01 |
| Upstream push credentials missing | 1 | 1 | 2026-05-23 |
| State push non-fast-forward | 1 | 1 | 2026-05-16 |
| Runner shutdown / exit 143 | 1 | 1 | 2026-07-07 |

| Root cause | Evidence and affected period | Attribution | Resolution status |
| --- | --- | --- | --- |
| Momogram keystore path was missing | Run `24981950788`; early Actions revisions | CI signing layout did not match the upstream Gradle layout | Fixed by commits `27a979c6`, `1db3b101`, and `fd4789f7` |
| Momogram keystore password or alias mismatch | Run `25910452683` | Generated or copied credentials did not match every Gradle signing location | Fixed by PR #2 / commit `f2c4fe8b` |
| Momogram native FFmpeg/dav1d outputs were missing | Repeated failures from late April through early May; errors included missing `libswscale.a` | Native dependency update and ABI-specific build assumptions drifted from upstream | Fixed incrementally by `e4dbfa22`, `90fabe73`, `27c99659`, and `327c4371` |
| Momogram dynamic-feature configuration was invalid | Seven early Gradle failures from 2026-04-27 through 2026-04-29 | Upstream module configuration and selected release tasks were incompatible with the initial CI patch | Ended after the early Android workflow compatibility fixes |
| Momogram R8 exhausted the Java heap | Ten runs from 2026-06-29 through 2026-07-11; `minifyFdroidReleaseWithR8` raised `OutOfMemoryError` | The patch limited Gradle to 1.5 GB, below observed R8 demand | PR #5 raises the Gradle heap to 4 GB and keeps native/Gradle workers serialized |
| Telegram returned HTTP 413 for large artifacts | Momogram and v2rayNG artifacts exceeded the standard Bot API limit | Publication failed after compilation succeeded | Superseded by explicit size checks; durable intact-file fallback added in PR #5 |
| Telegram size policy permanently rejected successful builds | Latest run `29165489752`; Momogram APKs were 61,935,583 and 62,302,014 bytes | Splitting was intentionally disabled while the only upload backend retained a 50,000,000-byte limit | PR #5 publishes the intact asset to GitHub Releases and sends its link to Telegram; real run `29176966966` tracks verification |
| Upstream branch push had no credentials | Run `26334741152` | Generated HTTPS remote did not include Actions authentication | Fixed by PR #3 / commit `ef380979` |
| GitHub Releases response was truncated | Runs `26108693436`, `27289316720`, and `28278358325`; mostly sing-box and upstream sync | Large 20-22 MB API responses raised `http.client.IncompleteRead`, which was outside the retry handler | PR #5 now retries `IncompleteRead` and timeouts; covered by a regression test |
| Upstream metadata sync blocked every build | The build job had a hard dependency on the sync job | An auxiliary metadata failure caused the full matrix to be skipped | PR #5 allows builds whenever preparation succeeds, while still reporting sync failure separately |
| State commit was rejected as non-fast-forward | Runs `25963837338` and verification run `29176966966` | Concurrent writers or a long build finishing after its branch advanced caused a stale checkout | PR #5 resets to the latest branch before merging state and retries push with rebase |
| Runner shutdown or cancellation | Isolated exit 143 / operation-cancelled run on 2026-07-07, plus six cancelled runs | Hosted-runner or externally cancelled infrastructure event | Classified as transient; rerun is appropriate |
| Runner disk was full | Two isolated runs on 2026-04-28 and 2026-06-17 | Hosted workspace was exhausted by large Android/native intermediates | Classified as infrastructure/resource pressure; cleanup between releases and serialized project jobs limit recurrence |
| Android NDK archive was invalid | One run on 2026-06-12 reported an unknown archive format | SDK download was incomplete or corrupt | Classified as transient download corruption; rerun is appropriate |
| Telegram TLS stream ended early | One run on 2026-05-01 raised an SSL EOF | Transient network failure during publication | Classified as transient; later publication retries/reruns succeeded |

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
