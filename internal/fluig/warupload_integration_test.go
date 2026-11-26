//go:build integration

package fluig

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"testing"
	"time"
)

// integrationWidgetCode é o widget descartável do envio de WAR. O Fluig não
// tem rota para remover widget (só a Central desativa), por isso o teste
// reenvia sempre o MESMO código: o servidor guarda um widget só.
const integrationWidgetCode = "zz_fluigcli_test_upload"

// TestIntegrationUploadWARServlet publica o widget descartável pelo servlet
// ApplicationUpload (ROADMAP §5.24, issue #8) e confere que ele aparece no
// page-management. Opt-in extra: FLUIGCLI_TEST_WAR_UPLOAD=1, porque o teste
// faz deploy no servidor e deixa o widget lá.
func TestIntegrationUploadWARServlet(t *testing.T) {
	if os.Getenv("FLUIGCLI_TEST_WAR_UPLOAD") != "1" {
		t.Skip("defina FLUIGCLI_TEST_WAR_UPLOAD=1 para publicar o widget descartável " + integrationWidgetCode)
	}
	opts := integrationOptions(t)
	opts.WARUpload = WARUploadServlet
	c, err := NewClient(opts)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	if _, err := c.FindLayout(ctx, integrationWidgetCode); err == nil {
		t.Fatalf("%q existe como LAYOUT no servidor; o envio sobrescreveria o layout", integrationWidgetCode)
	}
	if err := c.UploadWidgetWAR(ctx, integrationWidgetCode+".war", minimalWidgetWAR(t)); err != nil {
		t.Fatalf("UploadWidgetWAR (servlet): %v", err)
	}

	// A instalação é assíncrona: na homologação levou menos de 5 s.
	deadline := time.Now().Add(60 * time.Second)
	for {
		w, err := c.FindWidgetNative(ctx, integrationWidgetCode)
		if err == nil && w.Code == integrationWidgetCode {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("o widget não apareceu no page-management em 60 s: %v", err)
		}
		time.Sleep(2 * time.Second)
	}
}

// minimalWidgetWAR monta um WAR de widget mínimo e válido.
func minimalWidgetWAR(t *testing.T) []byte {
	t.Helper()
	files := map[string]string{
		"WEB-INF/jboss-web.xml": `<?xml version="1.0" encoding="UTF-8"?>
<jboss-web>
    <context-root>/` + integrationWidgetCode + `</context-root>
    <disable-cross-context>false</disable-cross-context>
</jboss-web>
`,
		"WEB-INF/web.xml": `<?xml version="1.0" encoding="UTF-8"?>
<web-app xmlns="http://java.sun.com/xml/ns/javaee" version="3.0"/>
`,
		"WEB-INF/classes/application.info": `application.type=widget
application.code=` + integrationWidgetCode + `
application.title=zz fluigcli teste upload
application.description=Widget descartável dos testes de integração do fluigcli
application.category=SYSTEM
application.renderer=freemarker
developer.code=fluigcli
developer.name=fluigcli
developer.url=http://www.fluig.com
application.uiwidget=true
application.mobileapp=false
application.version=1.0.0
view.file=view.ftl
edit.file=edit.ftl
locale.file.base.name=` + integrationWidgetCode + `
`,
		"WEB-INF/classes/view.ftl": `<div id="zzFluigcli_${instanceId}">zz fluigcli teste upload</div>
`,
		"WEB-INF/classes/edit.ftl": `<div id="zzFluigcliEdit_${instanceId}">zz fluigcli teste upload</div>
`,
		"WEB-INF/classes/" + integrationWidgetCode + ".properties":       "application.title=zz fluigcli teste upload\n",
		"WEB-INF/classes/" + integrationWidgetCode + "_pt_BR.properties": "application.title=zz fluigcli teste upload\n",
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
