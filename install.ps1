# Instalador do fluigcli para Windows.
#
# Baixa a última release do GitHub, confere o checksum e instala o binário:
#   irm https://raw.githubusercontent.com/alorenco/fluig-cli/main/install.ps1 | iex
#
# Variável opcional:
#   FLUIGCLI_VERSION  versão a instalar (ex.: 0.1.0); padrão: última release

$ErrorActionPreference = "Stop"

$repo = "alorenco/fluig-cli"

# Garante TLS 1.2 no Windows PowerShell 5.1 (o GitHub exige).
[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor 3072

$versao = $env:FLUIGCLI_VERSION
if ($versao) {
    $versao = $versao.TrimStart("v")
} else {
    # Descobre a última versão pelo redirect de /releases/latest — como o
    # install.sh e o fluigcli upgrade fazem. NÃO usa a api.github.com, que
    # tem rate limit por IP (60/h sem autenticação) e quebra em redes
    # compartilhadas/CGNAT.
    $req = [System.Net.WebRequest]::Create("https://github.com/$repo/releases/latest")
    $req.AllowAutoRedirect = $false
    try {
        $resp = $req.GetResponse()
        $location = $resp.Headers["Location"]
        $resp.Close()
    } catch {
        throw "não consegui consultar a última release — verifique sua conexão (ou informe a versão com `$env:FLUIGCLI_VERSION)"
    }
    if (-not $location -or $location -notmatch "/tag/v([^/]+)$") {
        throw "não consegui descobrir a última versão — informe com: `$env:FLUIGCLI_VERSION = 'x.y.z'"
    }
    $versao = $Matches[1]
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
