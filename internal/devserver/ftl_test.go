package devserver

import (
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, root, rel, content string) string {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRenderViewFTL(t *testing.T) {
	root := t.TempDir()
	p := writeFile(t, root, "view.ftl",
		"<#-- comentário\n multi-linha -->\n<div id=\"W_${instanceId}\">oi ${instanceId}</div>")
	out, err := renderViewFTL(p, "42")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "<#--") || !strings.Contains(out, `id="W_42"`) || !strings.Contains(out, "oi 42") {
		t.Errorf("render: %q", out)
	}

	// FreeMarker de verdade → erro (o chamador mantém o render do servidor).
	for _, ftl := range []string{
		`<div>${i18n.getTranslation("x")}</div>`,
		`<#if x><div></div></#if>`,
		`<@macro/>`,
	} {
		p := writeFile(t, root, "unsupported.ftl", ftl)
		if _, err := renderViewFTL(p, "1"); err == nil {
			t.Errorf("FTL %q deveria ser recusado", ftl)
		}
	}
}

// pageWith monta uma página de portal sintética com dois envelopes de widget:
// um do projeto (appcode ramal_teste) e um de fora (html_editor).
func pageWith(inner string) string {
	return `<html><body>` +
		`<div id="_instance_100_" appcode="html_editor" class="widget-custom-inside">` +
		`<div class="wcm_widget"><div class="wcm_corpo_widget_single">` +
		`<div id="X_100" class="super-widget wcm-widget-class"><div>do servidor</div></div>` +
		`</div></div></div>` +
		`<div id="_instance_200_" appcode="ramal_teste" class="widget-custom-inside">` +
		`<div class="wcm_widget"><div class="wcm_corpo_widget_single">` +
		inner +
		`</div></div></div>` +
		`</body></html>`
}

func ftlTestServer(t *testing.T, ftl string) *Server {
	t.Helper()
	root := t.TempDir()
	writeFile(t, root, "wcm/widget/ramal_teste/src/main/resources/application.info",
		"application.code=ramal_teste\nview.file=view.ftl\n")
	writeFile(t, root, "wcm/widget/ramal_teste/src/main/resources/view.ftl", ftl)
	writeFile(t, root, "wcm/widget/ramal_teste/src/main/webapp/resources/js/a.js", "//")
	u, _ := url.Parse("http://fluig:8080")
	jar, _ := cookiejar.New(nil)
	s, err := New(Options{Root: root, Upstream: u, Jar: jar, Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestRewriteWidgetBlocks(t *testing.T) {
	s := ftlTestServer(t, `<div id="MyWidget_${instanceId}" class="super-widget wcm-widget-class">
<div>versão local ${instanceId}</div>
</div>`)
	serverSide := `<div id="MyWidget_200" class="super-widget wcm-widget-class"><div>versão antiga</div></div>`
	out := s.rewriteWidgetBlocks(pageWith(serverSide))

	if !strings.Contains(out, "versão local 200") || strings.Contains(out, "versão antiga") {
		t.Errorf("bloco da widget do projeto deveria vir do view.ftl local:\n%s", out)
	}
	if !strings.Contains(out, "do servidor") {
		t.Error("widget de fora do projeto deveria ficar intacta")
	}
	// O envelope (chrome do portal) fica; só a saída do FTL muda.
	if !strings.Contains(out, `id="_instance_200_" appcode="ramal_teste"`) ||
		!strings.Contains(out, `id="MyWidget_200"`) {
		t.Errorf("envelope/instanceId perdidos:\n%s", out)
	}
}

func TestRewriteWidgetBlocksFTLNaoSuportado(t *testing.T) {
	var warns []string
	s := ftlTestServer(t, `<div class="wcm-widget-class">${i18n.x}</div>`)
	s.opts.Warnf = func(f string, a ...any) { warns = append(warns, f) }

	page := pageWith(`<div id="MyWidget_200" class="wcm-widget-class"><div>render do servidor</div></div>`)
	out := s.rewriteWidgetBlocks(page)
	if out != page {
		t.Error("FTL não suportado: a página deveria ficar intacta")
	}
	if len(warns) != 1 {
		t.Fatalf("quer 1 aviso, veio %d", len(warns))
	}
	// warnOnce: segunda página não repete; clearWarn re-arma.
	_ = s.rewriteWidgetBlocks(page)
	if len(warns) != 1 {
		t.Errorf("aviso repetido: %d", len(warns))
	}
	s.clearWarn("ftl:ramal_teste")
	_ = s.rewriteWidgetBlocks(page)
	if len(warns) != 2 {
		t.Errorf("clearWarn deveria re-armar o aviso: %d", len(warns))
	}
}

func TestRewriteWidgetBlocksSemFechamento(t *testing.T) {
	s := ftlTestServer(t, `<div class="wcm-widget-class">ok ${instanceId}</div>`)
	// div raiz sem fechamento dentro do envelope: não mexe (não corromper).
	page := `<div id="_instance_9_" appcode="ramal_teste"><div class="c">` +
		`<div id="W_9" class="wcm-widget-class"><div>aberto`
	if out := s.rewriteWidgetBlocks(page); out != page {
		t.Errorf("página malformada deveria ficar intacta:\n%s", out)
	}
}

func TestByViewFTLEAppCode(t *testing.T) {
	s := ftlTestServer(t, `<div class="wcm-widget-class"></div>`)
	m, ok := s.mounts.byAppCode("ramal_teste")
	if !ok || m.appCode != "ramal_teste" || m.viewFTL == "" {
		t.Fatalf("byAppCode: ok=%v m=%+v", ok, m)
	}
	if _, ok := s.mounts.byAppCode("html_editor"); ok {
		t.Error("appcode de fora do projeto não deveria resolver")
	}
	if _, ok := s.mounts.byViewFTL(m.viewFTL); !ok {
		t.Error("byViewFTL deveria achar o próprio view.ftl")
	}
	if _, ok := s.mounts.byViewFTL(filepath.Join(s.opts.Root, "outro.ftl")); ok {
		t.Error("arquivo qualquer não é view.ftl de mount")
	}
}
