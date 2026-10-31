//go:build browser

// Teste com o navegador REAL contra uma página local que imita o
// pageworkflowview do Fluig (estrutura medida no Gate E). Rode com:
//
//	go test -tags=browser ./internal/browser/
package browser

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

const fakeWorkflowView = `<!doctype html><html><body>
<div id="nps"></div>
<button data-send>Enviar</button><ul><li data-save>Salvar</li></ul>
<iframe id="workflowView-cardViewer" name="workflowView-cardViewer" src="/form"></iframe>
<div id="fluigRightbar" style="display:none">
  <select id="nextActivity"><option value="28">Integração</option><option value="13">Revisar</option></select>
  <input id="nextResponsible">
  <button id="moviment-button">Enviar</button><button id="cancel-button">Voltar</button>
</div>
<div class="container-modal"></div>
<script>
fetch('/nps/api/v1/surveys?productLine=x').then(function () {
  document.getElementById('nps').innerHTML = '<div class="container-modal">NPS</div>';
}).catch(function () {});
document.querySelector('[data-send]').onclick = function () { document.getElementById('fluigRightbar').style.display = 'block'; };
document.getElementById('cancel-button').onclick = function () { document.getElementById('fluigRightbar').style.display = 'none'; };
function mensagem(t) {
  document.querySelector('.container-modal').innerHTML =
    '<div id="fluig-message-1" class="modal in" style="display:block"><div class="modal-body">' + t +
    ' Ver detalhes técnicos</div><button onclick="this.parentNode.remove()">Ok</button></div>';
}
document.getElementById('moviment-button').onclick = function () {
  var w = document.getElementById('workflowView-cardViewer').contentWindow;
  var erro = w.beforeSendValidate(0, document.getElementById('nextActivity').value);
  if (erro) { mensagem(erro); return; }
  var d = w.document;
  var fd = {};
  Array.prototype.forEach.call(d.querySelectorAll('[name]'), function (e) {
    if ((e.type === 'radio' || e.type === 'checkbox') && !e.checked) return;
    if (e.name.charAt(0) !== '_') fd[e.name] = e.value;
  });
  var x = new XMLHttpRequest();
  x.open('POST', '/ecm/api/rest/ecm/workflowView/send');
  x.onload = function () {
    if (x.status >= 400) mensagem(JSON.parse(x.responseText).message.message);
    document.getElementById('fluigRightbar').style.display = 'none';
  };
  x.send(JSON.stringify({ selectedState: document.getElementById('nextActivity').value, formData: fd }));
};
</script></body></html>`

const fakeForm = `<!doctype html><html><body><form>
<input type="hidden" name="codColigada" value="29">
<input type="hidden" name="controleEnvio" id="controleEnvio" value="">
<div id="painelFin" style="background-color: rgb(255, 243, 205)">
  <input type="radio" name="aprFin" value="aprovado" id="a1"><label for="a1" style="position:absolute;left:0;top:0;width:40px;height:40px">A</label>
  <input type="radio" name="aprFin" value="reprovado" id="a2"><label for="a2">R</label>
  <textarea name="comentFin" id="comentFin"></textarea>
  <input name="valor" id="valor">
</div>
<div id="painelAnexos" style="display:none">x</div>
<input name="_codColigada" id="_codColigada" value="29" readonly>
<span id="aprovadoPor">por João Silva em 31/10/2026</span>
</form>
<script>
var mascara = 0;
document.getElementById('valor').addEventListener('change', function () { mascara++; this.value = 'R$ ' + this.value; });
var beforeSendValidate = function () {
  document.getElementById('controleEnvio').value = String(Date.now());
  var r = document.querySelector('[name=aprFin]:checked');
  if (!r) return 'Escolha a decisão';
};
</script></body></html>`

func fakeFluig(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	var posts []string
	mux := http.NewServeMux()
	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if c, err := r.Cookie("JSESSIONID"); err != nil || c.Value != "sessao-teste" {
				http.Redirect(w, r, "/portal/login", http.StatusFound)
				return
			}
			h(w, r)
		}
	}
	mux.HandleFunc("/portal/home", auth(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/portal/p/01/home", http.StatusFound)
	}))
	mux.HandleFunc("/portal/p/01/home", auth(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "<html>home</html>") }))
	mux.HandleFunc("/portal/p/01/pageworkflowview", auth(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, fakeWorkflowView) }))
	mux.HandleFunc("/form", auth(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, fakeForm) }))
	mux.HandleFunc("/nps/api/v1/surveys", func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "[]") })
	mux.HandleFunc("/ecm/api/rest/ecm/workflowView/send", auth(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		posts = append(posts, string(b))
		if strings.Contains(string(b), `"comentFin":""`) {
			w.WriteHeader(500)
			io.WriteString(w, `{"content":"ERROR","message":{"message":"<b>É obrigatório incluir um COMENTÁRIO!</b>"}}`)
			return
		}
		io.WriteString(w, `{"content":{"processInstanceId":1}}`)
	}))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &posts
}

func TestBrowserReal(t *testing.T) {
	exe, err := Find("")
	if err != nil {
		t.Skip(err)
	}
	srv, posts := fakeFluig(t)
	base, _ := url.Parse(srv.URL)
	jar, _ := cookiejar.New(nil)
	jar.SetCookies(base, []*http.Cookie{{Name: "JSESSIONID", Value: "sessao-teste"}})
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	s, err := Open(ctx, exe, false, base, jar)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.OpenTask(ctx, 1, 7, "c-jsilva"); err != nil {
		t.Fatalf("abrir a tarefa (cookie injetado + tenant 01): %v", err)
	}
	if s.tenant != "01" {
		t.Errorf("tenant: %q", s.tenant)
	}

	st, err := s.Inspect(ctx, []string{"painelFin", "painelAnexos", "aprFin", "comentFin", "codColigada", "aprovadoPor"},
		map[string][]string{"painelFin": {"background-color"}})
	if err != nil {
		t.Fatal(err)
	}
	e := st.Elems
	if !e["painelFin"].Visible || e["painelAnexos"].Visible || !e["aprFin"].Editable || !e["comentFin"].Editable {
		t.Errorf("visível/editável: %+v", e)
	}
	if c := e["codColigada"]; !c.Exists || c.Editable || c.Value == nil || *c.Value != "29" {
		t.Errorf("campo bloqueado pelo setEnabled (hidden + _campo readonly): %+v", c)
	}
	if tx := e["aprovadoPor"].Text; tx == nil || !strings.Contains(*tx, "por João") {
		t.Errorf("texto: %v", tx)
	}
	if st.CSS["painelFin"]["background-color"] != "rgb(255, 243, 205)" {
		t.Errorf("css: %v", st.CSS)
	}
	// O NPS foi bloqueado: o modal dele não nasceu.
	if nps, err := chromedp.Run(s.with(ctx), chromedp.Evaluate[string]("document.getElementById('nps').innerHTML")); err != nil || nps != "" {
		t.Errorf("o modal do NPS apareceu: %q %v", nps, err)
	}

	// Recusa do beforeSendValidate: nada sai.
	r, err := s.Send(ctx, 13, 20*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if r.Posted || !strings.Contains(r.Message, "Escolha a decisão") {
		t.Errorf("recusa do navegador: %+v", r)
	}
	s.Reset(ctx)

	// Recusa do servidor: o POST sai, a mensagem aparece.
	if err := s.Fill(ctx, [][2]string{{"aprFin", "reprovado"}, {"comentFin", ""}, {"valor", "10"}}); err != nil {
		t.Fatal(err)
	}
	h, _ := s.Hidden(ctx, []string{"valor"})
	if h["valor"] == nil || *h["valor"] != "R$ 10" {
		t.Errorf("o change disparou a máscara: %v", h["valor"])
	}
	r, err = s.Send(ctx, 13, 20*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Posted || r.Status != 500 || !strings.Contains(r.Message, "COMENTÁRIO") || r.FormData["controleEnvio"] == "" {
		t.Errorf("recusa do servidor: %+v", r)
	}
	s.Reset(ctx)

	// Envio aceito: o formData traz o hidden do beforeSendValidate.
	if err := s.Fill(ctx, [][2]string{{"comentFin", "ok"}}); err != nil {
		t.Fatal(err)
	}
	r, err = s.Send(ctx, 13, 20*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Posted || r.Status != 200 || r.Message != "" || r.FormData["aprFin"] != "reprovado" {
		t.Errorf("envio aceito: %+v", r)
	}
	if len(*posts) != 2 {
		t.Errorf("POSTs: %d", len(*posts))
	}
	if err := s.Fill(ctx, [][2]string{{"codColigada", "21"}}); err == nil || !strings.Contains(err.Error(), "bloqueado") {
		t.Errorf("fill em campo bloqueado: %v", err)
	}
	if _, err := s.Send(ctx, 52, 5*time.Second); err == nil {
		t.Error("etapa fora das opções")
	} else if !strings.Contains(fmt.Sprint(err), "13=Revisar") {
		t.Errorf("mensagem com as opções: %v", err)
	}
	if err := s.SetViewport(ctx, Mobile); err != nil {
		t.Fatal(err)
	}
	png, err := s.Screenshot(ctx)
	if err != nil || len(png) < 100 {
		t.Errorf("screenshot: %d bytes, %v", len(png), err)
	}
}
