#!/usr/bin/env bash
set -euo pipefail

tracking_file="${TRACKING_FILE:-.github/upstream-tracking.env}"
xhttp_branch="${XHTTP_BRANCH:-xhttp}"

if [[ -f "$tracking_file" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "$tracking_file"
  set +a
fi

: "${OFFICIAL_REPO:=https://github.com/SagerNet/sing-box.git}"
: "${OFFICIAL_BRANCH:=testing}"
: "${OFFICIAL_LAST_SEEN_COMMIT:=}"
: "${YELNOO_REPO:=https://github.com/yelnoo/sing-box.git}"
: "${YELNOO_BRANCH:=stable}"
: "${YELNOO_LAST_SEEN_COMMIT:=}"
: "${MIHOMO_REPO:=https://github.com/daiaji/mihomo.git}"
: "${MIHOMO_BRANCH:=feat/splithttp}"
: "${MIHOMO_LAST_SEEN_COMMIT:=}"

start_head="$(git rev-parse HEAD)"
previous_official="${OFFICIAL_LAST_SEEN_COMMIT}"
previous_yelnoo="${YELNOO_LAST_SEEN_COMMIT}"
previous_mihomo="${MIHOMO_LAST_SEEN_COMMIT}"

git remote remove official >/dev/null 2>&1 || true
git remote remove yelnoo >/dev/null 2>&1 || true
git remote remove mihomo >/dev/null 2>&1 || true
git remote add official "${OFFICIAL_REPO}"
git remote add yelnoo "${YELNOO_REPO}"
git remote add mihomo "${MIHOMO_REPO}"

git fetch origin --prune
git fetch official "${OFFICIAL_BRANCH}" --prune
git fetch yelnoo "${YELNOO_BRANCH}" --prune
git fetch mihomo "${MIHOMO_BRANCH}" --prune

official_head="$(git rev-parse "official/${OFFICIAL_BRANCH}")"
yelnoo_head="$(git rev-parse "yelnoo/${YELNOO_BRANCH}")"
mihomo_head="$(git rev-parse "mihomo/${MIHOMO_BRANCH}")"

git branch -f upstream-testing "official/${OFFICIAL_BRANCH}"
git branch -f upstream-yelnoo-stable "yelnoo/${YELNOO_BRANCH}"
git branch -f upstream-mihomo-splithttp "mihomo/${MIHOMO_BRANCH}"

git checkout "${xhttp_branch}"

if ! git merge-base --is-ancestor "official/${OFFICIAL_BRANCH}" HEAD; then
  git merge "official/${OFFICIAL_BRANCH}" --no-edit
fi

OFFICIAL_LAST_SEEN_COMMIT="${official_head}"
YELNOO_LAST_SEEN_COMMIT="${yelnoo_head}"
MIHOMO_LAST_SEEN_COMMIT="${mihomo_head}"

cat > "${tracking_file}" <<EOF
# official testing is auto-merged into xhttp by automation.
# yelnoo and mihomo refs are mirrored and recorded for manual port review.

OFFICIAL_REPO=${OFFICIAL_REPO}
OFFICIAL_BRANCH=${OFFICIAL_BRANCH}
OFFICIAL_LAST_SEEN_COMMIT=${OFFICIAL_LAST_SEEN_COMMIT}

YELNOO_REPO=${YELNOO_REPO}
YELNOO_BRANCH=${YELNOO_BRANCH}
YELNOO_LAST_SEEN_COMMIT=${YELNOO_LAST_SEEN_COMMIT}

MIHOMO_REPO=${MIHOMO_REPO}
MIHOMO_BRANCH=${MIHOMO_BRANCH}
MIHOMO_LAST_SEEN_COMMIT=${MIHOMO_LAST_SEEN_COMMIT}
EOF

git add "${tracking_file}"
if ! git diff --cached --quiet -- "${tracking_file}"; then
  git commit -m "sync: refresh upstream source refs"
fi

current_head="$(git rev-parse HEAD)"
branch_changed=false
if [[ "${start_head}" != "${current_head}" ]]; then
  branch_changed=true
fi

changed_sources=()
if [[ -n "${previous_official}" && "${previous_official}" != "${official_head}" ]]; then
  changed_sources+=("official")
fi
if [[ -n "${previous_yelnoo}" && "${previous_yelnoo}" != "${yelnoo_head}" ]]; then
  changed_sources+=("yelnoo")
fi
if [[ -n "${previous_mihomo}" && "${previous_mihomo}" != "${mihomo_head}" ]]; then
  changed_sources+=("mihomo")
fi

changed_csv=""
if (( ${#changed_sources[@]} > 0 )); then
  changed_csv="$(IFS=,; echo "${changed_sources[*]}")"
fi

donor_changed=false
if [[ "${changed_csv}" == *"yelnoo"* || "${changed_csv}" == *"mihomo"* ]]; then
  donor_changed=true
fi

if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
  {
    echo "branch_changed=${branch_changed}"
    echo "changed_sources=${changed_csv}"
    echo "donor_changed=${donor_changed}"
    echo "official_head=${official_head}"
    echo "yelnoo_head=${yelnoo_head}"
    echo "mihomo_head=${mihomo_head}"
  } >> "${GITHUB_OUTPUT}"
fi

if [[ -n "${GITHUB_STEP_SUMMARY:-}" ]]; then
  {
    echo "### Upstream Heads"
    echo
    echo "- official/testing: \`${official_head}\`"
    echo "- yelnoo/stable: \`${yelnoo_head}\`"
    echo "- mihomo/feat/splithttp: \`${mihomo_head}\`"
    echo
    if [[ -n "${changed_csv}" ]]; then
      echo "### Source Changes"
      echo
      echo "- changed since last recorded run: \`${changed_csv}\`"
      echo
    fi
    if [[ "${donor_changed}" == "true" ]]; then
      echo "### Manual Follow-Up"
      echo
      echo "- donor mirrors moved; review yelnoo/mihomo changes before claiming the xhttp branch is fully caught up."
      echo
    fi
  } >> "${GITHUB_STEP_SUMMARY}"
fi
