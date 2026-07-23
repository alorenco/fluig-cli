package strsim

import (
	"reflect"
	"testing"
)

func TestSuggestTypo(t *testing.T) {
	cands := []string{"Compras", "AprovacaoContrato", "FLUIGADHOCPROCESS"}
	got := Suggest("Compra", cands, 3)
	if len(got) == 0 || got[0] != "Compras" {
		t.Errorf("erro de digitação deveria sugerir Compras primeiro, veio %v", got)
	}
}

// Nomes reorganizados que só compartilham um token (o caso real do ROADMAP):
// "SolicitacaoAdiantamento" (nome do arquivo) ↔ "Adiantamento ao Fornecedor"
// (processId no servidor). A distância de edição é grande; o token resolve.
func TestSuggestTokenOverlap(t *testing.T) {
	cands := []string{
		"Adiantamento ao Fornecedor",
		"compras_solicitacao",
		"contratos_escritura_publica",
		"rh_justificativa_ponto",
	}
	got := Suggest("SolicitacaoAdiantamento", cands, 3)
	has := func(s string) bool {
		for _, g := range got {
			if g == s {
				return true
			}
		}
		return false
	}
	if !has("Adiantamento ao Fornecedor") {
		t.Errorf("deveria sugerir 'Adiantamento ao Fornecedor' (token adiantamento), veio %v", got)
	}
	if !has("compras_solicitacao") {
		t.Errorf("deveria sugerir 'compras_solicitacao' (token solicitacao), veio %v", got)
	}
	if has("rh_justificativa_ponto") {
		t.Errorf("não deveria sugerir nome sem token/distância em comum, veio %v", got)
	}
}

func TestSuggestNoMatch(t *testing.T) {
	got := Suggest("xyzqwerty", []string{"Compras", "Financeiro"}, 3)
	if len(got) != 0 {
		t.Errorf("nada parecido deveria devolver nil, veio %v", got)
	}
}

func TestSuggestLimit(t *testing.T) {
	cands := []string{"compras_um", "compras_dois", "compras_tres", "compras_quatro"}
	got := Suggest("compras_zero", cands, 2)
	if len(got) != 2 {
		t.Errorf("limit=2 deveria devolver 2, veio %d (%v)", len(got), got)
	}
}

func TestSuggestEmpty(t *testing.T) {
	if got := Suggest("", []string{"a"}, 3); got != nil {
		t.Errorf("target vazio deveria devolver nil, veio %v", got)
	}
	if got := Suggest("a", nil, 3); got != nil {
		t.Errorf("sem candidatos deveria devolver nil, veio %v", got)
	}
}

func TestTokens(t *testing.T) {
	got := tokens("SolicitacaoAdiantamento ao_Fornecedor")
	want := []string{"solicitacao", "adiantamento", "fornecedor"} // "ao" (2) descartado
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tokens = %v, quer %v", got, want)
	}
}
