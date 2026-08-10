package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Baseline é o retrato dos achados aceitos do projeto (.fluigcli/audit-baseline.json).
//
// Motivo (ROADMAP §5.5): o gate do audit nos comandos de publish era
// tudo-ou-nada. Num arquivo com dívida antiga, a única saída era `--no-audit`,
// que desliga também a checagem do código NOVO — e foi assim que um bug real
// quase passou (relato de 2026-08-10). Com o baseline, o gate barra só o que
// não estava lá antes. Troca "desliga tudo" por "não deixa piorar".
//
// O arquivo é SEPARADO do `.fluigcli/audit.json` de propósito: aquele é
// configuração escrita à mão e validada; este é snapshot gerado. Misturar os
// dois atrapalharia a revisão no Git.
type Baseline struct {
	Version string          `json:"version"`
	Entries []BaselineEntry `json:"entries"`
}

// BaselineEntry é um achado aceito, com quantas vezes ele aparece.
//
// A chave NÃO usa o número da linha: ele anda a cada edição do arquivo, e um
// baseline que se invalida sozinho não serve para nada. A identidade é o
// arquivo, a regra e o hash do TEXTO da linha apontada (espaços normalizados).
// Count segura o caso de a mesma linha-texto repetir no arquivo: o excedente
// volta a barrar.
type BaselineEntry struct {
	File   string `json:"file"`
	Rule   string `json:"rule"`
	Hash   string `json:"hash"`
	Count  int    `json:"count"`
	Sample string `json:"sample,omitempty"` // trecho da linha, para leitura humana
}

// BaselineVersion é o schema do arquivo.
const BaselineVersion = "1.0.0"

// BaselinePath devolve o caminho do baseline do projeto.
func BaselinePath(root string) string {
	return filepath.Join(root, ".fluigcli", "audit-baseline.json")
}

// LoadBaseline lê o baseline do projeto. Arquivo ausente devolve (nil, nil) —
// projeto sem baseline é o caso normal, não um erro.
func LoadBaseline(root string) (*Baseline, error) {
	raw, err := os.ReadFile(BaselinePath(root))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var b Baseline
	if err := json.Unmarshal(raw, &b); err != nil {
		return nil, fmt.Errorf(".fluigcli/audit-baseline.json inválido: %w", err)
	}
	if b.Version != BaselineVersion {
		return nil, fmt.Errorf(".fluigcli/audit-baseline.json: versão %q desconhecida (esperado %s); regrave com audit --save-baseline",
			b.Version, BaselineVersion)
	}
	return &b, nil
}

// Save grava o baseline (cria .fluigcli/ se preciso).
func (b *Baseline) Save(root string) error {
	path := BaselinePath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// NewBaseline monta o baseline a partir dos achados de uma auditoria.
func NewBaseline(root string, findings []Finding) *Baseline {
	lines := newLineCache(root)
	byKey := map[string]*BaselineEntry{}
	var ordem []string
	for _, f := range findings {
		key, hash, texto := baselineKey(lines, f)
		e, ok := byKey[key]
		if !ok {
			e = &BaselineEntry{File: f.File, Rule: f.Rule, Hash: hash, Sample: sample(texto)}
			byKey[key] = e
			ordem = append(ordem, key)
		}
		e.Count++
	}
	sort.Strings(ordem)
	b := &Baseline{Version: BaselineVersion, Entries: make([]BaselineEntry, 0, len(ordem))}
	for _, k := range ordem {
		b.Entries = append(b.Entries, *byKey[k])
	}
	return b
}

// Partition separa os achados em conhecidos (com par no baseline) e novos, e
// conta quantos achados do baseline sumiram — a dívida quitada.
//
// scannedFiles limita a contagem de resolvidos aos arquivos desta rodada: o
// audit de um arquivo só não pode alegar que os achados dos outros sumiram.
// Nil = a rodada cobriu tudo o que o baseline referencia.
func (b *Baseline) Partition(root string, findings []Finding, scannedFiles []string) (conhecidos, novos []Finding, resolvidos int) {
	restante := map[string]int{}
	if b != nil {
		for _, e := range b.Entries {
			restante[e.File+"|"+e.Rule+"|"+e.Hash] += e.Count
		}
	}
	lines := newLineCache(root)
	for _, f := range findings {
		key, _, _ := baselineKey(lines, f)
		if restante[key] > 0 {
			restante[key]--
			f.Baseline = true
			conhecidos = append(conhecidos, f)
			continue
		}
		novos = append(novos, f)
	}
	if b == nil {
		return conhecidos, novos, 0
	}
	auditado := map[string]bool{}
	for _, f := range scannedFiles {
		auditado[f] = true
	}
	for _, e := range b.Entries {
		if scannedFiles != nil && !auditado[e.File] {
			continue
		}
		resolvidos += restante[e.File+"|"+e.Rule+"|"+e.Hash]
		restante[e.File+"|"+e.Rule+"|"+e.Hash] = 0
	}
	return conhecidos, novos, resolvidos
}

// baselineKey monta a chave do achado e devolve também o hash e o texto da
// linha (para o Sample).
func baselineKey(lines *lineCache, f Finding) (key, hash, texto string) {
	texto = lines.line(f.File, f.Line)
	sum := sha256.Sum256([]byte(normalizaLinha(texto)))
	hash = hex.EncodeToString(sum[:])[:16]
	return f.File + "|" + f.Rule + "|" + hash, hash, texto
}

// normalizaLinha tira indentação e colapsa espaços: reformatar o arquivo não
// pode invalidar o baseline inteiro.
func normalizaLinha(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// sample encurta a linha para caber no arquivo sem virar um despejo de código.
func sample(s string) string {
	s = normalizaLinha(s)
	if len(s) > 120 {
		return s[:117] + "..."
	}
	return s
}

// lineCache lê cada arquivo uma vez só por operação de baseline.
type lineCache struct {
	root  string
	files map[string][]string
}

func newLineCache(root string) *lineCache {
	return &lineCache{root: root, files: map[string][]string{}}
}

func (c *lineCache) line(rel string, n int) string {
	lines, ok := c.files[rel]
	if !ok {
		raw, err := os.ReadFile(filepath.Join(c.root, filepath.FromSlash(rel)))
		if err == nil {
			lines = strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
		}
		c.files[rel] = lines
	}
	if n < 1 || n > len(lines) {
		// Arquivo sumiu ou o achado não aponta linha (regras de processo).
		// A chave fica com o hash do vazio, que ainda distingue por arquivo+regra.
		return ""
	}
	return lines[n-1]
}
