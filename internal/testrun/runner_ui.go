package testrun

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// uiStep executa um passo ui e avalia as asserções de tela e as da API.
func (r *scenarioRun) uiStep(ctx, parent context.Context, st *Step, sr *StepResult) {
	u := st.UI
	var e *UIExpect
	if st.Expect != nil {
		e = st.Expect.UI
	}
	fail := func(result, msg string) {
		sr.Result, sr.Error = result, msg
		r.uiScreenshot(ctx, st, sr)
	}
	infra := func(err error) string {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) && parent.Err() == nil {
			return ResErrored
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return ResErrored
		}
		return ResFailed
	}

	if r.opts.UI == nil {
		sr.Result, sr.Error = ResErrored, "o passo ui precisa de navegador, e nenhum foi configurado"
		return
	}
	if r.ui == nil {
		sess, err := r.opts.UI.NewSession(ctx)
		if err != nil {
			sr.Result, sr.Error = ResErrored, "o navegador não abriu: "+err.Error()
			return
		}
		r.ui = sess
	}
	vp := u.Viewport
	if vp == nil {
		vp = r.suite.Defaults.UIViewport
	}
	if vp == nil {
		vp = &UIViewport{Name: "desktop", Width: 1366, Height: 900}
	}
	if err := r.ui.SetViewport(ctx, *vp); err != nil {
		fail(ResErrored, "falha ao mudar o tamanho da janela: "+err.Error())
		return
	}

	// changed/unchanged: o valor de antes vem da tela no início do passo.
	var watch []string
	if e != nil {
		watch = append(append(watch, e.Changed...), e.Unchanged...)
	}
	var before map[string]*string
	if len(watch) > 0 && u.Open == "" {
		b, err := r.ui.Hidden(ctx, watch)
		if err != nil {
			fail(infra(err), "falha ao ler a tela: "+err.Error())
			return
		}
		before = b
	}

	var send *UISendResult
	var err error
	switch {
	case u.Open == "task":
		var info *RequestInfo
		info, err = r.eng.Request(ctx, r.requestID)
		if err == nil {
			cur := dedupSteps(info.Current)
			if len(cur) == 0 {
				err = fmt.Errorf("a solicitação %d não tem tarefa aberta (status %s)", r.requestID, info.Status)
			} else {
				err = r.ui.OpenTask(ctx, r.requestID, cur[0].Movement, r.opts.SelfCode)
			}
		}
	case u.Open == "view":
		err = r.ui.OpenView(ctx, r.requestID)
	case u.Click != "":
		err = r.ui.Click(ctx, u.Click)
	}
	if err == nil && len(u.Fill) > 0 {
		fill, ferr := r.resolvePairs(u.Fill)
		if ferr != nil {
			err = ferr
		} else {
			err = r.ui.Fill(ctx, fill)
		}
	}
	if err == nil && u.Send > 0 {
		send, err = r.ui.Send(ctx, u.Send)
	}
	if err == nil && u.Save {
		send, err = r.ui.Save(ctx)
	}
	if err != nil {
		fail(infra(err), err.Error())
		return
	}
	// Recusa na tela sem "message" no expect = falha com o texto da recusa.
	if send != nil && send.Message != "" && (e == nil || e.Message == nil) {
		r.ui.Reset(ctx)
		fail(ResFailed, "o Fluig recusou o envio: "+NormalizeServerText(send.Message))
		return
	}

	var as []Assertion
	if e != nil {
		var state *UIState
		if e.needsInspect() {
			state, err = r.ui.Inspect(ctx, e.uiNames(), e.cssQuery())
			if err != nil {
				fail(infra(err), "falha ao ler a tela: "+err.Error())
				return
			}
		}
		after := map[string]*string{}
		if len(watch) > 0 {
			if send != nil && send.Posted && send.FormData != nil {
				// O valor que conta é o ENVIADO (o beforeSendValidate muda o
				// hidden imediatamente antes do POST).
				for _, n := range watch {
					if v, ok := send.FormData[n]; ok {
						after[n] = &v
					}
				}
			} else if h, herr := r.ui.Hidden(ctx, watch); herr == nil {
				after = h
			}
			if before == nil {
				before = map[string]*string{}
			}
		}
		uas, err := evalUI(e, state, send, before, after, r.env)
		if err != nil {
			fail(ResFailed, err.Error())
			return
		}
		as = append(as, uas...)
	}
	if send != nil && send.Message != "" {
		r.ui.Reset(ctx)
	}

	// Asserções da API (state, status, card, path, assignee).
	if st.Expect != nil {
		need := NeedOf(st.Expect)
		if need.Request || need.Tasks || need.Card {
			snap, err := r.snapshot(ctx, need)
			if err != nil {
				_, msg, _ := classify(err)
				fail(ResErrored, "falha ao reler a solicitação: "+msg)
				return
			}
			api, err := Evaluate(st.Expect, snap, nil, r.env)
			if err != nil {
				fail(ResFailed, err.Error())
				return
			}
			as = append(as, api...)
		}
	}
	sr.Assertions = as
	sr.Result = ResPassed
	for _, a := range as {
		if !a.Passed {
			sr.Result = ResFailed
		}
	}
	if sr.Result != ResPassed {
		r.uiScreenshot(ctx, st, sr)
	}
}

func (r *scenarioRun) resolvePairs(pairs [][2]string) ([][2]string, error) {
	out := make([][2]string, len(pairs))
	for i, p := range pairs {
		v, err := Resolve(p[1], r.env)
		if err != nil {
			return nil, fmt.Errorf("fill.%s: %w", p[0], err)
		}
		out[i] = [2]string{p[0], v}
	}
	return out, nil
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)

// uiScreenshot grava o PNG da tela na pasta de artefatos e anota o caminho.
func (r *scenarioRun) uiScreenshot(ctx context.Context, st *Step, sr *StepResult) {
	if r.ui == nil || r.opts.UIArtifacts == "" {
		return
	}
	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
	defer cancel()
	png, err := r.ui.Screenshot(sctx)
	if err != nil || len(png) == 0 {
		return
	}
	base := strings.TrimSuffix(filepath.Base(r.suite.File), filepath.Ext(r.suite.File))
	name := fmt.Sprintf("%s-c%02d-p%02d-%d.png", unsafeName.ReplaceAllString(base, "_"), r.sc.Index, st.Index, r.requestID)
	if err := os.MkdirAll(r.opts.UIArtifacts, 0o755); err != nil {
		return
	}
	p := filepath.Join(r.opts.UIArtifacts, name)
	if os.WriteFile(p, png, 0o644) == nil {
		sr.Screenshot = p
	}
}
