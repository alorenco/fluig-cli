# Instalador do fluigcli para Windows.
#
# Baixa a última release do GitHub, confere o checksum e instala o binário:
#   irm https://raw.githubusercontent.com/alorenco/fluig-cli/main/install.ps1 | iex
#
# Variável opcional:
#   FLUIGCLI_VERSION  versão a instalar (ex.: 0.1.0); padrão: última release

$ErrorActionPreference = "Stop"

$repo = "alorenco/fluig-cli"

$versao = $env:FLUIGCLI_VERSION
if ($versao) {
    $versao = $versao.TrimStart("v")
} else {
    $release = Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest"
    $versao = $release.tag_name.TrimStart("v")
}

$arquivo = "fluigcli_${versao}_windows_amd64.zip"
$base = "https://github.com/$repo/releases/download/v$versao"
$tmp = Join-Path $env:TEMP "fluigcli-install-$PID"
New-Item -ItemType Directory -Force -Path $tmp | Out-Null

try {
    Write-Host "Baixando o fluigcli v$versao (windows/amd64)…"
    Invoke-WebRequest "$base/$arquivo" -OutFile (Join-Path $tmp $arquivo)
    Invoke-WebRequest "$base/checksums.txt" -OutFile (Join-Path $tmp "checksums.txt")

    $linha = Select-String -Path (Join-Path $tmp "checksums.txt") -Pattern $arquivo -SimpleMatch
    if (-not $linha) { throw "não achei o $arquivo no checksums.txt" }
    $esperado = ($linha.Line -split "\s+")[0].ToLower()
    $obtido = (Get-FileHash (Join-Path $tmp $arquivo) -Algorithm SHA256).Hash.ToLower()
    if ($esperado -ne $obtido) { throw "o checksum não confere — download corrompido; tente de novo" }
    Write-Host "Checksum conferido."

    $destino = Join-Path $env:LOCALAPPDATA "Programs\fluigcli"
    Expand-Archive (Join-Path $tmp $arquivo) -DestinationPath $destino -Force

    $pathUsuario = [Environment]::GetEnvironmentVariable("Path", "User")
    if (($pathUsuario -split ";") -notcontains $destino) {
        [Environment]::SetEnvironmentVariable("Path", "$pathUsuario;$destino", "User")
        Write-Host "Pasta adicionada ao PATH do usuário — abra um novo terminal para usar o fluigcli."
    }

    Write-Host "fluigcli v$versao instalado em $destino"
} finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}
