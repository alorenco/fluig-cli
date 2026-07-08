package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/alorenco/fluig-cli/internal/config"
	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
)

// authenticate resolve a senha do servidor, abre o cliente e garante uma sessão
// válida. Se a senha veio do keyring e a autenticação falha, remove-a (para não
// deixar o usuário preso com uma senha inválida em cache).
func (a *App) authenticate(ctx context.Context, server *config.Server, passwordStdin bool) (*fluig.Client, error) {
	pw, err := a.passwordSource(passwordStdin).Resolve(server)
	if err != nil {
		return nil, err
	}
	client, err := a.clientFor(server, pw.Password)
	if err != nil {
		return nil, output.Usagef("%s", err.Error())
	}
	if err := client.EnsureSession(ctx); err != nil {
		if errors.Is(err, fluig.ErrAuthFailed) && pw.Source == config.SourceKeyring {
			if delErr := a.Keyring.Delete(server.ID); delErr == nil {
				return nil, output.AuthFailedf(
					"a senha salva no keyring para %q estava incorreta e foi removida; "+
						"rode o comando de novo para informar a senha correta", server.Name)
			}
		}
		return nil, mapFluigError(err)
	}
	if err := pw.SaveIfRequested(); err != nil {
		a.printer.Warnf("não foi possível salvar a senha no keyring: %v", err)
	}
	return client, nil
}

// connect resolve o servidor alvo (--server/env/padrão/seleção) e autentica.
func (a *App) connect(ctx context.Context, passwordStdin bool) (*config.Server, *fluig.Client, error) {
	server, err := a.resolveServer("")
	if err != nil {
		return nil, nil, err
	}
	a.printer.Server = server.Name
	client, err := a.authenticate(ctx, server, passwordStdin)
	if err != nil {
		return nil, nil, err
	}
	return server, client, nil
}

// connectWrite é o connect das operações que alteram o servidor: em servidor
// marcado como produção, exige confirmação (ou --yes) antes de autenticar.
func (a *App) connectWrite(ctx context.Context, passwordStdin bool, action string) (*config.Server, *fluig.Client, error) {
	server, err := a.resolveServer("")
	if err != nil {
		return nil, nil, err
	}
	a.printer.Server = server.Name
	if err := a.guardProdWrite(server, action); err != nil {
		return nil, nil, err
	}
	client, err := a.authenticate(ctx, server, passwordStdin)
	if err != nil {
		return nil, nil, err
	}
	return server, client, nil
}

// guardProdWrite pede confirmação para escrever em servidor de produção,
// respeitando --yes e o modo não-interativo (via App.confirm).
func (a *App) guardProdWrite(server *config.Server, action string) error {
	if server.Env != config.EnvProd {
		return nil
	}
	return a.confirm(fmt.Sprintf("O servidor %q é de PRODUÇÃO — %s mesmo assim?", server.Name, action))
}
