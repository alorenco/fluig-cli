package cli

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alorenco/fluig-cli/internal/output"
)

// layoutExportProject monta um projeto com o layout mínimo publicável em
// wcm/layout/<code>, com o application.info informado.
func layoutExportProject(t *testing.T, stub *widgetStub, code, info string) string {
	t.Helper()
	proj := widgetProject(t, stub.server(t).URL)
	base := filepath.Join(proj, "wcm", "layout", code)
	write := func(rel string, content []byte) {
		p := filepath.Join(base, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("src/main/webapp/WEB-INF/jboss-web.xml", []byte("<jboss-web/>"))
	write("src/main/resources/layout.ftl", []byte("<#import \"/wcm.ftl\" as wcm />"))
	write("src/main/webapp/resources/js/"+code+".js", []byte("console.log(1)"))
	write("src/main/webapp/resources/images/icon.png", widgetBinary)
	if info != "" {
		write("src/main/resources/application.info", []byte(info))
	}
	return proj
}

// O export empacota a pasta do layout com o MESMO mapa do widget e envia
// <code>.war pelo deploy nativo.
func TestLayoutExportPacks(t *testing.T) {
	stub := &widgetStub{}
	proj := layoutExportProject(t, stub, "meu_layout", "application.type=layout\napplication.code=meu_layout\n")

	code, out := runMain(t, "layout", "export", "meu_layout", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d; saída: %s", code, out)
	}
	var env struct {
		Data struct {
			Layout string `json:"layout"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil || env.Data.Layout != "meu_layout" {
		t.Errorf("envelope inesperado: %s (%v)", out, err)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.uploadedName != "meu_layout.war" {
		t.Errorf("nome do WAR = %q", stub.uploadedName)
	}
	zr, err := zip.NewReader(bytes.NewReader(stub.uploadedWAR), int64(len(stub.uploadedWAR)))
	if err != nil {
		t.Fatal(err)
	}
	entries := map[string][]byte{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		entries[f.Name] = b
	}
	for _, want := range []string{
		"WEB-INF/classes/application.info", "WEB-INF/classes/layout.ftl",
		"WEB-INF/jboss-web.xml", "resources/js/meu_layout.js", "resources/images/icon.png",
	} {
		if _, ok := entries[want]; !ok {
			t.Errorf("WAR sem %s; entradas: %v", want, keysOf(entries))
		}
	}
	if !bytes.Equal(entries["resources/images/icon.png"], widgetBinary) {
		t.Errorf("binário corrompido no WAR")
	}
}

// Pasta inexistente em wcm/layout → exit 4, sem tocar o servidor.
func TestLayoutExportNaoEncontrado(t *testing.T) {
	stub := &widgetStub{}
	proj := widgetProject(t, stub.server(t).URL)

	code, out := runMain(t, "layout", "export", "nao_existe", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitNotFound {
		t.Fatalf("exit=%d, esperado %d; saída: %s", code, output.ExitNotFound, out)
	}
	// No Windows a pasta é wcm\layout, e o JSON escapa a barra invertida —
	// compara com a forma que o envelope realmente carrega.
	wantDir, _ := json.Marshal(filepath.Join("wcm", "layout"))
	if !strings.Contains(out, strings.Trim(string(wantDir), `"`)) {
		t.Errorf("mensagem não aponta a pasta convencional: %s", out)
	}
}

// Sem application.info o servidor aceitaria o WAR e falharia depois, em
// silêncio. A CLI barra antes (exit 2) e nada é enviado.
func TestLayoutExportExigeApplicationInfo(t *testing.T) {
	stub := &widgetStub{}
	proj := layoutExportProject(t, stub, "meu_layout", "")

	code, out := runMain(t, "layout", "export", "meu_layout", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitUsage {
		t.Fatalf("exit=%d, esperado %d; saída: %s", code, output.ExitUsage, out)
	}
	if !strings.Contains(out, "application.info") {
		t.Errorf("mensagem não cita o application.info: %s", out)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.uploadedName != "" {
		t.Errorf("WAR enviado sem application.info (%q)", stub.uploadedName)
	}
}

// application.type=widget na pasta de layout: é um widget no lugar errado. O
// comando recusa e aponta o widget export.
func TestLayoutExportRecusaTipoWidget(t *testing.T) {
	stub := &widgetStub{}
	proj := layoutExportProject(t, stub, "meu_layout", "application.type=widget\napplication.code=meu_layout\n")

	code, out := runMain(t, "layout", "export", "meu_layout", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitUsage {
		t.Fatalf("exit=%d, esperado %d; saída: %s", code, output.ExitUsage, out)
	}
	if !strings.Contains(out, "application.type=widget") || !strings.Contains(out, "widget export") {
		t.Errorf("mensagem não explica o tipo nem aponta a saída: %s", out)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.uploadedName != "" {
		t.Errorf("WAR enviado com tipo errado (%q)", stub.uploadedName)
	}
}

// Guarda invertida (espelho do §3.1): código que já é WIDGET no servidor →
// recusa com exit 2 e nada é enviado.
func TestLayoutExportRecusaColisaoComWidget(t *testing.T) {
	stub := &widgetStub{widgets: map[string]string{"painel": "Painel Widget"}}
	proj := layoutExportProject(t, stub, "painel", "application.type=layout\n")

	code, out := runMain(t, "layout", "export", "painel", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitUsage {
		t.Fatalf("exit=%d, esperado %d; saída: %s", code, output.ExitUsage, out)
	}
	if !strings.Contains(out, "WIDGET") || !strings.Contains(out, "Painel Widget") || !strings.Contains(out, "--force") {
		t.Errorf("mensagem não explica a colisão, não cita o widget ou não oferece --force: %s", out)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.uploadedName != "" {
		t.Errorf("o WAR foi enviado mesmo com a colisão (%q)", stub.uploadedName)
	}
}

// --force é a saída consciente.
func TestLayoutExportForcePublicaApesarDaColisao(t *testing.T) {
	stub := &widgetStub{widgets: map[string]string{"painel": "Painel Widget"}}
	proj := layoutExportProject(t, stub, "painel", "application.type=layout\n")

	code, _, stderr := runMainStderr(t, "layout", "export", "painel", "--force", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d com --force; stderr: %s", code, stderr)
	}
	if !strings.Contains(stderr, "--force") {
		t.Errorf("sem o aviso de sobrescrita no stderr: %s", stderr)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.uploadedName != "painel.war" {
		t.Errorf("nome do WAR = %q", stub.uploadedName)
	}
}

// Código que já existe como LAYOUT no servidor NÃO dispara a guarda: é a
// atualização normal (a coleção applications só enxerga widgets).
func TestLayoutExportRepublicaLayoutExistente(t *testing.T) {
	stub := &widgetStub{layouts: map[string]string{"meu_layout": "Meu Layout"}}
	proj := layoutExportProject(t, stub, "meu_layout", "application.type=layout\n")

	code, out := runMain(t, "layout", "export", "meu_layout", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d ao republicar layout existente; saída: %s", code, out)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.uploadedName != "meu_layout.war" {
		t.Errorf("nome do WAR = %q", stub.uploadedName)
	}
}

// GET por código com 500: o fallback pela listagem nativa ainda vê a colisão.
func TestLayoutExportColisaoPelaListagemQuandoGetFalha(t *testing.T) {
	// "meu_widget" está na listagem nativa do stub.
	stub := &widgetStub{widgetGetBroken: true}
	proj := layoutExportProject(t, stub, "meu_widget", "application.type=layout\n")

	code, out := runMain(t, "layout", "export", "meu_widget", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitUsage {
		t.Fatalf("exit=%d, esperado %d via listagem; saída: %s", code, output.ExitUsage, out)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.uploadedName != "" {
		t.Errorf("o WAR foi enviado mesmo com a colisão (%q)", stub.uploadedName)
	}
}

// Preflight sem resposta nos dois caminhos: falha em aberto (avisa e publica).
func TestLayoutExportPreflightIndisponivelPublica(t *testing.T) {
	stub := &widgetStub{widgetGetBroken: true, applicationsListBroken: true}
	proj := layoutExportProject(t, stub, "meu_layout", "application.type=layout\n")

	code, _, stderr := runMainStderr(t, "layout", "export", "meu_layout", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d com preflight indisponível; stderr: %s", code, stderr)
	}
	if !strings.Contains(stderr, "não consegui checar") {
		t.Errorf("sem o aviso de preflight indisponível: %s", stderr)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.uploadedName != "meu_layout.war" {
		t.Errorf("nome do WAR = %q", stub.uploadedName)
	}
}

// layout list: só os customizados por padrão; --all inclui os internos.
func TestLayoutListJSON(t *testing.T) {
	stub := &widgetStub{layouts: map[string]string{"kit_layout": "Portal"}}
	proj := widgetProject(t, stub.server(t).URL)

	code, out := runMain(t, "layout", "list", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d; saída: %s", code, out)
	}
	var env struct {
		Data struct {
			All     bool `json:"all"`
			Layouts []struct {
				Code     string `json:"code"`
				Title    string `json:"title"`
				Internal bool   `json:"internal"`
			} `json:"layouts"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("envelope inválido: %v\n%s", err, out)
	}
	if env.Data.All || len(env.Data.Layouts) != 1 || env.Data.Layouts[0].Code != "kit_layout" || env.Data.Layouts[0].Title != "Portal" {
		t.Errorf("data inesperado: %+v", env.Data)
	}
}

func TestLayoutListTabela(t *testing.T) {
	stub := &widgetStub{layouts: map[string]string{"kit_layout": "Portal"}}
	proj := widgetProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "layout", "list", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d", code)
	}
	for _, want := range []string{"│", "Código", "Título", "kit_layout", "Portal"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("tabela sem %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, "Origem") {
		t.Errorf("a coluna Origem só entra com --all:\n%s", stdout)
	}
}

// Sem layout customizado: mensagem orienta o --all; o JSON traz lista vazia.
func TestLayoutListVazioEAll(t *testing.T) {
	stub := &widgetStub{}
	proj := widgetProject(t, stub.server(t).URL)

	code, stdout, stderr := runMainStderr(t, "layout", "list", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d", code)
	}
	if !strings.Contains(stdout+stderr, "--all") {
		t.Errorf("lista vazia sem orientar o --all: %s %s", stdout, stderr)
	}
	code, out := runMain(t, "layout", "list", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK || !strings.Contains(out, `"layouts":[]`) {
		t.Errorf("JSON da lista vazia: exit=%d %s", code, out)
	}
}

// --all: inclui os internos e ganha a coluna Origem.
func TestLayoutListAllTabela(t *testing.T) {
	stub := &widgetStub{layouts: map[string]string{"kit_layout": "Portal"}, layoutsInternal: map[string]string{"layoutdefault": "Amplo"}}
	proj := widgetProject(t, stub.server(t).URL)

	code, stdout := runMain(t, "layout", "list", "--all", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d", code)
	}
	for _, want := range []string{"Origem", "kit_layout", "customizado", "layoutdefault", "plataforma"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("tabela --all sem %q:\n%s", want, stdout)
		}
	}
	code, stdout = runMain(t, "layout", "list", "--project", proj, "--server", "homolog")
	if code != output.ExitOK || strings.Contains(stdout, "layoutdefault") {
		t.Errorf("sem --all o interno não devia aparecer:\n%s", stdout)
	}
}

// --- layout import ---

// Import desempacota o WAR do layout em wcm/layout/<code>, pelo mesmo mapa do
// widget (layout.ftl volta para src/main/resources), preservando binário e
// ignorando META-INF.
func TestLayoutImportUnpacks(t *testing.T) {
	stub := &widgetStub{}
	proj := widgetProject(t, stub.server(t).URL)

	code, out := runMain(t, "layout", "import", "kit_layout", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d; saída: %s", code, out)
	}
	base := filepath.Join(proj, "wcm", "layout", "kit_layout")
	for _, rel := range []string{
		"src/main/resources/layout.ftl",
		"src/main/resources/application.info",
		"src/main/webapp/WEB-INF/jboss-web.xml",
		"src/main/webapp/resources/js/kit_layout.js",
		"src/main/webapp/resources/images/icon.png",
	} {
		if _, err := os.Stat(filepath.Join(base, filepath.FromSlash(rel))); err != nil {
			t.Errorf("arquivo esperado não existe: %s", rel)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(base, "src/main/webapp/resources/images/icon.png")); !bytes.Equal(b, widgetBinary) {
		t.Errorf("binário corrompido no import")
	}
	if _, err := os.Stat(filepath.Join(base, "META-INF")); err == nil {
		t.Errorf("META-INF não devia ser extraído")
	}
	if _, err := os.Stat(filepath.Join(proj, "wcm", "widget", "kit_layout")); err == nil {
		t.Errorf("o layout foi parar em wcm/widget")
	}
	if !strings.Contains(out, `"action":"imported"`) {
		t.Errorf("envelope sem o resultado: %s", out)
	}
}

// Código inexistente no servidor: exit 4 (um só pedido) e o resto do lote segue.
func TestLayoutImportNaoEncontrado(t *testing.T) {
	stub := &widgetStub{}
	proj := widgetProject(t, stub.server(t).URL)

	code, out := runMain(t, "layout", "import", "nao_existe", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitNotFound {
		t.Fatalf("exit=%d, esperado %d; saída: %s", code, output.ExitNotFound, out)
	}
	code, _ = runMain(t, "layout", "import", "nao_existe", "kit_layout", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitPartial {
		t.Errorf("lote com uma falha: exit=%d, esperado %d", code, output.ExitPartial)
	}
	if _, err := os.Stat(filepath.Join(proj, "wcm", "layout", "kit_layout", "src", "main", "resources", "layout.ftl")); err != nil {
		t.Errorf("o item bom do lote não foi importado")
	}
}

// --all importa todos os layouts customizados listados pelo helper.
func TestLayoutImportAll(t *testing.T) {
	stub := &widgetStub{}
	proj := widgetProject(t, stub.server(t).URL)
	code, out := runMain(t, "layout", "import", "--all", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK || !strings.Contains(out, `"id":"kit_layout"`) {
		t.Fatalf("exit=%d; saída: %s", code, out)
	}
	code, out = runMain(t, "layout", "import", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitUsage {
		t.Errorf("sem código e sem --all devia ser exit 2, veio %d: %s", code, out)
	}
}

// Sem o helper: exit 7 com a orientação de instalar.
func TestLayoutImportSemHelper(t *testing.T) {
	stub := &widgetStub{helperMissing: true}
	proj := widgetProject(t, stub.server(t).URL)
	code, out := runMain(t, "layout", "import", "kit_layout", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitMissingHelper || !strings.Contains(out, "install-helper") {
		t.Fatalf("exit=%d; saída: %s", code, out)
	}
}

// Helper instalado mas anterior ao 0.12.0 (sem /layouts): exit 7 dizendo que
// está DESATUALIZADO — a mensagem precisa apontar o --force do install-helper.
func TestLayoutImportHelperAntigo(t *testing.T) {
	stub := &widgetStub{layoutRouteMissing: true}
	proj := widgetProject(t, stub.server(t).URL)
	code, out := runMain(t, "layout", "import", "kit_layout", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitMissingHelper || !strings.Contains(out, "desatualizado") || !strings.Contains(out, "--force") {
		t.Fatalf("exit=%d; saída: %s", code, out)
	}
}
