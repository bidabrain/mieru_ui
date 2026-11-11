#!/usr/bin/env bash
# mieru-ui installer
#
#   bash install.sh             install / upgrade (mita + panel from GitHub releases)
#   bash install.sh local       install the panel from ./mieru-ui or ./dist/mieru-ui-linux-<arch>
#   bash install.sh uninstall   remove the panel (keeps mita and /etc/mieru-ui)
#   bash install.sh purge [-y]  remove the panel, its data (/etc/mieru-ui) and mita completely
#
# Environment:
#   MIERU_UI_REPO   GitHub repo that publishes mieru-ui releases (default below)
#   MIERU_UI_PORT   panel port for a fresh install (default random 10000-60000)
#   MITA_VERSION    mieru server version to install (default latest)

set -euo pipefail

MIERU_UI_REPO="${MIERU_UI_REPO:-bidabrain/mieru_ui}"
INSTALL_DIR=/usr/local/mieru-ui
DATA_DIR=/etc/mieru-ui
SERVICE=/etc/systemd/system/mieru-ui.service

red()   { printf '\033[31m%s\033[0m\n' "$*"; }
green() { printf '\033[32m%s\033[0m\n' "$*"; }
yellow(){ printf '\033[33m%s\033[0m\n' "$*"; }

[[ $EUID -eq 0 ]] || { red "请使用 root 运行"; exit 1; }
command -v systemctl >/dev/null || { red "需要 systemd"; exit 1; }

case "$(uname -m)" in
  x86_64|amd64) ARCH=amd64; RPM_ARCH=x86_64 ;;
  aarch64|arm64) ARCH=arm64; RPM_ARCH=aarch64 ;;
  *) red "不支持的架构: $(uname -m)"; exit 1 ;;
esac

need() {
  for c in "$@"; do
    command -v "$c" >/dev/null && continue
    yellow "安装依赖 $c"
    if command -v apt-get >/dev/null; then apt-get update -y && apt-get install -y "$c"
    elif command -v dnf >/dev/null; then dnf install -y "$c"
    elif command -v yum >/dev/null; then yum install -y "$c"
    else red "请手动安装 $c"; exit 1; fi
  done
}

rand_str() { tr -dc 'a-zA-Z0-9' </dev/urandom | head -c "$1" || true; }

latest_tag() { # owner/repo
  # Read the whole response first: piping into `grep -m1` makes grep exit early, curl then
  # fails writing the rest (error 23), and pipefail aborts the script.
  local json
  json="$(curl -fsSL "https://api.github.com/repos/$1/releases/latest")" || return 1
  if [[ $json =~ \"tag_name\":[[:space:]]*\"([^\"]+)\" ]]; then
    printf '%s\n' "${BASH_REMATCH[1]}"
  fi
}

install_mita() {
  if command -v mita >/dev/null; then
    green "已安装 mita: $(mita version 2>/dev/null || echo unknown)"
    return
  fi
  local tag="${MITA_VERSION:-$(latest_tag enfein/mieru)}"
  [[ -n "$tag" ]] || { red "无法获取 mieru 最新版本号，可设置 MITA_VERSION=v3.x.x 重试"; exit 1; }
  local ver="${tag#v}" tmp
  tmp="$(mktemp -d)"
  yellow "安装 mieru 服务端 mita ${tag}"
  if command -v dpkg >/dev/null; then
    curl -fL -o "$tmp/mita.deb" "https://github.com/enfein/mieru/releases/download/${tag}/mita_${ver}_${ARCH}.deb"
    dpkg -i "$tmp/mita.deb"
  elif command -v rpm >/dev/null; then
    curl -fL -o "$tmp/mita.rpm" "https://github.com/enfein/mieru/releases/download/${tag}/mita-${ver}-1.${RPM_ARCH}.rpm"
    rpm -Uvh --force "$tmp/mita.rpm"
  else
    red "未检测到 dpkg/rpm，请参考 https://github.com/enfein/mieru 手动安装 mita"; exit 1
  fi
  rm -rf "$tmp"
  systemctl enable --now mita >/dev/null 2>&1 || true
  sleep 2
  green "mita 安装完成: $(mita version 2>/dev/null || true)"
}

install_panel_binary() {
  local src="${1:-}"
  mkdir -p "$INSTALL_DIR" "$DATA_DIR"
  chmod 700 "$DATA_DIR"
  if [[ -n "$src" ]]; then
    install -m 755 "$src" "$INSTALL_DIR/mieru-ui"
  else
    local tag tmp
    tag="$(latest_tag "$MIERU_UI_REPO")"
    [[ -n "$tag" ]] || { red "无法获取 ${MIERU_UI_REPO} 的发布版本，可用 'bash install.sh local' 安装本地编译的二进制"; exit 1; }
    tmp="$(mktemp -d)"
    yellow "下载 mieru-ui ${tag}"
    local base="https://github.com/${MIERU_UI_REPO}/releases/download/${tag}"
    local name="mieru-ui-linux-${ARCH}.tar.gz"
    curl -fL -o "$tmp/$name" "$base/$name"
    if curl -fsSL -o "$tmp/SHA256SUMS" "$base/SHA256SUMS"; then
      if ! (cd "$tmp" && grep " ${name}\$" SHA256SUMS | sha256sum -c --status -); then
        rm -rf "$tmp"
        red "SHA256 校验失败，下载的文件可能被篡改或损坏，已中止安装"; exit 1
      fi
      green "SHA256 校验通过"
    else
      yellow "该版本没有 SHA256SUMS，跳过校验"
    fi
    tar -xzf "$tmp/$name" -C "$tmp"
    install -m 755 "$tmp/mieru-ui" "$INSTALL_DIR/mieru-ui"
    rm -rf "$tmp"
  fi
  ln -sf "$INSTALL_DIR/mieru-ui" /usr/bin/mieru-ui
}

write_service() {
  cat >"$SERVICE" <<'EOF'
[Unit]
Description=mieru-ui web panel
After=network-online.target mita.service
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=/usr/local/mieru-ui
ExecStart=/usr/local/mieru-ui/mieru-ui run
Restart=always
RestartSec=3
User=root
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
}

first_setup() {
  if [[ -f "$DATA_DIR/mieru-ui.json" ]]; then
    green "检测到已有配置，保留现有账号与设置"
    return
  fi
  local user pass port path
  user="$(rand_str 8)"; pass="$(rand_str 16)"; path="/$(rand_str 12)/"
  port="${MIERU_UI_PORT:-$(( (RANDOM % 50000) + 10000 ))}"
  MIERU_UI_DATA="$DATA_DIR" "$INSTALL_DIR/mieru-ui" setting -username "$user" -password "$pass" -port "$port" -basepath "$path" >/dev/null
  CRED_USER="$user"; CRED_PASS="$pass"; CRED_PORT="$port"; CRED_PATH="$path"
}

open_firewall() {
  local port="$1"
  if command -v ufw >/dev/null && ufw status | grep -q active; then ufw allow "$port"/tcp >/dev/null; fi
  if command -v firewall-cmd >/dev/null && firewall-cmd --state >/dev/null 2>&1; then
    firewall-cmd --permanent --add-port="$port"/tcp >/dev/null && firewall-cmd --reload >/dev/null
  fi
}

do_install() {
  need curl tar
  install_mita
  local src=""
  if [[ "${1:-}" == "local" ]]; then
    for f in ./mieru-ui "./dist/mieru-ui-linux-${ARCH}"; do [[ -f "$f" ]] && src="$f" && break; done
    [[ -n "$src" ]] || { red "未找到本地二进制 ./mieru-ui 或 ./dist/mieru-ui-linux-${ARCH}"; exit 1; }
  fi
  systemctl stop mieru-ui >/dev/null 2>&1 || true
  install_panel_binary "$src"
  write_service
  CRED_USER=""
  first_setup
  systemctl enable --now mieru-ui >/dev/null
  systemctl restart mieru-ui

  local ip
  ip="$(curl -fsS4 --max-time 5 https://api.ipify.org || hostname -I | awk '{print $1}')"
  echo
  green "========== mieru-ui 安装完成 =========="
  if [[ -n "$CRED_USER" ]]; then
    open_firewall "$CRED_PORT"
    echo "面板地址: http://${ip}:${CRED_PORT}${CRED_PATH}"
    echo "用户名:   ${CRED_USER}"
    echo "密码:     ${CRED_PASS}"
    yellow "请妥善保存以上信息。查看设置: mieru-ui setting -show"
  else
    MIERU_UI_DATA="$DATA_DIR" mieru-ui setting -show
  fi
  echo
  yellow "提示: 记得在云服务商安全组中放行面板端口，以及你在面板中为 mieru 配置的 TCP/UDP 端口。"
  echo "常用命令: systemctl {status|restart|stop} mieru-ui ; journalctl -u mieru-ui -f"
}

remove_panel() {
  systemctl disable --now mieru-ui >/dev/null 2>&1 || true
  rm -f "$SERVICE" /usr/bin/mieru-ui
  rm -rf "$INSTALL_DIR"
  systemctl daemon-reload
}

do_uninstall() {
  remove_panel
  green "已卸载 mieru-ui（保留了 $DATA_DIR 和 mita；彻底删除请运行: bash install.sh purge）"
}

do_purge() {
  if [[ "${1:-}" != "-y" ]]; then
    red "警告：将彻底删除 mieru-ui 面板、面板数据（$DATA_DIR，含所有用户）以及 mieru 服务端 mita 及其配置。"
    red "所有用户将立即无法连接，且无法恢复。建议先在面板「面板设置 → 备份」中下载备份。"
    local answer=""
    # Read from the terminal so this also works when the script is piped from curl.
    if [[ -r /dev/tty ]]; then
      read -r -p "确认彻底卸载请输入 yes: " answer </dev/tty
    fi
    [[ "$answer" == "yes" ]] || { yellow "已取消"; exit 0; }
  fi

  yellow "卸载 mieru-ui 面板"
  remove_panel

  if command -v mita >/dev/null; then
    yellow "停止 mita"
    mita stop >/dev/null 2>&1 || true
  fi
  systemctl disable --now mita >/dev/null 2>&1 || true
  if command -v dpkg >/dev/null && dpkg -s mita >/dev/null 2>&1; then
    yellow "卸载 mita (dpkg)"
    if command -v apt-get >/dev/null; then apt-get purge -y mita; else dpkg --purge mita; fi
  elif command -v rpm >/dev/null && rpm -q mita >/dev/null 2>&1; then
    yellow "卸载 mita (rpm)"
    rpm -e mita
  elif command -v mita >/dev/null; then
    yellow "mita 不是通过 deb/rpm 安装的，请手动删除: $(command -v mita)"
  fi
  rm -rf /etc/mita
  rm -rf "$DATA_DIR"
  systemctl daemon-reload

  green "已彻底卸载 mieru-ui 和 mita。"
  yellow "防火墙规则未改动：如需关闭之前放行的端口，请使用 ufw / firewall-cmd 手动删除，并检查云服务商安全组。"
}

case "${1:-install}" in
  install|"") do_install ;;
  local) do_install local ;;
  uninstall) do_uninstall ;;
  purge) do_purge "${2:-}" ;;
  *) echo "用法: bash install.sh [install|local|uninstall|purge [-y]]"; exit 1 ;;
esac
