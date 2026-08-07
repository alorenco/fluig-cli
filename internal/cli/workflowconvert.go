package cli

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
)

// workflow convert — converte solicitações abertas de uma versão do processo
// para outra (o wizard "Converter processos" do portal, sem o portal).
//
// O comando tem três níveis, do leitura ao efeito:
//   1. sem flags de versão  → tabela de versões com tarefas abertas;
//   2. --from/--to          → plano: de-para de etapas + solicitações (read-only);
//   3. --instance/--all     → converte, com confirmação e results[] por item.
//
// O de-para é identidade (mesma sequence nas duas versões) completado por
// --map. O servidor só valida a etapa ABERTA de cada solicitação (validado ao
// vivo em 2026-08-07) — por isso o --map só é exigido para etapa aberta órfã.
// O sucesso de cada conversão é confirmado pelo processVersion da solicitação
// (REST v2), porque o convertProcess responde HTTP 200 com texto localizado
// tanto no sucesso quanto no erro.
func newWorkflowConvertCmd(app *App) *cobra.Command {
	var (
		passwordStdin bool
		fromVersion   int
		toVersion     int
		instanceIDs   []int
		all           bool
		mapPairs      []string
	)
	cmd := &cobra.Command{
		Use:   "convert <processId>",
		Short: "Converte solicitações abertas de uma versão do processo para outra",
		Long: "Converte solicitações abertas de uma versão do processo para outra,\n" +
			"como o wizard \"Converter processos\" do portal. Exige papel de\n" +
			"administrador no servidor.\n\n" +
			"Sem --from/--to, lista as versões do processo com as tarefas abertas\n" +
			"de cada uma. Com --from e --to, mostra o plano: o de-para de etapas e\n" +
			"as solicitações abertas — nada é convertido. Para converter, selecione\n" +
			"com --all (todas) ou --instance (uma ou mais).\n\n" +
			"O de-para liga cada etapa da origem à etapa de mesmo número no\n" +
			"destino. Quando a etapa não existe no destino, complete com --map\n" +
			"(ex.: --map 66=29). O --map só é obrigatório para etapa com tarefa\n" +
			"aberta. A conversão pede confirmação (pule com --yes) e reporta um\n" +
			"resultado por solicitação; falha parcial termina com exit 6.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			processID := args[0]
			hasFrom := cmd.Flags().Changed("from")
			hasTo := cmd.Flags().Changed("to")
			selection := all || len(instanceIDs) > 0

			switch {
			case hasFrom != hasTo:
				return output.Usagef("--from e --to andam juntos: informe a versão de origem e a de destino")
			case all && len(instanceIDs) > 0:
				return output.Usagef("--all e --instance não se combinam: escolha um")
			case selection && !hasFrom:
				return output.Usagef("--all/--instance exigem --from e --to")
			case len(mapPairs) > 0 && !hasFrom:
				return output.Usagef("--map exige --from e --to")
			case hasFrom && fromVersion == toVersion:
				return output.Usagef("--from e --to devem ser versões diferentes")
			}

			ctx := context.Background()
			if !selection {
				_, client, err := app.connect(ctx, passwordStdin)
				if err != nil {
					return err
				}
				if !hasFrom {
					return convertShowVersions(ctx, p, client, processID)
				}
				return convertShowPlan(ctx, p, client, processID, fromVersion, toVersion, mapPairs)
			}
			_, client, err := app.connectWrite(ctx, passwordStdin, "converter solicitações de versão")
			if err != nil {
				return err
			}
			return convertRun(ctx, app, p, client, processID, fromVersion, toVersion, mapPairs, instanceIDs, all)
		},
	}
	cmd.Flags().IntVar(&fromVersion, "from", 0, "versão de origem das solicitações")
	cmd.Flags().IntVar(&toVersion, "to", 0, "versão de destino")
	cmd.Flags().IntSliceVar(&instanceIDs, "instance", nil, "solicitação a converter (repetível)")
	cmd.Flags().BoolVar(&all, "all", false, "converte todas as solicitações abertas da versão de origem")
	cmd.Flags().StringSliceVar(&mapPairs, "map", nil, "de-para manual de etapas origem=destino (ex.: --map 66=29,72=13)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// convertShowVersions imprime as versões do processo com as tarefas abertas.
// O ProcessVersions (REST v2) valida o processo e traz ativa/em edição; o
// ConvertVersions (legado) traz a contagem de tarefas abertas por versão.
func convertShowVersions(ctx context.Context, p *output.Printer, client *fluig.Client, processID string) error {
	versions, err := client.ProcessVersions(ctx, processID)
	if err != nil {
		if errors.Is(err, fluig.ErrNotFound) {
			return processNotFound(ctx, client, processID, false)
		}
		return mapFluigError(err)
	}
	if len(versions) == 0 {
		return processNotFound(ctx, client, processID, false)
	}
	open, err := client.ConvertVersions(ctx, processID)
	if err != nil {
		return mapFluigError(err)
	}
	openByVersion := make(map[int]int, len(open))
	for _, v := range open {
		openByVersion[v.Version] = v.OpenTasks
	}

	type versionRow struct {
		Version   int  `json:"version"`
		Active    bool `json:"active"`
		Editing   bool `json:"editing"`
		OpenTasks int  `json:"openTasks"`
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i].Version > versions[j].Version })
	list := make([]versionRow, 0, len(versions))
	rows := make([][]string, 0, len(versions))
	for _, v := range versions {
		r := versionRow{Version: v.Version, Active: v.Active, Editing: v.Editing, OpenTasks: openByVersion[v.Version]}
		list = append(list, r)
		openCell := strconv.Itoa(r.OpenTasks)
		if r.Editing {
			openCell = "-" // versão em edição não participa da conversão
		}
		rows = append(rows, []string{strconv.Itoa(r.Version), simNao(r.Active), openCell})
	}
	p.Table(output.Table{
		Headers: []string{"Versão", "Ativa", "Tarefas abertas"},
		Rows:    rows,
		Style: output.BoldHeaderStyle(func(row, col int, padded string) string {
			if col == 1 && list[row].Active {
				return output.Green(padded)
			}
			if col == 2 && list[row].OpenTasks > 0 && !list[row].Editing {
				return output.Yellow(padded)
			}
			return padded
		}),
	})
	p.Infof("converta com: workflow convert %s --from <versão> --to <versão>", processID)
	p.Done(map[string]any{"processId": processID, "versions": list})
	return nil
}

// convertMapping é o de-para de uma etapa da origem, para o plano e o --json.
type convertMapping struct {
	From      int    `json:"from"`
	FromName  string `json:"fromName"`
	To        int    `json:"to,omitempty"`
	ToName    string `json:"toName,omitempty"`
	OpenTasks int    `json:"openTasks"`
	Manual    bool   `json:"manual,omitempty"` // veio de --map
}

// convertPlan reúne tudo que o plano e a conversão precisam: as etapas das
// duas versões, o de-para resolvido (identidade + --map), as tarefas abertas
// por etapa e as solicitações abertas da origem.
type convertPlan struct {
	mappings  []convertMapping
	instances []fluig.ConvertInstanceSummary
	destNames map[int]string
}

// mappedArrays devolve os arrays paralelos do convertProcess (só as etapas
// com destino resolvido).
func (pl *convertPlan) mappedArrays() (actual, next []int) {
	for _, m := range pl.mappings {
		if m.To != 0 {
			actual = append(actual, m.From)
			next = append(next, m.To)
		}
	}
	return actual, next
}

// mappingFor devolve o de-para de uma etapa da origem (nil se não mapeada).
func (pl *convertPlan) mappingFor(sequence int) *convertMapping {
	for i := range pl.mappings {
		if pl.mappings[i].From == sequence {
			return &pl.mappings[i]
		}
	}
	return nil
}

// buildConvertPlan valida as versões, resolve o de-para e busca as
// solicitações abertas. Erros de uso (versão inexistente, --map inválido)
// saem como Usagef, antes de qualquer escrita.
func buildConvertPlan(ctx context.Context, client *fluig.Client, processID string, from, to int, mapPairs []string) (*convertPlan, error) {
	sourceStates, err := convertVersionStates(ctx, client, processID, from)
	if err != nil {
		return nil, err
	}
	destStates, err := convertVersionStates(ctx, client, processID, to)
	if err != nil {
		return nil, err
	}
	destNames := make(map[int]string, len(destStates))
	for _, s := range destStates {
		destNames[s.Sequence] = s.Name
	}

	manual := map[int]int{}
	for _, pair := range mapPairs {
		src, dst, err := parseMapPair(pair)
		if err != nil {
			return nil, err
		}
		manual[src] = dst
	}

	sourceSeqs := make(map[int]bool, len(sourceStates))
	plan := &convertPlan{destNames: destNames}
	for _, s := range sourceStates {
		sourceSeqs[s.Sequence] = true
		m := convertMapping{From: s.Sequence, FromName: s.Name}
		if dst, ok := manual[s.Sequence]; ok {
			if _, exists := destNames[dst]; !exists {
				return nil, output.Usagef("--map %d=%d: a etapa %d não existe na versão %d", s.Sequence, dst, dst, to)
			}
			m.To, m.ToName, m.Manual = dst, destNames[dst], true
		} else if name, ok := destNames[s.Sequence]; ok {
			m.To, m.ToName = s.Sequence, name
		}
		plan.mappings = append(plan.mappings, m)
	}
	for src := range manual {
		if !sourceSeqs[src] {
			return nil, output.Usagef("--map %d=%d: a etapa %d não existe na versão %d", src, manual[src], src, from)
		}
	}

	plan.instances, err = client.ConvertInstances(ctx, processID, from)
	if err != nil {
		return nil, mapFluigError(err)
	}
	sort.Slice(plan.instances, func(i, j int) bool { return plan.instances[i].ID < plan.instances[j].ID })
	return plan, nil
}

// convertVersionStates busca as etapas de uma versão, traduzindo o 404 do
// processo/versão para erro de uso claro.
func convertVersionStates(ctx context.Context, client *fluig.Client, processID string, version int) ([]fluig.ProcessState, error) {
	states, err := client.ProcessStates(ctx, processID, version)
	if err != nil {
		if errors.Is(err, fluig.ErrNotFound) {
			return nil, processNotFound(ctx, client, processID, false)
		}
		return nil, mapFluigError(err)
	}
	if len(states) == 0 {
		return nil, output.Usagef("a versão %d do processo %q não existe (liste com: workflow convert %s)", version, processID, processID)
	}
	return states, nil
}

// parseMapPair interpreta um item do --map ("66=29").
func parseMapPair(pair string) (src, dst int, err error) {
	parts := strings.SplitN(pair, "=", 2)
	if len(parts) == 2 {
		s, errS := strconv.Atoi(strings.TrimSpace(parts[0]))
		d, errD := strconv.Atoi(strings.TrimSpace(parts[1]))
		if errS == nil && errD == nil {
			return s, d, nil
		}
	}
	return 0, 0, output.Usagef("--map %q: use o formato etapaOrigem=etapaDestino (ex.: --map 66=29)", pair)
}

// fillOpenTasks preenche a contagem de tarefas abertas por etapa no plano.
func fillOpenTasks(ctx context.Context, client *fluig.Client, processID string, from int, plan *convertPlan) error {
	seqs := make([]int, 0, len(plan.mappings))
	for _, m := range plan.mappings {
		seqs = append(seqs, m.From)
	}
	counts, err := client.ConvertStateCounts(ctx, processID, from, seqs)
	if err != nil {
		return mapFluigError(err)
	}
	for i := range plan.mappings {
		plan.mappings[i].OpenTasks = counts[plan.mappings[i].From]
	}
	return nil
}

// convertShowPlan imprime o plano: o de-para e as solicitações abertas.
func convertShowPlan(ctx context.Context, p *output.Printer, client *fluig.Client, processID string, from, to int, mapPairs []string) error {
	plan, err := buildConvertPlan(ctx, client, processID, from, to, mapPairs)
	if err != nil {
		return err
	}
	if err := fillOpenTasks(ctx, client, processID, from, plan); err != nil {
		return err
	}

	var unmappedOpen []int
	rows := make([][]string, 0, len(plan.mappings))
	for _, m := range plan.mappings {
		dest := "sem destino"
		destName := "-"
		if m.To != 0 {
			dest = strconv.Itoa(m.To)
			destName = m.ToName
			if m.Manual {
				dest += " (--map)"
			}
		} else if m.OpenTasks > 0 {
			unmappedOpen = append(unmappedOpen, m.From)
		}
		rows = append(rows, []string{
			strconv.Itoa(m.From), m.FromName, strconv.Itoa(m.OpenTasks), dest, destName,
		})
	}
	mappings := plan.mappings
	p.Table(output.Table{
		Headers: []string{"Etapa", "Nome", "Abertas", "Destino", "Nome no destino"},
		Rows:    rows,
		Style: output.BoldHeaderStyle(func(row, col int, padded string) string {
			if mappings[row].To == 0 && mappings[row].OpenTasks > 0 {
				return output.Red(padded)
			}
			if col == 2 && mappings[row].OpenTasks > 0 {
				return output.Yellow(padded)
			}
			return padded
		}),
	})

	if len(plan.instances) == 0 {
		p.Infof("nenhuma solicitação aberta na versão %d do processo %q", from, processID)
	} else {
		instRows := make([][]string, 0, len(plan.instances))
		for _, in := range plan.instances {
			instRows = append(instRows, []string{
				strconv.Itoa(in.ID), in.Requester, in.State, in.Assignee, in.Deadline,
			})
		}
		p.Table(output.Table{
			Headers: []string{"Solicitação", "Solicitante", "Etapa", "Responsável", "Prazo"},
			Rows:    instRows,
			Style:   output.BoldHeaderStyle(nil),
		})
	}

	switch {
	case len(unmappedOpen) > 0:
		p.Warnf("etapas com tarefa aberta e sem destino: %s — complete o de-para com --map antes de converter", joinInts(unmappedOpen))
	case len(plan.instances) > 0:
		p.Infof("plano apenas — nada foi convertido; selecione com --all ou --instance")
	}
	p.Done(map[string]any{
		"processId": processID,
		"from":      from,
		"to":        to,
		"mapping":   plan.mappings,
		"instances": plan.instances,
	})
	return nil
}

// convertRun converte as solicitações selecionadas, com pré-checagem do
// de-para e confirmação. Cada conversão é verificada pelo processVersion da
// solicitação (REST v2) — o convertProcess responde 200 até no erro.
func convertRun(ctx context.Context, app *App, p *output.Printer, client *fluig.Client, processID string, from, to int, mapPairs []string, instanceIDs []int, all bool) error {
	plan, err := buildConvertPlan(ctx, client, processID, from, to, mapPairs)
	if err != nil {
		return err
	}

	byID := make(map[int]fluig.ConvertInstanceSummary, len(plan.instances))
	for _, in := range plan.instances {
		byID[in.ID] = in
	}
	var selected []int
	if all {
		if len(plan.instances) == 0 {
			p.Infof("nenhuma solicitação aberta na versão %d do processo %q — nada a converter", from, processID)
			p.Done(map[string]any{"processId": processID, "from": from, "to": to, "results": []itemResult{}})
			return nil
		}
		for _, in := range plan.instances {
			selected = append(selected, in.ID)
		}
	} else {
		var missing []int
		for _, id := range instanceIDs {
			if _, ok := byID[id]; !ok {
				missing = append(missing, id)
				continue
			}
			selected = append(selected, id)
		}
		if len(missing) > 0 {
			return output.NotFoundf("solicitação(ões) %s não estão abertas na versão %d do processo %q (liste com: workflow convert %s --from %d --to %d)",
				joinInts(missing), from, processID, processID, from, to)
		}
	}

	if err := convertPrecheck(ctx, client, processID, from, plan, selected, all); err != nil {
		return err
	}

	if err := app.confirm(fmt.Sprintf("Converter %d solicitação(ões) do processo %q da versão %d para a %d?",
		len(selected), processID, from, to)); err != nil {
		return err
	}

	actual, next := plan.mappedArrays()
	var results []itemResult
	var lastErr error
	failures := 0
	for _, id := range selected {
		msg, cerr := convertOne(ctx, client, id, to, actual, next)
		if cerr != nil {
			failures++
			lastErr = cerr
			results = append(results, itemResult{ID: strconv.Itoa(id), Action: "failed", Success: false, Error: output.AsError(cerr).Message})
			p.Warnf("solicitação %d: %s", id, output.AsError(cerr).Message)
			continue
		}
		results = append(results, itemResult{ID: strconv.Itoa(id), Action: "converted", Success: true})
		p.Successf("solicitação %d convertida para a versão %d%s", id, to, msg)
	}
	return finishBatch(p, lastErr, map[string]any{
		"processId": processID, "from": from, "to": to, "results": results,
	}, failures, len(selected))
}

// convertPrecheck garante, ANTES do primeiro POST, que toda etapa aberta das
// solicitações selecionadas tem destino no de-para. Com --all a contagem por
// etapa resolve numa chamada; com --instance a etapa aberta de cada
// solicitação vem da própria solicitação (REST v2).
func convertPrecheck(ctx context.Context, client *fluig.Client, processID string, from int, plan *convertPlan, selected []int, all bool) error {
	blocked := map[int]bool{}
	if all {
		if err := fillOpenTasks(ctx, client, processID, from, plan); err != nil {
			return err
		}
		for _, m := range plan.mappings {
			if m.To == 0 && m.OpenTasks > 0 {
				blocked[m.From] = true
			}
		}
	} else {
		for _, id := range selected {
			req, err := client.GetRequest(ctx, id)
			if err != nil {
				return mapFluigError(err)
			}
			for _, step := range req.CurrentSteps {
				if m := plan.mappingFor(step.Sequence); m == nil || m.To == 0 {
					blocked[step.Sequence] = true
				}
			}
		}
	}
	if len(blocked) == 0 {
		return nil
	}
	seqs := make([]int, 0, len(blocked))
	for s := range blocked {
		seqs = append(seqs, s)
	}
	sort.Ints(seqs)
	parts := make([]string, 0, len(seqs))
	for _, s := range seqs {
		name := ""
		if m := plan.mappingFor(s); m != nil {
			name = " (" + m.FromName + ")"
		}
		parts = append(parts, fmt.Sprintf("%d%s", s, name))
	}
	return output.Usagef("etapa(s) com tarefa aberta e sem destino na versão nova: %s — complete o de-para com --map (ex.: --map %d=<etapa destino>)",
		strings.Join(parts, ", "), seqs[0])
}

// convertOne converte uma solicitação e confirma o resultado pela versão da
// solicitação. O texto do servidor (conversionLog) vira o erro quando a
// versão não muda.
func convertOne(ctx context.Context, client *fluig.Client, id, to int, actual, next []int) (string, error) {
	log, err := client.ConvertInstanceVersion(ctx, fluig.ConvertCall{
		InstanceID:   id,
		NewVersion:   to,
		ActualStates: actual,
		NewStates:    next,
	})
	if err != nil {
		return "", mapFluigError(err)
	}
	req, err := client.GetRequest(ctx, id)
	if err != nil {
		return "", output.ServerErrorf("conversão enviada, mas a verificação falhou: %s (confira a versão da solicitação %d)",
			output.AsError(mapFluigError(err)).Message, id)
	}
	if req.ProcessVersion != to {
		msg := strings.TrimSpace(log)
		if msg == "" {
			msg = fmt.Sprintf("a solicitação continua na versão %d", req.ProcessVersion)
		}
		return "", output.ServerErrorf("%s", msg)
	}
	return "", nil
}

// joinInts imprime uma lista de inteiros separada por vírgula.
func joinInts(list []int) string {
	parts := make([]string, len(list))
	for i, v := range list {
		parts[i] = strconv.Itoa(v)
	}
	return strings.Join(parts, ", ")
}
