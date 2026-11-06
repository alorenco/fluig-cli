package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
)

// envServicePassword guarda a senha da autenticação basic de um serviço. A
// senha nunca vem de argumento de linha de comando (regra 5 do projeto).
const envServicePassword = "FLUIGCLI_SERVICE_PASSWORD"

// envServiceCustomHeaders guarda os headers da autenticação custom (uma linha
// "Nome: valor" por header). Alternativa: --custom-headers-stdin.
const envServiceCustomHeaders = "FLUIGCLI_SERVICE_CUSTOM_HEADERS"

// serviceStdin é a origem do --custom-headers-stdin (trocada nos testes).
var serviceStdin io.Reader = os.Stdin

func newServiceCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service",
		Short: "Serviços do Painel de Controle (REST, SOAP, JDBC): lista e cadastra serviços REST",
		Long: "Gerencia o cadastro de serviços do Fluig (Painel de Controle > Serviços).\n\n" +
			"Os datasets chamam um serviço REST pelo código, com\n" +
			"fluigAPI.getAuthorizeClientService().invoke({ serviceCode: ... }).\n" +
			"A CLI nunca mostra senha, token ou outro segredo do serviço.",
	}
	cmd.AddCommand(newServiceListCmd(app))
	cmd.AddCommand(newServiceShowCmd(app))
	cmd.AddCommand(newServiceCreateCmd(app))
	cmd.AddCommand(newServiceUpdateCmd(app))
	cmd.AddCommand(newServiceDeleteCmd(app))
	cmd.AddCommand(newServiceTestCmd(app))
	cmd.AddCommand(newServiceExportCmd(app))
	cmd.AddCommand(newServiceApplyCmd(app))
	return cmd
}

// --- service list ---

func newServiceListCmd(app *App) *cobra.Command {
	var (
		types         []string
		filter        string
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Lista os serviços cadastrados (código, tipo, URL e autenticação)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			for _, t := range types {
				if !slices.Contains([]string{fluig.ServiceTypeREST, fluig.ServiceTypeSOAP, fluig.ServiceTypeJDBC}, t) {
					return output.Usagef("tipo inválido %q; use rest, soap ou jdbc", t)
				}
			}
			ctx := context.Background()
			_, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}
			list, err := client.ListServices(ctx, fluig.ServiceFilter{Pattern: filter, Types: types})
			if err != nil {
				return mapFluigError(err)
			}
			if len(list) == 0 {
				p.Infof("Nenhum serviço encontrado. Cadastre um com: fluigcli service create <código> --url <URL base>")
			} else {
				rows := make([][]string, 0, len(list))
				for _, s := range list {
					auth := s.AuthType
					if len(s.CustomHeaders) > 0 {
						auth += ": " + strings.Join(s.CustomHeaders, ", ")
					}
					rows = append(rows, []string{s.Code, strings.ToUpper(s.Type), s.URL, auth})
				}
				// Padrão de listagem (ver CLAUDE.md).
				p.Table(output.Table{
					Headers: []string{"Código", "Tipo", "URL", "Autenticação"},
					Rows:    rows,
					Style:   output.BoldHeaderStyle(nil),
				})
			}
			if list == nil {
				list = []fluig.Service{}
			}
			p.Done(map[string]any{"services": list})
			return nil
		},
	}
	cmd.Flags().StringSliceVar(&types, "type", nil, "filtra por tipo: rest, soap ou jdbc (repita ou separe por vírgula)")
	cmd.Flags().StringVar(&filter, "filter", "", "filtra pelo trecho do código (busca do servidor)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// --- service show ---

func newServiceShowCmd(app *App) *cobra.Command {
	var passwordStdin bool
	cmd := &cobra.Command{
		Use:   "show <código>",
		Short: "Mostra um serviço sem os segredos",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			ctx := context.Background()
			_, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}
			s, err := client.GetService(ctx, args[0])
			if err != nil {
				return mapFluigError(err)
			}
			p.Successf("%s (%s)", s.Code, strings.ToUpper(s.Type))
			p.Successf("Descrição: %s", s.Description)
			p.Successf("URL: %s", s.URL)
			if s.AuthType != "" {
				p.Successf("Autenticação: %s", s.AuthType)
			}
			if s.Username != "" {
				p.Successf("Usuário: %s", s.Username)
			}
			if len(s.CustomHeaders) > 0 {
				p.Successf("Headers custom: %s (valores ocultos)", strings.Join(s.CustomHeaders, ", "))
			}
			if s.TestPath != "" {
				p.Successf("Caminho de teste: %s", s.TestPath)
			}
			if s.Swagger != "" {
				p.Successf("Swagger: %s", s.Swagger)
			}
			p.Done(map[string]any{"service": s})
			return nil
		},
	}
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// serviceFlags reúne as flags comuns de create/update.
type serviceFlags struct {
	description string
	url         string
	auth        string
	testPath    string
	swagger     string
	username    string
	customStdin bool
}

func (f *serviceFlags) register(cmd *cobra.Command, create bool) {
	urlHelp := "URL base do serviço (ex.: https://api.empresa.com)"
	if create {
		urlHelp += " (obrigatório)"
	}
	authHelp := "autenticação: none, basic, fluigAPI ou custom"
	if create {
		authHelp += " (default: none)"
	}
	cmd.Flags().StringVar(&f.description, "description", "", "descrição do serviço")
	cmd.Flags().StringVar(&f.url, "url", "", urlHelp)
	cmd.Flags().StringVar(&f.auth, "auth", "", authHelp)
	cmd.Flags().StringVar(&f.testPath, "test-path", "", "caminho de teste do serviço (ex.: /health)")
	cmd.Flags().StringVar(&f.swagger, "swagger", "", "URL do swagger do serviço")
	cmd.Flags().StringVar(&f.username, "username", "", "usuário da autenticação basic (a senha vem de "+envServicePassword+" ou do prompt)")
	cmd.Flags().BoolVar(&f.customStdin, "custom-headers-stdin", false, "lê do stdin os headers da autenticação custom, uma linha \"Nome: valor\" por header (alternativa: "+envServiceCustomHeaders+")")
}

// input monta o ServiceInput só com as flags informadas. currentAuth é a
// autenticação atual do serviço ("" no create).
func (f *serviceFlags) input(app *App, cmd *cobra.Command, currentAuth string, passwordStdin bool) (fluig.ServiceInput, error) {
	var in fluig.ServiceInput
	changed := cmd.Flags().Changed
	str := func(name, v string) *string {
		if changed(name) {
			s := v
			return &s
		}
		return nil
	}
	in.Description = str("description", f.description)
	in.URL = str("url", f.url)
	in.AuthType = str("auth", f.auth)
	in.TestPath = str("test-path", f.testPath)
	in.Swagger = str("swagger", f.swagger)
	in.Username = str("username", f.username)

	if in.URL != nil && !strings.HasPrefix(*in.URL, "http://") && !strings.HasPrefix(*in.URL, "https://") {
		return in, output.Usagef("--url precisa começar com http:// ou https:// (veio %q)", *in.URL)
	}
	auth := currentAuth
	if in.AuthType != nil {
		auth = *in.AuthType
		if !slices.Contains(fluig.ServiceAuthWritable, auth) {
			return in, output.Usagef("--auth %q não é suportado pela CLI; use none, basic, fluigAPI ou custom. Os tipos oauth1 e oauth2 são configurados pelo Painel de Controle", auth)
		}
	}
	if in.Username != nil && auth != fluig.ServiceAuthBasic {
		return in, output.Usagef("--username só vale com --auth basic")
	}
	// Senha basic: obrigatória ao passar para basic (create ou troca de tipo).
	// Num serviço que já é basic, só é relida quando o usuário muda ou quando
	// a env var está definida; fora disso, o update preserva a senha guardada.
	if auth == fluig.ServiceAuthBasic {
		switching := currentAuth != fluig.ServiceAuthBasic
		if switching && in.Username == nil {
			return in, output.Usagef("--auth basic exige --username")
		}
		if switching || in.Username != nil || os.Getenv(envServicePassword) != "" {
			pw, err := app.resolveServicePassword()
			if err != nil {
				return in, err
			}
			in.Password = &pw
		}
	}
	// Headers custom: obrigatórios ao passar para custom. Num serviço que já é
	// custom, só são relidos com --custom-headers-stdin ou com a env var;
	// fora disso, o update preserva o valor guardado.
	envCustom := os.Getenv(envServiceCustomHeaders)
	if auth != fluig.ServiceAuthCustom && f.customStdin {
		return in, output.Usagef("--custom-headers-stdin só vale com --auth custom")
	}
	if auth == fluig.ServiceAuthCustom {
		if f.customStdin && passwordStdin {
			return in, output.Usagef("--custom-headers-stdin e --password-stdin leem o mesmo stdin; use %s para os headers", envServiceCustomHeaders)
		}
		switching := currentAuth != fluig.ServiceAuthCustom
		if switching || f.customStdin || envCustom != "" {
			var raw string
			switch {
			case f.customStdin:
				b, err := io.ReadAll(io.LimitReader(serviceStdin, 64<<10))
				if err != nil {
					return in, output.Usagef("falha ao ler os headers do stdin: %v", err)
				}
				raw = string(b)
			case envCustom != "":
				raw = envCustom
			default:
				return in, output.Usagef("--auth custom exige os headers: use --custom-headers-stdin ou a variável %s (nunca em argumento de linha de comando)", envServiceCustomHeaders)
			}
			text, _, err := fluig.ParseCustomHeaders(raw)
			if err != nil {
				return in, output.Usagef("headers custom inválidos: %v", err)
			}
			in.Custom = &text
		}
	}
	return in, nil
}

// resolveServicePassword lê a senha basic do serviço da env var ou, em modo
// interativo, de um prompt oculto.
func (a *App) resolveServicePassword() (string, error) {
	if pw := os.Getenv(envServicePassword); pw != "" {
		return pw, nil
	}
	if !a.Interactive() {
		return "", output.Usagef("defina a senha do serviço na variável %s (nunca em argumento de linha de comando)", envServicePassword)
	}
	pw, err := promptPassword("Senha do serviço")
	if err != nil {
		return "", err
	}
	if pw == "" {
		return "", output.Usagef("a senha do serviço não pode ficar vazia")
	}
	return pw, nil
}

// --- service create ---

func newServiceCreateCmd(app *App) *cobra.Command {
	var (
		f             serviceFlags
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "create <código>",
		Short: "Cadastra um serviço REST",
		Long: "Cadastra um serviço REST. O código é o serviceCode que os datasets usam.\n\n" +
			"A CLI grava as autenticações none, basic, fluigAPI e custom. Na basic, a\n" +
			"senha vem da variável " + envServicePassword + " ou de um prompt oculto.\n" +
			"Na custom, os headers (uma linha \"Nome: valor\" por header) vêm do\n" +
			"--custom-headers-stdin ou da variável " + envServiceCustomHeaders + ".\n" +
			"Configure oauth1 e oauth2 pelo Painel de Controle.\n\n" +
			"Atenção: o Fluig guarda a senha basic e os headers custom em texto aberto.\n" +
			"Quem acessa o Painel de Controle lê esses valores. A CLI só não os mostra.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			code := args[0]
			if !cmd.Flags().Changed("url") {
				return output.Usagef("informe a URL base com --url")
			}
			if !cmd.Flags().Changed("auth") {
				_ = cmd.Flags().Set("auth", fluig.ServiceAuthNone)
			}
			in, err := f.input(app, cmd, "", passwordStdin)
			if err != nil {
				return err
			}
			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, "cadastrar serviço")
			if err != nil {
				return err
			}
			if err := client.CreateService(ctx, code, in); err != nil {
				if errors.Is(err, fluig.ErrServiceExists) {
					return output.ServerErrorf("o serviço %q já existe no servidor; altere com: fluigcli service update %s", code, code).WithCause(err)
				}
				return mapFluigError(err)
			}
			s, err := client.GetService(ctx, code)
			if err != nil {
				return mapFluigError(err)
			}
			p.Successf("serviço %q cadastrado (%s, autenticação %s)", s.Code, s.URL, s.AuthType)
			p.Done(map[string]any{"service": s})
			return nil
		},
	}
	f.register(cmd, true)
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha de AUTENTICAÇÃO do stdin")
	return cmd
}

// --- service update ---

func newServiceUpdateCmd(app *App) *cobra.Command {
	var (
		f             serviceFlags
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "update <código>",
		Short: "Altera um serviço REST (só os campos informados)",
		Long: "Altera um serviço REST. A CLI troca só os campos das flags informadas e\n" +
			"mantém o resto, inclusive os segredos guardados no servidor.\n\n" +
			"Para trocar a senha basic, defina " + envServicePassword + ". Para trocar\n" +
			"os headers custom, use --custom-headers-stdin ou " + envServiceCustomHeaders + ".",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			code := args[0]
			changedAny := false
			for _, n := range []string{"description", "url", "auth", "test-path", "swagger", "username", "custom-headers-stdin"} {
				changedAny = changedAny || cmd.Flags().Changed(n)
			}
			if !changedAny && os.Getenv(envServicePassword) == "" && os.Getenv(envServiceCustomHeaders) == "" {
				return output.Usagef("informe ao menos uma alteração (--url, --description, --auth, --test-path, --swagger ou --username)")
			}
			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, "alterar serviço")
			if err != nil {
				return err
			}
			current, err := client.GetService(ctx, code)
			if err != nil {
				return mapFluigError(err)
			}
			if current.Type != fluig.ServiceTypeREST {
				return output.Usagef("o serviço %q é %s; a CLI só altera serviços REST", code, strings.ToUpper(current.Type))
			}
			in, err := f.input(app, cmd, current.AuthType, passwordStdin)
			if err != nil {
				return err
			}
			if err := client.UpdateService(ctx, current, in); err != nil {
				return mapFluigError(err)
			}
			s, err := client.GetService(ctx, code)
			if err != nil {
				return mapFluigError(err)
			}
			p.Successf("serviço %q alterado (%s, autenticação %s)", s.Code, s.URL, s.AuthType)
			p.Done(map[string]any{"service": s})
			return nil
		},
	}
	f.register(cmd, false)
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha de AUTENTICAÇÃO do stdin")
	return cmd
}

// --- service delete ---

func newServiceDeleteCmd(app *App) *cobra.Command {
	var passwordStdin bool
	cmd := &cobra.Command{
		Use:   "delete <código>...",
		Short: "Exclui serviços (datasets que usam o serviço deixam de funcionar)",
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			if len(args) == 0 {
				return output.Usagef("informe um ou mais códigos de serviço a excluir")
			}
			if err := app.confirm(fmt.Sprintf("Excluir %d serviço(s) no servidor? Datasets e widgets que usam o serviço deixam de funcionar", len(args))); err != nil {
				return err
			}
			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, "excluir serviços")
			if err != nil {
				return err
			}
			var results []itemResult
			var lastErr error
			failures := 0
			for _, code := range args {
				if derr := client.DeleteService(ctx, code); derr != nil {
					failures++
					lastErr = mapFluigError(derr)
					results = append(results, itemResult{ID: code, Action: "failed", Success: false, Error: output.AsError(lastErr).Message})
					p.Warnf("serviço %q: %s", code, output.AsError(lastErr).Message)
					continue
				}
				results = append(results, itemResult{ID: code, Action: "deleted", Success: true})
				p.Successf("serviço %q excluído", code)
			}
			return finishBatch(p, lastErr, map[string]any{"results": results}, failures, len(args))
		},
	}
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha de AUTENTICAÇÃO do stdin")
	return cmd
}

// --- service test ---

func newServiceTestCmd(app *App) *cobra.Command {
	var (
		full          bool
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "test <código>",
		Short: "Chama o caminho de teste do serviço pelo próprio Fluig e mostra a resposta",
		Long: "Chama o caminho de teste cadastrado no serviço (--test-path), pelo próprio\n" +
			"servidor Fluig, com a autenticação do cadastro. É o botão \"Testar\" do\n" +
			"Painel de Controle. Por isso, o teste confere a URL, a rede do servidor e a\n" +
			"autenticação de uma vez.\n\n" +
			"Sem caminho de teste, a chamada vai para a raiz da URL base. O Fluig não\n" +
			"aceita outro caminho na hora do teste: troque com service update\n" +
			"--test-path.\n\n" +
			"Exit 0 quando o destino responde 2xx. Exit 5 nos outros casos. O corpo da\n" +
			"resposta sai truncado em 2000 caracteres; use --full para o corpo inteiro.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			code := args[0]
			ctx := context.Background()
			_, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}
			s, err := client.GetService(ctx, code)
			if err != nil {
				return mapFluigError(err)
			}
			if s.Type != fluig.ServiceTypeREST {
				return output.Usagef("o serviço %q é %s; o teste só existe para serviços REST", code, strings.ToUpper(s.Type))
			}
			if s.TestPath == "" {
				p.Warnf("o serviço %q não tem caminho de teste; a chamada vai para a raiz de %s (cadastre com: fluigcli service update %s --test-path /health)", code, s.URL, code)
			}
			r, err := client.TestService(ctx, code)
			if err != nil {
				return mapFluigError(err)
			}
			body := r.Body
			truncated := false
			if !full && len(body) > 2000 {
				body, truncated = body[:2000], true
			}
			target := strings.TrimRight(s.URL, "/") + s.TestPath
			data := map[string]any{"code": code, "url": target, "httpStatus": r.HTTPStatus, "body": body, "truncated": truncated}
			if r.HTTPStatus == 0 {
				data["error"] = r.Error
				p.FailData(data, output.CodeServerError, "a chamada não saiu do Fluig: "+r.Error)
				return output.ServerErrorf("a chamada do serviço %q não saiu do Fluig: %s", code, r.Error)
			}
			p.Successf("GET %s → HTTP %d", target, r.HTTPStatus)
			if body != "" {
				p.Successf("%s", body)
			}
			if truncated {
				p.Infof("(corpo truncado em 2000 caracteres; use --full)")
			}
			if r.HTTPStatus < 200 || r.HTTPStatus >= 300 {
				p.FailData(data, output.CodeServerError, fmt.Sprintf("o serviço respondeu HTTP %d", r.HTTPStatus))
				return output.ServerErrorf("o serviço %q respondeu HTTP %d em %s", code, r.HTTPStatus, target)
			}
			p.Done(data)
			return nil
		},
	}
	cmd.Flags().BoolVar(&full, "full", false, "mostra o corpo inteiro da resposta")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}
