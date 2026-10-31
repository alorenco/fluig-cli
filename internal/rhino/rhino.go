// Package rhino roda scripts no motor Rhino oficial, na máquina local, para o
// teste de unidade dos scripts Fluig (ROADMAP §5.18, spec em
// docs/interno/SPEC-TEST-D.md).
//
// O servidor Fluig (Voyager 2) roda os scripts no fluig-rhino-1.7.15.1.jar,
// uma recompilação da TOTVS da versão 1.7.15.1. Este pacote usa o jar
// OFICIAL da mesma versão, baixado do Maven Central com o SHA-256 conferido. O
// binário do fluigcli não embute o jar.
//
// Sem cobra e sem I/O de terminal (regra 3): a CLI traduz os erros.
package rhino

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	// Version é a versão do Rhino que o servidor usa (Gate D, 2026-10-31).
	Version = "1.7.15.1"
	// SHA256 é o hash do rhino-1.7.15.1.jar oficial do Maven Central.
	SHA256 = "f3520a0e2e22d550f1216a456be233eb5385bf08e3219407dece7a16369eb38f"
	// LanguageVersion é a versão de linguagem do servidor (ES6). Só 200
	// explica for…of, Map/Set e parâmetro rest juntos (Gate D, D0.2).
	LanguageVersion = 200
	// MinJava é a versão mínima do Java (a mesma do servidor: 11).
	MinJava = 11
	// TimeZone é o fuso dos servidores Fluig da casa.
	TimeZone = "America/Sao_Paulo"
)

// wantSHA256 é o hash conferido. Variável só para os testes.
var wantSHA256 = SHA256

// DownloadURL é o endereço do jar oficial. Variável para os testes.
var DownloadURL = "https://repo1.maven.org/maven2/org/mozilla/rhino/" + Version + "/rhino-" + Version + ".jar"

// Erros que a CLI traduz para exit 7.
var (
	ErrJavaNotFound = errors.New("java não encontrado")
	ErrJavaTooOld   = errors.New("java antigo")
	ErrUnavailable  = errors.New("rhino indisponível")
)

// Java é o executável escolhido.
type Java struct {
	Path    string `json:"path"`
	Version string `json:"version"`
	Major   int    `json:"major"`
}

// Jar é o jar do motor escolhido.
type Jar struct {
	Path string `json:"path"`
	// Source: "flag" (--rhino-jar), "cache" ou "maven" (baixado agora).
	Source string `json:"source"`
}

// lookPath e runVersion são variáveis para os testes.
var (
	lookPath   = exec.LookPath
	runVersion = func(ctx context.Context, path string) (string, error) {
		out, err := exec.CommandContext(ctx, path, "-version").CombinedOutput()
		return string(out), err
	}
)

// FindJava acha o java na ordem: explicit (--java), $FLUIGCLI_JAVA,
// $JAVA_HOME/bin/java, java no PATH. Confere a versão mínima.
func FindJava(ctx context.Context, explicit string) (*Java, error) {
	var cands []string
	switch {
	case explicit != "":
		cands = []string{explicit}
	case os.Getenv("FLUIGCLI_JAVA") != "":
		cands = []string{os.Getenv("FLUIGCLI_JAVA")}
	default:
		if h := os.Getenv("JAVA_HOME"); h != "" {
			cands = append(cands, filepath.Join(h, "bin", "java"))
		}
		cands = append(cands, "java")
	}
	var lastErr error
	for _, c := range cands {
		p, err := lookPath(c)
		if err != nil {
			lastErr = err
			continue
		}
		out, err := runVersion(ctx, p)
		if err != nil {
			lastErr = err
			continue
		}
		v, major := ParseJavaVersion(out)
		if major == 0 {
			lastErr = fmt.Errorf("versão ilegível em %q", strings.TrimSpace(firstLine(out)))
			continue
		}
		if major < MinJava {
			return nil, fmt.Errorf("%w: %s é Java %s; o mínimo é Java %d", ErrJavaTooOld, p, v, MinJava)
		}
		return &Java{Path: p, Version: v, Major: major}, nil
	}
	return nil, fmt.Errorf("%w (%s): %v", ErrJavaNotFound, strings.Join(cands, ", "), lastErr)
}

var javaVersionRe = regexp.MustCompile(`version "([^"]+)"`)

// ParseJavaVersion lê a saída do `java -version`. "1.8.0_292" = 8;
// "17.0.19" = 17; "21" = 21.
func ParseJavaVersion(out string) (string, int) {
	m := javaVersionRe.FindStringSubmatch(out)
	if m == nil {
		return "", 0
	}
	v := m[1]
	parts := strings.FieldsFunc(v, func(r rune) bool { return r == '.' || r == '_' || r == '-' || r == '+' })
	if len(parts) == 0 {
		return v, 0
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return v, 0
	}
	if major == 1 && len(parts) > 1 {
		major, _ = strconv.Atoi(parts[1])
	}
	return v, major
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// CacheDir é <cache do usuário>/fluigcli/rhino. Variável para os testes.
var CacheDir = func() (string, error) {
	d, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "fluigcli", "rhino"), nil
}

// EnsureJar devolve o jar do motor. Com explicit (--rhino-jar), usa o arquivo
// sem conferir hash. Senão, usa o cache se o hash bater; senão, baixa do Maven
// Central, confere o hash e grava no cache. Hash diferente = recusa, e nada é
// gravado.
func EnsureJar(ctx context.Context, explicit string) (*Jar, error) {
	if explicit != "" {
		if _, err := os.Stat(explicit); err != nil {
			return nil, fmt.Errorf("%w: %s: %v", ErrUnavailable, explicit, err)
		}
		return &Jar{Path: explicit, Source: "flag"}, nil
	}
	dir, err := CacheDir()
	if err != nil {
		return nil, fmt.Errorf("%w: sem pasta de cache: %v", ErrUnavailable, err)
	}
	path := filepath.Join(dir, "rhino-"+Version+".jar")
	if data, err := os.ReadFile(path); err == nil {
		if hashOf(data) == wantSHA256 {
			return &Jar{Path: path, Source: "cache"}, nil
		}
		// Cache adulterado ou corrompido: não usa e baixa de novo.
		_ = os.Remove(path)
	}
	data, err := download(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: download de %s falhou: %v", ErrUnavailable, DownloadURL, err)
	}
	if h := hashOf(data); h != wantSHA256 {
		return nil, fmt.Errorf("%w: o jar baixado de %s tem SHA-256 %s, esperado %s; nada foi gravado", ErrUnavailable, DownloadURL, h, wantSHA256)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("%w: não consegui criar %s: %v", ErrUnavailable, dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".rhino-*.jar")
	if err != nil {
		return nil, fmt.Errorf("%w: não consegui gravar em %s: %v", ErrUnavailable, dir, err)
	}
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if werr != nil || cerr != nil {
		_ = os.Remove(tmp.Name())
		return nil, fmt.Errorf("%w: não consegui gravar %s: %v", ErrUnavailable, tmp.Name(), errors.Join(werr, cerr))
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		_ = os.Remove(tmp.Name())
		return nil, fmt.Errorf("%w: não consegui gravar %s: %v", ErrUnavailable, path, err)
	}
	return &Jar{Path: path, Source: "maven"}, nil
}

func hashOf(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func download(ctx context.Context) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, DownloadURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 32<<20))
}

// Args monta a linha de comando do shell do Rhino com a configuração do
// servidor: ES6 (200), modo interpretado (-opt -1, sem o limite de 64 KB por
// método da compilação para bytecode) e o fuso dos servidores.
func Args(jar, script string, scriptArgs ...string) []string {
	a := []string{
		"-Duser.timezone=" + TimeZone,
		"-Dfile.encoding=UTF-8",
		"-cp", jar,
		"org.mozilla.javascript.tools.shell.Main",
		"-version", strconv.Itoa(LanguageVersion),
		"-opt", "-1",
		script,
	}
	return append(a, scriptArgs...)
}

// Run executa o shell do Rhino. O ctx carrega o teto de tempo: no estouro, o
// processo morre e o erro é context.DeadlineExceeded.
func Run(ctx context.Context, java *Java, jar *Jar, script string, scriptArgs ...string) (stdout, stderr []byte, err error) {
	cmd := exec.CommandContext(ctx, java.Path, Args(jar.Path, script, scriptArgs...)...)
	var o, e bytes.Buffer
	cmd.Stdout, cmd.Stderr = &o, &e
	cmd.Stdin = nil
	err = cmd.Run()
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	return o.Bytes(), e.Bytes(), err
}
