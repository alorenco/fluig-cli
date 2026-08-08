package audit

import (
	"strings"
	"testing"

	"github.com/alorenco/fluig-cli/internal/scaffold"
)

// O eventSeqParam duplica, por recorte, o catálogo de eventos do
// internal/scaffold (o do `workflow new-script`). Duplicar é de propósito: o
// audit precisa só de "qual parâmetro é a sequence", e importar o scaffold em
// produção arrastaria os templates embutidos. Este teste é o que impede as
// duas listas de divergirem — se uma assinatura mudar no catálogo, ele quebra.
//
// O import do scaffold vive SÓ aqui, no teste.
func TestEventoDeEtapaBateComOCatalogo(t *testing.T) {
	catalogo := map[string][]string{}
	for _, ev := range scaffold.ProcessEvents() {
		var params []string
		for _, p := range strings.Split(ev.Params, ",") {
			params = append(params, strings.TrimSpace(p))
		}
		catalogo[ev.Name] = params
	}

	for evento, param := range eventSeqParam {
		params, ok := catalogo[evento]
		if !ok {
			t.Errorf("evento %q não existe no catálogo do scaffold", evento)
			continue
		}
		achou := false
		for _, p := range params {
			if p == param {
				achou = true
			}
		}
		if !achou {
			t.Errorf("evento %q: o catálogo tem os parâmetros %v, sem %q", evento, params, param)
		}
	}

	// O contrário: um evento do catálogo cujo parâmetro se chama como os de
	// sequence precisa estar mapeado aqui. Pega assinatura nova no catálogo.
	for _, ev := range scaffold.ProcessEvents() {
		for _, raw := range strings.Split(ev.Params, ",") {
			p := strings.TrimSpace(raw)
			if p != "sequenceId" && p != "nextSequenceId" && p != "iCurrentState" {
				continue
			}
			if eventSeqParam[ev.Name] != p {
				t.Errorf("o evento %q recebe %q, mas o eventSeqParam não o mapeia", ev.Name, p)
			}
		}
	}
}
