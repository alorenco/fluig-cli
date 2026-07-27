#!/usr/bin/env bash
# Prova que helper/fluigcliHelper.war (versionado no Git e embutido na CLI por
# go:embed) foi mesmo gerado das fontes deste repositório.
#
# Por que isto existe (ROADMAP §2.11-F): o `.srchash` cobre `pom.xml` e `src/**`,
# NÃO o WAR. Um PR que alterasse só o binário mantinha o hash válido, passava no
# `TestHelperWARAtualizado`, entrava no binário da CLI e era publicado por
# `install-helper` no servidor de quem usa. Num repositório público isso é um
# vetor de supply chain real.
#
# Como funciona: rebuilda o WAR e compara com o versionado **entrada por
# entrada** (nome + sha256 do conteúdo), ignorando a ordem e os timestamps do
# zip, que não são reprodutíveis.
#
# Uso:  ./helper/verify-war.sh
# Saída: 0 = o WAR corresponde à fonte · 1 = divergência · 2 = erro de ambiente

set -uo pipefail
cd "$(dirname "$0")"

if ! command -v mvn >/dev/null 2>&1; then
	echo "erro: mvn não encontrado no PATH" >&2
	exit 2
fi
if [ ! -f fluigcliHelper.war ]; then
	echo "erro: helper/fluigcliHelper.war não existe" >&2
	exit 2
fi

echo "== Rebuildando o WAR a partir da fonte =="
if ! mvn -q -B package -DskipTests; then
	echo "erro: o build do helper falhou" >&2
	exit 2
fi

python3 - fluigcliHelper.war target/fluigcliHelper.war <<'PY'
import hashlib, sys, zipfile

versionado, recem = sys.argv[1], sys.argv[2]

def entradas(caminho):
    out = {}
    with zipfile.ZipFile(caminho) as z:
        for info in z.infolist():
            if info.is_dir():
                continue
            out[info.filename] = hashlib.sha256(z.read(info.filename)).hexdigest()
    return out

a, b = entradas(versionado), entradas(recem)

so_versionado = sorted(set(a) - set(b))
so_recem = sorted(set(b) - set(a))
diferentes = sorted(k for k in set(a) & set(b) if a[k] != b[k])

if not (so_versionado or so_recem or diferentes):
    print(f"== OK: {len(a)} entradas conferem com a fonte ==")
    sys.exit(0)

print("== DIVERGÊNCIA entre o WAR versionado e o recém-buildado ==")
for k in so_versionado:
    print(f"  só no versionado : {k}")
for k in so_recem:
    print(f"  só no rebuild    : {k}")
for k in diferentes:
    print(f"  conteúdo difere  : {k}")
print()
print("Se você mexeu na fonte, rode ./helper/build.sh e commite o WAR.")
print("Se NÃO mexeu, o WAR versionado não corresponde à fonte — investigue antes")
print("de seguir: ele vai embutido no binário da CLI e é publicado nos servidores.")
sys.exit(1)
PY
