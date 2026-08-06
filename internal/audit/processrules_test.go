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
