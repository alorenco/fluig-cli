// Package browser abre o render REAL das tarefas do Fluig num navegador
// headless (Chrome, Chromium ou Edge) para o passo "ui" do `fluigcli test`
// (ROADMAP §5.19, spec em docs/interno/SPEC-TEST-E.md).
//
// A sessão do fluigcli é a credencial universal: o pacote injeta os cookies da
// sessão no navegador e navega direto no host do servidor, sem login nem 2FA.
// Sem cobra e sem I/O de terminal (regra 3).
package browser

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

//go:embed page.js
var pageJS string

// ErrNotFound marca a ausência de navegador (a CLI traduz para exit 7).
var ErrNotFound = errors.New("navegador não encontrado")

// lookPath e homeDir são variáveis para os testes.
var (
	lookPath = exec.LookPath
	homeDir  = os.UserHomeDir
)

// candidates são os executáveis procurados no PATH, em ordem.
var candidates = []string{
	"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "chrome",
	"microsoft-edge", "microsoft-edge-stable", "msedge", "headless-shell",
}

// Find acha o navegador: explicit (--browser), $FLUIGCLI_BROWSER, os nomes
// conhecidos no PATH, os caminhos padrão do SO e o Chromium do Playwright em
// ~/.cache/ms-playwright.
func Find(explicit string) (string, error) {
	if explicit == "" {
		explicit = os.Getenv("FLUIGCLI_BROWSER")
	}
	if explicit != "" {
		p, err := lookPath(explicit)
		if err != nil {
			return "", fmt.Errorf("%w: %s: %v", ErrNotFound, explicit, err)
		}
		return p, nil
	}
	for _, c := range candidates {
		if p, err := lookPath(c); err == nil {
			return p, nil
		}
	}
	for _, p := range osPaths() {
		if fileExists(p) {
			return p, nil
		}
	}
	if p := playwrightChromium(); p != "" {
		return p, nil
	}
	return "", fmt.Errorf("%w: procurei --browser, $FLUIGCLI_BROWSER, %s no PATH e o Chromium do Playwright",
		ErrNotFound, strings.Join(candidates[:5], ", "))
}

func osPaths() []string {
	switch runtime.GOOS {
	case "windows":
		var out []string
		for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)", "LocalAppData"} {
			if b := os.Getenv(env); b != "" {
				out = append(out,
					filepath.Join(b, "Google", "Chrome", "Application", "chrome.exe"),
					filepath.Join(b, "Microsoft", "Edge", "Application", "msedge.exe"))
			}
		}
		return out
	case "darwin":
		return []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
		}
	}
	return []string{"/usr/bin/google-chrome", "/usr/bin/chromium", "/snap/bin/chromium"}
}

// playwrightChromium devolve o Chromium mais novo do cache do Playwright.
func playwrightChromium() string {
	h, err := homeDir()
	if err != nil {
		return ""
	}
	dirs := []string{filepath.Join(h, ".cache", "ms-playwright"), filepath.Join(h, "Library", "Caches", "ms-playwright")}
	if la := os.Getenv("LOCALAPPDATA"); la != "" {
		dirs = append(dirs, filepath.Join(la, "ms-playwright"))
	}
	var found []string
	for _, d := range dirs {
		ms, _ := filepath.Glob(filepath.Join(d, "chromium-*", "chrome-*", "chrome*"))
		for _, m := range ms {
			base := strings.ToLower(filepath.Base(m))
			if (base == "chrome" || base == "chrome.exe") && fileExists(m) {
				found = append(found, m)
			}
		}
	}
	sort.Strings(found)
	if len(found) == 0 {
		return ""
	}
	return found[len(found)-1]
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// Viewport é a área da janela.
type Viewport struct {
	Width, Height int
	Mobile        bool
}

// Desktop e Mobile são os tamanhos padrão (SPEC-TEST-E §2.1).
var (
	Desktop = Viewport{Width: 1366, Height: 900}
	Mobile  = Viewport{Width: 390, Height: 844, Mobile: true}
)

// Session é uma janela de navegador com a sessão do Fluig.
type Session struct {
	ctx    context.Context
	cancel func()
	base   *url.URL
	tenant string
	vp     Viewport
}

// Open sobe o navegador, injeta os cookies da sessão e o ajudante da página, e
// bloqueia o NPS da TOTVS (o modal dele cobre a página; ver Gate E: remover o
// .container-modal apagaria também as mensagens de erro).
func Open(parent context.Context, execPath string, headed bool, base *url.URL, jar http.CookieJar) (*Session, error) {
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	// Ubuntu >= 23.10 bloqueia o sandbox do Chromium (AppArmor): "No usable
	// sandbox!". O navegador só abre o servidor Fluig do próprio usuário.
	opts = append(opts, chromedp.NoSandbox, chromedp.ExecPath(execPath), chromedp.WindowSize(Desktop.Width, Desktop.Height))
	if headed {
		opts = append(opts, chromedp.Flag("headless", false))
	}
	actx, c1 := chromedp.NewExecAllocator(parent, opts...)
	ctx, c2 := chromedp.NewContext(actx)
	s := &Session{ctx: ctx, cancel: func() { c2(); c1() }, base: base, vp: Desktop}
	if _, err := chromedp.Call(ctx, network.Enable, network.EnableParams{}); err != nil {
		s.Close()
		return nil, fmt.Errorf("o navegador não abriu (%s): %w", execPath, err)
	}
	if jar != nil {
		root := base.Scheme + "://" + base.Host + "/"
		for _, c := range jar.Cookies(base) {
			if _, err := chromedp.Call(ctx, network.SetCookie, network.SetCookieParams{Name: c.Name, Value: c.Value, URL: root}); err != nil {
				s.Close()
				return nil, fmt.Errorf("falha ao passar a sessão ao navegador: %w", err)
			}
		}
	}
	nps := base.Scheme + "://" + base.Host + "/nps/api/v1/surveys*"
	if _, err := chromedp.Call(ctx, network.SetBlockedURLs, network.SetBlockedURLsParams{
		URLPatterns: []*network.BlockPattern{{URLPattern: nps, Block: true}},
	}); err != nil {
		s.Close()
		return nil, err
	}
	if _, err := chromedp.Call(ctx, page.AddScriptToEvaluateOnNewDocument, page.AddScriptToEvaluateOnNewDocumentParams{Source: pageJS}); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

// Close fecha o navegador.
func (s *Session) Close() {
	if s != nil && s.cancel != nil {
		s.cancel()
	}
}

// SetViewport muda o tamanho da janela (e o modo celular).
func (s *Session) SetViewport(ctx context.Context, vp Viewport) error {
	if vp == s.vp {
		return nil
	}
	ctx = s.with(ctx)
	opts := []chromedp.EmulateViewportOption{}
	if vp.Mobile {
		opts = append(opts, chromedp.EmulateMobile, chromedp.EmulateTouch, chromedp.EmulatePortrait)
	}
	if err := chromedp.Do(ctx, chromedp.EmulateViewport(int64(vp.Width), int64(vp.Height), opts...)); err != nil {
		return err
	}
	if vp.Mobile {
		ua := "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148"
		if _, err := chromedp.Call(ctx, emulation.SetUserAgentOverride, emulation.SetUserAgentOverrideParams{UserAgent: ua}); err != nil {
			return err
		}
	}
	s.vp = vp
	return nil
}

// with liga o ctx do chamador (prazo do passo) ao contexto do navegador.
func (s *Session) with(ctx context.Context) context.Context {
	c, cancel := context.WithCancel(s.ctx)
	go func() {
		select {
		case <-ctx.Done():
			cancel()
		case <-c.Done():
		}
	}()
	return c
}

// call roda uma função do ajudante (window.__fluigcli) e decodifica o JSON.
func (s *Session) call(ctx context.Context, fn string, out any, args ...any) error {
	parts := make([]string, len(args))
	for i, a := range args {
		b, err := json.Marshal(a)
		if err != nil {
			return err
		}
		parts[i] = string(b)
	}
	expr := fmt.Sprintf("JSON.stringify(window.__fluigcli ? window.__fluigcli.%s(%s) : null)", fn, strings.Join(parts, ","))
	raw, err := chromedp.Run(s.with(ctx), chromedp.Evaluate[string](expr))
	if err != nil {
		return err
	}
	if raw == "null" || raw == "" {
		return errors.New("a página do Fluig não carregou o ajudante do fluigcli (a sessão expirou ou a URL caiu no login?)")
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal([]byte(raw), out)
}

type readyState struct {
	Form     bool     `json:"form"`
	Messages []string `json:"messages"`
	Tenant   string   `json:"tenant"`
}

// tenantCode descobre o código do tenant pelo redirecionamento do portal. Com
// o companyId no caminho (/portal/p/1/...), o Fluig redireciona para o código
// (/p/01/) e PERDE a query string (Gate E).
func (s *Session) tenantCode(ctx context.Context) (string, error) {
	if s.tenant != "" {
		return s.tenant, nil
	}
	c := s.with(ctx)
	if err := chromedp.Do(c, chromedp.Navigate(s.base.Scheme+"://"+s.base.Host+"/portal/home")); err != nil {
		return "", err
	}
	loc, err := chromedp.Run(c, chromedp.Evaluate[string]("location.pathname"))
	if err != nil {
		return "", err
	}
	parts := strings.Split(strings.Trim(loc, "/"), "/")
	if len(parts) >= 3 && parts[0] == "portal" && parts[1] == "p" {
		s.tenant = parts[2]
		return s.tenant, nil
	}
	if strings.Contains(loc, "login") {
		return "", errors.New("o navegador caiu no login: a sessão do fluigcli não vale no navegador")
	}
	return "", fmt.Errorf("não achei o código do tenant (o portal abriu %s)", loc)
}

// OpenTask abre a tarefa em modo edição (a URL do Gate E, E0.1).
func (s *Session) OpenTask(ctx context.Context, requestID, movement int, userCode string) error {
	q := url.Values{}
	q.Set("app_ecm_workflowview_processInstanceId", fmt.Sprint(requestID))
	q.Set("app_ecm_workflowview_currentMovto", fmt.Sprint(movement))
	q.Set("app_ecm_workflowview_taskUserId", userCode)
	q.Set("app_ecm_workflowview_managerMode", "false")
	return s.openWorkflowView(ctx, q)
}

// OpenView abre os detalhes da solicitação (modo visualização).
func (s *Session) OpenView(ctx context.Context, requestID int) error {
	q := url.Values{}
	q.Set("app_ecm_workflowview_detailsProcessInstanceID", fmt.Sprint(requestID))
	return s.openWorkflowView(ctx, q)
}

func (s *Session) openWorkflowView(ctx context.Context, q url.Values) error {
	tenant, err := s.tenantCode(ctx)
	if err != nil {
		return err
	}
	u := fmt.Sprintf("%s://%s/portal/p/%s/pageworkflowview?%s", s.base.Scheme, s.base.Host, tenant, q.Encode())
	if err := chromedp.Do(s.with(ctx), chromedp.Navigate(u)); err != nil {
		return err
	}
	return s.waitForm(ctx)
}

// waitForm espera o iframe do formulário. Uma mensagem de erro do Fluig no
// lugar do formulário (tarefa de outro usuário, solicitação inexistente) vira
// erro com o texto dela.
func (s *Session) waitForm(ctx context.Context) error {
	limite := time.Now().Add(60 * time.Second)
	for {
		var st readyState
		err := s.call(ctx, "ready", &st)
		if err == nil && st.Form {
			// Um respiro para o form.js terminar o $(document).ready.
			return sleep(ctx, 700*time.Millisecond)
		}
		if err == nil && len(st.Messages) > 0 {
			return fmt.Errorf("o Fluig recusou abrir a tarefa: %s", st.Messages[0])
		}
		if time.Now().After(limite) {
			if err != nil {
				return err
			}
			return errors.New("o formulário não carregou em 60 s")
		}
		if err := sleep(ctx, 300*time.Millisecond); err != nil {
			return err
		}
	}
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Elem é o que a tela mostra de uma chave (campo, id ou css:).
type Elem struct {
	Exists   bool    `json:"exists"`
	Visible  bool    `json:"visible"`
	Editable bool    `json:"editable"`
	Locked   bool    `json:"locked"`
	Value    *string `json:"value"`
	Text     *string `json:"text"`
}

// State é o resultado de uma inspeção.
type State struct {
	Elems map[string]Elem              `json:"elems"`
	CSS   map[string]map[string]string `json:"css"`
}

// Inspect lê o estado dos elementos e as propriedades de CSS pedidas.
func (s *Session) Inspect(ctx context.Context, names []string, css map[string][]string) (*State, error) {
	var st State
	q := map[string]any{"names": names, "css": css}
	if err := s.call(ctx, "inspect", &st, q); err != nil {
		return nil, err
	}
	return &st, nil
}

// Hidden lê o valor atual dos campos (o input com o nome, inclusive hidden).
func (s *Session) Hidden(ctx context.Context, names []string) (map[string]*string, error) {
	out := map[string]*string{}
	if err := s.call(ctx, "hidden", &out, names); err != nil {
		return nil, err
	}
	return out, nil
}

// Fill preenche os campos na ordem dada (pares nome/valor).
func (s *Session) Fill(ctx context.Context, pairs [][2]string) error {
	var errs []string
	if err := s.call(ctx, "fill", &errs, pairs); err != nil {
		return err
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return sleep(ctx, 300*time.Millisecond)
}

// Click clica num elemento do formulário.
func (s *Session) Click(ctx context.Context, key string) error {
	var msg string
	if err := s.call(ctx, "click", &msg, key); err != nil {
		return err
	}
	if msg != "" {
		return errors.New(msg)
	}
	return sleep(ctx, 500*time.Millisecond)
}

// SendResult é o desfecho de um envio pela tela.
type SendResult struct {
	Posted   bool              // o POST do workflowView/send saiu
	Status   int               // HTTP do POST (0 = não saiu)
	Message  string            // a mensagem FLUIGC.message, se apareceu
	Response string            // corpo da resposta (até 20 KB)
	FormData map[string]string // os campos enviados (do corpo do POST)
}

type sendRec struct {
	Body     string `json:"body"`
	Status   int    `json:"status"`
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

type statusState struct {
	Sends    []sendRec `json:"sends"`
	Messages []string  `json:"messages"`
}

// Send clica em Enviar, escolhe a etapa na gaveta "Movimentar Atividade" e
// confirma. Espera o desfecho: a resposta do POST, ou a mensagem do
// beforeSendValidate (que não manda nada ao servidor).
func (s *Session) Send(ctx context.Context, state int, wait time.Duration) (*SendResult, error) {
	var antes statusState
	if err := s.call(ctx, "status", &antes); err != nil {
		return nil, err
	}
	c := s.with(ctx)
	if err := chromedp.Do(c, chromedp.Click("button[data-send]", chromedp.NodeVisible)); err != nil {
		return nil, fmt.Errorf("botão Enviar: %w", err)
	}
	if err := chromedp.Do(c, chromedp.WaitVisible("#fluigRightbar #nextActivity")); err != nil {
		return nil, fmt.Errorf("a janela de envio não abriu: %w", err)
	}
	var msg string
	if err := s.call(ctx, "choose", &msg, state); err != nil {
		return nil, err
	}
	if msg != "" {
		var ops []string
		_ = s.call(ctx, "options", &ops)
		return nil, fmt.Errorf("%s (opções: %s)", msg, strings.Join(ops, ", "))
	}
	if err := chromedp.Do(c, chromedp.Click("#fluigRightbar #moviment-button", chromedp.NodeVisible)); err != nil {
		return nil, fmt.Errorf("botão Enviar da janela: %w", err)
	}
	return s.awaitSend(ctx, len(antes.Sends), wait)
}

// Save clica em Salvar (menu do Enviar).
func (s *Session) Save(ctx context.Context, wait time.Duration) (*SendResult, error) {
	var antes statusState
	if err := s.call(ctx, "status", &antes); err != nil {
		return nil, err
	}
	if err := chromedp.Do(s.with(ctx), chromedp.Click("[data-save]", chromedp.NodeReady)); err != nil {
		return nil, fmt.Errorf("botão Salvar: %w", err)
	}
	return s.awaitSend(ctx, len(antes.Sends), wait)
}

func (s *Session) awaitSend(ctx context.Context, before int, wait time.Duration) (*SendResult, error) {
	limite := time.Now().Add(wait)
	var msgDesde time.Time
	for {
		var st statusState
		if err := s.call(ctx, "status", &st); err != nil {
			return nil, err
		}
		novos := st.Sends[min(before, len(st.Sends)):]
		pronto := len(novos) > 0 && novos[len(novos)-1].Done
		if len(st.Messages) > 0 && msgDesde.IsZero() {
			msgDesde = time.Now()
		}
		// Mensagem sem POST por 1,5 s = recusa do beforeSendValidate.
		if pronto || (!msgDesde.IsZero() && len(novos) == 0 && time.Since(msgDesde) > 1500*time.Millisecond) {
			if pronto && novos[len(novos)-1].Status >= 400 && len(st.Messages) == 0 && time.Now().Before(limite) {
				// A recusa do servidor vira mensagem logo depois da resposta.
				if err := sleep(ctx, 500*time.Millisecond); err != nil {
					return nil, err
				}
				if err := s.call(ctx, "status", &st); err != nil {
					return nil, err
				}
			}
			res := &SendResult{}
			if len(st.Messages) > 0 {
				res.Message = st.Messages[len(st.Messages)-1]
			}
			if len(novos) > 0 {
				last := novos[len(novos)-1]
				res.Posted, res.Status, res.Response = true, last.Status, last.Response
				res.FormData = formData(last.Body)
			}
			return res, nil
		}
		if time.Now().After(limite) {
			return nil, fmt.Errorf("o envio não terminou em %s", wait)
		}
		if err := sleep(ctx, 250*time.Millisecond); err != nil {
			return nil, err
		}
	}
}

// formData extrai os campos do corpo do POST workflowView/send. O formato
// muda entre versões; aceita objeto {campo: valor} e lista [{name, value}].
func formData(body string) map[string]string {
	if body == "" {
		return nil
	}
	var outer map[string]json.RawMessage
	if json.Unmarshal([]byte(body), &outer) != nil {
		vals, err := url.ParseQuery(body)
		if err != nil {
			return nil
		}
		if fd := vals.Get("formData"); fd != "" {
			return formData(`{"formData":` + fd + `}`)
		}
		return nil
	}
	raw, ok := outer["formData"]
	if !ok {
		return nil
	}
	var asStr string
	if json.Unmarshal(raw, &asStr) == nil {
		raw = json.RawMessage(asStr)
	}
	out := map[string]string{}
	var obj map[string]any
	if json.Unmarshal(raw, &obj) == nil {
		for k, v := range obj {
			out[k] = fmt.Sprint(v)
		}
		return out
	}
	var arr []struct {
		Name  string `json:"name"`
		Value any    `json:"value"`
	}
	if json.Unmarshal(raw, &arr) == nil {
		for _, it := range arr {
			if it.Value == nil {
				out[it.Name] = ""
			} else {
				out[it.Name] = fmt.Sprint(it.Value)
			}
		}
		return out
	}
	return nil
}

// Reset fecha a mensagem e a janela de envio que ficaram abertas.
func (s *Session) Reset(ctx context.Context) {
	_ = s.call(ctx, "reset", nil)
	_ = sleep(ctx, 600*time.Millisecond)
}

// Screenshot captura a página inteira (PNG).
func (s *Session) Screenshot(ctx context.Context) ([]byte, error) {
	return chromedp.Run(s.with(ctx), chromedp.FullScreenshot(100))
}
