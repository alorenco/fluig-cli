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
	// naoBloqueantes conta achados de nível ERRO que o recorte de regras deste
	// comando deixou passar (ver auditGateOpts.regras).
	naoBloqueantes int
	// ran informa se a auditoria rodou (false com --no-audit ou se ela falhou).
	ran bool
	opts auditGateOpts
}

// auditGateOpts ajusta a pré-checagem por comando.
type auditGateOpts struct {
	// skip desliga a checagem (--no-audit).
	skip bool
	// regras limita, por PREFIXO, quais regras podem barrar a publicação. Vazio
	// = toda regra de nível erro barra.
	//
	// Existe para o `form export`: as regras SG* são de tema visual (cor fixa,
	// recurso externo) e barrar a publicação de um formulário legado por causa
	// delas seria intromissão — o usuário pediu para publicar o formulário, não
	// para redesenhá-lo. Já um erro de sintaxe do Rhino (RHINO*) ou uma API que
	// não existe (FL*) quebram o formulário em runtime.
	regras []string
}

// bloqueia informa se a regra pode barrar a publicação neste comando.
func (o auditGateOpts) bloqueia(rule string) bool {
	if len(o.regras) == 0 {
		return true
	}
	for _, prefixo := range o.regras {
		if strings.HasPrefix(rule, prefixo) {
			return true
		}
	}
	return false
}

// regrasDeRuntime é o recorte usado no `form export`: só o que quebra em
// execução barra a publicação.
var regrasDeRuntime = []string{"RHINO", "FL"}

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
func (a *App) auditBeforePublish(p *output.Printer, files []string, opts auditGateOpts) *auditGate {
	gate := &auditGate{
		blocked: map[string][]audit.Finding{},
		byFile:  map[string][]audit.Finding{},
		opts:    opts,
	}
	if opts.skip {
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

	// Casa cada achado com o alvo do lote: o audit reporta o caminho relativo à
	// raiz, e o usuário pode ter informado relativo ao cwd ou absoluto.
	byRel := map[string]string{} // caminho relativo à raiz → argumento original
	for _, file := range files {
		byRel[auditRelPath(root, file)] = file
	}
	for _, f := range res.Findings {
		file, ok := byRel[f.File]
		if !ok {
			// Alvo que é PASTA (o `form export` manda a pasta do formulário): o
			// achado é de um arquivo dentro dela. Sem isto, erro em
			// forms/X/events/y.js não barraria o envio de forms/X.
			file, ok = donoDoAchado(byRel, f.File)
		}
		if ok {
			gate.byFile[file] = append(gate.byFile[file], f)
		}
		if f.Severity != audit.SeverityError {
			gate.warnings++
			continue
		}
		if !opts.bloqueia(f.Rule) {
			gate.naoBloqueantes++
			continue
		}
		if ok {
			gate.blocked[file] = append(gate.blocked[file], f)
		}
	}
	return gate
}

// donoDoAchado encontra o alvo (pasta) que contém o arquivo do achado. Com mais
// de um candidato, ganha o mais específico — a pasta mais funda.
func donoDoAchado(byRel map[string]string, rel string) (string, bool) {
	melhor, dono := "", ""
	for candidato, original := range byRel {
		if candidato == "" || candidato == "." {
			continue
		}
		if strings.HasPrefix(rel, candidato+"/") && len(candidato) > len(melhor) {
			melhor, dono = candidato, original
		}
	}
	return dono, dono != ""
}

// bloqueados devolve os arquivos reprovados, na ordem em que foram informados.
func (g *auditGate) bloqueados(files []string) []string {
	var out []string
	for _, f := range files {
		if len(g.blocked[f]) > 0 {
			out = append(out, f)
		}
	}
	return out
}

// auditBeforeAtomicPublish é a pré-checagem dos comandos que publicam o conjunto
// de UMA VEZ (workflow publish/export): qualquer arquivo reprovado aborta tudo.
//
// Não existe publicação parcial de versão de processo — publicar metade dos
// eventos deixaria o processo num estado que ninguém pediu. Mesmo espírito da
// regra que já valia ali: script de evento inexistente interrompe o comando
// antes de qualquer mudança.
func (a *App) auditBeforeAtomicPublish(p *output.Printer, files []string, opts auditGateOpts) error {
	gate := a.auditBeforePublish(p, files, opts)
	gate.report(p)
	reprovados := gate.bloqueados(files)
	if len(reprovados) == 0 {
		return nil
	}
	err := gate.blockedError(reprovados[0])
	if len(reprovados) > 1 {
		err = output.AuditFailedf("%s (%d arquivos reprovados; nada foi publicado)",
			err.Message, len(reprovados))
	}
	return err
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
	// A tabela mostra só o que BARRA: um achado de nível erro que este comando
	// deixa passar viraria uma linha vermelha sem consequência, e o usuário
	// procuraria um problema que não existe. Esses entram na contagem abaixo.
	var errs []audit.Finding
	for _, f := range g.findings {
		if f.Severity == audit.SeverityError && g.opts.bloqueia(f.Rule) {
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
	if g.naoBloqueantes > 0 {
		p.Infof("%d achado(s) de nível erro não barram ESTE comando (regras de tema visual) — veja com: fluigcli audit",
			g.naoBloqueantes)
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
