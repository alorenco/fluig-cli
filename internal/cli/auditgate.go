package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alorenco/fluig-cli/internal/audit"
	"github.com/alorenco/fluig-cli/internal/output"
)

// Pré-checagem local antes de publicar script (ROADMAP2 §3.2).
//
// Motivo: o servidor recusa script com erro de compilação com uma mensagem
// genérica ("Não foi possível compilar os scripts para customização Model"),
// enquanto o `audit` local já sabe a linha e o motivo. Pior: um `const` no corpo
// de um laço (RHINO003) compila e roda ERRADO em silêncio no Rhino do Fluig.
// Rodar o audit antes do envio troca dois ciclos de tentativa e erro por zero.

// auditGate é o veredito da pré-checagem de um lote de arquivos.
type auditGate struct {
	// blocked mapeia o caminho do arquivo (como o usuário informou) para os
	// achados de ERRO que barram a publicação dele.
	blocked map[string][]audit.Finding
	// byFile são todos os achados por arquivo do lote, na mesma chave de blocked.
	byFile map[string][]audit.Finding
	// findings são todos os achados dos arquivos do lote (erros e avisos), na
	// ordem do audit. Vão para o envelope --json.
	findings []audit.Finding
	warnings int
	// ran informa se a auditoria rodou (false com --no-audit ou se ela falhou).
	ran bool
}

// compileRelevantRules são as regras que podem, de fato, impedir a COMPILAÇÃO do
// script no servidor. Só elas entram como causa provável de uma recusa de
// compilação. Apontar um aviso de outra natureza (uma variável WK* inexistente,
// por exemplo) mandaria o usuário investigar o arquivo errado.
//
// RHINO002 (sintaxe ES6) dá SyntaxError no Rhino do Fluig. RHINO003 (`const` no
// corpo de um laço) **compila** na homologação (medido em 2026-07-29) e o efeito
// é silencioso, mas o relato de origem do §3.2 veio de um servidor que recusou
// um script com esse defeito — fica na lista por isso.
var compileRelevantRules = map[string]bool{"RHINO002": true, "RHINO003": true}

// blockedError monta a mensagem de recusa de um arquivo, citando a primeira
// causa. As demais saem na tabela.
func (g *auditGate) blockedError(file string) *output.Error {
	fs := g.blocked[file]
	if len(fs) == 0 {
		return nil
	}
	f := fs[0]
	msg := fmt.Sprintf("a auditoria local reprovou o script: %s em %s:%d — %s",
		f.Rule, f.File, f.Line, f.Message)
	if f.Suggestion != "" {
		msg += " → " + f.Suggestion
	}
	if len(fs) > 1 {
		msg += fmt.Sprintf(" (e %d outro(s) erro(s) no mesmo arquivo)", len(fs)-1)
	}
	msg += ". Corrija e publique de novo, ou envie sem checar com --no-audit"
	return output.AuditFailedf("%s", msg)
}

// auditBeforePublish roda o audit nos arquivos que serão publicados e devolve o
// veredito por arquivo. Nunca falha o comando por conta própria: problema na
// própria auditoria vira aviso e a publicação segue (a checagem é uma rede de
// proteção, não uma dependência nova do publish).
func (a *App) auditBeforePublish(p *output.Printer, files []string, skip bool) *auditGate {
	gate := &auditGate{
		blocked: map[string][]audit.Finding{},
		byFile:  map[string][]audit.Finding{},
	}
	if skip {
		return gate
	}
	root, err := a.projectRootForFiles()
	if err != nil {
		p.Warnf("não consegui localizar a raiz do projeto para auditar (%v). A publicação segue.", err)
		return gate
	}
	// Catálogo embutido: a pré-checagem não faz rede (o --sync é só do `audit`).
	cat, err := audit.Embedded()
	if err != nil {
		p.Warnf("não consegui carregar o catálogo da auditoria (%v). A publicação segue.", err)
		return gate
	}
	cfg, err := loadAuditConfig(root)
	if err != nil {
		// Um audit.json inválido é erro de uso do projeto, mas não pode barrar a
		// publicação: o usuário pediu export, não auditoria.
		p.Warnf("%v — a publicação segue sem a checagem local.", err)
		return gate
	}
	res, err := audit.Run(root, files, cat, cfg)
	if err != nil {
		p.Warnf("a auditoria local falhou (%v). A publicação segue.", err)
		return gate
	}
	gate.ran = true
	gate.findings = res.Findings

	// Casa cada achado com o arquivo do lote: o audit reporta o caminho relativo
	// à raiz, e o usuário pode ter informado relativo ao cwd ou absoluto.
	byRel := map[string]string{} // caminho relativo à raiz → argumento original
	for _, file := range files {
		byRel[auditRelPath(root, file)] = file
	}
	for _, f := range res.Findings {
		file, ok := byRel[f.File]
		if ok {
			gate.byFile[file] = append(gate.byFile[file], f)
		}
		if f.Severity != audit.SeverityError {
			gate.warnings++
			continue
		}
		if ok {
			gate.blocked[file] = append(gate.blocked[file], f)
		}
	}
	return gate
}

// auditRelPath devolve o caminho do arquivo relativo à raiz, no formato que o
// audit usa em Finding.File (barras normais).
func auditRelPath(root, file string) string {
	abs := file
	if !filepath.IsAbs(abs) {
		if _, err := os.Stat(abs); err != nil {
			abs = filepath.Join(root, file)
		} else if v, err := filepath.Abs(abs); err == nil {
			abs = v
		}
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		rel = file
	}
	return filepath.ToSlash(rel)
}

// reportAuditGate imprime o resultado da pré-checagem no modo humano: tabela só
// com os erros (que barram) e os avisos numa linha, para não poluir o publish.
func (g *auditGate) report(p *output.Printer) {
	if !g.ran {
		return
	}
	var errs []audit.Finding
	for _, f := range g.findings {
		if f.Severity == audit.SeverityError {
			errs = append(errs, f)
		}
	}
	// Com um único erro, a mensagem de recusa do arquivo já traz regra, local e
	// correção — a tabela só repetiria. Ela ganha valor a partir de dois.
	if len(errs) > 1 {
		p.Table(auditFindingsTable(errs))
	}
	if g.warnings > 0 {
		p.Infof("%d aviso(s) da auditoria não barram a publicação — veja com: fluigcli audit", g.warnings)
	}
}

// compileFailureHint complementa a recusa de compilação do servidor, que é
// genérica ("Não foi possível compilar os scripts para customização Model") e não
// cita a linha.
//
// Só entram achados de regras que podem impedir a compilação
// (compileRelevantRules). Sem nenhum, o texto diz isso em vez de apontar um
// achado sem relação — dica errada custa mais que dica nenhuma. Vazio quando a
// auditoria não rodou (`--no-audit`).
func compileFailureHint(g *auditGate, file string) string {
	if g == nil || !g.ran {
		return ""
	}
	var parts []string
	for _, f := range g.byFile[file] {
		if compileRelevantRules[f.Rule] {
			parts = append(parts, fmt.Sprintf("%s em %s:%d — %s", f.Rule, f.File, f.Line, f.Message))
		}
	}
	if len(parts) == 0 {
		return "a auditoria local não achou nada que explique a recusa — procure erro de sintaxe " +
			"não coberto pelas regras (o servidor não informa a linha)"
	}
	return "a auditoria local aponta a causa provável: " + strings.Join(parts, "; ")
}

// isCompileRejection informa se o erro é a recusa de compilação do servidor.
// A comparação é por trecho em minúsculas: a mensagem vem do Fluig em pt-BR e
// pode variar no restante da frase.
func isCompileRejection(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(output.AsError(err).Message), "não foi possível compilar")
}

// withExtraMessage devolve o mesmo erro tipado com um complemento na mensagem
// (código e exit code preservados — o contrato não muda).
func withExtraMessage(err error, extra string) error {
	if extra == "" {
		return err
	}
	e := output.AsError(err)
	msg := strings.TrimRight(e.Message, " .") + ". " + extra
	return (&output.Error{Code: e.Code, Message: msg, Exit: e.Exit}).WithCause(err)
}
