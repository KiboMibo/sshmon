#!/bin/sh
# install.sh — установка sshmon (Linux/macOS, amd64/arm64) из GitHub Releases.
#
#   curl -fsSL https://raw.githubusercontent.com/idesyatov/sshmon/main/install.sh | sh
#   curl -fsSL .../install.sh | BINDIR="$HOME/.local/bin" sh   # своя папка, без sudo
#   curl -fsSL .../install.sh | VERSION=v0.5.0 sh              # конкретная версия
#
# Скрипт определяет ОС/архитектуру, берёт нужный релиз, сверяет SHA-256 и ставит
# бинарник в BINDIR (по умолчанию /usr/local/bin). sudo используется только если
# в целевую папку нет прав на запись.
set -eu

REPO="idesyatov/sshmon"
BINARY="sshmon"
BINDIR="${BINDIR:-/usr/local/bin}"
VERSION="${VERSION:-}"

info() { printf '\033[36m>>\033[0m %s\n' "$*"; }
err()  { printf '\033[31merror:\033[0m %s\n' "$*" >&2; exit 1; }

need() {
	command -v "$1" >/dev/null 2>&1 || err "нужна утилита '$1' — установите её пакетным менеджером (apt/dnf/apk/brew/...)"
}
need curl
need tar

# curl с запасным вариантом по IPv4 (некоторые сети без рабочего IPv6).
fetch_out() { curl -fsSL -o "$2" "$1" || curl -fsSL --ipv4 -o "$2" "$1"; }

# SHA-256: Linux — sha256sum, macOS — shasum -a 256.
sha256() {
	if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}'
	elif command -v shasum >/dev/null 2>&1; then shasum -a 256 "$1" | awk '{print $1}'
	else err "нет sha256sum/shasum для проверки контрольной суммы"; fi
}

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$os" in
	linux)  os=linux ;;
	darwin) os=darwin ;;
	*) err "неподдерживаемая ОС: $os (нужна Linux или macOS)" ;;
esac
arch="$(uname -m)"
case "$arch" in
	x86_64|amd64)   arch=amd64 ;;
	aarch64|arm64)  arch=arm64 ;;
	*) err "неподдерживаемая архитектура: $arch (нужна amd64 или arm64)" ;;
esac

if [ -z "$VERSION" ]; then
	info "определяю последний релиз $BINARY..."
	# /releases/latest редиректит на /releases/tag/<version> — вытаскиваем тег из URL.
	# Если релизов нет, GitHub ведёт на список /releases (без /tag/) — сообщаем явно.
	url="$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$REPO/releases/latest")"
	case "$url" in
		*/tag/*) VERSION="${url##*/tag/}" ;;
		*) err "не удалось определить последнюю версию (нет опубликованных релизов?) — задайте VERSION вручную" ;;
	esac
fi
info "версия: $VERSION · платформа: ${os}_${arch}"

asset="${BINARY}_${VERSION}_${os}_${arch}.tar.gz"
base="https://github.com/$REPO/releases/download/$VERSION"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

info "скачиваю $asset..."
fetch_out "$base/$asset" "$tmp/$asset" || err "не удалось скачать $asset — нет такой сборки для $VERSION/${os}_${arch}?"

info "проверяю SHA-256..."
fetch_out "$base/checksums.txt" "$tmp/checksums.txt" || err "в релизе $VERSION нет checksums.txt — возьмите релиз с контрольными суммами (VERSION=...)"
want="$(awk -v f="$asset" '$2==f || $2=="*"f {print $1}' "$tmp/checksums.txt" | head -n1)"
[ -n "$want" ] || err "в checksums.txt нет строки для $asset"
got="$(sha256 "$tmp/$asset")"
[ "$want" = "$got" ] || err "контрольная сумма не совпала (ожидалось $want, получено $got)"

info "распаковываю..."
tar xzf "$tmp/$asset" -C "$tmp"
[ -f "$tmp/$BINARY" ] || err "в архиве нет $BINARY"
chmod +x "$tmp/$BINARY"

dest="$BINDIR/$BINARY"
dir="$(dirname "$dest")"
if [ -w "$dir" ] || { [ ! -e "$dir" ] && [ -w "$(dirname "$dir")" ]; }; then
	mkdir -p "$dir"
	mv "$tmp/$BINARY" "$dest"
else
	info "нет прав на запись в $dir — использую sudo"
	command -v sudo >/dev/null 2>&1 || err "нужен sudo или задайте BINDIR в доступную папку (например BINDIR=\$HOME/.local/bin)"
	sudo mkdir -p "$dir"
	sudo mv "$tmp/$BINARY" "$dest"
fi

info "готово: $dest"
"$dest" --version 2>/dev/null || info "добавьте $dir в PATH, если ещё не добавлен"
