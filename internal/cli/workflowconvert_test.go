package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/alorenco/fluig-cli/internal/output"
)

// convertStub simula o wizard de conversão: REST v2 (versões, etapas e
// solicitações) + a API legada processconvert. O estado por solicitação
// (versão + etapa aberta) muda quando o convertProcess "converte" — é assim
// que a verificação pós-conversão do comando é exercitada.
type convertStub struct {
	instances map[int]*convertStubInstance
	failIDs   map[int]bool // solicitações cujo convertProcess responde erro
	bodies    []string     // corpos recebidos no convertProcess
}

type convertStubInstance struct {
	version  int
	sequence int
	state    string
}

func newConvertStub() *convertStub {
	return &convertStub{
		instances: map[int]*convertStubInstance{
			111: {version: 20, sequence: 17, state: "Aguardar Nota"},
			222: {version: 20, sequence: 66, state: "Intermediário"},
		},
		failIDs: map[int]bool{},
	}
}

func (s *convertStub) server(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	// Etapas por versão (REST v2): a 66 (Intermediário) só existe na origem —
	// é a etapa órfã dos testes de --map. A 30 só existe no destino.
	mux.HandleFunc("/process-management/api/v2/processes/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, "/process-versions/20/states"):
			io.WriteString(w, `{"items":[`+
				`{"sequence":4,"stateName":"Início","stateDescription":"Início","stateType":"SIMPLE","bpmnType":"START_EVENT_NORMAL"},`+
				`{"sequence":17,"stateName":"Aguardar Nota","stateDescription":"Aguardar Nota","stateType":"SIMPLE","bpmnType":"TASK_NORMAL"},`+
				`{"sequence":66,"stateName":"Intermediário","stateDescription":"Intermediário","stateType":"SIMPLE","bpmnType":"TASK_NORMAL"}`+
				`],"hasNext":false}`)
		case strings.HasSuffix(path, "/process-versions/29/states"):
			io.WriteString(w, `{"items":[`+
				`{"sequence":4,"stateName":"Início","stateDescription":"Início","stateType":"SIMPLE","bpmnType":"START_EVENT_NORMAL"},`+
				`{"sequence":17,"stateName":"Aguardar Nota","stateDescription":"Aguardar Nota","stateType":"SIMPLE","bpmnType":"TASK_NORMAL"},`+
				`{"sequence":30,"stateName":"Corrigir Integração","stateDescription":"Corrigir Integração","stateType":"SIMPLE","bpmnType":"TASK_NORMAL"}`+
				`],"hasNext":false}`)
		case strings.HasSuffix(path, "/process-versions/99/states"):
			io.WriteString(w, `{"items":[],"hasNext":false}`)
		case strings.HasSuffix(path, "/process-versions"):
			io.WriteString(w, `{"items":[`+
				`{"version":29,"active":true,"editing":false},`+
				`{"version":27,"active":false,"editing":true},`+
				`{"version":20,"active":false,"editing":false}`+
				`],"hasNext":false}`)
		default:
			http.NotFound(w, r)
		}
	})
	// Solicitação individual (REST v2): versão + etapa aberta atual, no shape
	// activities do Fluig 1.8 (o expand cai nele quando a versão é desconhecida).
	mux.HandleFunc("/process-management/api/v2/requests/", func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/process-management/api/v2/requests/"))
		in, ok := s.instances[id]
		if !ok {
			http.Error(w, `{"message":"não existe"}`, http.StatusNotFound)
			return
		}
		fmt.Fprintf(w, `{"processInstanceId":%d,"processId":"Compras","processVersion":%d,"status":"OPEN",`+
			`"activities":[{"movementSequence":2,"active":true,"state":{"sequence":%d,"stateName":"%s"}}]}`,
			id, in.version, in.sequence, in.state)
	})
	// API legada do wizard.
	mux.HandleFunc("/ecm/api/rest/ecm/processconvert/getAllProcessVersions", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `[{"processId":"Compras","version":20,"openInstances":41},{"processId":"Compras","version":29,"openInstances":95}]`)
	})
	mux.HandleFunc("/ecm/api/rest/ecm/processconvert/getOpenProcessStateVersions", func(w http.ResponseWriter, r *http.Request) {
		var items []string
		counts := map[int]int{}
		for _, in := range s.instances {
			if in.version == 20 {
				counts[in.sequence]++
			}
		}
		for seq, n := range counts {
			items = append(items, fmt.Sprintf(`{"processStatePK":{"companyId":1,"processId":"Compras","version":20,"sequence":%d},"openInstances":%d}`, seq, n))
		}
		io.WriteString(w, `{"content":[`+strings.Join(items, ",")+`]}`)
	})
	mux.HandleFunc("/ecm/api/rest/ecm/processconvert/getInstancesToConvert", func(w http.ResponseWriter, r *http.Request) {
		var items []string
		for id, in := range s.instances {
			if in.version == 20 {
				items = append(items, fmt.Sprintf(`{"processInstanceId":%d,"processDescription":"Compras",`+
					`"requesterName":"João Silva","stateDescription":"%s","colleagueName":"Maria Souza","deadlineText":"Desde 01/09/2024 00:00:00"}`,
					id, in.state))
			}
		}
		io.WriteString(w, `{"totalpages":1,"totalrecords":"`+strconv.Itoa(len(items))+`","currpage":1,"invdata":[`+strings.Join(items, ",")+`]}`)
	})
	mux.HandleFunc("/ecm/api/rest/ecm/processconvert/convertProcess", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s.bodies = append(s.bodies, string(body))
		var call struct {
			ProcessInstanceID int   `json:"processInstanceId"`
			NewVersion        int   `json:"newVersion"`
			ActualStates      []int `json:"actualStates"`
			NewStates         []int `json:"newStates"`
		}
		json.Unmarshal(body, &call)
		if s.failIDs[call.ProcessInstanceID] {
			io.WriteString(w, `{"conversionLog":"Erro ao converter a solicitação: Não foi encontrada a atividade destino correspondente para a atividade 66 . \n","conversionSequence":0}`)
			return
		}
		if in, ok := s.instances[call.ProcessInstanceID]; ok {
			in.version = call.NewVersion // a conversão "acontece" — a verificação vê a versão nova
			for i, seq := range call.ActualStates {
				if seq == in.sequence {
					in.sequence = call.NewStates[i]
					break
				}
			}
		}
		io.WriteString(w, `{"conversionLog":"Solicitação convertida com sucesso!","conversionSequence":0}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// Sem --from/--to: tabela de versões com as tarefas abertas (a versão em
// edição fica com "-", fora da conversão).
func TestWorkflowConvertVersionsTabela(t *testing.T) {
	stub := newConvertStub()
	proj := workflowProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "workflow", "convert", "Compras", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	for _, want := range []string{"│", "Versão", "Ativa", "Tarefas abertas", "29", "95", "20", "41", "-"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("tabela sem %q:\n%s", want, stdout)
		}
	}
}

// Plano (--from/--to sem seleção): de-para com a etapa órfã marcada, tabela de
// solicitações e nada convertido.
func TestWorkflowConvertPlanTabela(t *testing.T) {
	stub := newConvertStub()
	proj := workflowProject(t, stub.server(t).URL)
	code, stdout, stderr := runMainStderr(t, "workflow", "convert", "Compras", "--from", "20", "--to", "29",
		"--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	for _, want := range []string{"Etapa", "Destino", "sem destino", "Aguardar Nota", "Solicitação", "111", "222"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("plano sem %q:\n%s", want, stdout)
		}
	}
	if len(stub.bodies) != 0 {
		t.Errorf("plano não pode converter; convertProcess foi chamado %d vez(es)", len(stub.bodies))
	}
	if !strings.Contains(stdout+stderr, "--map") {
		t.Errorf("plano com etapa órfã aberta deveria apontar o --map:\n%s%s", stdout, stderr)
	}
}

// Contrato --json do plano: mapping com identidade + órfã, instances completas.
func TestWorkflowConvertPlanJSON(t *testing.T) {
	stub := newConvertStub()
	proj := workflowProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "workflow", "convert", "Compras", "--from", "20", "--to", "29", "--json",
		"--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("stdout não é um envelope JSON: %v\n%s", err, stdout)
	}
	data, _ := env.Data.(map[string]any)
	mapping, _ := data["mapping"].([]any)
	if len(mapping) != 3 {
		t.Fatalf("esperava 3 etapas no mapping, veio %d", len(mapping))
	}
	byFrom := map[float64]map[string]any{}
	for _, m := range mapping {
		mm, _ := m.(map[string]any)
		byFrom[mm["from"].(float64)] = mm
	}
	if byFrom[17]["to"] != float64(17) {
		t.Errorf("etapa 17 deveria mapear por identidade: %+v", byFrom[17])
	}
	if _, temDestino := byFrom[66]["to"]; temDestino {
		t.Errorf("etapa 66 é órfã e não deveria ter destino: %+v", byFrom[66])
	}
	if inst, _ := data["instances"].([]any); len(inst) != 2 {
		t.Errorf("esperava 2 solicitações, veio %d", len(inst))
	}
}

// Conversão de uma solicitação em etapa mapeada: sucesso verificado pela
// versão nova, arrays paralelos corretos no POST.
func TestWorkflowConvertInstanceOK(t *testing.T) {
	stub := newConvertStub()
	proj := workflowProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "workflow", "convert", "Compras", "--from", "20", "--to", "29",
		"--instance", "111", "--yes", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	if len(stub.bodies) != 1 {
		t.Fatalf("esperava 1 convertProcess, veio %d", len(stub.bodies))
	}
	for _, want := range []string{`"processInstanceId":111`, `"newVersion":29`, `"actualStates":[4,17]`, `"newStates":[4,17]`} {
		if !strings.Contains(stub.bodies[0], want) {
			t.Errorf("corpo sem %s: %s", want, stub.bodies[0])
		}
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	results, _ := data["results"].([]any)
	if len(results) != 1 {
		t.Fatalf("esperava 1 resultado, veio %d", len(results))
	}
	first, _ := results[0].(map[string]any)
	if first["action"] != "converted" || first["success"] != true {
		t.Errorf("resultado inesperado: %+v", first)
	}
}

// Etapa aberta órfã sem --map trava ANTES de qualquer POST (com --instance a
// etapa vem da própria solicitação).
func TestWorkflowConvertPrecheckOrfa(t *testing.T) {
	stub := newConvertStub()
	proj := workflowProject(t, stub.server(t).URL)
	code, _, stderr := runMainStderr(t, "workflow", "convert", "Compras", "--from", "20", "--to", "29",
		"--instance", "222", "--yes", "--project", proj, "--server", "homolog")
	if code != output.ExitUsage {
		t.Fatalf("esperava exit %d, veio %d (stderr=%s)", output.ExitUsage, code, stderr)
	}
	if !strings.Contains(stderr, "66") || !strings.Contains(stderr, "--map") {
		t.Errorf("erro deveria apontar a etapa 66 e o --map: %s", stderr)
	}
	if len(stub.bodies) != 0 {
		t.Errorf("pré-checagem falhou mas convertProcess foi chamado %d vez(es)", len(stub.bodies))
	}
}

// --all com etapa aberta órfã: a pré-checagem usa a contagem por etapa e trava
// antes do primeiro POST.
func TestWorkflowConvertAllPrecheckOrfa(t *testing.T) {
	stub := newConvertStub()
	proj := workflowProject(t, stub.server(t).URL)
	code, _, stderr := runMainStderr(t, "workflow", "convert", "Compras", "--from", "20", "--to", "29",
		"--all", "--yes", "--project", proj, "--server", "homolog")
	if code != output.ExitUsage {
		t.Fatalf("esperava exit %d, veio %d (stderr=%s)", output.ExitUsage, code, stderr)
	}
	if len(stub.bodies) != 0 {
		t.Errorf("pré-checagem falhou mas convertProcess foi chamado %d vez(es)", len(stub.bodies))
	}
}

// --all com --map completo: converte as duas; uma falha no servidor → exit 6 e
// results[] com o erro do conversionLog.
func TestWorkflowConvertAllParcial(t *testing.T) {
	stub := newConvertStub()
	stub.failIDs[222] = true
	proj := workflowProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "workflow", "convert", "Compras", "--from", "20", "--to", "29",
		"--all", "--map", "66=30", "--yes", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitPartial {
		t.Fatalf("esperava exit %d, veio %d\n%s", output.ExitPartial, code, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	results, _ := data["results"].([]any)
	if len(results) != 2 {
		t.Fatalf("esperava 2 resultados, veio %d", len(results))
	}
	byID := map[string]map[string]any{}
	for _, r := range results {
		rr, _ := r.(map[string]any)
		byID[rr["id"].(string)] = rr
	}
	if byID["111"]["action"] != "converted" {
		t.Errorf("111 deveria converter: %+v", byID["111"])
	}
	if byID["222"]["action"] != "failed" || !strings.Contains(byID["222"]["error"].(string), "atividade destino") {
		t.Errorf("222 deveria falhar com o texto do servidor: %+v", byID["222"])
	}
}

// Erros de uso: combinações inválidas de flags e --map malformado/inexistente.
func TestWorkflowConvertUsage(t *testing.T) {
	stub := newConvertStub()
	proj := workflowProject(t, stub.server(t).URL)
	base := []string{"workflow", "convert", "Compras", "--project", proj, "--server", "homolog"}
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"from sem to", []string{"--from", "20"}, "--from e --to andam juntos"},
		{"all com instance", []string{"--from", "20", "--to", "29", "--all", "--instance", "111"}, "não se combinam"},
		{"selecao sem versoes", []string{"--all"}, "exigem --from e --to"},
		{"map sem versoes", []string{"--map", "66=30"}, "--map exige"},
		{"from igual to", []string{"--from", "20", "--to", "20"}, "versões diferentes"},
		{"map malformado", []string{"--from", "20", "--to", "29", "--map", "66-30"}, "etapaOrigem=etapaDestino"},
		{"map destino inexistente", []string{"--from", "20", "--to", "29", "--map", "66=99"}, "não existe na versão 29"},
		{"map origem inexistente", []string{"--from", "20", "--to", "29", "--map", "55=30"}, "não existe na versão 20"},
		{"versao destino inexistente", []string{"--from", "20", "--to", "99"}, "não existe"},
	}
	for _, tc := range cases {
		code, _, stderr := runMainStderr(t, append(base, tc.args...)...)
		if code != output.ExitUsage {
			t.Errorf("%s: esperava exit %d, veio %d (stderr=%s)", tc.name, output.ExitUsage, code, stderr)
			continue
		}
		if !strings.Contains(stderr, tc.want) {
			t.Errorf("%s: erro sem %q: %s", tc.name, tc.want, stderr)
		}
	}
}

// Solicitação que não está aberta na versão de origem: NOT_FOUND antes de
// qualquer POST.
func TestWorkflowConvertInstanceInexistente(t *testing.T) {
	stub := newConvertStub()
	proj := workflowProject(t, stub.server(t).URL)
	code, _, stderr := runMainStderr(t, "workflow", "convert", "Compras", "--from", "20", "--to", "29",
		"--instance", "999", "--yes", "--project", proj, "--server", "homolog")
	if code != output.ExitNotFound {
		t.Fatalf("esperava exit %d, veio %d (stderr=%s)", output.ExitNotFound, code, stderr)
	}
	if !strings.Contains(stderr, "999") {
		t.Errorf("erro deveria citar a solicitação 999: %s", stderr)
	}
	if len(stub.bodies) != 0 {
		t.Errorf("convertProcess não deveria ter sido chamado")
	}
}
