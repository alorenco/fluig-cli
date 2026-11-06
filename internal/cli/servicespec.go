package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
	"github.com/alorenco/fluig-cli/internal/project"
)

// serviceSpec é o arquivo versionável de um serviço REST
// (services/<código>.json). O arquivo NUNCA guarda segredo: SecretEnv é o
// NOME da variável de ambiente que traz a senha basic ou os headers custom
// na hora do apply.
type serviceSpec struct {
	Code        string `json:"code"`
	Description string `json:"description,omitempty"`
	URL         string `json:"url"`
	Auth        string `json:"auth"`
	Username    string `json:"username,omitempty"`
	TestPath    string `json:"testPath,omitempty"`
	Swagger     string `json:"swagger,omitempty"`
	SecretEnv   string `json:"secretEnv,omitempty"`
}

var envNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// secretEnvName sugere o nome da variável do segredo para o código.
func secretEnvName(code string) string {
	up := strings.ToUpper(regexp.MustCompile(`[^A-Za-z0-9]+`).ReplaceAllString(code, "_"))
	return "FLUIGCLI_SECRET_" + strings.Trim(up, "_")
}

func readServiceSpec(path string) (*serviceSpec, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, output.NotFoundf("arquivo %q não encontrado", path)
		}
		return nil, output.LocalIOf("falha ao ler %s: %v", path, err)
	}
	var spec serviceSpec
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&spec); err != nil {
		return nil, output.Usagef("%s não é uma especificação de serviço válida: %v", path, err)
	}
	if err := spec.validate(); err != nil {
		return nil, output.Usagef("%s: %s", path, err.Error())
	}
	return &spec, nil
}

func (s *serviceSpec) validate() error {
	switch {
	case s.Code == "":
		return errors.New("falta \"code\"")
	case !strings.HasPrefix(s.URL, "http://") && !strings.HasPrefix(s.URL, "https://"):
		return fmt.Errorf("\"url\" precisa começar com http:// ou https:// (veio %q)", s.URL)
	case !slices.Contains(fluig.ServiceAuthWritable, s.Auth):
		return fmt.Errorf("\"auth\" aceita none, basic, fluigAPI, custom, oauth1 ou oauth2 (veio %q)", s.Auth)
	case s.Auth == fluig.ServiceAuthBasic && s.Username == "":
		return errors.New("\"auth\": \"basic\" exige \"username\"")
	case s.Auth != fluig.ServiceAuthBasic && s.Username != "":
		return errors.New("\"username\" só vale com \"auth\": \"basic\"")
	case s.SecretEnv != "" && !s.hasSecret():
		return errors.New("\"secretEnv\" só vale com \"auth\": basic, custom, oauth1 ou oauth2")
	case s.SecretEnv != "" && !envNameRe.MatchString(s.SecretEnv):
		return fmt.Errorf("\"secretEnv\" precisa ser um nome de variável de ambiente (veio %q)", s.SecretEnv)
	case s.hasSecret() && s.SecretEnv == "":
		return fmt.Errorf("\"auth\": %q exige \"secretEnv\" com o nome da variável que traz o segredo", s.Auth)
	}
	return nil
}

// hasSecret diz se o tipo de autenticação guarda segredo.
func (s *serviceSpec) hasSecret() bool {
	switch s.Auth {
	case fluig.ServiceAuthBasic, fluig.ServiceAuthCustom, fluig.ServiceAuthOAuth1, fluig.ServiceAuthOAuth2:
		return true
	}
	return false
}

// secret lê o segredo da variável. ok=false quando a variável está vazia.
func (s *serviceSpec) secret() (string, bool, error) {
	if s.SecretEnv == "" {
		return "", false, nil
	}
	v := os.Getenv(s.SecretEnv)
	if v == "" {
		return "", false, nil
	}
	switch s.Auth {
	case fluig.ServiceAuthCustom:
		text, _, err := fluig.ParseCustomHeaders(v)
		if err != nil {
			return "", false, output.Usagef("os headers da variável %s são inválidos: %v", s.SecretEnv, err)
		}
		return text, true, nil
	case fluig.ServiceAuthOAuth1, fluig.ServiceAuthOAuth2:
		if _, err := fluig.ParseOAuthSecret(s.Auth, v); err != nil {
			return "", false, output.Usagef("os campos OAuth da variável %s são inválidos: %v", s.SecretEnv, err)
		}
	}
	return v, true, nil
}

// applyServiceSpec cria ou alinha o serviço com a especificação. Devolve as
// ações (para o relatório). Com dryRun, só descreve.
func applyServiceSpec(ctx context.Context, client *fluig.Client, spec *serviceSpec, dryRun bool) ([]string, error) {
	secret, hasSecret, err := spec.secret()
	if err != nil {
		return nil, err
	}
	cur, err := client.GetService(ctx, spec.Code)
	if err != nil && !errors.Is(err, fluig.ErrNotFound) {
		return nil, mapFluigError(err)
	}
	str := func(v string) *string { return &v }

	if cur == nil {
		needsSecret := spec.hasSecret()
		if needsSecret && !hasSecret {
			return nil, output.Usagef("o serviço %q não existe e a autenticação %s exige o segredo: defina a variável %s", spec.Code, spec.Auth, spec.SecretEnv)
		}
		actions := []string{fmt.Sprintf("criar o serviço %s (%s, autenticação %s)", spec.Code, spec.URL, spec.Auth)}
		if dryRun {
			return actions, nil
		}
		in := fluig.ServiceInput{
			Description: str(spec.Description), URL: str(spec.URL), AuthType: str(spec.Auth),
			TestPath: str(spec.TestPath), Swagger: str(spec.Swagger),
		}
		if spec.Description == "" {
			in.Description = nil
		}
		switch spec.Auth {
		case fluig.ServiceAuthBasic:
			in.Username, in.Password = str(spec.Username), str(secret)
		case fluig.ServiceAuthCustom:
			in.Custom = str(secret)
		case fluig.ServiceAuthOAuth1, fluig.ServiceAuthOAuth2:
			in.OAuth, _ = fluig.ParseOAuthSecret(spec.Auth, secret)
		}
		if err := client.CreateService(ctx, spec.Code, in); err != nil {
			return nil, mapFluigError(err)
		}
		return actions, nil
	}

	if cur.Type != fluig.ServiceTypeREST {
		return nil, output.Usagef("o serviço %q no servidor é %s; o apply só trata serviços REST", spec.Code, strings.ToUpper(cur.Type))
	}
	var in fluig.ServiceInput
	var actions []string
	diff := func(label, from, to string, set **string) {
		if from != to {
			*set = str(to)
			actions = append(actions, fmt.Sprintf("%s: %q → %q", label, from, to))
		}
	}
	if spec.Description != "" {
		diff("descrição", cur.Description, spec.Description, &in.Description)
	}
	diff("url", cur.URL, spec.URL, &in.URL)
	diff("caminho de teste", cur.TestPath, spec.TestPath, &in.TestPath)
	diff("swagger", cur.Swagger, spec.Swagger, &in.Swagger)
	authChanged := cur.AuthType != spec.Auth
	if authChanged {
		in.AuthType = str(spec.Auth)
		actions = append(actions, fmt.Sprintf("autenticação: %s → %s", cur.AuthType, spec.Auth))
	}
	if spec.Auth == fluig.ServiceAuthBasic && (authChanged || cur.Username != spec.Username) {
		in.Username = str(spec.Username)
		if !authChanged {
			actions = append(actions, fmt.Sprintf("usuário: %q → %q", cur.Username, spec.Username))
		}
	}
	needsSecret := spec.hasSecret()
	switch {
	case needsSecret && authChanged && !hasSecret:
		return nil, output.Usagef("a troca para a autenticação %s exige o segredo: defina a variável %s", spec.Auth, spec.SecretEnv)
	case needsSecret && hasSecret && (authChanged || !cur.SecretEquals(secret)):
		switch spec.Auth {
		case fluig.ServiceAuthBasic:
			in.Password = str(secret)
		case fluig.ServiceAuthCustom:
			in.Custom = str(secret)
		default:
			in.OAuth, _ = fluig.ParseOAuthSecret(spec.Auth, secret)
		}
		actions = append(actions, "segredo atualizado (valor da variável "+spec.SecretEnv+")")
	}
	if len(actions) == 0 {
		return []string{"serviço " + spec.Code + " já está igual à especificação"}, nil
	}
	if dryRun {
		return actions, nil
	}
	if err := client.UpdateService(ctx, cur, in); err != nil {
		return nil, mapFluigError(err)
	}
	return actions, nil
}

// --- service export ---

func newServiceExportCmd(app *App) *cobra.Command {
	var (
		outPath       string
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "export <código>",
		Short: "Grava a especificação do serviço num arquivo JSON versionável (sem segredo)",
		Long: "Grava a especificação de um serviço REST em services/<código>.json (ou no\n" +
			"--output). O arquivo vai para o Git e é aplicado em outro servidor com\n" +
			"service apply ou com o passo \"service\" do deploy.\n\n" +
			"O arquivo NUNCA guarda segredo. Na autenticação basic ou custom, o campo\n" +
			"secretEnv traz o NOME da variável de ambiente que o apply lê. Ajuste o nome\n" +
			"se quiser.",
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
				return output.Usagef("o serviço %q é %s; o export só trata serviços REST", code, strings.ToUpper(s.Type))
			}
			if !slices.Contains(fluig.ServiceAuthWritable, s.AuthType) {
				return output.Usagef("o serviço %q usa a autenticação %s, que a CLI não conhece", code, s.AuthType)
			}
			spec := serviceSpec{
				Code: s.Code, Description: s.Description, URL: s.URL, Auth: s.AuthType,
				Username: s.Username, TestPath: s.TestPath, Swagger: s.Swagger,
			}
			if spec.hasSecret() {
				spec.SecretEnv = secretEnvName(code)
			}
			path := outPath
			if path == "" {
				root, err := app.projectRootForFiles()
				if err != nil {
					return err
				}
				path = filepath.Join(root, project.ServicesDirName, code+".json")
			}
			if err := writeJSONFile(path, spec); err != nil {
				return err
			}
			p.Successf("serviço %q exportado para %s", code, path)
			if spec.SecretEnv != "" {
				p.Infof("O segredo não foi gravado. No apply, defina a variável %s.", spec.SecretEnv)
			}
			p.Done(map[string]any{"path": path, "spec": spec})
			return nil
		},
	}
	cmd.Flags().StringVarP(&outPath, "output", "o", "", "arquivo de saída (default: services/<código>.json no projeto)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// --- service apply ---

func newServiceApplyCmd(app *App) *cobra.Command {
	var (
		dryRun        bool
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "apply <arquivo.json>",
		Short: "Cria ou alinha o serviço com a especificação do arquivo (idempotente)",
		Long: "Cria o serviço quando ele não existe, ou altera só os campos que diferem\n" +
			"da especificação. Rodar de novo não muda nada.\n\n" +
			"Na autenticação basic ou custom, o segredo vem da variável nomeada em\n" +
			"secretEnv. Na criação, a variável é obrigatória. Na alteração, sem a\n" +
			"variável, o segredo guardado no servidor fica igual. Com a variável, a\n" +
			"CLI compara o valor sem mostrá-lo e só grava quando ele mudou.\n\n" +
			"Use --dry-run para ver as mudanças sem gravar.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			spec, err := readServiceSpec(args[0])
			if err != nil {
				return err
			}
			ctx := context.Background()
			var client *fluig.Client
			if dryRun {
				_, client, err = app.connect(ctx, passwordStdin)
			} else {
				_, client, err = app.connectWrite(ctx, passwordStdin, "aplicar serviço")
			}
			if err != nil {
				return err
			}
			actions, err := applyServiceSpec(ctx, client, spec, dryRun)
			if err != nil {
				return err
			}
			verb := "aplicado"
			if dryRun {
				verb = "plano (nada gravado)"
			}
			p.Successf("serviço %q: %s", spec.Code, verb)
			for _, a := range actions {
				p.Successf("  - %s", a)
			}
			p.Done(map[string]any{"code": spec.Code, "dryRun": dryRun, "actions": actions})
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "mostra as mudanças sem gravar")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha de AUTENTICAÇÃO do stdin")
	return cmd
}

// writeJSONFile grava o valor como JSON indentado, criando a pasta.
func writeJSONFile(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return output.LocalIOf("falha ao criar a pasta de %s: %v", path, err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return output.LocalIOf("falha ao gravar %s: %v", path, err)
	}
	return nil
}
