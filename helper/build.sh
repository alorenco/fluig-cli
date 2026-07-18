#!/bin/sh
# Builda o fluigcliHelper.war e atualiza o artefato versionado + o hash das
# fontes que o teste anti-drift (TestHelperWARAtualizado) confere.
set -e
cd "$(dirname "$0")"

mvn -q package
cp target/fluigcliHelper.war fluigcliHelper.war

{
	printf '%s\0' pom.xml
	cat pom.xml
	for f in $(find src -type f | LC_ALL=C sort); do
		printf '%s\0' "$f"
		cat "$f"
	done
} | sha256sum | cut -d' ' -f1 >.srchash

echo "ok: fluigcliHelper.war e .srchash atualizados"
