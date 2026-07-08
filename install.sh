#!/bin/sh
# Instalador do fluigcli para Linux e macOS.
#
# Baixa a última release do GitHub, confere o checksum e instala o binário:
#   curl -fsSL https://raw.githubusercontent.com/alorenco/fluig-cli/main/install.sh | sh
#
# Variáveis opcionais:
#   FLUIGCLI_VERSION      versão a instalar (ex.: 0.1.0); padrão: última release
#   FLUIGCLI_INSTALL_DIR  diretório de destino; padrão: /usr/local/bin
#                         (cai para ~/.local/bin se não houver permissão)
set -eu

REPO="alorenco/fluig-cli"

erro() { printf 'erro: %s\n' "$1" >&2; exit 1; }
info() { printf '%s\n' "$1" >&2; }

command -v curl >/dev/null 2>&1 || erro "o curl é necessário para instalar"

case "$(uname -s)" in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  *) erro "sistema não suportado: $(uname -s) — no Windows, use o install.ps1" ;;
esac

case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  aarch64 | arm64) arch=arm64 ;;
  *) erro "arquitetura não suportada: $(uname -m)" ;;
esac

if [ -n "${FLUIGCLI_VERSION:-}" ]; then
  version="${FLUIGCLI_VERSION#v}"
else
  location=$(curl -fsSLI -o /dev/null -w '%{url_effective}' \
    "https://github.com/$REPO/releases/latest") ||
    erro "não consegui consultar a última release — verifique sua conexão"
  case "$location" in
  */tag/v*) version="${location##*/tag/v}" ;;
  *) erro "não consegui descobrir a última versão — informe com FLUIGCLI_VERSION=x.y.z" ;;
  esac
fi

arquivo="fluigcli_${version}_${os}_${arch}.tar.gz"
base="https://github.com/$REPO/releases/download/v${version}"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

info "Baixando o fluigcli v${version} (${os}/${arch})…"
curl -fsSL -o "$tmp/$arquivo" "$base/$arquivo" ||
  erro "download falhou: $base/$arquivo"
curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt" ||
  erro "download do checksums.txt falhou"

(
  cd "$tmp"
  if command -v sha256sum >/dev/null 2>&1; then
    grep " $arquivo\$" checksums.txt | sha256sum -c - >/dev/null 2>&1
  else
    grep " $arquivo\$" checksums.txt | shasum -a 256 -c - >/dev/null 2>&1
  fi
) || erro "o checksum não confere — download corrompido; tente de novo"
info "Checksum conferido."

tar -xzf "$tmp/$arquivo" -C "$tmp" fluigcli

if [ -n "${FLUIGCLI_INSTALL_DIR:-}" ]; then
  destino="$FLUIGCLI_INSTALL_DIR"
  mkdir -p "$destino"
  install -m 0755 "$tmp/fluigcli" "$destino/fluigcli"
elif [ -w /usr/local/bin ]; then
  destino=/usr/local/bin
  install -m 0755 "$tmp/fluigcli" "$destino/fluigcli"
elif command -v sudo >/dev/null 2>&1; then
  destino=/usr/local/bin
  info "Vou pedir sua senha (sudo) para gravar em /usr/local/bin."
  sudo install -m 0755 "$tmp/fluigcli" "$destino/fluigcli"
else
  destino="$HOME/.local/bin"
  mkdir -p "$destino"
  install -m 0755 "$tmp/fluigcli" "$destino/fluigcli"
fi

info "fluigcli v${version} instalado em $destino/fluigcli"
case ":$PATH:" in
  *":$destino:"*) ;;
  *) info "aviso: $destino não está no PATH — adicione-o ao perfil do seu shell" ;;
esac
