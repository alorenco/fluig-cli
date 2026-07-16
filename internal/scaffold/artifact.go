package scaffold

// Esqueletos dos demais artefatos do projeto (além de widgets): dataset
// customizado, evento global, mecanismo de atribuição, formulário e script
// de evento de processo. Templates em templates/artifact/, mesmas convenções
// do scaffold de widgets ([[ ]], __code__).

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Erros sentinela — a CLI os traduz para erro de uso (exit 2).
var (
	// ErrFileExists indica que o arquivo de destino já existe.
	ErrFileExists = errors.New("o arquivo já existe")
	// ErrUnknownEvent indica evento de processo fora do catálogo.
	ErrUnknownEvent = errors.New("evento de processo desconhecido")
)

// artifactNameRe valida nomes de dataset/evento global/mecanismo/formulário:
// viram nome de arquivo e, no evento global, nome de função JS — letras,
// dígitos e _, começando por letra (camelCase permitido, ≠ código de widget).
var artifactNameRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)

// processIDRe valida o id de processo do script. Pontos intermediários são
// aceitos (o separador do nome do arquivo é o ÚLTIMO ponto), mas não no fim.
var processIDRe = regexp.MustCompile(`^[A-Za-z]([A-Za-z0-9_.]*[A-Za-z0-9_])?$`)

// ValidateArtifactName confere o nome de dataset/evento/mecanismo/formulário.
func ValidateArtifactName(name string) error {
	if !artifactNameRe.MatchString(name) || len(name) > 100 {
		return fmt.Errorf("%w: %q (use letras, dígitos e _, começando por letra; máx. 100)", ErrInvalidCode, name)
	}
	return nil
}

// ProcessEvent descreve um evento de script de processo do Fluig.
type ProcessEvent struct {
	Name   string // nome do evento (= nome da função no script)
	Params string // parâmetros da assinatura
	Doc    string // quando roda (frase curta, começando em minúscula)
}

// processEvents é o catálogo dos eventos de processo, em ordem alfabética,
// com as assinaturas da documentação oficial do Fluig (beforeTaskSave
// confirmada em export real da homologação).
var processEvents = []ProcessEvent{
	{"afterCancelProcess", "colleagueId, processId", "depois do cancelamento da solicitação"},
	{"afterProcessCreate", "processId", "depois da criação da solicitação"},
	{"afterProcessFinish", "processId", "depois do encerramento da solicitação"},
	{"afterReleaseVersion", "processXML", "depois da liberação de uma versão do processo"},
	{"afterStateEntry", "sequenceId", "depois da entrada em uma atividade"},
	{"afterStateLeave", "sequenceId", "depois da saída de uma atividade"},
	{"afterTaskComplete", "colleagueId, nextSequenceId, userList", "depois da conclusão da tarefa (movimentação efetivada)"},
	{"afterTaskCreate", "colleagueId", "depois da criação da tarefa para o usuário"},
	{"afterTaskSave", "colleagueId, nextSequenceId, userList", "depois de salvar a tarefa"},
	{"beforeCancelProcess", "colleagueId, processId", "antes do cancelamento da solicitação"},
	{"beforeSendData", "customFields, customFacts", "antes do envio de dados ao analytics (BAM)"},
	{"beforeStateEntry", "sequenceId", "antes da entrada em uma atividade"},
	{"beforeStateLeave", "sequenceId", "antes da saída de uma atividade"},
	{"beforeTaskComplete", "colleagueId, nextSequenceId, userList", "antes da conclusão da tarefa"},
	{"beforeTaskCreate", "colleagueId", "antes da criação da tarefa para o usuário"},
	{"beforeTaskSave", "colleagueId, nextSequenceId, userList", "antes de salvar/enviar a tarefa (validações de movimentação)"},
	{"onNotify", "subject, receivers, template, params", "na geração das notificações do processo"},
	{"subProcessCreated", "processId", "depois da criação de um subprocesso"},
	{"validateAvailableStates", "iCurrentState, stateList", "ao montar a lista de próximas atividades da movimentação"},
}

// ProcessEvents devolve o catálogo de eventos de processo (cópia).
func ProcessEvents() []ProcessEvent {
	out := make([]ProcessEvent, len(processEvents))
	copy(out, processEvents)
	return out
}

// FindProcessEvent localiza um evento por nome (case-insensitive) e devolve a
// forma canônica do catálogo.
func FindProcessEvent(name string) (ProcessEvent, bool) {
	for _, e := range processEvents {
		if strings.EqualFold(e.Name, name) {
			return e, true
		}
	}
	return ProcessEvent{}, false
}

// ProcessEventNames devolve os nomes do catálogo (para mensagens e help).
func ProcessEventNames() []string {
	names := make([]string, len(processEvents))
	for i, e := range processEvents {
		names[i] = e.Name
	}
	return names
}

// artifactData é o contexto dos templates de artefato de arquivo único e de
// formulário.
type artifactData struct {
	Name  string
	Title string
}

// processScriptData é o contexto do template de script de processo.
type processScriptData struct {
	ProcessID string
	Event     string
	Params    string
	Doc       string
}

// CreateDatasetFile grava o esqueleto de dataset customizado em path.
func CreateDatasetFile(path, name string) error {
	if err := ValidateArtifactName(name); err != nil {
		return err
	}
	return createFileFromTemplate(path, "templates/artifact/dataset.js", artifactData{Name: name})
}

// CreateGlobalEventFile grava o esqueleto de evento global em path — o nome
// vira o nome da função.
func CreateGlobalEventFile(path, name string) error {
	if err := ValidateArtifactName(name); err != nil {
		return err
	}
	return createFileFromTemplate(path, "templates/artifact/event.js", artifactData{Name: name})
}

// CreateMechanismFile grava o esqueleto de mecanismo de atribuição em path.
func CreateMechanismFile(path, name string) error {
	if err := ValidateArtifactName(name); err != nil {
		return err
	}
	return createFileFromTemplate(path, "templates/artifact/mechanism.js", artifactData{Name: name})
}

// CreateProcessScriptFile grava o esqueleto de um evento de processo em path,
// com a assinatura do catálogo. Devolve o evento canônico usado.
func CreateProcessScriptFile(path, processID, eventName string) (ProcessEvent, error) {
	if !processIDRe.MatchString(processID) || len(processID) > 200 {
		return ProcessEvent{}, fmt.Errorf("%w: id de processo %q (use letras, dígitos, _ e ., começando por letra)", ErrInvalidCode, processID)
	}
	ev, ok := FindProcessEvent(eventName)
	if !ok {
		return ProcessEvent{}, fmt.Errorf("%w: %q (disponíveis: %s)", ErrUnknownEvent, eventName, strings.Join(ProcessEventNames(), ", "))
	}
	err := createFileFromTemplate(path, "templates/artifact/workflow_script.js", processScriptData{
		ProcessID: processID,
		Event:     ev.Name,
		Params:    ev.Params,
		Doc:       ev.Doc,
	})
	return ev, err
}

// CreateFormDir materializa a pasta de um formulário novo em dir (que não
// pode existir): HTML principal com <form> + events/ comuns. Devolve os
// caminhos relativos criados.
func CreateFormDir(dir, name, title string) ([]string, error) {
	if err := ValidateArtifactName(name); err != nil {
		return nil, err
	}
	if _, err := os.Stat(dir); err == nil {
		return nil, fmt.Errorf("%w: %s", ErrDirExists, dir)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if title == "" {
		title = name
	}
	data := artifactData{Name: name, Title: title}

	const base = "templates/artifact/form"
	var created []string
	err := fs.WalkDir(templatesFS, base, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel := strings.TrimPrefix(p, base+"/")
		rel = strings.ReplaceAll(rel, "__code__", name)
		raw, err := templatesFS.ReadFile(p)
		if err != nil {
			return err
		}
		content, err := render(p, raw, data)
		if err != nil {
			return err
		}
		dst := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, content, 0o644); err != nil {
			return err
		}
		created = append(created, rel)
		return nil
	})
	if err != nil {
		os.RemoveAll(dir) // não deixa árvore pela metade
		return nil, err
	}
	return created, nil
}

// createFileFromTemplate renderiza um template de arquivo único em path,
// falhando se o destino já existir.
func createFileFromTemplate(path, tplPath string, data any) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%w: %s", ErrFileExists, path)
	} else if !os.IsNotExist(err) {
		return err
	}
	raw, err := templatesFS.ReadFile(tplPath)
	if err != nil {
		return err
	}
	content, err := render(tplPath, raw, data)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0o644)
}
