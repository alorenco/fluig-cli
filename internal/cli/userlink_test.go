package cli

// Testes dos comandos de vínculo pelo lado do usuário (ROADMAP §5.1).

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alorenco/fluig-cli/internal/output"
)

// Regra 7 do CLAUDE.md: toda listagem tem tabela no modo humano.
func TestUserRolesListaTabela(t *testing.T) {
	stub := &adminUserStub{}
	proj := adminUserProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "user", "roles", "user1", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	for _, want := range []string{"│", "Código", "Descrição", "gestor_compras", "Gestor de Compras"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("tabela sem %q:\n%s", want, stdout)
		}
	}
}

func TestUserGroupsListaTabela(t *testing.T) {
	stub := &adminUserStub{}
	proj := adminUserProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "user", "groups", "user1", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	// O grupo tem coluna Tipo; o papel não.
	for _, want := range []string{"Código", "Descrição", "Tipo", "TI", "community"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("tabela sem %q:\n%s", want, stdout)
		}
	}
}

func TestUserRolesJSON(t *testing.T) {
	stub := &adminUserStub{}
	proj := adminUserProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "user", "roles", "user1", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("json inválido: %v", err)
	}
	data, _ := env.Data.(map[string]any)
	if data["login"] != "user1" {
		t.Errorf("o envelope precisa dizer de quem são os papéis: %+v", data)
	}
	roles, _ := data["roles"].([]any)
	if len(roles) != 3 {
		t.Errorf("esperava 3 papéis, veio %d", len(roles))
	}
}

// O vínculo escrito é o mesmo do `role add-user`, só que pela outra porta:
// o corpo do POST leva o CÓDIGO do papel (não o login).
func TestUserAddRemoveRole(t *testing.T) {
	stub := &adminUserStub{}
	proj := adminUserProject(t, stub.server(t).URL)

	code, stdout := runMain(t, "user", "add-role", "user1", "faturista", "--yes", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("add-role exit=%d stdout=%s", code, stdout)
	}
	if len(stub.vinculosPost) != 1 || !strings.Contains(stub.vinculosPost[0], `roles:{"code":"faturista"}`) {
		t.Errorf("POST inesperado: %v", stub.vinculosPost)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	if data["login"] != "user1" || data["role"] != "faturista" || data["member"] != true {
		t.Errorf("envelope inesperado: %+v", data)
	}

	code, _ = runMain(t, "user", "remove-role", "user1", "faturista", "--yes", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("remove-role exit=%d", code)
	}
	if len(stub.vinculosDelete) != 1 || stub.vinculosDelete[0] != "roles/faturista" {
		t.Errorf("DELETE inesperado: %v", stub.vinculosDelete)
	}
}

func TestUserAddRemoveGroup(t *testing.T) {
	stub := &adminUserStub{}
	proj := adminUserProject(t, stub.server(t).URL)

	code, stdout := runMain(t, "user", "add-group", "user1", "TI", "--yes", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("add-group exit=%d stdout=%s", code, stdout)
	}
	if len(stub.vinculosPost) != 1 || !strings.Contains(stub.vinculosPost[0], `groups:{"code":"TI"}`) {
		t.Errorf("POST inesperado: %v", stub.vinculosPost)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	if data["group"] != "TI" || data["member"] != true {
		t.Errorf("envelope inesperado: %+v", data)
	}

	code, _ = runMain(t, "user", "remove-group", "user1", "TI", "--yes", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("remove-group exit=%d", code)
	}
	if len(stub.vinculosDelete) != 1 || stub.vinculosDelete[0] != "groups/TI" {
		t.Errorf("DELETE inesperado: %v", stub.vinculosDelete)
	}
}

// Alvo inexistente: exit 4 e NENHUMA escrita. No caso do grupo a API real
// responde 200 sem criar vínculo, então quem garante é a pré-validação.
func TestUserLinkAlvoInexistente(t *testing.T) {
	casos := []struct{ nome, sub, alvo string }{
		{"papel", "add-role", "zz_nao_existe"},
		{"grupo", "add-group", "zz_nao_existe"},
	}
	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			stub := &adminUserStub{}
			proj := adminUserProject(t, stub.server(t).URL)
			code, stdout := runMain(t, "user", caso.sub, "user1", caso.alvo, "--yes", "--json", "--project", proj, "--server", "homolog")
			if code != output.ExitNotFound {
				t.Fatalf("exit=%d, quer %d\n%s", code, output.ExitNotFound, stdout)
			}
			if len(stub.vinculosPost) != 0 {
				t.Errorf("nenhum POST podia ter saído: %v", stub.vinculosPost)
			}
		})
	}
}

// Usuário inexistente: exit 4 nas listagens e nas escritas.
func TestUserLinkUsuarioInexistenteCLI(t *testing.T) {
	stub := &adminUserStub{}
	proj := adminUserProject(t, stub.server(t).URL)
	for _, args := range [][]string{
		{"user", "roles", "zz_ninguem"},
		{"user", "groups", "zz_ninguem"},
		{"user", "add-role", "zz_ninguem", "faturista", "--yes"},
	} {
		full := append(args, "--json", "--project", proj, "--server", "homolog")
		if code, stdout := runMain(t, full...); code != output.ExitNotFound {
			t.Errorf("%v: exit=%d, quer %d\n%s", args, code, output.ExitNotFound, stdout)
		}
	}
}
