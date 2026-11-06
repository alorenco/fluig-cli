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

// serviceSpec é o arquivo versionável de um serviço (services/<código>.json).
// O arquivo NUNCA guarda segredo: SecretEnv é o NOME da variável de ambiente
// que traz a senha, os headers custom ou os campos OAuth na hora do apply.
type serviceSpec struct {
	Code string `json:"code"`
	// Type é rest (default), soap ou jdbc.
	Type        string `json:"type,omitempty"`
	Driver      string `json:"driver,omitempty"`      // jdbc
	SOAPPackage string `json:"soapPackage,omitempty"` // soap
	Description string `json:"description,omitempty"`
	URL         string `json:"url"`
	Auth        string `json:"auth,omitempty"`
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
	s.Type = strings.ToLower(s.Type)
	switch s.Type {
	case "", fluig.ServiceTypeREST:
		s.Type = fluig.ServiceTypeREST
		if s.Driver != "" || s.SOAPPackage != "" {
			return errors.New("\"driver\" e \"soapPackage\" não valem para serviço REST")
		}
	case fluig.ServiceTypeSOAP, fluig.ServiceTypeJDBC:
		return s.validateSOAPJDBC()
	default:
		return fmt.Errorf("\"type\" aceita rest, soap ou jdbc (veio %q)", s.Type)
	}
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

func (s *serviceSpec) validateSOAPJDBC() error {
	switch {
	case s.Code == "":
		return errors.New("falta \"code\"")
	case s.TestPath != "" || s.Swagger != "":
		return fmt.Errorf("\"testPath\" e \"swagger\" não valem para serviço %s", strings.ToUpper(s.Type))
	case s.SecretEnv != "" && !envNameRe.MatchString(s.SecretEnv):
		return fmt.Errorf("\"secretEnv\" precisa ser um nome de variável de ambiente (veio %q)", s.SecretEnv)
	}
	if s.Type == fluig.ServiceTypeSOAP {
		if s.Auth == "" {
			s.Auth = fluig.ServiceAuthNone
		}
		switch {
		case s.Driver != "":
			return errors.New("\"driver\" não vale para serviço SOAP")
		case !strings.HasPrefix(s.URL, "http://") && !strings.HasPrefix(s.URL, "https://"):
			return fmt.Errorf("\"url\" do SOAP é a URL do WSDL, com http:// ou https:// (veio %q)", s.URL)
		case s.Auth != fluig.ServiceAuthNone && s.Auth != fluig.ServiceAuthBasic:
			return fmt.Errorf("o SOAP aceita \"auth\" none ou basic (veio %q)", s.Auth)
		case s.Auth == fluig.ServiceAuthBasic && (s.Username == "" || s.SecretEnv == ""):
			return errors.New("o SOAP basic exige \"username\" e \"secretEnv\"")
		case s.Auth == fluig.ServiceAuthNone && (s.Username != "" || s.SecretEnv != ""):
			return errors.New("\"username\" e \"secretEnv\" só valem com \"auth\": \"basic\"")
		}
		return nil
	}
	switch {
	case s.Auth != "" || s.SOAPPackage != "":
		return errors.New("\"auth\" e \"soapPackage\" não valem para serviço JDBC")
	case !strings.HasPrefix(s.URL, "jdbc:"):
		return fmt.Errorf("\"url\" do JDBC começa com jdbc: (veio %q)", s.URL)
	case s.Driver == "" || s.Username == "" || s.SecretEnv == "":
		return errors.New("o JDBC exige \"driver\", \"username\" e \"secretEnv\"")
	}
	return nil
}

// hasSecret diz se o tipo de autenticação guarda segredo.
func (s *serviceSpec) hasSecret() bool {
	if s.Type == fluig.ServiceTypeJDBC {
		return true
	}
	if s.Type == fluig.ServiceTypeSOAP {
		return s.Auth == fluig.ServiceAuthBasic
	}
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
	if s.Type != fluig.ServiceTypeREST {
		return v, true, nil
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
	if spec.Type != fluig.ServiceTypeREST {
		return applySOAPJDBCSpec(ctx, client, spec, cur, secret, hasSecret, dryRun)
	}

	if cur == nil {
		needsSecret := spec.hasSecret()
		if needsSecret && !hasSecret {
			return nil, output.Usagef("o serviço %q não existe e a autenticação %s exige o segredo: defina a variável %s", spec.Code, spec.Auth, spec.SecretEnv)
		}
		actions := []string{fmt.Sprintf("criação do serviço %s (%s, autenticação %s)", spec.Code, spec.URL, spec.Auth)}
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
		return nil, output.Usagef("o serviço %q no servidor é %s, e o arquivo diz rest; o apply não troca o tipo", spec.Code, strings.ToUpper(cur.Type))
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
		actions = append(actions, "segredo: valor novo da variável "+spec.SecretEnv)
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

// applySOAPJDBCSpec cria ou alinha um serviço SOAP/JDBC. A senha do JDBC
// não é conferida (o Fluig não a devolve): ela só é enviada na criação e
// quando outro campo muda (aí é obrigatória).
func applySOAPJDBCSpec(ctx context.Context, client *fluig.Client, spec *serviceSpec, cur *fluig.Service, secret string, hasSecret, dryRun bool) ([]string, error) {
	kind := strings.ToUpper(spec.Type)
	driverType := ""
	if spec.Type == fluig.ServiceTypeJDBC {
		drivers, err := client.ListJDBCDrivers(ctx)
		if err != nil {
			return nil, mapFluigError(err)
		}
		for _, d := range drivers {
			if d.Available && (strings.EqualFold(d.Type, spec.Driver) || strings.EqualFold(d.Name, spec.Driver)) {
				driverType = d.Type
			}
		}
		if driverType == "" {
			return nil, output.Usagef("o driver %q não está disponível no servidor (veja: fluigcli service drivers)", spec.Driver)
		}
	}
	desc := orDefault(spec.Description, spec.Code)
	save := func(update bool) error {
		if spec.Type == fluig.ServiceTypeSOAP {
			in := fluig.SOAPInput{Description: desc, WSDL: spec.URL, Package: spec.SOAPPackage, Auth: spec.Auth, Username: spec.Username}
			if hasSecret {
				in.Password = secret
			}
			var c *fluig.Service
			if update {
				c = cur
			}
			return client.SaveSOAPService(ctx, spec.Code, in, c)
		}
		return client.SaveJDBCService(ctx, spec.Code, fluig.JDBCInput{Description: desc, URL: spec.URL, DriverType: driverType, Username: spec.Username, Password: secret}, update)
	}
	if cur == nil {
		if spec.hasSecret() && !hasSecret {
			return nil, output.Usagef("o serviço %q não existe e o %s exige a senha: defina a variável %s", spec.Code, kind, spec.SecretEnv)
		}
		actions := []string{fmt.Sprintf("criação do serviço %s (%s, %s)", spec.Code, kind, spec.URL)}
		if dryRun {
			return actions, nil
		}
		if err := save(false); err != nil {
			return nil, mapFluigError(err)
		}
		return actions, nil
	}
	if cur.Type != spec.Type {
		return nil, output.Usagef("o serviço %q no servidor é %s, e o arquivo diz %s; o apply não troca o tipo", spec.Code, strings.ToUpper(cur.Type), kind)
	}
	var actions []string
	diff := func(label, from, to string) {
		if from != to {
			actions = append(actions, fmt.Sprintf("%s: %q → %q", label, from, to))
		}
	}
	if spec.Description != "" {
		diff("descrição", cur.Description, spec.Description)
	}
	diff("url", cur.URL, spec.URL)
	diff("usuário", cur.Username, spec.Username)
	if spec.Type == fluig.ServiceTypeSOAP {
		diff("pacote SOAP", cur.SOAPPackage, spec.SOAPPackage)
		diff("autenticação", orDefault(cur.AuthType, fluig.ServiceAuthNone), spec.Auth)
		if spec.Auth == fluig.ServiceAuthBasic && hasSecret && !cur.SecretEquals(secret) {
			actions = append(actions, "senha: valor novo da variável "+spec.SecretEnv)
		}
		if spec.Auth == fluig.ServiceAuthBasic && cur.AuthType != fluig.ServiceAuthBasic && !hasSecret {
			return nil, output.Usagef("a troca para basic exige a senha: defina a variável %s", spec.SecretEnv)
		}
	} else {
		diff("driver", strings.ToLower(cur.Driver), strings.ToLower(driverTypeName(spec.Driver, cur.Driver)))
		if len(actions) > 0 && !hasSecret {
			return nil, output.Usagef("a alteração do JDBC %q exige a senha (o Fluig não devolve a guardada): defina a variável %s", spec.Code, spec.SecretEnv)
		}
	}
	if len(actions) == 0 {
		msg := "serviço " + spec.Code + " já está igual à especificação"
		if spec.Type == fluig.ServiceTypeJDBC {
			msg += " (a senha do JDBC não é conferida: o Fluig não a devolve)"
		}
		return []string{msg}, nil
	}
	if dryRun {
		return actions, nil
	}
	if err := save(true); err != nil {
		return nil, mapFluigError(err)
	}
	return actions, nil
}

// driverTypeName normaliza o driver do arquivo para comparar com o nome
// guardado (o servidor guarda "sqlserver"; o arquivo pode dizer "SQLSERVER").
func driverTypeName(spec, cur string) string {
	if strings.EqualFold(spec, cur) {
		return cur
	}
	return spec
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
		Long: "Grava a especificação de um serviço (REST, SOAP ou JDBC) em\n" +
			"services/<código>.json (ou no\n" +
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
			if s.Type == fluig.ServiceTypeSOAP || s.Type == fluig.ServiceTypeJDBC {
				spec := serviceSpec{Code: s.Code, Type: s.Type, Description: s.Description, URL: s.URL, Username: s.Username, Driver: s.Driver, SOAPPackage: s.SOAPPackage}
				if s.Type == fluig.ServiceTypeSOAP {
					spec.Auth = orDefault(s.AuthType, fluig.ServiceAuthNone)
				}
				if spec.hasSecret() {
					spec.SecretEnv = secretEnvName(code)
				}
				return app.writeServiceSpec(p, code, outPath, spec)
			}
			if s.Type != fluig.ServiceTypeREST {
				return output.Usagef("o serviço %q é do tipo %s, que o export não trata", code, s.Type)
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
			return app.writeServiceSpec(p, code, outPath, spec)
		},
	}
	cmd.Flags().StringVarP(&outPath, "output", "o", "", "arquivo de saída (default: services/<código>.json no projeto)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// resolveSpecPath devolve o arquivo da especificação. O argumento pode ser
// um caminho ou só o código. Um código (sem ".json" e sem separador de
// pasta) vira <projeto>/<dir>/<código>.json, o mesmo arquivo do export.
// Um arquivo que existe com o nome exato do argumento tem prioridade.
func (a *App) resolveSpecPath(arg, dir string) (string, error) {
	if strings.HasSuffix(strings.ToLower(arg), ".json") || strings.ContainsAny(arg, `/\`) {
		return arg, nil
	}
	if st, err := os.Stat(arg); err == nil && !st.IsDir() {
		return arg, nil
	}
	root, err := a.projectRootForFiles()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, dir, arg+".json"), nil
}

// writeServiceSpec grava a especificação no --output ou em
// services/<código>.json do projeto.
func (a *App) writeServiceSpec(p *output.Printer, code, outPath string, spec serviceSpec) error {
	path := outPath
	if path == "" {
		root, err := a.projectRootForFiles()
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
}

// --- service apply ---

func newServiceApplyCmd(app *App) *cobra.Command {
	var (
		dryRun        bool
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "apply <código | arquivo.json>",
		Short: "Cria ou alinha o serviço com a especificação do arquivo (idempotente)",
		Long: "Cria o serviço quando ele não existe, ou altera só os campos que diferem\n" +
			"da especificação. Rodar de novo não muda nada.\n\n" +
			"Na autenticação basic ou custom, o segredo vem da variável nomeada em\n" +
			"secretEnv. Na criação, a variável é obrigatória. Na alteração, sem a\n" +
			"variável, o segredo guardado no servidor fica igual. Com a variável, a\n" +
			"CLI compara o valor sem mostrá-lo e só grava quando ele mudou.\n\n" +
			"Informe o caminho do arquivo ou só o código. O código lê o arquivo\n" +
			"services/<código>.json do projeto, o mesmo que o export grava.\n\n" +
			"Use --dry-run para ver as mudanças sem gravar.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			path, err := app.resolveSpecPath(args[0], project.ServicesDirName)
			if err != nil {
				return err
			}
			spec, err := readServiceSpec(path)
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
