package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alorenco/fluig-cli/internal/config"
	"github.com/alorenco/fluig-cli/internal/output"
)

// documentStub simula o GED: raízes via SOAP (fixture real), conteúdo de
// pasta via REST (fixture real sanitizada), metadados/stream/upload/delete.
type documentStub struct {
	moveBody string // envelope recebido no moveDocument
	uploadedName string
	uploadedBody []byte
	mkdirBody    string
	deleted      []string
}

func (s *documentStub) server(t *testing.T) *httptest.Server {
	readTD := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	mux.HandleFunc("/portal/api/rest/wcmservice/rest/user/findUserByLogin", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"content":{"login":"u","userCode":"uc"}}`)
	})
	mux.HandleFunc("/webdesk/ECMDocumentService", func(w http.ResponseWriter, r *http.Request) {
		// moveDocument: "[OK] - <id>" = sucesso; "[NOK] - msg" = recusa
		// (semântica validada ao vivo em 2026-08-07).
		if r.Header.Get("SOAPAction") != "moveDocument" {
			http.Error(w, "op?", 500)
			return
		}
		b, _ := io.ReadAll(r.Body)
		s.moveBody = string(b)
		result := "[OK] - 103"
		if strings.Contains(s.moveBody, "<destFolderId>999</destFolderId>") {
			result = "[NOK] - A pasta destino não pode ser localizada. Destino: 999"
		}
		w.Header().Set("Content-Type", "text/xml")
		io.WriteString(w, `<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body>`+
			`<ns2:moveDocumentResponse xmlns:ns2="http://ws.dm.ecm.technology.totvs.com/"><result>`+result+
			`</result></ns2:moveDocumentResponse></soap:Body></soap:Envelope>`)
	})
	mux.HandleFunc("/webdesk/ECMFolderService", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		if r.Header.Get("SOAPAction") == "getRootFolders" {
			w.Write(readTD("soap_rootFolders.xml"))
			return
		}
		http.Error(w, "op?", 500)
	})
	mux.HandleFunc("/content-management/api/v2/folders/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/44/documents"):
			// A API exige `order`; o cliente sempre manda.
			if r.URL.Query().Get("order") == "" {
				io.WriteString(w, `{"totalpages":1,"totalrecords":"0","currpage":1,"invdata":[]}`)
				return
			}
			w.Write(readTD("rest_ged_documents.json"))
		// Árvore sintética para o --recursive/find: 100 → {pasta 101, arq A} e
		// 101 → {Notificação nº 7.pdf}. Shape mínimo do invdata real.
		case strings.HasSuffix(r.URL.Path, "/100/documents"):
			io.WriteString(w, `{"totalpages":1,"currpage":1,"invdata":[
				{"documentId":101,"documentType":"1","documentDescription":"Contratos","version":1000,"parentDocumentId":100,"publisherName":"João Silva"},
				{"documentId":102,"documentType":"2","documentDescription":"leia-me.txt","version":1000,"parentDocumentId":100,"publisherName":"João Silva","size":0.001}]}`)
		case strings.HasSuffix(r.URL.Path, "/101/documents"):
			io.WriteString(w, `{"totalpages":1,"currpage":1,"invdata":[
				{"documentId":103,"documentType":"2","documentDescription":"Notificação nº 7.pdf","version":2000,"parentDocumentId":101,"publisherName":"João Silva","size":0.2}]}`)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/44"):
			b, _ := io.ReadAll(r.Body)
			s.mkdirBody = string(b)
			io.WriteString(w, `{"documentId":1111277,"documentDescription":"zz_fluigcli_test_ged","folderId":44}`)
		default:
			http.Error(w, `{"code":"NotFound","message":"pasta não encontrada"}`, http.StatusNotFound)
		}
	})
	mux.HandleFunc("/content-management/api/v2/documents/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/content-management/api/v2/documents/")
		switch {
		case r.Method == http.MethodPost && strings.HasPrefix(path, "upload/"):
			s.uploadedName = strings.Split(path, "/")[1]
			r.ParseMultipartForm(1 << 20)
			f, _, _ := r.FormFile("file")
			s.uploadedBody, _ = io.ReadAll(f)
			io.WriteString(w, `{"documentId":1111278,"version":1000,"documentDescription":"`+s.uploadedName+`","folderId":1111277}`)
		case r.Method == http.MethodDelete:
			s.deleted = append(s.deleted, path)
			w.WriteHeader(http.StatusNoContent)
		case path == "103":
			io.WriteString(w, `{"companyId":1,"id":103,"version":2000,"type":"FileDocument","description":"Notificação nº 7.pdf","parentId":101}`)
		case path == "101":
			io.WriteString(w, `{"companyId":1,"id":101,"version":1000,"type":"Folder","description":"Contratos","parentId":100}`)
		case path == "926468":
			// Metadados reais (homolog 2026-07-14).
			io.WriteString(w, `{"companyId":1,"id":926468,"version":1000,"type":"FileDocument","description":"manual.pdf","parentId":962589,"downloadEnabled":true}`)
		case path == "926468/stream":
			if r.Header.Get("Accept") == "application/json" {
				// Comportamento real: o stream rejeita Accept json (406).
				http.Error(w, `{"code":"NotAcceptableException"}`, http.StatusNotAcceptable)
				return
			}
			w.Write([]byte("BYTES-DO-PDF"))
		case path == "999999" || path == "999999/stream":
			http.Error(w, `{"code":"NotFound","message":"documento não encontrado"}`, http.StatusNotFound)
		case strings.HasSuffix(path, "/stream"):
			// Arquivo físico ausente no volume (registro órfão — caso real).
			http.Error(w, `{"code":"NoSuchFileException","message":"","detailedMessage":""}`, http.StatusInternalServerError)
		default:
			io.WriteString(w, `{"companyId":1,"id":`+path+`,"version":1000,"type":"FileDocument","description":"orfao.bin","parentId":1,"downloadEnabled":true}`)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func documentProject(t *testing.T, stubURL string) string {
	t.Helper()
	u := mustParseHostPort(t, stubURL)
	proj := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv(config.EnvPassword, "p")
	server := config.Server{ID: "doc-srv", Name: "homolog", Host: u.host, Port: u.port, SSL: false, Username: "u", CompanyID: 1}
	if err := config.NewStore(proj).Add(server, false); err != nil {
		t.Fatal(err)
	}
	return proj
}

// list sem argumento: pastas raiz via SOAP.
func TestDocumentListRaizes(t *testing.T) {
	stub := &documentStub{}
	proj := documentProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "document", "list", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	for _, want := range []string{"│", "ID", "Pasta", "2864", "01 - Documentos Oficiais"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("tabela sem %q:\n%s", want, stdout)
		}
	}
}

// list de pasta: mistura real de pasta/arquivo/artigo, com tipos traduzidos.
func TestDocumentListPasta(t *testing.T) {
	stub := &documentStub{}
	proj := documentProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "document", "list", "44", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	for _, want := range []string{"pasta", "arquivo", "artigo", "zz_fluigcli_test_ged", "Ana Andrade", "926468"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("tabela sem %q:\n%s", want, stdout)
		}
	}

	code, stdout = runMain(t, "document", "list", "44", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("--json exit=%d", code)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	items, _ := data["items"].([]any)
	if len(items) != 4 {
		t.Fatalf("esperava 4 itens, veio %d", len(items))
	}
	first, _ := items[0].(map[string]any)
	if first["type"] != "folder" || first["description"] != "zz_fluigcli_test_ged" {
		t.Errorf("item[0] inesperado: %+v", first)
	}
}

// download: usa o nome dos metadados e grava byte a byte.
func TestDocumentDownload(t *testing.T) {
	stub := &documentStub{}
	proj := documentProject(t, stub.server(t).URL)
	dir := t.TempDir()
	code, stdout := runMain(t, "document", "download", "926468", "--dir", dir, "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	got, err := os.ReadFile(filepath.Join(dir, "manual.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "BYTES-DO-PDF" {
		t.Errorf("conteúdo inesperado: %q", got)
	}

	// Inexistente → exit 4; arquivo físico ausente → exit 5 com mensagem clara.
	code, _ = runMain(t, "document", "download", "999999", "--dir", dir, "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitNotFound {
		t.Errorf("inexistente: exit=%d, quer %d", code, output.ExitNotFound)
	}
	code, stdout = runMain(t, "document", "download", "111", "--dir", dir, "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitServer {
		t.Errorf("órfão: exit=%d, quer %d", code, output.ExitServer)
	}
	if !strings.Contains(stdout, "não existe no volume") {
		t.Errorf("mensagem do órfão deveria explicar o NoSuchFile:\n%s", stdout)
	}
}

// upload: multipart com o conteúdo e publish em uma etapa.
func TestDocumentUpload(t *testing.T) {
	stub := &documentStub{}
	proj := documentProject(t, stub.server(t).URL)
	file := filepath.Join(t.TempDir(), "relatorio.txt")
	os.WriteFile(file, []byte("conteudo do relatorio"), 0o644)

	code, stdout := runMain(t, "document", "upload", file, "--folder", "1111277", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	if stub.uploadedName != "relatorio.txt" || string(stub.uploadedBody) != "conteudo do relatorio" {
		t.Errorf("upload inesperado: nome=%q corpo=%q", stub.uploadedName, stub.uploadedBody)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	results, _ := data["results"].([]any)
	first, _ := results[0].(map[string]any)
	if first["id"] != "1111278" || first["action"] != "published" {
		t.Errorf("results inesperado: %+v", results)
	}

	// Sem --folder: erro de uso.
	code, _ = runMain(t, "document", "upload", file, "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitUsage {
		t.Errorf("sem --folder: exit=%d, quer %d", code, output.ExitUsage)
	}
}

// mkdir + delete: criação de pasta e lixeira (com --yes).
func TestDocumentMkdirDelete(t *testing.T) {
	stub := &documentStub{}
	proj := documentProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "document", "mkdir", "44", "zz_fluigcli_test_ged", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("mkdir exit=%d stdout=%s", code, stdout)
	}
	if !strings.Contains(stub.mkdirBody, `"alias":"zz_fluigcli_test_ged"`) {
		t.Errorf("corpo do mkdir inesperado: %s", stub.mkdirBody)
	}

	code, _ = runMain(t, "document", "delete", "1111278", "1111277", "--yes", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("delete exit=%d", code)
	}
	if len(stub.deleted) != 2 || stub.deleted[0] != "1111278" {
		t.Errorf("deletes inesperados: %v", stub.deleted)
	}

	// delete sem --yes em modo não-interativo: exit 2, sem tocar o servidor.
	code, _ = runMain(t, "document", "delete", "1", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitUsage {
		t.Errorf("sem confirmação: exit=%d, quer %d", code, output.ExitUsage)
	}
}

// --- document list --recursive / document find (ROADMAP3 §4.15) ---

func TestDocumentListRecursive(t *testing.T) {
	stub := &documentStub{}
	proj := documentProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "document", "list", "100", "--recursive",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	items, _ := data["items"].([]any)
	if len(items) != 3 {
		t.Fatalf("esperava 3 itens (pasta + 2 arquivos), veio %d\n%s", len(items), stdout)
	}
	// O caminho do arquivo aninhado inclui a pasta pai.
	achou := false
	for _, raw := range items {
		it, _ := raw.(map[string]any)
		if it["path"] == "Contratos/Notificação nº 7.pdf" {
			achou = true
			if it["depth"].(float64) != 2 {
				t.Errorf("depth do aninhado: %+v", it)
			}
		}
	}
	if !achou {
		t.Errorf("caminho aninhado ausente: %s", stdout)
	}
	if data["truncated"] != false {
		t.Errorf("varredura pequena não pode truncar: %+v", data)
	}
}

func TestDocumentListRecursiveDepth1(t *testing.T) {
	stub := &documentStub{}
	proj := documentProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "document", "list", "100", "--recursive", "--depth", "1",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d", code)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	if items, _ := data["items"].([]any); len(items) != 2 {
		t.Errorf("--depth 1 = só o conteúdo direto (2 itens): %s", stdout)
	}
}

func TestDocumentListRecursiveExigeFolder(t *testing.T) {
	stub := &documentStub{}
	proj := documentProject(t, stub.server(t).URL)
	code, _ := runMain(t, "document", "list", "--recursive", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitUsage {
		t.Errorf("exit=%d, quer %d", code, output.ExitUsage)
	}
}

func TestDocumentFind(t *testing.T) {
	stub := &documentStub{}
	proj := documentProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "document", "find", "--name", "notificação nº*", "--under", "100",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	items, _ := data["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("o glob (case-insensitive) deveria achar só o PDF: %s", stdout)
	}
	it, _ := items[0].(map[string]any)
	if it["id"].(float64) != 103 || it["path"] != "Contratos/Notificação nº 7.pdf" {
		t.Errorf("item inesperado: %+v", it)
	}
}

func TestDocumentFindSemFlags(t *testing.T) {
	stub := &documentStub{}
	proj := documentProject(t, stub.server(t).URL)
	if code, _ := runMain(t, "document", "find", "--name", "x*", "--json", "--project", proj, "--server", "homolog"); code != output.ExitUsage {
		t.Errorf("sem --under: exit=%d, quer %d", code, output.ExitUsage)
	}
	if code, _ := runMain(t, "document", "find", "--under", "100", "--json", "--project", proj, "--server", "homolog"); code != output.ExitUsage {
		t.Errorf("sem --name: exit=%d, quer %d", code, output.ExitUsage)
	}
}

// --- document show / document move (ROADMAP3 §4.16) ---

func TestDocumentShow(t *testing.T) {
	stub := &documentStub{}
	proj := documentProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "document", "show", "103",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	if data["parentId"].(float64) != 101 || data["parentName"] != "Contratos" {
		t.Errorf("pai não resolvido: %+v", data)
	}
	// Alias de id (consistência entre comandos — §4.18).
	if data["id"].(float64) != 103 || data["documentId"].(float64) != 103 {
		t.Errorf("id e documentId devem ser sinônimos: %+v", data)
	}
}

func TestDocumentMove(t *testing.T) {
	stub := &documentStub{}
	proj := documentProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "document", "move", "103", "--folder", "101",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	for _, want := range []string{"<item>103</item>", "<destFolderId>101</destFolderId>", "<colleagueId>uc</colleagueId>"} {
		if !strings.Contains(stub.moveBody, want) {
			t.Errorf("envelope sem %s:\n%s", want, stub.moveBody)
		}
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	results, _ := data["results"].([]any)
	first, _ := results[0].(map[string]any)
	// A confirmação relê o parentId (o stub devolve 101 = destino).
	if first["success"] != true {
		t.Errorf("resultado: %+v", first)
	}
}

// "[NOK] - msg" vira recusa com a mensagem do servidor.
func TestDocumentMoveRecusado(t *testing.T) {
	stub := &documentStub{}
	proj := documentProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "document", "move", "103", "--folder", "999",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitServer {
		t.Fatalf("exit=%d, quer %d\n%s", code, output.ExitServer, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	if env.Error == nil || !strings.Contains(env.Error.Message, "pasta destino") {
		t.Errorf("mensagem da recusa: %+v", env.Error)
	}
}

func TestDocumentMoveSemFolder(t *testing.T) {
	stub := &documentStub{}
	proj := documentProject(t, stub.server(t).URL)
	if code, _ := runMain(t, "document", "move", "103", "--json", "--project", proj, "--server", "homolog"); code != output.ExitUsage {
		t.Errorf("sem --folder: exit=%d, quer %d", code, output.ExitUsage)
	}
}

// §4.18 — id e documentId são sinônimos nos envelopes de list (o mesmo alias
// já existe no show). O percorredor de agente não pode quebrar pelo nome.
func TestDocumentListAliasDeID(t *testing.T) {
	stub := &documentStub{}
	proj := documentProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "document", "list", "100",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d", code)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	items, _ := data["items"].([]any)
	first, _ := items[0].(map[string]any)
	if first["id"] != first["documentId"] || first["id"] == nil {
		t.Errorf("id e documentId devem ser sinônimos: %+v", first)
	}
}
