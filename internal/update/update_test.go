package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int // sinal esperado
	}{
		{"0.1.0", "0.2.0", -1},
		{"0.2.0", "0.1.0", 1},
		{"0.1.0", "0.1.0", 0},
		{"v0.1.0", "0.1.0", 0},
		{"0.10.0", "0.9.9", 1},
		{"1.0.0-rc1", "1.0.0", -1},
		{"1.0.0", "1.0.0-rc1", 1},
		{"1.0.0-rc1", "1.0.0-rc2", -1},
		{"", "0.1.0", -1},
		{"1.0", "1.0.0", 0},
	}
	for _, c := range cases {
		got := Compare(c.a, c.b)
		if sign(got) != c.want {
			t.Errorf("Compare(%q, %q) = %d, quer sinal %d", c.a, c.b, got, c.want)
		}
	}
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	default:
		return 0
	}
}

func TestIsNewer(t *testing.T) {
	if !IsNewer("0.2.0", "0.1.0") {
		t.Error("0.2.0 deveria ser mais nova que 0.1.0")
	}
	if IsNewer("0.2.0", "dev") {
		t.Error("build dev nunca é considerada desatualizada")
	}
	if IsNewer("0.2.0", "") {
		t.Error("versão vazia nunca é considerada desatualizada")
	}
	if IsNewer("0.1.0", "0.1.0") {
		t.Error("mesma versão não é mais nova")
	}
}

func TestLatest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/releases/latest" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "https://example.com/releases/tag/v0.3.1", http.StatusFound)
	}))
	defer srv.Close()

	c := NewClient(5 * time.Second)
	c.BaseURL = srv.URL + "/releases"
	got, err := c.Latest(context.Background())
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if got != "0.3.1" {
		t.Errorf("Latest = %q, quer 0.3.1", got)
	}
}

func TestLatestSemRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient(5 * time.Second)
	c.BaseURL = srv.URL + "/releases"
	if _, err := c.Latest(context.Background()); err == nil {
		t.Fatal("Latest deveria falhar sem redirect para /tag/v…")
	}
}

// fakeRelease monta um servidor que serve uma release completa (tar.gz, zip e
// checksums.txt) da versão dada, com o conteúdo do binário informado.
func fakeRelease(t *testing.T, version string, binContent []byte) *httptest.Server {
	t.Helper()

	tgz := makeTarGz(t, "fluigcli", binContent)
	zipPkg := makeZip(t, "fluigcli.exe", binContent)

	tgzName := ArchiveName(version, "linux", "amd64")
	zipName := ArchiveName(version, "windows", "amd64")
	sums := fmt.Sprintf("%s  %s\n%s  %s\n",
		sha256hex(tgz), tgzName, sha256hex(zipPkg), zipName)

	mux := http.NewServeMux()
	prefix := "/releases/download/v" + version + "/"
	mux.HandleFunc(prefix+tgzName, serveBytes(tgz))
	mux.HandleFunc(prefix+zipName, serveBytes(zipPkg))
	mux.HandleFunc(prefix+"checksums.txt", serveBytes([]byte(sums)))
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://example.com/releases/tag/v"+version, http.StatusFound)
	})
	return httptest.NewServer(mux)
}

func serveBytes(b []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(b)
	}
}

func makeTarGz(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(content))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func makeZip(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sha256hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func TestDownloadTarGz(t *testing.T) {
	want := []byte("binario-linux")
	srv := fakeRelease(t, "0.2.0", want)
	defer srv.Close()

	c := NewClient(5 * time.Second)
	c.BaseURL = srv.URL + "/releases"
	bin, err := c.Download(context.Background(), "0.2.0", "linux", "amd64", t.TempDir())
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	got, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("binário extraído = %q, quer %q", got, want)
	}
}

func TestDownloadZip(t *testing.T) {
	want := []byte("binario-windows")
	srv := fakeRelease(t, "0.2.0", want)
	defer srv.Close()

	c := NewClient(5 * time.Second)
	c.BaseURL = srv.URL + "/releases"
	bin, err := c.Download(context.Background(), "0.2.0", "windows", "amd64", t.TempDir())
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if filepath.Base(bin) != "fluigcli.exe" {
		t.Errorf("binário = %s, quer fluigcli.exe", bin)
	}
	got, _ := os.ReadFile(bin)
	if !bytes.Equal(got, want) {
		t.Errorf("binário extraído = %q, quer %q", got, want)
	}
}

func TestDownloadChecksumErrado(t *testing.T) {
	version := "0.2.0"
	tgz := makeTarGz(t, "fluigcli", []byte("conteudo"))
	tgzName := ArchiveName(version, "linux", "amd64")
	sums := "deadbeef  " + tgzName + "\n"

	mux := http.NewServeMux()
	prefix := "/releases/download/v" + version + "/"
	mux.HandleFunc(prefix+tgzName, serveBytes(tgz))
	mux.HandleFunc(prefix+"checksums.txt", serveBytes([]byte(sums)))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewClient(5 * time.Second)
	c.BaseURL = srv.URL + "/releases"
	if _, err := c.Download(context.Background(), version, "linux", "amd64", t.TempDir()); err == nil {
		t.Fatal("Download deveria falhar com checksum errado")
	}
}

func TestDownloadVersaoInexistente(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(http.NotFound))
	defer srv.Close()

	c := NewClient(5 * time.Second)
	c.BaseURL = srv.URL + "/releases"
	if _, err := c.Download(context.Background(), "9.9.9", "linux", "amd64", t.TempDir()); err == nil {
		t.Fatal("Download deveria falhar para versão inexistente")
	}
}

func TestReplaceExecutable(t *testing.T) {
	for _, goos := range []string{"linux", "windows"} {
		t.Run(goos, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "fluigcli")
			newBin := filepath.Join(dir, "novo")
			if err := os.WriteFile(target, []byte("antigo"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(newBin, []byte("novo"), 0o755); err != nil {
				t.Fatal(err)
			}

			if err := replaceExecutable(newBin, target, goos); err != nil {
				t.Fatalf("replaceExecutable: %v", err)
			}
			got, _ := os.ReadFile(target)
			if string(got) != "novo" {
				t.Errorf("alvo = %q, quer \"novo\"", got)
			}
			_, err := os.Stat(target + ".old")
			if goos == "windows" && err != nil {
				t.Error("no windows o binário antigo deveria ficar como .old")
			}
			if goos != "windows" && err == nil {
				t.Error("fora do windows não deveria sobrar .old")
			}
		})
	}
}

func TestCheckForNotice(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.Redirect(w, r, "https://example.com/releases/tag/v0.5.0", http.StatusFound)
	}))
	defer srv.Close()

	c := NewClient(5 * time.Second)
	c.BaseURL = srv.URL + "/releases"
	cache := filepath.Join(t.TempDir(), "update-check.json")
	now := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)

	// Primeira chamada: consulta a rede e grava o cache.
	latest, newer := CheckForNotice(context.Background(), c, cache, "0.1.0", now, 24*time.Hour)
	if latest != "0.5.0" || !newer {
		t.Fatalf("primeira checagem = (%q, %v), quer (0.5.0, true)", latest, newer)
	}
	if hits != 1 {
		t.Fatalf("hits = %d, quer 1", hits)
	}

	// Cache fresco: não vai à rede.
	latest, newer = CheckForNotice(context.Background(), c, cache, "0.1.0", now.Add(time.Hour), 24*time.Hour)
	if latest != "0.5.0" || !newer {
		t.Fatalf("checagem em cache = (%q, %v), quer (0.5.0, true)", latest, newer)
	}
	if hits != 1 {
		t.Fatalf("cache fresco não deveria ir à rede (hits = %d)", hits)
	}

	// Cache vencido: consulta de novo.
	_, _ = CheckForNotice(context.Background(), c, cache, "0.1.0", now.Add(25*time.Hour), 24*time.Hour)
	if hits != 2 {
		t.Fatalf("cache vencido deveria ir à rede (hits = %d)", hits)
	}

	// Versão atual já é a última: sem aviso.
	_, newer = CheckForNotice(context.Background(), c, cache, "0.5.0", now.Add(26*time.Hour), 24*time.Hour)
	if newer {
		t.Error("não deveria avisar quando já está na última versão")
	}
}

func TestCheckForNoticeFalhaSilenciosa(t *testing.T) {
	c := NewClient(200 * time.Millisecond)
	c.BaseURL = "http://127.0.0.1:1/releases" // porta fechada
	cache := filepath.Join(t.TempDir(), "update-check.json")

	latest, newer := CheckForNotice(context.Background(), c, cache, "0.1.0", time.Now(), 24*time.Hour)
	if latest != "" || newer {
		t.Fatalf("falha de rede deveria resultar em (\"\", false), veio (%q, %v)", latest, newer)
	}
	// A falha também fica em cache (cache negativo) para não insistir offline.
	if _, err := os.Stat(cache); err != nil {
		t.Error("a checagem falha deveria gravar cache negativo")
	}
}
