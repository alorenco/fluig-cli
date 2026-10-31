package testrun

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/alorenco/fluig-cli/internal/browser"
)

// BrowserDriver é o UIDriver real: Chrome/Chromium/Edge via chromedp, com os
// cookies da sessão do fluigcli.
type BrowserDriver struct {
	ExecPath string
	Headed   bool
	Base     *url.URL
	Jar      http.CookieJar
	// SendWait é o teto de espera do desfecho de um envio pela tela.
	SendWait time.Duration
}

// NewSession abre uma janela nova.
func (d *BrowserDriver) NewSession(ctx context.Context) (UISession, error) {
	// O navegador vive o cenário inteiro: não herda o prazo do passo.
	s, err := browser.Open(context.WithoutCancel(ctx), d.ExecPath, d.Headed, d.Base, d.Jar)
	if err != nil {
		return nil, err
	}
	wait := d.SendWait
	if wait <= 0 {
		wait = 2 * time.Minute
	}
	return &browserSession{s: s, wait: wait}, nil
}

type browserSession struct {
	s    *browser.Session
	wait time.Duration
}

func (b *browserSession) SetViewport(ctx context.Context, vp UIViewport) error {
	return b.s.SetViewport(ctx, browser.Viewport{Width: vp.Width, Height: vp.Height, Mobile: vp.Mobile})
}

func (b *browserSession) OpenTask(ctx context.Context, requestID, movement int, userCode string) error {
	return b.s.OpenTask(ctx, requestID, movement, userCode)
}

func (b *browserSession) OpenView(ctx context.Context, requestID int) error {
	return b.s.OpenView(ctx, requestID)
}

func (b *browserSession) Fill(ctx context.Context, pairs [][2]string) error {
	return b.s.Fill(ctx, pairs)
}

func (b *browserSession) Click(ctx context.Context, key string) error { return b.s.Click(ctx, key) }

func (b *browserSession) Send(ctx context.Context, state int) (*UISendResult, error) {
	r, err := b.s.Send(ctx, state, b.wait)
	return toUISend(r), err
}

func (b *browserSession) Save(ctx context.Context) (*UISendResult, error) {
	r, err := b.s.Save(ctx, b.wait)
	return toUISend(r), err
}

func toUISend(r *browser.SendResult) *UISendResult {
	if r == nil {
		return nil
	}
	return &UISendResult{Posted: r.Posted, Status: r.Status, Message: r.Message, FormData: r.FormData}
}

func (b *browserSession) Inspect(ctx context.Context, names []string, css map[string][]string) (*UIState, error) {
	st, err := b.s.Inspect(ctx, names, css)
	if err != nil {
		return nil, err
	}
	out := &UIState{Elems: map[string]UIElem{}, CSS: st.CSS}
	for k, e := range st.Elems {
		out.Elems[k] = UIElem{Exists: e.Exists, Visible: e.Visible, Editable: e.Editable, Value: e.Value, Text: e.Text}
	}
	return out, nil
}

func (b *browserSession) Hidden(ctx context.Context, names []string) (map[string]*string, error) {
	return b.s.Hidden(ctx, names)
}

func (b *browserSession) Reset(ctx context.Context) { b.s.Reset(ctx) }

func (b *browserSession) Screenshot(ctx context.Context) ([]byte, error) { return b.s.Screenshot(ctx) }

func (b *browserSession) Close() { b.s.Close() }
