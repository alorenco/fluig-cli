package rhino

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestParseJavaVersion(t *testing.T) {
	casos := []struct {
		out   string
		v     string
		major int
	}{
		{`openjdk version "11.0.7" 2020-04-14`, "11.0.7", 11},
		{`openjdk version "17.0.19" 2026-04-21`, "17.0.19", 17},
		{`java version "1.8.0_292"`, "1.8.0_292", 8},
		{`openjdk version "21" 2023-09-19`, "21", 21},
		{`openjdk version "21.0.12.1" 2026-08-18`, "21.0.12.1", 21},
		{`bash: java: command not found`, "", 0},
	}
	for _, c := range casos {
		v, m := ParseJavaVersion(c.out)
		if v != c.v || m != c.major {
			t.Errorf("%q: obtido %q/%d, quer %q/%d", c.out, v, m, c.v, c.major)
		}
	}
}

func stubJava(t *testing.T, version string, err error) {
	t.Helper()
	oldLook, oldRun := lookPath, runVersion
	t.Cleanup(func() { lookPath, runVersion = oldLook, oldRun })
	lookPath = func(p string) (string, error) {
		if err != nil {
			return "", err
		}
		return "/opt/" + filepath.Base(p), nil
	}
	runVersion = func(context.Context, string) (string, error) {
		return `openjdk version "` + version + `" 2026-01-01`, nil
	}
}

func TestFindJava(t *testing.T) {
	t.Setenv("FLUIGCLI_JAVA", "")
	t.Setenv("JAVA_HOME", "")
	stubJava(t, "17.0.19", nil)
	j, err := FindJava(context.Background(), "")
	if err != nil || j.Major != 17 || j.Path != "/opt/java" {
		t.Fatalf("java 17: %+v %v", j, err)
	}

	stubJava(t, "1.8.0_292", nil)
	if _, err := FindJava(context.Background(), ""); !errors.Is(err, ErrJavaTooOld) || !strings.Contains(err.Error(), "mínimo é Java 11") {
		t.Errorf("java 8 tem de ser recusado: %v", err)
	}

	stubJava(t, "", errors.New("não achei"))
	if _, err := FindJava(context.Background(), "/x/java"); !errors.Is(err, ErrJavaNotFound) {
		t.Errorf("sem java: %v", err)
	}
}

// O --java vence o FLUIGCLI_JAVA, que vence o JAVA_HOME.
func TestFindJavaPrecedencia(t *testing.T) {
	var visto []string
	oldLook, oldRun := lookPath, runVersion
	t.Cleanup(func() { lookPath, runVersion = oldLook, oldRun })
	lookPath = func(p string) (string, error) { visto = append(visto, p); return p, nil }
	runVersion = func(context.Context, string) (string, error) { return `version "17.0.1"`, nil }

	t.Setenv("JAVA_HOME", "/jh")
	t.Setenv("FLUIGCLI_JAVA", "/env/java")
	FindJava(context.Background(), "/flag/java")
	FindJava(context.Background(), "")
	t.Setenv("FLUIGCLI_JAVA", "")
	FindJava(context.Background(), "")
	want := []string{"/flag/java", "/env/java", filepath.Join("/jh", "bin", "java")}
	if strings.Join(visto, "|") != strings.Join(want, "|") {
		t.Errorf("ordem: %v", visto)
	}
}

func jarServer(t *testing.T, body string) (*httptest.Server, *int32) {
	t.Helper()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	oldURL, oldSHA, oldDir := DownloadURL, wantSHA256, CacheDir
	t.Cleanup(func() { DownloadURL, wantSHA256, CacheDir = oldURL, oldSHA, oldDir })
	DownloadURL = srv.URL + "/rhino.jar"
	dir := t.TempDir()
	CacheDir = func() (string, error) { return filepath.Join(dir, "rhino"), nil }
	return srv, &hits
}

func TestEnsureJarDownloadECache(t *testing.T) {
	_, hits := jarServer(t, "jar-de-teste")
	wantSHA256 = hashOf([]byte("jar-de-teste"))

	j, err := EnsureJar(context.Background(), "")
	if err != nil || j.Source != "maven" {
		t.Fatalf("1º uso baixa: %+v %v", j, err)
	}
	j, err = EnsureJar(context.Background(), "")
	if err != nil || j.Source != "cache" || atomic.LoadInt32(hits) != 1 {
		t.Fatalf("2º uso vem do cache, sem rede: %+v %v hits=%d", j, err, *hits)
	}

	// Cache adulterado: descarta e baixa de novo.
	os.WriteFile(j.Path, []byte("adulterado"), 0o644)
	j, err = EnsureJar(context.Background(), "")
	if err != nil || j.Source != "maven" || atomic.LoadInt32(hits) != 2 {
		t.Fatalf("cache adulterado: %+v %v", j, err)
	}
	if b, _ := os.ReadFile(j.Path); string(b) != "jar-de-teste" {
		t.Error("o cache não foi refeito")
	}
}

func TestEnsureJarHashDiferenteNaoGrava(t *testing.T) {
	jarServer(t, "jar-trocado")
	wantSHA256 = hashOf([]byte("o-oficial"))
	_, err := EnsureJar(context.Background(), "")
	if !errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), "nada foi gravado") {
		t.Fatalf("hash diferente tem de ser recusado: %v", err)
	}
	dir, _ := CacheDir()
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("nada pode ficar no cache: %v", entries)
	}
}

func TestEnsureJarExplicito(t *testing.T) {
	_, hits := jarServer(t, "x")
	p := filepath.Join(t.TempDir(), "meu.jar")
	os.WriteFile(p, []byte("qualquer coisa"), 0o644)
	j, err := EnsureJar(context.Background(), p)
	if err != nil || j.Source != "flag" || j.Path != p || atomic.LoadInt32(hits) != 0 {
		t.Fatalf("--rhino-jar: %+v %v", j, err)
	}
	if _, err := EnsureJar(context.Background(), "/nao/existe.jar"); !errors.Is(err, ErrUnavailable) {
		t.Errorf("jar ausente: %v", err)
	}
}

func TestEnsureJarSemRede(t *testing.T) {
	srv, _ := jarServer(t, "x")
	srv.Close()
	if _, err := EnsureJar(context.Background(), ""); !errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), "download") {
		t.Errorf("sem rede e sem cache: %v", err)
	}
}

// A linha de comando repete a configuração do servidor.
func TestArgs(t *testing.T) {
	a := strings.Join(Args("r.jar", "boot.js", "plano.json"), " ")
	for _, want := range []string{"-Duser.timezone=America/Sao_Paulo", "-cp r.jar", "org.mozilla.javascript.tools.shell.Main",
		"-version 200", "-opt -1", "boot.js plano.json"} {
		if !strings.Contains(a, want) {
			t.Errorf("sem %q em %s", want, a)
		}
	}
}
