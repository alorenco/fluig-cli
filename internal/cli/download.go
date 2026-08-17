package cli

import (
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alorenco/fluig-cli/internal/output"
	"github.com/alorenco/fluig-cli/internal/project"
)

// fileResult é o item de results[] dos comandos que gravam arquivo. Estende o
// itemResult com o que quem automatiza precisa para casar o item PEDIDO com o
// arquivo GRAVADO: documentId/sequence, fileName e path. Relato de 2026-08-17,
// num lote de 2.382 downloads do GED: o campo id trazia o nome do arquivo, não
// o id pedido, e não havia como fazer o mapa id → arquivo.
type fileResult struct {
	ID         string `json:"id"`
	Action     string `json:"action"`
	Success    bool   `json:"success"`
	Error      string `json:"error,omitempty"`
	DocumentID int    `json:"documentId,omitempty"`
	Sequence   int    `json:"sequence,omitempty"`
	FileName   string `json:"fileName,omitempty"`
	Path       string `json:"path,omitempty"`
}

// mimeExtensions fixa a extensão dos tipos comuns. A tabela existe porque
// mime.ExtensionsByType devolve uma LISTA e a primeira opção nem sempre é a
// esperada (image/jpeg começa por ".jfif"), e porque o resultado varia com o
// /etc/mime.types da máquina — a CLI precisa ser determinística.
var mimeExtensions = map[string]string{
	"application/pdf":               ".pdf",
	"application/json":              ".json",
	"application/rtf":               ".rtf",
	"application/xml":               ".xml",
	"application/zip":               ".zip",
	"application/msword":            ".doc",
	"application/vnd.ms-excel":      ".xls",
	"application/vnd.ms-powerpoint": ".ppt",
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document":   ".docx",
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         ".xlsx",
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": ".pptx",
	"image/bmp":     ".bmp",
	"image/gif":     ".gif",
	"image/jpeg":    ".jpg",
	"image/png":     ".png",
	"image/svg+xml": ".svg",
	"image/tiff":    ".tif",
	"text/csv":      ".csv",
	"text/html":     ".html",
	"text/plain":    ".txt",
	"text/xml":      ".xml",
}

// extensionForMime devolve a extensão do mime type, ou "" quando o tipo é
// genérico (application/octet-stream) ou desconhecido.
func extensionForMime(mimeType string) string {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	if mimeType == "" || mimeType == "application/octet-stream" {
		return ""
	}
	if ext, ok := mimeExtensions[mimeType]; ok {
		return ext
	}
	exts, err := mime.ExtensionsByType(mimeType)
	if err != nil || len(exts) == 0 {
		return ""
	}
	sort.Strings(exts)
	return exts[0]
}

// hasFileExtension diz se o nome JÁ termina em extensão. O critério é estreito
// de propósito: até 5 caracteres alfanuméricos com pelo menos uma letra. Assim
// "relatorio.pdf" conta como extensão e "Contrato 12.056" não — a descrição do
// documento no Fluig costuma ter número com ponto no fim.
func hasFileExtension(name string) bool {
	ext := strings.TrimPrefix(filepath.Ext(name), ".")
	if ext == "" || len(ext) > 5 {
		return false
	}
	letra := false
	for _, r := range ext {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
			letra = true
		case r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return letra
}

// ensureFileExtension completa a extensão pelo mime type quando o nome não tem
// uma. Sem isso, um PDF cuja descrição não termina em ".pdf" era gravado sem
// extensão e não abria com dois cliques (relato de 2026-08-17).
func ensureFileExtension(name, mimeType string) string {
	if name == "" || hasFileExtension(name) {
		return name
	}
	return name + extensionForMime(mimeType)
}

// nameGuard evita que dois itens DA MESMA execução gravem no mesmo arquivo. O
// segundo ganha sufixo " (2)" antes da extensão. Arquivo de execução ANTERIOR
// continua sendo sobrescrito — repetir o mesmo comando não empilha cópias.
// Motivo: no GED a descrição se repete, e um documento apagava o outro em
// silêncio, com success:true nos dois (relato de 2026-08-17).
type nameGuard map[string]bool

// unique devolve um caminho ainda não usado nesta execução. A comparação ignora
// caixa, porque no Windows e no macOS "X.pdf" e "x.pdf" são o mesmo arquivo.
func (g nameGuard) unique(path string) string {
	if g == nil {
		return path
	}
	key := strings.ToLower(path)
	if !g[key] {
		g[key] = true
		return path
	}
	ext := filepath.Ext(path)
	stem := strings.TrimSuffix(path, ext)
	for n := 2; ; n++ {
		cand := fmt.Sprintf("%s (%d)%s", stem, n, ext)
		key := strings.ToLower(cand)
		if !g[key] {
			g[key] = true
			return cand
		}
	}
}

// writeDownloadFile grava o arquivo baixado, criando a pasta intermediária
// quando o nome tem subpasta. A falha vira LOCAL_IO_ERROR (exit 1): o servidor
// respondeu bem e quem falhou foi o disco. Antes saía como SERVER_ERROR
// (exit 5) e fazia quem automatiza gastar retentativas num erro que nunca ia
// passar (relato de 2026-08-17).
func writeDownloadFile(path string, content []byte) error {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return output.LocalIOf("não foi possível criar o diretório %s: %v", dir, err).WithCause(err)
		}
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return output.LocalIOf("não foi possível gravar %s: %v", path, err).WithCause(err)
	}
	return nil
}

// absOrSame devolve o caminho absoluto, ou o próprio caminho se a conversão
// falhar (o path do results[] é informativo, não vale abortar por isso).
func absOrSame(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

// nameTemplateHelp lista os marcadores aceitos pelo --name-template. Serve ao
// help do comando e às mensagens de erro, para não divergirem.
const nameTemplateHelp = "{id}, {name}, {ext}, {fileName}"

// resolveDownloadName decide o nome local do arquivo. A ordem de preferência é
// nome do arquivo físico (Content-Disposition) > descrição do documento > id.
// O nome escolhido é saneado, ganha a extensão do mime type se não tiver uma e,
// com --name-template, entra nos marcadores do template.
func resolveDownloadName(tmpl string, id int, physical, description, mimeType string) (string, error) {
	base := project.SafeFileName(physical)
	if base == "" {
		base = project.SafeFileName(description)
	}
	if base == "" {
		base = fmt.Sprintf("documento_%d", id)
	}
	base = ensureFileExtension(base, mimeType)
	if tmpl == "" {
		return base, nil
	}
	ext := filepath.Ext(base)
	return expandNameTemplate(tmpl, map[string]string{
		"id":       fmt.Sprintf("%d", id),
		"name":     strings.TrimSuffix(base, ext),
		"ext":      ext,
		"fileName": base,
	})
}

// expandNameTemplate troca os marcadores pelos valores do documento. Marcador
// desconhecido é erro de uso: gravar "{foo}" no nome do arquivo seria pior.
// A "/" do próprio template é preservada, então o template pode criar subpasta
// (ex.: --name-template "{id}/{fileName}"). Os VALORES já vêm saneados, logo
// nenhum deles injeta separador.
func expandNameTemplate(tmpl string, values map[string]string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(tmpl); {
		if tmpl[i] != '{' {
			b.WriteByte(tmpl[i])
			i++
			continue
		}
		end := strings.IndexByte(tmpl[i:], '}')
		if end < 0 {
			return "", output.Usagef("--name-template tem { sem } em %q; marcadores aceitos: %s", tmpl, nameTemplateHelp)
		}
		key := tmpl[i+1 : i+end]
		v, ok := values[key]
		if !ok {
			return "", output.Usagef("--name-template não conhece o marcador {%s}; marcadores aceitos: %s", key, nameTemplateHelp)
		}
		b.WriteString(v)
		i += end + 1
	}
	out := strings.TrimSpace(b.String())
	if base := filepath.Base(filepath.FromSlash(out)); out == "" || base == "." || base == string(filepath.Separator) {
		return "", output.Usagef("--name-template %q resultou em nome de arquivo vazio", tmpl)
	}
	return out, nil
}
