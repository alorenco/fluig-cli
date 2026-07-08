package update

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DefaultBaseURL é a raiz das releases do projeto no GitHub.
const DefaultBaseURL = "https://github.com/alorenco/fluig-cli/releases"

// Client consulta e baixa releases. BaseURL é parametrizável para testes.
type Client struct {
	HTTP    *http.Client
	BaseURL string
}

func NewClient(timeout time.Duration) *Client {
	return &Client{
		HTTP:    &http.Client{Timeout: timeout},
		BaseURL: DefaultBaseURL,
	}
}

// Latest descobre a última versão publicada ("0.1.0", sem o "v") seguindo o
// redirect de /releases/latest — não usa a API do GitHub, então não sofre
// rate limit.
func (c *Client) Latest(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/latest", nil)
	if err != nil {
		return "", err
	}
	// Não segue o redirect: o destino (…/tag/vX.Y.Z) já diz a versão.
	noRedirect := *c.HTTP
	noRedirect.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := noRedirect.Do(req)
	if err != nil {
		return "", fmt.Errorf("consulta da última release: %w", err)
	}
	defer resp.Body.Close()

	loc := resp.Header.Get("Location")
	_, tag, ok := strings.Cut(loc, "/tag/v")
	if !ok || tag == "" {
		return "", fmt.Errorf("resposta inesperada ao consultar a última release (status %d)", resp.StatusCode)
	}
	return tag, nil
}

// ArchiveName devolve o nome do pacote publicado pelo GoReleaser para a
// plataforma (padrão fluigcli_<versão>_<so>_<arquitetura>).
func ArchiveName(version, goos, goarch string) string {
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("fluigcli_%s_%s_%s%s", version, goos, goarch, ext)
}

// Download baixa o pacote da versão para dir, confere o sha256 contra o
// checksums.txt da release e extrai o binário. Retorna o caminho do binário
// extraído.
func (c *Client) Download(ctx context.Context, version, goos, goarch, dir string) (string, error) {
	name := ArchiveName(version, goos, goarch)
	base := fmt.Sprintf("%s/download/v%s", c.BaseURL, version)

	pkg := filepath.Join(dir, name)
	if err := c.fetch(ctx, base+"/"+name, pkg); err != nil {
		return "", err
	}
	sums := filepath.Join(dir, "checksums.txt")
	if err := c.fetch(ctx, base+"/checksums.txt", sums); err != nil {
		return "", err
	}
	if err := verifyChecksum(pkg, name, sums); err != nil {
		return "", err
	}

	bin := "fluigcli"
	if goos == "windows" {
		bin = "fluigcli.exe"
	}
	dest := filepath.Join(dir, bin)
	var err error
	if goos == "windows" {
		err = extractZip(pkg, bin, dest)
	} else {
		err = extractTarGz(pkg, bin, dest)
	}
	if err != nil {
		return "", err
	}
	return dest, nil
}

func (c *Client) fetch(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("download de %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("não encontrado no servidor: %s (a versão existe?)", url)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download de %s: status %d", url, resp.StatusCode)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("download de %s: %w", url, err)
	}
	return nil
}

// verifyChecksum confere o sha256 do pacote contra a linha correspondente do
// checksums.txt (formato do sha256sum: "<hash>  <arquivo>").
func verifyChecksum(pkg, name, sumsPath string) error {
	raw, err := os.ReadFile(sumsPath)
	if err != nil {
		return err
	}
	var want string
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == name {
			want = fields[0]
			break
		}
	}
	if want == "" {
		return fmt.Errorf("%s não consta no checksums.txt da release", name)
	}

	f, err := os.Open(pkg)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("o checksum de %s não confere — download corrompido", name)
	}
	return nil
}

func extractTarGz(archive, member, dest string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("pacote inválido: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("pacote inválido: %w", err)
		}
		if hdr.Typeflag == tar.TypeReg && filepath.Base(hdr.Name) == member {
			return writeFileFrom(tr, dest)
		}
	}
	return fmt.Errorf("binário %s não encontrado no pacote", member)
}

func extractZip(archive, member, dest string) error {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return fmt.Errorf("pacote inválido: %w", err)
	}
	defer zr.Close()
	for _, zf := range zr.File {
		if filepath.Base(zf.Name) != member || zf.FileInfo().IsDir() {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			return err
		}
		defer rc.Close()
		return writeFileFrom(rc, dest)
	}
	return fmt.Errorf("binário %s não encontrado no pacote", member)
}

func writeFileFrom(r io.Reader, dest string) error {
	f, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, r)
	return err
}
