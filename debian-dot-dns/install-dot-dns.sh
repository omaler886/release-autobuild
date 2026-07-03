#!/usr/bin/env bash
set -Eeuo pipefail
IFS=$'\n\t'

# This script uses only the DNS stack shipped with a normal Debian VPS:
# systemd-resolved provides the local stub resolver and forwards upstream
# queries with DNS-over-TLS.  No dnsproxy/cloudflared/mosdns binary is needed.

PROFILE_NAME="dot-dns"
BACKUP_DIR="/var/backups/${PROFILE_NAME}"
RESOLVED_DROPIN_DIR="/etc/systemd/resolved.conf.d"
RESOLVED_DROPIN_PATH="${RESOLVED_DROPIN_DIR}/zz-${PROFILE_NAME}.conf"
RESOLV_CONF="/etc/resolv.conf"
RESOLVED_STUB="/run/systemd/resolve/stub-resolv.conf"

DOT_TLS_MODE="${DOT_TLS_MODE:-yes}"
DOT_DNSSEC="${DOT_DNSSEC:-no}"
DOT_ENABLE_IPV6="${DOT_ENABLE_IPV6:-auto}"
DOT_USE_STUB_SYMLINK="${DOT_USE_STUB_SYMLINK:-1}"
DOT_DISABLE_DOH_DNS="${DOT_DISABLE_DOH_DNS:-1}"
VERIFY_NAME="${DOT_VERIFY_NAME:-example.com}"
BACKUP_RUN_DIR=""
INSTALL_ROLLBACK_NEEDED=0

IPV4_UPSTREAMS=(
  "1.1.1.1#cloudflare-dns.com"
  "1.0.0.1#cloudflare-dns.com"
  "8.8.8.8#dns.google"
  "8.8.4.4#dns.google"
)

IPV6_UPSTREAMS=(
  "2606:4700:4700::1111#cloudflare-dns.com"
  "2606:4700:4700::1001#cloudflare-dns.com"
  "2001:4860:4860::8888#dns.google"
  "2001:4860:4860::8844#dns.google"
)

log() {
  printf '[%s] %s\n' "$PROFILE_NAME" "$*"
}

warn() {
  printf '[%s] WARNING: %s\n' "$PROFILE_NAME" "$*" >&2
}

die() {
  printf '[%s] ERROR: %s\n' "$PROFILE_NAME" "$*" >&2
  exit 1
}

usage() {
  cat <<'EOF'
Usage:
  sudo bash install-dot-dns.sh install
  sudo bash install-dot-dns.sh verify
  sudo bash install-dot-dns.sh status
  sudo bash install-dot-dns.sh uninstall

Environment overrides:
  DOT_TLS_MODE=yes                 systemd-resolved DNSOverTLS mode: yes or opportunistic.
  DOT_DNSSEC=no                    systemd-resolved DNSSEC mode: no, allow-downgrade, or yes.
  DOT_ENABLE_IPV6=auto             Include IPv6 upstreams: auto, 1, or 0.
  DOT_USE_STUB_SYMLINK=1           Make /etc/resolv.conf point at resolved's stub file.
  DOT_DISABLE_DOH_DNS=1            Stop/disable the earlier doh-dns.service during install.
  DOT_VERIFY_NAME=example.com      Name used by verify.
EOF
}

require_root() {
  if [ "$(id -u)" -ne 0 ]; then
    die "run this script as root"
  fi
}

require_systemd() {
  command -v systemctl >/dev/null 2>&1 || die "systemctl is required"
  [ -d /run/systemd/system ] || die "systemd is not running as PID 1"
}

require_systemd_resolved() {
  systemctl cat systemd-resolved.service >/dev/null 2>&1 ||
    die "systemd-resolved.service was not found on this VPS"
  command -v resolvectl >/dev/null 2>&1 ||
    die "resolvectl was not found; install/use a VPS image with systemd-resolved"
}

check_debian_family() {
  if [ -r /etc/os-release ]; then
    # shellcheck disable=SC1091
    . /etc/os-release
    case " ${ID:-} ${ID_LIKE:-} " in
      *debian*|*ubuntu*) ;;
      *) warn "designed for Debian-family VPS images; detected ID=${ID:-unknown}" ;;
    esac
  fi
}

validate_config() {
  case "$DOT_TLS_MODE" in
    yes|opportunistic) ;;
    *) die "invalid DOT_TLS_MODE=${DOT_TLS_MODE}; use yes or opportunistic" ;;
  esac

  case "$DOT_DNSSEC" in
    no|allow-downgrade|yes) ;;
    *) die "invalid DOT_DNSSEC=${DOT_DNSSEC}; use no, allow-downgrade, or yes" ;;
  esac

  case "$DOT_ENABLE_IPV6" in
    auto|1|true|yes|0|false|no) ;;
    *) die "invalid DOT_ENABLE_IPV6=${DOT_ENABLE_IPV6}; use auto, 1, or 0" ;;
  esac

  case "$DOT_USE_STUB_SYMLINK" in
    1|true|yes|0|false|no) ;;
    *) die "invalid DOT_USE_STUB_SYMLINK=${DOT_USE_STUB_SYMLINK}; use 1 or 0" ;;
  esac

  case "$DOT_DISABLE_DOH_DNS" in
    1|true|yes|0|false|no) ;;
    *) die "invalid DOT_DISABLE_DOH_DNS=${DOT_DISABLE_DOH_DNS}; use 1 or 0" ;;
  esac
}

init_backup_run() {
  local stamp

  stamp="$(date -u +%Y%m%dT%H%M%SZ)"
  install -d -m 0755 "$BACKUP_DIR"
  BACKUP_RUN_DIR="$(mktemp -d "${BACKUP_DIR}/${stamp}.XXXXXX")"
  chmod 0755 "$BACKUP_RUN_DIR"
}

set_latest_backup() {
  ln -sfn "$BACKUP_RUN_DIR" "${BACKUP_DIR}/latest"
}

backup_file() {
  local path="$1"
  local dest

  [ -n "$BACKUP_RUN_DIR" ] || die "backup run was not initialized"
  if [ -e "$path" ] || [ -L "$path" ]; then
    dest="${BACKUP_RUN_DIR}${path}"
    install -d -m 0755 "$(dirname "$dest")"
    cp -a "$path" "$dest"
    log "backed up ${path} to ${dest}"
  fi
}

resolved_dropin_managed_by_us() {
  [ -f "$RESOLVED_DROPIN_PATH" ] &&
    grep -q "^# Managed by ${PROFILE_NAME}\." "$RESOLVED_DROPIN_PATH"
}

resolv_conf_static_managed_by_us() {
  [ -f "$RESOLV_CONF" ] &&
    grep -q "^# Managed by ${PROFILE_NAME}\." "$RESOLV_CONF"
}

resolv_conf_symlinks_to_stub() {
  [ -L "$RESOLV_CONF" ] &&
    [ "$(readlink -f "$RESOLV_CONF" 2>/dev/null || true)" = "$RESOLVED_STUB" ]
}

resolv_conf_managed_by_us() {
  resolv_conf_static_managed_by_us ||
    { resolved_dropin_managed_by_us && resolv_conf_symlinks_to_stub; }
}

resolv_conf_is_immutable() {
  command -v lsattr >/dev/null 2>&1 &&
    [ -e "$RESOLV_CONF" ] &&
    lsattr "$RESOLV_CONF" 2>/dev/null | awk '{ print $1 }' | grep -q 'i'
}

clear_resolv_conf_immutable_bit() {
  if resolv_conf_is_immutable; then
    command -v chattr >/dev/null 2>&1 ||
      die "${RESOLV_CONF} is immutable and chattr is unavailable"
    log "removing immutable bit from ${RESOLV_CONF}"
    chattr -i "$RESOLV_CONF"
  fi
}

backup_resolv_conf_if_needed() {
  # Preserve the first rollback point.  Re-running the installer after this
  # profile is already active must not replace the original resolv.conf backup.
  if resolv_conf_managed_by_us; then
    log "${RESOLV_CONF} is already managed by ${PROFILE_NAME}; preserving rollback backup"
    return
  fi

  backup_file "$RESOLV_CONF"
  set_latest_backup
}

backup_dropin_if_needed() {
  # Keep the previous drop-in so a failed reinstall can restore the last known
  # resolver profile instead of leaving the host without a managed drop-in.
  if [ -e "$RESOLVED_DROPIN_PATH" ] || [ -L "$RESOLVED_DROPIN_PATH" ]; then
    backup_file "$RESOLVED_DROPIN_PATH"
  fi
}

ipv6_upstreams_enabled() {
  case "$DOT_ENABLE_IPV6" in
    1|true|yes) return 0 ;;
    0|false|no) return 1 ;;
    auto)
      command -v ip >/dev/null 2>&1 &&
        ip -6 route show default 2>/dev/null | grep -q .
      ;;
  esac
}

dns_server_line() {
  local servers=("${IPV4_UPSTREAMS[@]}")
  local IFS=' '

  if ipv6_upstreams_enabled; then
    servers+=("${IPV6_UPSTREAMS[@]}")
  fi

  printf '%s' "${servers[*]}"
}

ensure_systemd_resolved_running() {
  systemctl enable systemd-resolved >/dev/null 2>&1 || true
  systemctl start systemd-resolved || die "failed to start systemd-resolved"
}

write_resolved_dropin() {
  local tmp

  backup_dropin_if_needed
  install -d -m 0755 "$RESOLVED_DROPIN_DIR"
  tmp="$(mktemp)"

  {
    printf '# Managed by %s. VPS-native DNS-over-TLS resolver profile.\n' "$PROFILE_NAME"
    printf '[Resolve]\n'
    # Empty assignments reset values inherited from vendor files or older drop-ins.
    printf 'DNS=\n'
    printf 'DNS=%s\n' "$(dns_server_line)"
    printf 'FallbackDNS=\n'
    printf 'Domains=~.\n'
    printf 'DNSOverTLS=%s\n' "$DOT_TLS_MODE"
    printf 'DNSSEC=%s\n' "$DOT_DNSSEC"
    printf 'DNSStubListener=yes\n'
    printf 'Cache=yes\n'
    printf 'ReadEtcHosts=yes\n'
  } >"$tmp"

  chmod 0644 "$tmp"
  mv "$tmp" "$RESOLVED_DROPIN_PATH"
  log "wrote ${RESOLVED_DROPIN_PATH}"
}

restart_systemd_resolved() {
  systemctl restart systemd-resolved ||
    die "failed to restart systemd-resolved"
}

rollback_install_changes() {
  local reason="$1"

  # Keep rollback best-effort so the original failure remains visible.
  set +e
  warn "${reason}; rolling back ${PROFILE_NAME} changes"
  restore_resolved_dropin_from_run_backup
  systemctl restart systemd-resolved >/dev/null 2>&1 ||
    warn "failed to restart systemd-resolved during rollback"
  restore_resolv_conf_from_run_backup
  set -e
}

rollback_install_on_exit() {
  local exit_code="$1"

  if [ "$INSTALL_ROLLBACK_NEEDED" != "1" ] || [ "$exit_code" -eq 0 ]; then
    return
  fi

  rollback_install_changes "install failed before completion"
}

write_resolv_conf() {
  local tmp

  backup_resolv_conf_if_needed
  clear_resolv_conf_immutable_bit

  case "$DOT_USE_STUB_SYMLINK" in
    1|true|yes)
      [ -e "$RESOLVED_STUB" ] ||
        die "${RESOLVED_STUB} does not exist; systemd-resolved did not create its stub file"
      rm -f "$RESOLV_CONF"
      ln -s "$RESOLVED_STUB" "$RESOLV_CONF"
      log "pointed ${RESOLV_CONF} at ${RESOLVED_STUB}"
      ;;
    0|false|no)
      # Some minimal images do not like /etc/resolv.conf as a symlink.  The
      # static variant still keeps all clients on systemd-resolved's local stub.
      tmp="$(mktemp)"
      {
        printf '# Managed by %s. System DNS uses systemd-resolved DNS-over-TLS.\n' "$PROFILE_NAME"
        printf 'nameserver 127.0.0.53\n'
        printf 'options edns0 trust-ad timeout:2 attempts:3\n'
        printf 'search .\n'
      } >"$tmp"
      chmod 0644 "$tmp"
      rm -f "$RESOLV_CONF"
      mv "$tmp" "$RESOLV_CONF"
      log "wrote static ${RESOLV_CONF} for systemd-resolved"
      ;;
  esac
}

doh_dns_unit_exists() {
  systemctl cat doh-dns.service >/dev/null 2>&1
}

record_doh_dns_state() {
  local state_dir

  [ -n "$BACKUP_RUN_DIR" ] || die "backup run was not initialized"
  state_dir="${BACKUP_RUN_DIR}/state"
  install -d -m 0755 "$state_dir"

  if systemctl is-enabled --quiet doh-dns.service; then
    printf 'enabled\n' >"${state_dir}/doh-dns.enabled"
  else
    printf 'disabled\n' >"${state_dir}/doh-dns.enabled"
  fi

  if systemctl is-active --quiet doh-dns.service; then
    printf 'active\n' >"${state_dir}/doh-dns.active"
  else
    printf 'inactive\n' >"${state_dir}/doh-dns.active"
  fi
}

disable_previous_doh_dns_if_requested() {
  case "$DOT_DISABLE_DOH_DNS" in
    0|false|no) return ;;
  esac

  doh_dns_unit_exists || return

  # This profile does not need the earlier dnsproxy-based service.  Record its
  # state first so uninstall can restore it when this script was used to switch
  # from the DoH profile to the native DoT profile.
  record_doh_dns_state
  systemctl disable --now doh-dns.service >/dev/null 2>&1 ||
    warn "failed to disable doh-dns.service; continuing because systemd-resolved is already configured"
  log "disabled doh-dns.service"
}

latest_backup_dir() {
  local latest backup

  [ -d "$BACKUP_DIR" ] || return 0

  latest="$(readlink -f "${BACKUP_DIR}/latest" 2>/dev/null || true)"
  backup="${latest}${RESOLV_CONF}"
  if [ -n "$latest" ] && [ -d "$latest" ] && { [ -e "$backup" ] || [ -L "$backup" ]; }; then
    printf '%s' "$latest"
    return
  fi

  latest="$(
    find "$BACKUP_DIR" -mindepth 1 -maxdepth 1 -type d 2>/dev/null |
      sort |
      while IFS= read -r candidate; do
        backup="${candidate}${RESOLV_CONF}"
        if [ -e "$backup" ] || [ -L "$backup" ]; then
          printf '%s\n' "$candidate"
        fi
      done |
      tail -n 1
  )"

  printf '%s' "$latest"
}

latest_doh_dns_state_dir() {
  local marker

  [ -d "$BACKUP_DIR" ] || return 0

  marker="$(
    find "$BACKUP_DIR" -mindepth 3 -maxdepth 3 -type f -path '*/state/doh-dns.enabled' 2>/dev/null |
      sort |
      tail -n 1
  )"

  if [ -n "$marker" ]; then
    dirname "$marker"
  fi
}

restore_resolv_conf() {
  local latest backup

  latest="$(latest_backup_dir)"
  backup="${latest}${RESOLV_CONF}"

  clear_resolv_conf_immutable_bit
  if [ -n "$latest" ] && { [ -e "$backup" ] || [ -L "$backup" ]; }; then
    rm -f "$RESOLV_CONF"
    cp -a "$backup" "$RESOLV_CONF"
    log "restored ${RESOLV_CONF} from ${backup}"
  elif [ -e "$RESOLVED_STUB" ]; then
    rm -f "$RESOLV_CONF"
    ln -s "$RESOLVED_STUB" "$RESOLV_CONF"
    warn "no backup found; kept ${RESOLV_CONF} on systemd-resolved stub"
  else
    warn "no backup or resolved stub found; writing temporary public resolver fallback"
    cat >"$RESOLV_CONF" <<'EOF'
nameserver 1.1.1.1
nameserver 8.8.8.8
options edns0 timeout:2 attempts:3
EOF
  fi
}

remove_resolved_dropin() {
  if [ ! -e "$RESOLVED_DROPIN_PATH" ]; then
    return
  fi

  if resolved_dropin_managed_by_us; then
    rm -f "$RESOLVED_DROPIN_PATH"
    log "removed ${RESOLVED_DROPIN_PATH}"
  else
    warn "${RESOLVED_DROPIN_PATH} is not managed by ${PROFILE_NAME}; leaving it in place"
  fi
}

restore_resolved_dropin_from_run_backup() {
  local backup

  backup="${BACKUP_RUN_DIR}${RESOLVED_DROPIN_PATH}"
  if [ -n "$BACKUP_RUN_DIR" ] && { [ -e "$backup" ] || [ -L "$backup" ]; }; then
    install -d -m 0755 "$RESOLVED_DROPIN_DIR"
    rm -f "$RESOLVED_DROPIN_PATH"
    cp -a "$backup" "$RESOLVED_DROPIN_PATH"
    log "restored ${RESOLVED_DROPIN_PATH} from ${backup}"
    return
  fi

  remove_resolved_dropin
}

restore_resolv_conf_from_run_backup() {
  local backup

  backup="${BACKUP_RUN_DIR}${RESOLV_CONF}"
  if [ -z "$BACKUP_RUN_DIR" ] || { [ ! -e "$backup" ] && [ ! -L "$backup" ]; }; then
    log "no ${RESOLV_CONF} backup was created in this run; leaving it unchanged"
    return
  fi

  clear_resolv_conf_immutable_bit
  rm -f "$RESOLV_CONF"
  cp -a "$backup" "$RESOLV_CONF"
  log "restored ${RESOLV_CONF} from ${backup}"
}

restore_doh_dns_state_if_needed() {
  local state_dir

  state_dir="$(latest_doh_dns_state_dir)"
  [ -d "$state_dir" ] || return
  doh_dns_unit_exists || return

  if [ "$(cat "${state_dir}/doh-dns.enabled" 2>/dev/null || true)" = "enabled" ]; then
    systemctl enable doh-dns.service >/dev/null 2>&1 || warn "failed to re-enable doh-dns.service"
  fi

  if [ "$(cat "${state_dir}/doh-dns.active" 2>/dev/null || true)" = "active" ]; then
    systemctl start doh-dns.service >/dev/null 2>&1 || warn "failed to restart doh-dns.service"
  fi
}

resolv_conf_points_to_resolved() {
  resolv_conf_symlinks_to_stub ||
    grep -Eq '^[[:space:]]*nameserver[[:space:]]+127\.0\.0\.53([[:space:]]|$)' "$RESOLV_CONF"
}

resolved_uses_dot() {
  resolvectl status 2>/dev/null |
    awk '/Protocols:/ && /[+]DNSOverTLS/ { found = 1 } END { exit found ? 0 : 1 }'
}

resolved_has_expected_upstream() {
  resolvectl dns 2>/dev/null |
    grep -Eq '1\.1\.1\.1#cloudflare-dns\.com|8\.8\.8\.8#dns\.google'
}

verify_install() {
  local failed=0
  local query_output

  if systemctl is-active --quiet systemd-resolved; then
    log "systemd-resolved is active"
  else
    warn "systemd-resolved is not active"
    failed=1
  fi

  if [ -f "$RESOLVED_DROPIN_PATH" ] && resolved_dropin_managed_by_us; then
    log "${RESOLVED_DROPIN_PATH} is installed"
  else
    warn "${RESOLVED_DROPIN_PATH} is missing or unmanaged"
    failed=1
  fi

  if resolv_conf_points_to_resolved; then
    log "${RESOLV_CONF} points to systemd-resolved"
  else
    warn "${RESOLV_CONF} does not point to systemd-resolved"
    failed=1
  fi

  if resolved_uses_dot; then
    log "systemd-resolved reports DNS-over-TLS enabled"
  else
    warn "systemd-resolved does not report DNS-over-TLS enabled"
    failed=1
  fi

  if resolved_has_expected_upstream; then
    log "systemd-resolved has Cloudflare/Google DoT upstreams"
  else
    warn "systemd-resolved does not show the expected DoT upstreams"
    failed=1
  fi

  query_output="$(mktemp)"
  if resolvectl query "$VERIFY_NAME" >"$query_output" 2>&1; then
    log "resolvectl query succeeded: $(tr '\n' ' ' <"$query_output")"
  else
    warn "resolvectl query failed"
    sed -n '1,20p' "$query_output" >&2 || true
    failed=1
  fi
  rm -f "$query_output"

  if getent hosts "$VERIFY_NAME" >/dev/null 2>&1; then
    log "system resolver query succeeded"
  else
    warn "system resolver query failed"
    failed=1
  fi

  return "$failed"
}

install_all() {
  require_root
  require_systemd
  require_systemd_resolved
  check_debian_family
  validate_config
  init_backup_run
  ensure_systemd_resolved_running
  INSTALL_ROLLBACK_NEEDED=1
  trap 'rollback_install_on_exit $?' EXIT
  write_resolved_dropin
  restart_systemd_resolved
  write_resolv_conf
  # Keep the previous resolver service alive until the native DoT path proves
  # itself.  If verification fails, restore the pre-install resolver state.
  if ! verify_install; then
    rollback_install_changes "verification failed"
    INSTALL_ROLLBACK_NEEDED=0
    trap - EXIT
    return 1
  fi
  disable_previous_doh_dns_if_requested
  INSTALL_ROLLBACK_NEEDED=0
  trap - EXIT
}

uninstall_all() {
  require_root
  require_systemd
  require_systemd_resolved
  remove_resolved_dropin
  restart_systemd_resolved
  restore_resolv_conf
  restore_doh_dns_state_if_needed
  log "uninstalled ${PROFILE_NAME}"
}

status_all() {
  systemctl status --no-pager -l systemd-resolved || true

  printf '\n%s:\n' "$RESOLV_CONF"
  ls -l "$RESOLV_CONF" || true
  sed -n '1,20p' "$RESOLV_CONF" || true

  if [ -f "$RESOLVED_DROPIN_PATH" ]; then
    printf '\n%s:\n' "$RESOLVED_DROPIN_PATH"
    sed -n '1,80p' "$RESOLVED_DROPIN_PATH" || true
  fi

  printf '\nresolvectl dns:\n'
  resolvectl dns || true

  printf '\nresolvectl status:\n'
  resolvectl status || true

  if doh_dns_unit_exists; then
    printf '\ndoh-dns.service:\n'
    systemctl is-enabled doh-dns.service || true
    systemctl is-active doh-dns.service || true
  fi
}

main() {
  local action="${1:-install}"

  case "$action" in
    install) install_all ;;
    verify) verify_install ;;
    status) status_all ;;
    uninstall|remove|rollback) uninstall_all ;;
    -h|--help|help) usage ;;
    *) usage; die "unknown action: ${action}" ;;
  esac
}

main "$@"
