package project

import (
	"path/filepath"
	"strings"
)

// maxFileNameBytes limita o nome gravado em disco. O limite real da maioria dos
// sistemas de arquivos é 255 bytes; 200 deixa folga para o sufixo de
// desambiguação (" (2)") e para acento, que ocupa 2 bytes em UTF-8.
const maxFileNameBytes = 200

// reservedFileNames são os nomes que o Windows não aceita para arquivo, em
// qualquer extensão. No Linux eles funcionam, mas o projeto pode ser clonado
// noutro sistema — por isso a CLI trata igual nas duas plataformas.
var reservedFileNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
	"COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
	"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// SafeFileName converte um nome vindo do servidor em nome de arquivo local
// válido. O Fluig aceita "/" e ":" na descrição do documento, mas o sistema de
// arquivos não: repassar o nome cru para os.WriteFile faz a gravação apontar
// para um diretório que não existe e falhar. Caso real de 2026-08-17: a
// descrição "… - Cancelado em Nov/24." derrubou um lote de download do GED.
//
// As regras são:
//   - separador e caractere reservado do Windows (/ \ : * ? " < > |) viram "_";
//   - caractere de controle é descartado;
//   - espaço e ponto no fim são cortados (o Windows não aceita);
//   - nome reservado do Windows (CON, NUL, COM1…) ganha "_" na frente;
//   - o resultado é cortado em 200 bytes, preservando a extensão;
//   - nome que sobra vazio devolve "" — quem chama decide o rótulo padrão.
//
// A função NÃO cria diretório e NÃO garante confinamento. Use SafeJoin depois.
func SafeFileName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r < 0x20 || r == 0x7f:
			// Caractere de controle: descarta.
		case strings.ContainsRune(`/\:*?"<>|`, r):
			b.WriteRune('_')
		default:
			b.WriteRune(r)
		}
	}
	out := strings.TrimSpace(b.String())
	out = strings.TrimRight(out, " .")
	out = strings.TrimSpace(out)
	if out == "" {
		return ""
	}
	ext := filepath.Ext(out)
	stem := strings.TrimSuffix(out, ext)
	if reservedFileNames[strings.ToUpper(stem)] {
		out = "_" + out
	}
	return truncateFileName(out)
}

// truncateFileName corta o nome em maxFileNameBytes preservando a extensão e
// sem partir um caractere multibyte no meio.
func truncateFileName(name string) string {
	if len(name) <= maxFileNameBytes {
		return name
	}
	ext := filepath.Ext(name)
	if len(ext) > 20 { // ponto no meio de um nome longo não é extensão
		ext = ""
	}
	stem := []rune(strings.TrimSuffix(name, ext))
	for len(stem) > 0 && len(string(stem))+len(ext) > maxFileNameBytes {
		stem = stem[:len(stem)-1]
	}
	out := strings.TrimRight(strings.TrimSpace(string(stem)), " .")
	if out == "" {
		return strings.TrimPrefix(ext, ".")
	}
	return out + ext
}
