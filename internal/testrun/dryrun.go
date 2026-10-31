package testrun

import (
	"context"
	"fmt"
	"strings"
)

// DryRunSuite valida a suíte contra o diagrama da versão ativa SEM escrever:
// destinos existentes, transições possíveis a partir da etapa esperada no passo
// anterior, etapas de expect.state existentes e assume em etapa de pool com o
// ator no papel/grupo. Nenhuma solicitação é aberta.
//
// Devolve o resultado (cenários "validated" ou "failed") e a lista de
// problemas encontrados.
func DryRunSuite(ctx context.Context, eng Engine, s *Suite, proc *ProcessInfo, opts Options) (SuiteResult, []string) {
	sr := SuiteResult{File: s.File, Process: s.Process, ProcessVersion: proc.Version, Description: s.Description}
	var problemas []string

	var roles, groups map[string]bool
	carregouMembros := false
	membros := func() (map[string]bool, map[string]bool, error) {
		if carregouMembros {
			return roles, groups, nil
		}
		rs, gs, err := eng.SelfMembership(ctx)
		if err != nil {
			return nil, nil, err
		}
		roles, groups = toSet(rs), toSet(gs)
		carregouMembros = true
		return roles, groups, nil
	}

	for _, sc := range s.Scenarios {
		if !opts.Selected(sc.Name) {
			continue
		}
		res := ScenarioResult{Name: sc.Name, Result: ResValidated}
		// atual: etapa onde a solicitação deve estar (0 = desconhecida).
		atual := 0
		avisouDesconhecida := false
		for _, st := range sc.Steps {
			step := StepResult{Index: st.Index, Name: st.Name, Action: st.Action, Result: ResValidated}
			var erros []string
			alvoExiste := func(seq int, papel string) {
				if _, ok := proc.States[seq]; !ok {
					erros = append(erros, fmt.Sprintf("%s %d não existe na versão %d do processo", papel, seq, proc.Version))
				}
			}
			switch st.Action {
			case ActStart:
				if st.Target > 0 {
					alvoExiste(st.Target, "a etapa de destino")
					ok := false
					for _, ini := range proc.Start {
						if hasTransition(proc, ini, st.Target) {
							ok = true
						}
					}
					if !ok && len(erros) == 0 {
						erros = append(erros, fmt.Sprintf("não há transição do início para a etapa %d", st.Target))
					}
				}
			case ActMove, ActManager:
				alvoExiste(st.Target, "a etapa de destino")
				if atual == 0 {
					if !avisouDesconhecida {
						sr.Warnings = append(sr.Warnings, fmt.Sprintf(
							"cenário %q, passo %d: transição não conferida, porque o passo anterior não tem expect.state",
							sc.Name, st.Index))
						avisouDesconhecida = true
					}
				} else if len(erros) == 0 && !hasTransition(proc, atual, st.Target) {
					erros = append(erros, fmt.Sprintf("não há transição da etapa %d (%s) para a %d; destinos possíveis: %s",
						atual, proc.States[atual].Name, st.Target, joinInts(proc.States[atual].Transitions)))
				}
			case ActUI:
				if st.UI.Send > 0 {
					alvoExiste(st.UI.Send, "a etapa do send")
					if atual != 0 && len(erros) == 0 && !hasTransition(proc, atual, st.UI.Send) {
						erros = append(erros, fmt.Sprintf("não há transição da etapa %d (%s) para a %d; destinos possíveis: %s",
							atual, proc.States[atual].Name, st.UI.Send, joinInts(proc.States[atual].Transitions)))
					}
				}
			case ActAssume:
				if atual != 0 {
					est := proc.States[atual]
					if !strings.HasPrefix(est.Mechanism, "Pool") {
						erros = append(erros, fmt.Sprintf("a etapa %d (%s) não é de pool (mecanismo %q); assume só vale em Pool Papel ou Pool Grupo",
							atual, est.Name, est.Mechanism))
					} else if rs, gs, err := membros(); err != nil {
						sr.Warnings = append(sr.Warnings, "não consegui ler os papéis e grupos do usuário: "+err.Error())
					} else {
						if est.AssignKind == "role" && !rs[est.AssignValue] {
							erros = append(erros, fmt.Sprintf("o usuário %s não está no papel %q da etapa %d", opts.SelfLogin, est.AssignValue, atual))
						}
						if est.AssignKind == "group" && !gs[est.AssignValue] {
							erros = append(erros, fmt.Sprintf("o usuário %s não está no grupo %q da etapa %d", opts.SelfLogin, est.AssignValue, atual))
						}
					}
				}
			}
			if st.Expect != nil && st.Expect.State != nil {
				alvoExiste(*st.Expect.State, "a etapa de expect.state")
			}
			// Atualiza a etapa esperada para o próximo passo.
			switch {
			case st.Expect != nil && st.Expect.Error != nil:
				// erro esperado: a solicitação fica onde estava
			case st.Expect != nil && st.Expect.UI != nil && st.Expect.UI.Message != nil:
				// recusa esperada na tela: idem
			case st.Expect != nil && st.Expect.State != nil:
				atual = *st.Expect.State
			case st.Action == ActWait && st.WaitState > 0:
				atual = st.WaitState
			case st.Action == ActStart, st.Action == ActMove, st.Action == ActManager, st.Action == ActCancel,
				st.Action == ActUI && st.UI.Send > 0:
				atual = 0
			}
			if len(erros) > 0 {
				step.Result = ResFailed
				step.Error = strings.Join(erros, "; ")
				res.Result = ResFailed
				for _, e := range erros {
					problemas = append(problemas, fmt.Sprintf("%s: cenário %q, passo %d (%s): %s", s.File, sc.Name, st.Index, st.Name, e))
				}
			}
			res.Steps = append(res.Steps, step)
		}
		sr.Scenarios = append(sr.Scenarios, res)
	}
	return sr, problemas
}

func hasTransition(proc *ProcessInfo, from, to int) bool {
	for _, t := range proc.States[from].Transitions {
		if t == to {
			return true
		}
	}
	return false
}

func toSet(xs []string) map[string]bool {
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}

func joinInts(xs []int) string {
	if len(xs) == 0 {
		return "(nenhum)"
	}
	p := make([]string, len(xs))
	for i, x := range xs {
		p[i] = fmt.Sprint(x)
	}
	return strings.Join(p, ", ")
}
