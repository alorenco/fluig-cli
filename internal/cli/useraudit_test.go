package cli

import (
	"archive/zip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alorenco/fluig-cli/internal/config"
	"github.com/alorenco/fluig-cli/internal/output"
)

// --- testes de unidade das funções puras ---

func TestResolveAuditPeriod(t *testing.T) {
	// --day
	f, to, err := resolveAuditPeriod("03/07/2026", "", "")
	if err != nil || f.Format("2006-01-02") != "2026-07-03" || !to.Equal(f) {
		t.Fatalf("--day: f=%v to=%v err=%v", f, to, err)
	}
	// aaaa-mm-dd + intervalo
	f, to, err = resolveAuditPeriod("", "2026-07-01", "2026-07-03")
	if err != nil || f.Format("2006-01-02") != "2026-07-01" || to.Format("2006-01-02") != "2026-07-03" {
		t.Fatalf("intervalo: f=%v to=%v err=%v", f, to, err)
	}
	// só --from → um dia
	f, to, err = resolveAuditPeriod("", "05/07/2026", "")
	if err != nil || !to.Equal(f) || f.Format("2006-01-02") != "2026-07-05" {
		t.Fatalf("só from: f=%v to=%v err=%v", f, to, err)
	}
	// sem nada → hoje (from==to)
	f, to, err = resolveAuditPeriod("", "", "")
	if err != nil || !to.Equal(f) {
		t.Fatalf("default hoje: f=%v to=%v err=%v", f, to, err)
	}
	// erros
	if _, _, err := resolveAuditPeriod("03/07/2026", "2026-07-01", ""); err == nil {
		t.Error("--day + --from deveria falhar")
	}
	if _, _, err := resolveAuditPeriod("", "2026-07-05", "2026-07-01"); err == nil {
		t.Error("--to anterior a --from deveria falhar")
	}
	if _, _, err := resolveAuditPeriod("31/13/2026", "", ""); err == nil {
		t.Error("data inválida deveria falhar")
	}
}

func TestParseAuditOnly(t *testing.T) {
	all, err := parseAuditOnly("")
	if err != nil || !all["tasks"] || !all["requests"] || !all["documents"] {
		t.Fatalf("vazio = todas: %v err=%v", all, err)
	}
	sub, err := parseAuditOnly("tasks, requests")
	if err != nil || !sub["tasks"] || !sub["requests"] || sub["documents"] {
		t.Fatalf("subset: %v err=%v", sub, err)
	}
	if _, err := parseAuditOnly("foo"); err == nil {
		t.Error("dimensão inválida deveria falhar")
	}
}

// --- teste ponta a ponta (stub das 3 dimensões) ---

type auditStub struct {
	taskQuery url.Values
	reqQuery  url.Values
	dsHits    int
}

func (s *auditStub) server(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	mux.HandleFunc("/api/public/wcm/version", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"value":"TOTVS Fluig Plataforma - Voyager 2.0.0-260707"}`)
	})
	mux.HandleFunc("/portal/api/rest/wcmservice/rest/user/findUserByLogin", func(w http.ResponseWriter, r *http.Request) {
		login := r.URL.Query().Get("login")
		io.WriteString(w, `{"content":{"login":"`+login+`","fullName":"João Silva","userCode":"uc-`+login+`"}}`)
	})
	mux.HandleFunc("/process-management/api/v2/tasks", func(w http.ResponseWriter, r *http.Request) {
		s.taskQuery = r.URL.Query()
		io.WriteString(w, `{"items":[{"processInstanceId":228655,"processId":"compras_entrada_documento","movementSequence":1,"assignee":{"name":"João Silva","login":"jsilva","code":"uc-jsilva"},"status":"COMPLETED","slaStatus":"ON_TIME","startDate":"2026-07-03T09:54:03.000-04:00","endDate":"2026-07-03T09:54:05.000-04:00","state":{"sequence":76,"stateName":"Passar por Revisão"}}],"hasNext":false}`)
	})
	mux.HandleFunc("/process-management/api/v2/requests", func(w http.ResponseWriter, r *http.Request) {
		s.reqQuery = r.URL.Query()
		io.WriteString(w, `{"items":[{"processInstanceId":228655,"processId":"compras_entrada_documento","status":"FINALIZED","slaStatus":"ON_TIME","requester":{"name":"João Silva","login":"jsilva","code":"uc-jsilva"},"startDate":"2026-07-03T09:54:03.000-04:00"}],"hasNext":false}`)
	})
	mux.HandleFunc("/dataset/api/v2/dataset-handle/search", func(w http.ResponseWriter, r *http.Request) {
		s.dsHits++
		// 2026-07-03 00:00:00 UTC em epoch millis (createDate é data-only UTC).
		cd := "1783036800000"
		io.WriteString(w, `{"columns":["documentPK.documentId","documentPK.version","documentDescription","documentType","createDate","deleted"],"values":[`+
			`{"documentPK.documentId":1237992,"documentPK.version":1000,"documentDescription":"APC - NF - N° 918","documentType":"7","createDate":"`+cd+`","deleted":false}]}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func userAuditProject(t *testing.T, stubURL string) string {
	t.Helper()
	u := mustParseHostPort(t, stubURL)
	proj := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv(config.EnvPassword, "p")
	server := config.Server{ID: "aud-srv", Name: "producao", Host: u.host, Port: u.port, SSL: false, Username: "admin", CompanyID: 1}
	if err := config.NewStore(proj).Add(server, false); err != nil {
		t.Fatal(err)
	}
	return proj
}

func TestUserAuditTabela(t *testing.T) {
	stub := &auditStub{}
	proj := userAuditProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "user", "audit", "jsilva", "--day", "03/07/2026", "--project", proj, "--server", "producao")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	// Filtros de data server-side chegaram nas duas rotas.
	if got := stub.taskQuery.Get("initialAssignEndDate"); got != "2026-07-03T00:00:00" {
		t.Errorf("task initialAssignEndDate=%q", got)
	}
	if got := stub.taskQuery.Get("finalAssignEndDate"); got != "2026-07-03T23:59:59" {
		t.Errorf("task finalAssignEndDate=%q", got)
	}
	if got := stub.reqQuery.Get("initialStartDate"); got != "2026-07-03T00:00:00" {
		t.Errorf("request initialStartDate=%q", got)
	}
	if stub.reqQuery.Get("requester") != "uc-jsilva" {
		t.Errorf("request requester deveria ser o userCode: %q", stub.reqQuery.Get("requester"))
	}
	for _, want := range []string{
		"Auditoria de João Silva (jsilva)", "producao", "03/07/2026",
		"Tarefas atuadas (1)", "Solicitações abertas (1)", "Documentos criados (1)",
		"2026-07-03 09:54:05", "228655", "APC - NF - N° 918",
		"Resumo: 1 tarefa(s) · 1 solicitação(ões) · 1 documento(s)",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("saída sem %q:\n%s", want, stdout)
		}
	}
}

func TestUserAuditJSONeOnly(t *testing.T) {
	stub := &auditStub{}
	proj := userAuditProject(t, stub.server(t).URL)
	// --only tasks: não deve consultar documentos (dataset) nem solicitações.
	code, stdout := runMain(t, "user", "audit", "jsilva", "--day", "2026-07-03",
		"--only", "tasks", "--json", "--project", proj, "--server", "producao")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	if stub.dsHits != 0 {
		t.Errorf("--only tasks não deveria consultar o dataset de documentos (%d hits)", stub.dsHits)
	}
	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("json inválido: %v\n%s", err, stdout)
	}
	data, _ := env.Data.(map[string]any)
	totals, _ := data["totals"].(map[string]any)
	if totals["tasks"].(float64) != 1 || totals["requests"].(float64) != 0 || totals["documents"].(float64) != 0 {
		t.Errorf("totais inesperados: %v", totals)
	}
	if data["from"] != "2026-07-03" || data["to"] != "2026-07-03" {
		t.Errorf("período no JSON inesperado: from=%v to=%v", data["from"], data["to"])
	}
}

func TestAuditOutputFormat(t *testing.T) {
	if _, err := auditOutputFormat(""); err != nil {
		t.Errorf("sem --output não deveria falhar: %v", err)
	}
	if f, _ := auditOutputFormat("/tmp/a.TXT"); f != "txt" {
		t.Errorf("txt (maiúsc.) => %q", f)
	}
	if f, _ := auditOutputFormat("rel.xlsx"); f != "xlsx" {
		t.Errorf("xlsx => %q", f)
	}
	if _, err := auditOutputFormat("rel.pdf"); err == nil {
		t.Error("extensão não suportada deveria falhar")
	}
}

// --output grava o relatório em .txt e .xlsx.
func TestUserAuditArquivo(t *testing.T) {
	stub := &auditStub{}
	proj := userAuditProject(t, stub.server(t).URL)
	dir := t.TempDir()

	txt := filepath.Join(dir, "aud.txt")
	code, _ := runMain(t, "user", "audit", "jsilva", "--day", "03/07/2026", "-o", txt, "--project", proj, "--server", "producao")
	if code != output.ExitOK {
		t.Fatalf("txt exit=%d", code)
	}
	body, err := os.ReadFile(txt)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Auditoria de João Silva (jsilva)", "Tarefas atuadas (1)", "APC - NF - N° 918", "Resumo:"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("txt sem %q:\n%s", want, body)
		}
	}

	xlsx := filepath.Join(dir, "aud.xlsx")
	code, _ = runMain(t, "user", "audit", "jsilva", "--day", "03/07/2026", "-o", xlsx, "--project", proj, "--server", "producao")
	if code != output.ExitOK {
		t.Fatalf("xlsx exit=%d", code)
	}
	info, err := os.Stat(xlsx)
	if err != nil || info.Size() == 0 {
		t.Fatalf("xlsx não gerado: %v", err)
	}
	zr, err := zip.OpenReader(xlsx)
	if err != nil {
		t.Fatalf("xlsx não é zip válido: %v", err)
	}
	defer zr.Close()
	var hasWorkbook, hasSheet3 bool
	for _, f := range zr.File {
		switch f.Name {
		case "xl/workbook.xml":
			hasWorkbook = true
		case "xl/worksheets/sheet4.xml": // Resumo + 3 dimensões
			hasSheet3 = true
		}
	}
	if !hasWorkbook || !hasSheet3 {
		t.Errorf("xlsx incompleto (workbook=%v sheet4=%v)", hasWorkbook, hasSheet3)
	}

	// extensão inválida
	code, _ = runMain(t, "user", "audit", "jsilva", "--day", "03/07/2026", "-o", filepath.Join(dir, "x.pdf"), "--project", proj, "--server", "producao")
	if code != output.ExitUsage {
		t.Errorf("extensão inválida: exit=%d, quer %d", code, output.ExitUsage)
	}
}
