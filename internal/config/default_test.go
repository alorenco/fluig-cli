package config

import (
	"testing"
)

func TestNormalizeEnv(t *testing.T) {
	ok := map[string]string{
		"":                "",
		"dev":             EnvDev,
		"DEV":             EnvDev,
		"desenvolvimento": EnvDev,
		"hml":             EnvHml,
		"homolog":         EnvHml,
		"homologação":     EnvHml,
		"staging":         EnvHml,
		"prod":            EnvProd,
		"producao":        EnvProd,
		"production":      EnvProd,
		"prd":             EnvProd,
		" Prod ":          EnvProd,
	}
	for in, want := range ok {
		got, err := NormalizeEnv(in)
		if err != nil || got != want {
			t.Errorf("NormalizeEnv(%q) = (%q, %v), quer (%q, nil)", in, got, err, want)
		}
	}
	if _, err := NormalizeEnv("banana"); err == nil {
		t.Error("NormalizeEnv deveria rejeitar ambiente desconhecido")
	}
}

func TestDefaultPrecedenceProjetoSobreGlobal(t *testing.T) {
	st := newTestStore(t, true)
	if err := st.Add(Server{ID: "1", Name: "global-srv", Host: "g", Username: "u"}, true); err != nil {
		t.Fatal(err)
	}
	if err := st.Add(Server{ID: "2", Name: "proj-srv", Host: "p", Username: "u"}, false); err != nil {
		t.Fatal(err)
	}

	if _, err := st.SetDefault("global-srv", true); err != nil {
		t.Fatal(err)
	}
	def, err := st.DefaultName()
	if err != nil || def != "global-srv" {
		t.Fatalf("padrão global = (%q, %v), quer global-srv", def, err)
	}

	// O padrão do projeto passa a vencer o global.
	if _, err := st.SetDefault("proj-srv", false); err != nil {
		t.Fatal(err)
	}
	def, err = st.DefaultName()
	if err != nil || def != "proj-srv" {
		t.Fatalf("padrão com projeto = (%q, %v), quer proj-srv", def, err)
	}
}

func TestSetDefaultServidorInexistente(t *testing.T) {
	st := newTestStore(t, false)
	if _, err := st.SetDefault("nao-existe", false); err == nil {
		t.Fatal("SetDefault deveria exigir que o servidor exista")
	}
}

func TestRemoveLimpaDefaultOrfao(t *testing.T) {
	st := newTestStore(t, false)
	if err := st.Add(Server{ID: "1", Name: "x", Host: "h", Username: "u"}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetDefault("x", false); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Remove("x"); err != nil {
		t.Fatal(err)
	}
	def, err := st.DefaultName()
	if err != nil {
		t.Fatal(err)
	}
	if def != "" {
		t.Errorf("padrão após remover o servidor = %q, quer vazio", def)
	}
}

func TestUpdateServidor(t *testing.T) {
	st := newTestStore(t, true)
	if err := st.Add(Server{ID: "1", Name: "x", Host: "a", Username: "u"}, false); err != nil {
		t.Fatal(err)
	}
	updated, err := st.Update("x", func(s *Server) {
		s.Env = EnvProd
		s.Host = "b"
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Env != EnvProd || updated.Host != "b" {
		t.Errorf("retorno do Update = %+v, quer env=prod host=b", updated)
	}
	got, err := st.Get("x")
	if err != nil {
		t.Fatal(err)
	}
	if got.Env != EnvProd || got.Host != "b" {
		t.Errorf("persistido = %+v, quer env=prod host=b", got)
	}
	if _, err := st.Update("nao-existe", func(*Server) {}); err == nil {
		t.Error("Update deveria falhar para servidor inexistente")
	}
}
