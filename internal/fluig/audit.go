package fluig

import (
	"context"
	"strconv"
	"time"
)

// AuditDocument é um documento GED criado por um usuário, como aparece na
// auditoria de atuação (`user audit`). Vem do dataset builtin `document`.
type AuditDocument struct {
	DocumentID  int64      `json:"documentId"`
	Version     int        `json:"version"`
	Description string     `json:"description"`
	Type        string     `json:"type"`      // documentType do GED (5, 7, ...)
	TypeLabel   string     `json:"typeLabel"` // rótulo legível (Anexo de processo, Registro de formulário...)
	CreatedAt   *time.Time `json:"createdAt,omitempty"`
	Deleted     bool       `json:"deleted"`
}

// documentTypeLabel traduz o documentType numérico do dataset `document` para o
// rótulo legível que o próprio Fluig usa no GET /v2/documents/{id} (validado na
// produção 2026-07-22: 5=Card, 7=WorkflowAttachment). Os demais vêm da
// convenção do GED. Desconhecido → "tipo N".
func documentTypeLabel(t string) string {
	switch t {
	case "0", "1":
		return "Pasta"
	case "2":
		return "Arquivo"
	case "5":
		return "Registro de formulário"
	case "7":
		return "Anexo de processo"
	case "8":
		return "Artigo"
	case "":
		return ""
	default:
		return "tipo " + t
	}
}

// auditDocPageSize é o tamanho de página da varredura do dataset `document`.
const auditDocPageSize = 500

// auditDocMaxScan limita a varredura para não percorrer o histórico inteiro de
// um autor de altíssimo volume caso o intervalo comece muito no passado —
// guarda de runaway, folgada (ordenando por data DESC com parada antecipada, um
// intervalo normal fica muito abaixo disto).
const auditDocMaxScan = 20000

// DocumentsCreatedBy lista os documentos GED criados por um usuário (userCode)
// no intervalo [from, to] (comparado por DATA de calendário, em UTC — o
// `createDate` do dataset `document` é epoch millis à meia-noite UTC do dia).
//
// Estratégia (validada na produção em 2026-07-22): o `document` só aceita
// constraint no autor (`colleagueId`); `createDate` NÃO é pesquisável (constraint
// nele devolve linha vazia). Então ordenamos por `createDate` DESC e paginamos
// com PARADA ANTECIPADA ao cruzar o início do intervalo — puxar tudo de um autor
// de alto volume estoura o timeout. Dedup por documentId (um doc com várias
// versões apareceria repetido).
func (c *Client) DocumentsCreatedBy(ctx context.Context, userCode string, from, to time.Time) ([]AuditDocument, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	fromDay := dayUTC(from)
	toDay := dayUTC(to)

	var out []AuditDocument
	seen := map[int64]bool{}
	scanned := 0
	for offset := 0; offset < auditDocMaxScan; offset += auditDocPageSize {
		res, err := c.QueryDataset(ctx, "document", DatasetQuery{
			Fields: []string{
				"documentPK.documentId", "documentPK.version", "documentDescription",
				"documentType", "createDate", "deleted",
			},
			Constraints: []DatasetConstraint{{Field: "colleagueId", Initial: userCode, Final: userCode}},
			OrderBy:     "createDate_DESC",
			Limit:       auditDocPageSize,
			Offset:      offset,
		})
		if err != nil {
			// Autor sem nenhum documento: o dataset responde vazio (não erro);
			// ErrNotFound aqui significa consulta inválida — propaga.
			return nil, err
		}
		if len(res.Rows) == 0 {
			break
		}
		stop := false
		for _, row := range res.Rows {
			scanned++
			created := epochMillisToTime(strDeref(row["createDate"]))
			if created == nil {
				continue
			}
			cd := dayUTC(*created)
			if cd.After(toDay) {
				continue // mais novo que o intervalo — ainda não chegamos
			}
			if cd.Before(fromDay) {
				stop = true // ordenado DESC: daqui p/ trás é tudo mais antigo
				break
			}
			id, _ := strconv.ParseInt(strDeref(row["documentPK.documentId"]), 10, 64)
			if id == 0 || seen[id] {
				continue
			}
			seen[id] = true
			ver, _ := strconv.Atoi(strDeref(row["documentPK.version"]))
			dtype := strDeref(row["documentType"])
			out = append(out, AuditDocument{
				DocumentID:  id,
				Version:     ver,
				Description: strDeref(row["documentDescription"]),
				Type:        dtype,
				TypeLabel:   documentTypeLabel(dtype),
				CreatedAt:   created,
				Deleted:     strDeref(row["deleted"]) == "true",
			})
		}
		if stop || len(res.Rows) < auditDocPageSize {
			break
		}
	}
	return out, nil
}

// dayUTC trunca um instante para a meia-noite UTC do seu dia de calendário.
func dayUTC(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

// epochMillisToTime converte "1783036800000" (epoch millis) em *time.Time UTC.
func epochMillisToTime(s string) *time.Time {
	if s == "" {
		return nil
	}
	ms, err := strconv.ParseInt(s, 10, 64)
	if err != nil || ms <= 0 {
		return nil
	}
	t := time.UnixMilli(ms).UTC()
	return &t
}

// strDeref devolve o valor de um *string (nil → "").
func strDeref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
