package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alorenco/fluig-cli/internal/output"
)

func newTestStore(t *testing.T, withProject bool) *Store {
	t.Helper()
	st := &Store{globalDir: t.TempDir()}
	if withProject {
		st.ProjectDir = t.TempDir()
	}
	return st
}

func TestStoreAddGetRemove(t *testing.T) {
	st := newTestStore(t, false)
	s := Server{ID: "abc123", Name: "homolog", Host: "fluig.test", Port: 443, SSL: true, Username: "admin", CompanyID: 1}

	if err := st.Add(s, false); err != nil {
		t.Fatalf("Add: %v", err)
	}
	got, err := st.Get("homolog")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Host != "fluig.test" || got.ID != "abc123" {
		t.Errorf("servidor incorreto: %+v", got)
	}

	if err := st.Add(s, false); err == nil {
		t.Errorf("Add duplicado deveria falhar")
	}

	removed, err := st.Remove("homolog")
	if err != nil || removed.ID != "abc123" {
		t.Fatalf("Remove: %v (%+v)", err, removed)
	}
	if _, err := st.Get("homolog"); output.ExitCodeFor(err) != output.ExitNotFound {
		t.Errorf("Get após remove deveria dar NOT_FOUND, veio %v", err)
	}
}

// Projeto tem precedência sobre o global.
func TestStoreProjectPrecedence(t *testing.T) {
	st := newTestStore(t, true)

	if err := st.Add(Server{ID: "g1", Name: "homolog", Host: "global.test"}, true); err != nil {
		t.Fatalf("Add global: %v", err)
	}
	if err := st.Add(Server{ID: "p1", Name: "homolog", Host: "projeto.test"}, false); err != nil {
		t.Fatalf("Add projeto: %v", err)
	}

	got, err := st.Get("homolog")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Host != "projeto.test" {
		t.Errorf("precedência errada: veio %q, quer projeto.test", got.Host)
	}

	list, err := st.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("List deveria deduplicar por nome, veio %d itens", len(list))
	}
}

// servers.json nunca pode conter senha.
func TestServersFileHasNoPassword(t *testing.T) {
	st := newTestStore(t, false)
	if err := st.Add(Server{ID: "x", Name: "n", Host: "h"}, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(st.globalDir, "servers.json"))
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(data))
	if strings.Contains(lower, "password") || strings.Contains(lower, "senha") {
		t.Errorf("servers.json contém campo de senha:\n%s", data)
	}
}

func TestBaseURL(t *testing.T) {
	cases := []struct {
		s    Server
		want string
	}{
		{Server{Host: "h.test", Port: 443, SSL: true}, "https://h.test"},
		{Server{Host: "h.test", Port: 8443, SSL: true}, "https://h.test:8443"},
		{Server{Host: "h.test", Port: 80, SSL: false}, "http://h.test"},
		{Server{Host: "h.test", Port: 8080, SSL: false}, "http://h.test:8080"},
	}
	for _, tc := range cases {
		if got := tc.s.BaseURL(); got != tc.want {
			t.Errorf("BaseURL(%+v) = %q, quer %q", tc.s, got, tc.want)
		}
	}
}

// mockKeyring simula o keyring do SO em memória.
type mockKeyring struct {
	data map[string]string
	err  error
}

func (m *mockKeyring) Get(id string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	pw, ok := m.data[id]
	if !ok {
		return "", ErrKeyringNotFound
	}
	return pw, nil
}
func (m *mockKeyring) Set(id, pw string) error {
	if m.err != nil {
		return m.err
	}
	m.data[id] = pw
	return nil
}
func (m *mockKeyring) Delete(id string) error { delete(m.data, id); return nil }
func (m *mockKeyring) Available() bool        { return m.err == nil }

// Ordem de precedência da senha.
func TestPasswordPrecedence(t *testing.T) {
	server := &Server{ID: "s1", Name: "homolog"}
	kr := &mockKeyring{data: map[string]string{"s1": "do-keyring"}}
	env := func(k string) string {
		if k == EnvPassword {
			return "da-env"
		}
		return ""
	}
	prompt := func(*Server) (string, bool, error) { return "do-prompt", false, nil }

	check := func(t *testing.T, res *PasswordResult, err error, wantPw string, wantSrc Source) {
		t.Helper()
		if err != nil {
			t.Fatalf("err=%v", err)
		}
		if res.Password != wantPw {
			t.Errorf("senha=%q, quer %q", res.Password, wantPw)
		}
		if res.Source != wantSrc {
			t.Errorf("origem=%d, quer %d", res.Source, wantSrc)
		}
	}

	t.Run("stdin vence tudo", func(t *testing.T) {
		src := PasswordSource{Stdin: strings.NewReader("do-stdin\n"), Getenv: env, Keyring: kr, Prompt: prompt}
		res, err := src.Resolve(server)
		check(t, res, err, "do-stdin", SourceStdin)
	})

	t.Run("env vence keyring", func(t *testing.T) {
		src := PasswordSource{Getenv: env, Keyring: kr, Prompt: prompt}
		res, err := src.Resolve(server)
		check(t, res, err, "da-env", SourceEnv)
	})

	t.Run("keyring vence prompt", func(t *testing.T) {
		src := PasswordSource{Getenv: func(string) string { return "" }, Keyring: kr, Prompt: prompt}
		res, err := src.Resolve(server)
		check(t, res, err, "do-keyring", SourceKeyring)
	})

	t.Run("prompt como último recurso", func(t *testing.T) {
		src := PasswordSource{Getenv: func(string) string { return "" }, Keyring: &mockKeyring{data: map[string]string{}}, Prompt: prompt}
		res, err := src.Resolve(server)
		check(t, res, err, "do-prompt", SourcePrompt)
	})

	t.Run("sem fonte → exit 3", func(t *testing.T) {
		src := PasswordSource{Getenv: func(string) string { return "" }, Keyring: &mockKeyring{data: map[string]string{}}}
		_, err := src.Resolve(server)
		if output.ExitCodeFor(err) != output.ExitAuth {
			t.Errorf("esperava AUTH_FAILED (exit 3), veio %v", err)
		}
	})

	t.Run("keyring indisponível cai para o prompt", func(t *testing.T) {
		src := PasswordSource{
			Getenv:  func(string) string { return "" },
			Keyring: &mockKeyring{err: errors.New("sem dbus")},
			Prompt:  prompt,
		}
		res, err := src.Resolve(server)
		check(t, res, err, "do-prompt", SourcePrompt)
	})
}

// Resolve NÃO grava no keyring; a gravação é deferida para SaveIfRequested,
// que só deve ser chamada após a autenticação dar certo (para nunca persistir
// uma senha errada). Regressão do bug achado na validação da Fase 0.
func TestPasswordSaveIsDeferred(t *testing.T) {
	server := &Server{ID: "s2", Name: "homolog"}
	kr := &mockKeyring{data: map[string]string{}}
	src := PasswordSource{
		Getenv:  func(string) string { return "" },
		Keyring: kr,
		Prompt:  func(*Server) (string, bool, error) { return "nova", true, nil },
	}
	res, err := src.Resolve(server)
	if err != nil {
		t.Fatal(err)
	}
	if _, saved := kr.data["s2"]; saved {
		t.Error("Resolve não pode gravar no keyring antes da validação")
	}
	if err := res.SaveIfRequested(); err != nil {
		t.Fatal(err)
	}
	if kr.data["s2"] != "nova" {
		t.Errorf("SaveIfRequested deveria gravar a senha, keyring=%+v", kr.data)
	}
}

// Quando o usuário não pede para salvar, SaveIfRequested é no-op.
func TestPasswordSaveNotRequested(t *testing.T) {
	server := &Server{ID: "s3", Name: "homolog"}
	kr := &mockKeyring{data: map[string]string{}}
	src := PasswordSource{
		Getenv:  func(string) string { return "" },
		Keyring: kr,
		Prompt:  func(*Server) (string, bool, error) { return "x", false, nil },
	}
	res, _ := src.Resolve(server)
	if err := res.SaveIfRequested(); err != nil {
		t.Fatal(err)
	}
	if len(kr.data) != 0 {
		t.Errorf("nada deveria ser salvo, keyring=%+v", kr.data)
	}
}

func TestReadPasswordStdin(t *testing.T) {
	pw, err := readPasswordStdin(strings.NewReader("segredo\r\n"))
	if err != nil || pw != "segredo" {
		t.Errorf("pw=%q err=%v", pw, err)
	}
	if _, err := readPasswordStdin(strings.NewReader("\n")); output.ExitCodeFor(err) != output.ExitAuth {
		t.Errorf("senha vazia deveria falhar com exit 3, veio %v", err)
	}
	// Sem newline no final (echo -n)
	pw, err = readPasswordStdin(strings.NewReader("sem-newline"))
	if err != nil || pw != "sem-newline" {
		t.Errorf("pw=%q err=%v", pw, err)
	}
}
