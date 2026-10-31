package testrun

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeSuite grava a suíte e uma fixture numa pasta temporária.
func writeSuite(t *testing.T, suite string) string {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "fixtures"), 0o755)
	os.WriteFile(filepath.Join(dir, "fixtures", "card.json"),
		[]byte(`{"valor": "10,00", "data": "{{today}}", "qtd": 3, "ok": true, "vazio": null}`), 0o644)
	path := filepath.Join(dir, "suite.json")
	if err := os.WriteFile(path, []byte(suite), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

const suiteValida = `{
  "process": "Compras",
  "description": "suíte de exemplo",
  "vars": {"venc": "10/11/2026"},
  "defaults": {"card": "fixtures/card.json", "cleanup": ["none"]},
  "scenarios": [
    {"name": "aprovação",
     "fields": {"vencimento___1": "{{var.venc}}"},
     "steps": [
       {"start": true, "expect": {"state": 5, "status": "open"}},
       {"assume": true},
       {"move": 9, "id": "m1"},
       {"move": {"target": 12, "fields": {"decisao": "aprovado", "n": 2}, "comment": "ok"},
        "expect": {"card": {"decisao": "aprovado", "nivel": {"in": ["1", 2]}, "obs": {"empty": true}},
                   "path": {"visited": [5, 9], "notVisited": [7]}, "stateName": "Fim {{steps.m1.nextState}}"}},
       {"setFields": {"x": "1"}},
       {"manager": {"target": 13, "comment": "gestor"}},
       {"wait": {"state": 15, "timeout": "1m", "interval": "2s"}},
       {"check": true, "expect": {"status": "FINALIZED"}},
       {"cancel": {"comment": "fim"}}
     ]},
    {"name": "erro esperado", "cleanup": ["cancel"],
     "steps": [{"start": {"target": 5, "fields": {"valor": ""}}, "expect": {"error": {"contains": "obrigatório", "code": "SERVER_ERROR"}}}]}
  ]
}`

func TestParseSuiteValida(t *testing.T) {
	s, err := LoadSuite(writeSuite(t, suiteValida), LoadOptions{Vars: map[string]string{"extra": "1"}})
	if err != nil {
		t.Fatal(err)
	}
	if s.Process != "Compras" || len(s.Scenarios) != 2 || s.Vars["venc"] != "10/11/2026" || s.Vars["extra"] != "1" {
		t.Fatalf("suíte inesperada: %+v", s)
	}
	sc := s.Scenarios[0]
	if sc.CardValues["valor"] != "10,00" || sc.CardValues["qtd"] != "3" || sc.CardValues["ok"] != "true" || sc.CardValues["vazio"] != "" {
		t.Errorf("fixture mal convertida: %+v", sc.CardValues)
	}
	if strings.Join(sc.Cleanup, ",") != "none" || strings.Join(s.Scenarios[1].Cleanup, ",") != "cancel" {
		t.Errorf("herança de cleanup errada: %v / %v", sc.Cleanup, s.Scenarios[1].Cleanup)
	}
	st := sc.Steps
	if st[2].Action != ActMove || st[2].Target != 9 || st[2].Name != "move → 9" || st[2].As != ActorSelf {
		t.Errorf("forma curta do move: %+v", st[2])
	}
	if st[3].Fields["n"] != "2" || st[3].Comment != "ok" {
		t.Errorf("forma longa do move: %+v", st[3])
	}
	if *st[0].Expect.Status != "OPEN" {
		t.Errorf("status não normalizado: %s", *st[0].Expect.Status)
	}
	card := st[3].Expect.Card
	if len(card) != 3 || card[0].Field != "decisao" || card[1].Op != "in" || card[1].List[1] != "2" || card[2].Op != "empty" {
		t.Errorf("card na ordem do arquivo: %+v", card)
	}
	if st[6].WaitState != 15 || st[6].Timeout != "1m" {
		t.Errorf("wait: %+v", st[6])
	}
	if s.Scenarios[1].Steps[0].Expect.Error.Code != "SERVER_ERROR" {
		t.Errorf("error forma longa: %+v", s.Scenarios[1].Steps[0].Expect.Error)
	}
	// Cleanup default sem defaults: cancel.
	s2, err := LoadSuite(writeSuite(t, `{"process":"P","scenarios":[{"name":"a","steps":[{"start":true}]}]}`), LoadOptions{})
	if err != nil || strings.Join(s2.Scenarios[0].Cleanup, ",") != "cancel" {
		t.Errorf("cleanup default deveria ser cancel: %v %v", s2.Scenarios[0].Cleanup, err)
	}
}

// Cada mensagem de validação local (spec §7.8). Todas são erro de uso.
func TestParseSuiteInvalida(t *testing.T) {
	casos := []struct {
		nome, suite, quer string
	}{
		{"chave desconhecida no topo", `{"process":"P","scenarioz":[]}`, `chave desconhecida "scenarioz"`},
		{"chave desconhecida no expect", `{"process":"P","scenarios":[{"name":"a","steps":[{"start":true,"expect":{"stat":1}}]}]}`,
			`scenarios[0] ("a").steps[0].expect: chave desconhecida "stat"`},
		{"expcet digitado errado", `{"process":"P","scenarios":[{"name":"a","steps":[{"start":true,"expcet":{}}]}]}`, `chave desconhecida "expcet"`},
		{"sem process", `{"scenarios":[{"name":"a","steps":[{"start":true}]}]}`, `"process" é obrigatória`},
		{"sem cenários", `{"process":"P","scenarios":[]}`, "pelo menos um cenário"},
		{"cenário sem nome", `{"process":"P","scenarios":[{"steps":[{"start":true}]}]}`, `"name" é obrigatória`},
		{"nome repetido", `{"process":"P","scenarios":[{"name":"a","steps":[{"start":true}]},{"name":"a","steps":[{"start":true}]}]}`, "nome de cenário repetido"},
		{"sem passos", `{"process":"P","scenarios":[{"name":"a","steps":[]}]}`, "pelo menos um passo"},
		{"primeiro não é start", `{"process":"P","scenarios":[{"name":"a","steps":[{"move":5}]}]}`, `primeiro passo do cenário deve ser "start"`},
		{"start repetido", `{"process":"P","scenarios":[{"name":"a","steps":[{"start":true},{"start":true}]}]}`, "só pode ser o primeiro passo"},
		{"duas ações", `{"process":"P","scenarios":[{"name":"a","steps":[{"start":true,"move":5}]}]}`, "mais de uma ação"},
		{"sem ação", `{"process":"P","scenarios":[{"name":"a","steps":[{"start":true},{"name":"x"}]}]}`, "passo sem ação"},
		{"manager com fields", `{"process":"P","scenarios":[{"name":"a","steps":[{"start":true},{"manager":{"target":5,"fields":{"a":"1"}}}]}]}`,
			"passo setFields antes do manager"},
		{"move sem destino", `{"process":"P","scenarios":[{"name":"a","steps":[{"start":true},{"move":{"comment":"x"}}]}]}`, "etapa de destino"},
		{"atores", `{"process":"P","actors":{"dir":"msouza"},"scenarios":[{"name":"a","steps":[{"start":true}]}]}`, "atores entram na fase B"},
		{"as outro ator", `{"process":"P","scenarios":[{"name":"a","steps":[{"start":true,"as":"dir"}]}]}`, `ator "dir" desconhecido`},
		{"holidays", `{"process":"P","holidays":{"dates":[]},"scenarios":[{"name":"a","steps":[{"start":true}]}]}`, "fase B"},
		{"expressão desconhecida", `{"process":"P","scenarios":[{"name":"a","fields":{"x":"{{amanha}}"},"steps":[{"start":true}]}]}`, "{{amanha}} desconhecida"},
		{"workday", `{"process":"P","scenarios":[{"name":"a","fields":{"x":"{{workday(+3)}}"},"steps":[{"start":true}]}]}`, "fase B"},
		{"var indefinida", `{"process":"P","scenarios":[{"name":"a","fields":{"x":"{{var.venc}}"},"steps":[{"start":true}]}]}`, `variável "venc" não definida`},
		{"passo referido antes", `{"process":"P","scenarios":[{"name":"a","steps":[{"start":{"fields":{"x":"{{steps.m.nextState}}"}}},{"move":5,"id":"m"}]}]}`,
			"não roda antes deste"},
		{"expressão sem fechamento", `{"process":"P","scenarios":[{"name":"a","fields":{"x":"{{today"},"steps":[{"start":true}]}]}`, "sem fechamento"},
		{"cleanup desconhecido", `{"process":"P","scenarios":[{"name":"a","cleanup":["apagar"],"steps":[{"start":true}]}]}`, `limpeza "apagar" desconhecida`},
		{"cleanup dataset", `{"process":"P","scenarios":[{"name":"a","cleanup":[{"dataset":"ds"}],"steps":[{"start":true}]}]}`, "fase B"},
		{"asserção fase B", `{"process":"P","scenarios":[{"name":"a","steps":[{"start":true,"expect":{"db":{}}}]}]}`, `asserção "db" ainda não é suportada`},
		{"check sem expect", `{"process":"P","scenarios":[{"name":"a","steps":[{"start":true},{"check":true}]}]}`, "check sem \"expect\""},
		{"operador desconhecido", `{"process":"P","scenarios":[{"name":"a","steps":[{"start":true,"expect":{"card":{"x":{"gt":"1"}}}}]}]}`, `operador "gt" desconhecido`},
		{"dois operadores", `{"process":"P","scenarios":[{"name":"a","steps":[{"start":true,"expect":{"card":{"x":{"eq":"1","ne":"2"}}}}]}]}`, "exatamente um operador"},
		{"regex inválida", `{"process":"P","scenarios":[{"name":"a","steps":[{"start":true,"expect":{"card":{"x":{"matches":"("}}}}]}]}`, "expressão regular inválida"},
		{"status inválido", `{"process":"P","scenarios":[{"name":"a","steps":[{"start":true,"expect":{"status":"ABERTA"}}]}]}`, "OPEN, FINALIZED ou CANCELED"},
		{"wait sem condição", `{"process":"P","scenarios":[{"name":"a","steps":[{"start":true},{"wait":{"timeout":"1m"}}]}]}`, `informe "state" e/ou "status"`},
		{"wait duração", `{"process":"P","scenarios":[{"name":"a","steps":[{"start":true},{"wait":{"state":5,"timeout":"tres"}}]}]}`, "duração"},
		{"campo objeto", `{"process":"P","scenarios":[{"name":"a","fields":{"tbl":{"a":1}},"steps":[{"start":true}]}]}`, "campo___N"},
		{"$extends", `{"process":"P","defaults":{"card":"fixtures/ext.json"},"scenarios":[{"name":"a","steps":[{"start":true}]}]}`, "$extends"},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			path := writeSuite(t, c.suite)
			os.WriteFile(filepath.Join(filepath.Dir(path), "fixtures", "ext.json"), []byte(`{"$extends":"card.json"}`), 0o644)
			_, err := LoadSuite(path, LoadOptions{})
			if err == nil {
				t.Fatalf("esperava erro contendo %q", c.quer)
			}
			if !errors.Is(err, ErrUsage) {
				t.Errorf("esperava ErrUsage, veio %v", err)
			}
			if !strings.Contains(err.Error(), c.quer) {
				t.Errorf("mensagem sem %q:\n%s", c.quer, err)
			}
		})
	}
}

func TestParseSuiteArquivosInexistentes(t *testing.T) {
	if _, err := LoadSuite(filepath.Join(t.TempDir(), "nada.json"), LoadOptions{}); !errors.Is(err, ErrMissing) {
		t.Errorf("suíte inexistente deveria ser ErrMissing: %v", err)
	}
	path := writeSuite(t, `{"process":"P","scenarios":[{"name":"a","card":"fixtures/nao.json","steps":[{"start":true}]}]}`)
	_, err := LoadSuite(path, LoadOptions{})
	if !errors.Is(err, ErrMissing) || !strings.Contains(err.Error(), "nao.json") {
		t.Errorf("fixture inexistente deveria ser ErrMissing com o caminho: %v", err)
	}
}

// --var vence o "vars" do arquivo.
func TestParseSuiteVarDaLinhaDeComandoVence(t *testing.T) {
	s, err := LoadSuite(writeSuite(t, suiteValida), LoadOptions{Vars: map[string]string{"venc": "01/01/2027"}})
	if err != nil {
		t.Fatal(err)
	}
	if s.Vars["venc"] != "01/01/2027" {
		t.Errorf("--var não venceu: %v", s.Vars)
	}
}
