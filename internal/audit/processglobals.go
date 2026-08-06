package audit

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// FL005 — método do hAPI chamado como função GLOBAL em script de processo
// (ROADMAP3 §4.11-A, bug real do feedback de 2026-08-03/04):
//
//	var codColigada = getCardValue("codColigada");   // não existe: é hAPI.getCardValue
//
// Em runtime isso derruba a service task e desvia para a tarefa de erro; no
// audit passava porque as FL* validavam membros DE objetos (hAPI.x), nunca a
// chamada solta.
//
// O desenho é uma DENYLIST, não uma allowlist de globais: apontar "global
// desconhecida" exigiria conhecer todos os helpers do usuário — e os scripts
// de eventos do MESMO processo compartilham o escopo do Rhino, então um helper
// declarado em Proc.servicetask7.js é visível em Proc.beforeTaskSave.js.
// Apontamos só o inequívoco: chamada global cujo nome é um método do hAPI no
// catálogo (getCardValue, setCardValue, listAttachments…) e que não foi
// declarado como função em NENHUM script do processo. `getValue` fica de fora
// (não é membro do hAPI — é a global legítima de script de processo).
//
// Por isso a regra nasce como ERRO (as demais FL* são aviso): o falso positivo
// exigiria um helper do usuário com o mesmo nome de um método do hAPI e fora
// dos scripts do processo — e o `.fluigcli/audit.json` rebaixa se acontecer.

// bareCallRe casa `nome(` NÃO precedido de ponto (chamada global).
var bareCallRe = regexp.MustCompile(`(^|[^.\w$])(\w+)\s*\(`)

// declaredFnRe casa as formas de declarar função visíveis no escopo do
// processo: `function nome(...)`, `var nome = function` e arrow.
var declaredFnRe = regexp.MustCompile(
	`\bfunction\s+(\w+)\s*\(|\b(?:var|let|const)\s+(\w+)\s*=\s*(?:function\b|\(?[\w\s,]*\)?\s*=>)`)

// processGlobalFindings roda a FL005 num script de processo
// (workflow/scripts/<Proc>.<evento>.js). root localiza os scripts irmãos do
// mesmo processo, cujas funções declaradas são descontadas.
func processGlobalFindings(root, rel string, content []byte) []Finding {
	cat := apiCatalog()
	declared := declaredProcessFunctions(root, rel, content)

	var out []Finding
	seen := map[string]bool{}
	for i, line := range strings.Split(string(maskSource(content)), "\n") {
		for _, m := range bareCallRe.FindAllStringSubmatch(line, -1) {
			name := m[2]
			if seen[name] || declared[name] || !cat.HasMember("hAPI", name) {
				continue
			}
			seen[name] = true // uma vez por arquivo: o conserto é o mesmo
			out = append(out, Finding{
				Rule: RuleBareHAPICall, Severity: SeverityError, File: rel, Line: i + 1,
				Message: fmt.Sprintf("%s(...) não existe como função global em script de processo — em runtime a "+
					"service task falha e a solicitação desvia para a tarefa de erro", name),
				Suggestion: fmt.Sprintf("use hAPI.%s(...)", name),
			})
		}
	}
	return out
}

// declaredProcessFunctions coleta os nomes de função declarados no arquivo E
// nos scripts irmãos do mesmo processo (mesmo prefixo `<Proc>.` na pasta) —
// todos compartilham o escopo do Rhino em runtime.
func declaredProcessFunctions(root, rel string, content []byte) map[string]bool {
	out := map[string]bool{}
	collect := func(src []byte) {
		for _, m := range declaredFnRe.FindAllStringSubmatch(string(maskSource(src)), -1) {
			for _, name := range m[1:] {
				if name != "" {
					out[name] = true
				}
			}
		}
	}
	collect(content)

	base := filepath.Base(rel)
	proc, _, ok := strings.Cut(base, ".")
	if !ok {
		return out
	}
	dir := filepath.Join(root, filepath.FromSlash(filepath.Dir(rel)))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || name == base || !strings.HasPrefix(name, proc+".") || !strings.HasSuffix(name, ".js") {
			continue
		}
		if src, err := os.ReadFile(filepath.Join(dir, name)); err == nil {
			collect(src)
		}
	}
	return out
}

// isProcessScriptJS informa se o arquivo é um script de processo — o único
// contexto onde o conjunto de globais é fechado o bastante para a FL005.
func isProcessScriptJS(rel string) bool {
	return strings.HasPrefix(rel, "workflow/scripts/")
}
