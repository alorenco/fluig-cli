package audit

import (
	"strings"
	"testing"
)

// Estados no formato do processo real do caso que motivou a regra
// (contratos_notificacao_vegetacao v10, homologação, 2026-08-06).
var estadosTeste = []ProcessActivity{
	{Sequence: 6, Name: "Início", Kind: "start"},
	{Sequence: 7, Name: "Mover Documentos", Kind: "service"},
	{Sequence: 9, Name: "Corrigir Integração", Kind: "task"},
	{Sequence: 13, Name: "Apto a Notificação", Kind: "gateway"},
	{Sequence: 21, Name: "Acompanhar Retornos", Kind: "task"},
	{Sequence: 24, Name: "Conferência de Retorno", Kind: "task"},
	{Sequence: 69, Name: "Fim", Kind: "end"},
}

func findByRule(fs []Finding, rule string) []Finding {
	var out []Finding
	for _, f := range fs {
		if f.Rule == rule {
			out = append(out, f)
		}
	}
	return out
}

func TestCheckFormActivities(t *testing.T) {
	t.Run("o caso do relato: nenhuma seção casa com o processo", func(t *testing.T) {
		html := `<div class="activity activity-0"></div>
<div class="activity activity-5"></div>
<div class="activity activity-10"></div>`
		fs := CheckFormActivities("forms/f/f.html", []byte(html), "proc_x", estadosTeste)
		bad := findByRule(fs, RuleActivityUnknown)
		if len(bad) != 2 {
			t.Fatalf("esperava WF001 para 5 e 10 (activity-0 é a abertura), veio %d: %+v", len(bad), bad)
		}
		if bad[0].Line != 2 || bad[1].Line != 3 {
			t.Errorf("linhas erradas: %+v", bad)
		}
		if !strings.Contains(bad[0].Message, "activity-5") || !strings.Contains(bad[0].Message, "proc_x") {
			t.Errorf("mensagem sem o essencial: %s", bad[0].Message)
		}
		// A sugestão diz onde estão os números certos.
		if !strings.Contains(bad[0].Suggestion, "9 (Corrigir Integração)") ||
			!strings.Contains(bad[0].Suggestion, "6 (início; a abertura usa activity-0)") {
			t.Errorf("sugestão sem as etapas: %s", bad[0].Suggestion)
		}
	})

	t.Run("formulário correto não gera WF001", func(t *testing.T) {
		html := `<div class="activity activity-0 activity-9 activity-21 activity-24"></div>`
		fs := CheckFormActivities("f.html", []byte(html), "p", estadosTeste)
		if bad := findByRule(fs, RuleActivityUnknown); len(bad) != 0 {
			t.Errorf("não esperava WF001: %+v", bad)
		}
		if miss := findByRule(fs, RuleActivityMissing); len(miss) != 0 {
			t.Errorf("todas as humanas têm seção — não esperava WF002: %+v", miss)
		}
	})

	t.Run("classe repetida vira UM achado com contagem", func(t *testing.T) {
		html := strings.Repeat(`<div class="activity activity-99"></div>`+"\n", 30)
		fs := CheckFormActivities("f.html", []byte(html), "p", estadosTeste)
		bad := findByRule(fs, RuleActivityUnknown)
		if len(bad) != 1 {
			t.Fatalf("30 repetições são UM problema: %+v", bad)
		}
		if bad[0].Line != 1 || !strings.Contains(bad[0].Message, "30 ocorrências") {
			t.Errorf("achado sem linha/contagem: %+v", bad[0])
		}
	})

	t.Run("atividade humana sem seção vira WF002 (aviso)", func(t *testing.T) {
		html := `<div class="activity activity-9 activity-21"></div>` // falta a 24
		fs := CheckFormActivities("f.html", []byte(html), "p", estadosTeste)
		miss := findByRule(fs, RuleActivityMissing)
		if len(miss) != 1 || miss[0].Severity != SeverityWarning {
			t.Fatalf("esperava WF002 só para a 24: %+v", miss)
		}
		if !strings.Contains(miss[0].Message, "24") || !strings.Contains(miss[0].Message, "Conferência de Retorno") {
			t.Errorf("mensagem sem a etapa: %s", miss[0].Message)
		}
	})

	t.Run("formulário sem a convenção fica em paz", func(t *testing.T) {
		html := `<div class="fs-md-12"><input name="campo"></div>`
		if fs := CheckFormActivities("f.html", []byte(html), "p", estadosTeste); len(fs) != 0 {
			t.Errorf("sem classes activity-* não há o que cruzar: %+v", fs)
		}
	})

	t.Run("service task e gateway não pedem seção", func(t *testing.T) {
		// 7 (service) e 13 (gateway) sem seção: nenhum WF002.
		html := `<div class="activity activity-9 activity-21 activity-24"></div>`
		fs := CheckFormActivities("f.html", []byte(html), "p", estadosTeste)
		if len(fs) != 0 {
			t.Errorf("só atividade HUMANA pede seção: %+v", fs)
		}
	})

	t.Run("texto activity- sem número não conta", func(t *testing.T) {
		html := `<script>$(".activity-" + currentState).show();</script>
<div class="activity activity-9 activity-21 activity-24"></div>`
		if fs := CheckFormActivities("f.html", []byte(html), "p", estadosTeste); len(fs) != 0 {
			t.Errorf("a concatenação do JS não é uma seção: %+v", fs)
		}
	})
}

// --- WF003: constante de etapa × sequences reais ---

func TestCheckProcessScriptStates(t *testing.T) {
	const rel = "workflow/scripts/proc_x.beforeStateEntry.js"

	t.Run("caso real do Compras: constante defasada de versão anterior", func(t *testing.T) {
		// Reduzido do Compras.beforeStateEntry.js do projeto real: o processo
		// não tem sequence 166, então atualizaMovimento nunca é chamado.
		js := `function beforeStateEntry(sequenceId) {
    var cancelaState = 166;
    var aprovacaoGerente = 7;

    if (sequenceId == cancelaState) {
        atualizaMovimento("Cancela");
    }
    if (sequenceId == aprovacaoGerente) {
        hAPI.setCardValue("x", "1");
    }
}`
		fs := CheckProcessScriptStates(rel, []byte(js), "proc_x", estadosTeste)
		bad := findByRule(fs, RuleStateConstUnknown)
		if len(bad) != 1 {
			t.Fatalf("esperava 1 achado, veio %d: %+v", len(bad), fs)
		}
		if bad[0].Line != 5 {
			t.Errorf("linha = %d, quer 5 (a comparação)", bad[0].Line)
		}
		for _, quer := range []string{"cancelaState", "166", "nunca é verdadeira"} {
			if !strings.Contains(bad[0].Message, quer) {
				t.Errorf("mensagem sem %q: %s", quer, bad[0].Message)
			}
		}
		// A sugestão precisa listar as etapas reais, inclusive gateway e
		// automática — o script compara com elas também.
		for _, quer := range []string{"7 (Mover Documentos)", "13 (Apto a Notificação)"} {
			if !strings.Contains(bad[0].Suggestion, quer) {
				t.Errorf("sugestão sem %q: %s", quer, bad[0].Suggestion)
			}
		}
	})

	t.Run("caso real do distrato: constante copiada de outro processo", func(t *testing.T) {
		// ETAPA_INICIOGRV = 4 é o "Início" de OUTRO processo; aqui o início é 6.
		js := `function beforeStateLeave(sequenceId) {
    var ETAPA_INICIOGRV = 4;
    if (sequenceId == ETAPA_INICIOGRV) { gravar(); }
}`
		fs := CheckProcessScriptStates(rel, []byte(js), "proc_x", estadosTeste)
		if bad := findByRule(fs, RuleStateConstUnknown); len(bad) != 1 {
			t.Fatalf("esperava 1 achado, veio %+v", fs)
		}
	})

	t.Run("etapas válidas não geram achado", func(t *testing.T) {
		js := `function beforeStateEntry(sequenceId) {
    var inicio = 6;
    var conferencia = 24;
    if (sequenceId == inicio || sequenceId == conferencia) { ok(); }
    if (sequenceId != 21) { ok(); }
}`
		if fs := CheckProcessScriptStates(rel, []byte(js), "proc_x", estadosTeste); len(fs) != 0 {
			t.Errorf("nenhum achado esperado, veio %+v", fs)
		}
	})

	t.Run("a etapa vem de getValue(WKNumState), com e sem parseInt", func(t *testing.T) {
		js := `function afterTaskCreate(colleagueId) {
    var currentState = parseInt(getValue("WKNumState"));
    if (currentState == 999) { nunca(); }
}`
		if bad := findByRule(CheckProcessScriptStates(rel, []byte(js), "proc_x", estadosTeste),
			RuleStateConstUnknown); len(bad) != 1 {
			t.Fatalf("esperava 1 achado para o 999, veio %+v", bad)
		}
		js2 := `function afterTaskCreate(colleagueId) {
    var atv = getValue("WKNumState");
    if (atv == 24) { ok(); }
}`
		if fs := CheckProcessScriptStates(rel, []byte(js2), "proc_x", estadosTeste); len(fs) != 0 {
			t.Errorf("24 é etapa válida: %+v", fs)
		}
	})

	t.Run("zero é sem-etapa, como o activity-0 do WF001", func(t *testing.T) {
		js := `function beforeTaskSave(colleagueId, nextSequenceId, userList) {
    var inicioState = 0;
    if (nextSequenceId == inicioState) { abertura(); }
    if (nextSequenceId == 0) { abertura(); }
}`
		if fs := CheckProcessScriptStates(rel, []byte(js), "proc_x", estadosTeste); len(fs) != 0 {
			t.Errorf("0 não pode ser acusado: %+v", fs)
		}
	})

	t.Run("número que não é etapa não conta fora de comparação com a etapa", func(t *testing.T) {
		// O parâmetro de afterTaskCreate é colleagueId, não uma sequence.
		// Comparar 999 com outra coisa qualquer não é problema desta regra.
		js := `function afterTaskCreate(colleagueId) {
    var limite = 999;
    var total = getTotal();
    if (total == limite) { ok(); }
    if (colleagueId == 999) { ok(); }
}`
		if fs := CheckProcessScriptStates(rel, []byte(js), "proc_x", estadosTeste); len(fs) != 0 {
			t.Errorf("nenhum achado esperado: %+v", fs)
		}
	})

	t.Run("variável reatribuída para não-número sai do conjunto", func(t *testing.T) {
		// O idioma real: inicializa com 0 e depois recebe o resultado da
		// função. Não é constante de etapa (8 dos 9 casos do projeto real).
		js := `function beforeStateEntry(sequenceId) {
    var statusLan = 111;
    statusLan = getStatusLan(idLan, codcoligada);
    var statusLan = getStatusLan(idLan, codcoligada);
    if (sequenceId == statusLan) { ok(); }
}`
		if fs := CheckProcessScriptStates(rel, []byte(js), "proc_x", estadosTeste); len(fs) != 0 {
			t.Errorf("variável suja não pode virar constante de etapa: %+v", fs)
		}
	})

	t.Run("comentário e string não viram código", func(t *testing.T) {
		js := `function beforeStateEntry(sequenceId) {
    // if (sequenceId == 888) { antigo(); }
    /* if (sequenceId == 777) { antigo(); } */
    var msg = "sequenceId == 666";
    if (sequenceId == 6) { ok(); }
}`
		if fs := CheckProcessScriptStates(rel, []byte(js), "proc_x", estadosTeste); len(fs) != 0 {
			t.Errorf("comentário/string não é comparação: %+v", fs)
		}
	})

	t.Run("a mesma constante errada em vários pontos é UM achado", func(t *testing.T) {
		js := `function beforeStateEntry(sequenceId) {
    var errada = 166;
    if (sequenceId == errada) { a(); }
    if (sequenceId != errada) { b(); }
    if (errada == sequenceId) { c(); }
}`
		if bad := findByRule(CheckProcessScriptStates(rel, []byte(js), "proc_x", estadosTeste),
			RuleStateConstUnknown); len(bad) != 1 {
			t.Fatalf("esperava 1 achado (não %d): %+v", len(bad), bad)
		}
	})

	t.Run("sem etapas do servidor a regra não roda", func(t *testing.T) {
		js := `function beforeStateEntry(sequenceId) { if (sequenceId == 166) { a(); } }`
		if fs := CheckProcessScriptStates(rel, []byte(js), "proc_x", nil); len(fs) != 0 {
			t.Errorf("sem etapas não há o que cruzar: %+v", fs)
		}
	})
}
