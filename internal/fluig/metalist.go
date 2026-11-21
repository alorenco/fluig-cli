package fluig

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Listas customizadas do Painel de Controle (metalist). Só existem na API
// legada do portal (/ecm/api/rest/ecm/metaList/...), fora dos swaggers.
// Comportamento medido na homologação em 2026-11-21 (ROADMAP §5.23):
//
//   - findAllListPaginator é defeituoso (totalrecords inventado, páginas que
//     se sobrepõem). findAllList devolve tudo sem paginar.
//   - findAllListRecords devolve todas as linhas, sem paginação. A coluna do
//     tipo Meta Lista (FK) vem com o RÓTULO do registro referenciado.
//     findRecord devolve o id cru da FK.
//   - A escrita aceita lixo sem avisar: coluna inexistente cria registro
//     vazio, data fora de dd/MM/yyyy vira null, FK inexistente é gravada mas
//     lida como null, updateRecord com id inexistente responde 200. Por isso
//     a validação mora aqui, antes do POST.
//   - deleteRecord só remove ids enviados como STRING. Com número, responde
//     "removido" e não remove nada.
//   - Erro de tipo (inteiro inválido, número com vírgula) volta como HTTP 500
//     com texto puro.

const restMetaListBase = "/ecm/api/rest/ecm/metaList/"

// Tipos de coluna (códigos do publisher da tela de listas).
const (
	MetaListTypeText     = 1 // Texto
	MetaListTypeTextArea = 2 // Área de texto
	MetaListTypeInteger  = 3 // Inteiro
	MetaListTypeDate     = 4 // Data (dd/MM/yyyy)
	MetaListTypeNumeric  = 5 // Numérico (ponto decimal)
	MetaListTypeMetaList = 6 // Meta Lista: FK para o registro de outra lista
)

// MetaListTypeName devolve o nome legível do tipo de coluna.
func MetaListTypeName(t int) string {
	switch t {
	case MetaListTypeText:
		return "texto"
	case MetaListTypeTextArea:
		return "area-texto"
	case MetaListTypeInteger:
		return "inteiro"
	case MetaListTypeDate:
		return "data"
	case MetaListTypeNumeric:
		return "numerico"
	case MetaListTypeMetaList:
		return "metalista"
	}
	return strconv.Itoa(t)
}

// MetaListSummary é uma lista na listagem.
type MetaListSummary struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// MetaListColumn é uma coluna da lista. Name é o identificador da coluna no
// banco (columnNameId, em maiúsculas): é a chave dos registros e o nome que
// a escrita usa. Coluna desativada não tem Name (o servidor não a expõe).
type MetaListColumn struct {
	Name        string `json:"name,omitempty"`
	Title       string `json:"title"`
	Seq         int    `json:"seq"`
	Type        int    `json:"type"`
	TypeName    string `json:"typeName"`
	Label       bool   `json:"label"`
	RefListID   int    `json:"refListId,omitempty"`
	RefListName string `json:"refListName,omitempty"`
	Disabled    bool   `json:"disabled"`
}

// MetaList é uma lista com as colunas.
type MetaList struct {
	ID          int              `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Columns     []MetaListColumn `json:"columns"`

	// FieldsUnavailable marca a lista que o servidor devolveu sem fieldsVO.
	// As colunas vieram só do findColumnNames (sem as desativadas, sem
	// rótulo). Editar uma lista assim é arriscado.
	FieldsUnavailable bool `json:"-"`
}

// MaxSeq devolve a maior sequência de coluna.
func (m *MetaList) MaxSeq() int {
	max := 0
	for _, c := range m.Columns {
		if c.Seq > max {
			max = c.Seq
		}
	}
	return max
}

// ActiveColumns devolve as colunas ativas (com Name), em ordem de sequência.
func (m *MetaList) ActiveColumns() []MetaListColumn {
	out := make([]MetaListColumn, 0, len(m.Columns))
	for _, c := range m.Columns {
		if !c.Disabled && c.Name != "" {
			out = append(out, c)
		}
	}
	return out
}

// MetaListRecord é uma linha da lista. Values usa o Name da coluna como chave;
// o valor é string, json.Number ou nil.
type MetaListRecord struct {
	ID     int            `json:"id"`
	Values map[string]any `json:"values"`
}

// ListMetaLists devolve as listas da empresa, ordenadas por nome.
func (c *Client) ListMetaLists(ctx context.Context) ([]MetaListSummary, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	body, err := c.metaListGet(ctx, "findAllList")
	if err != nil {
		return nil, err
	}
	var raw []struct {
		ListID   int    `json:"listId"`
		ListName string `json:"listName"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("resposta inesperada de findAllList: %w", err)
	}
	out := make([]MetaListSummary, 0, len(raw))
	for _, r := range raw {
		out = append(out, MetaListSummary{ID: r.ListID, Name: r.ListName})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return sortKeyPTBR(out[i].Name) < sortKeyPTBR(out[j].Name)
	})
	return out, nil
}

// foldPTBR tira os acentos do português para a ordenação ("Órgão" fica
// entre "N" e "P", não depois de "z"). Evita a dependência x/text/collate.
var foldPTBR = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a",
	"é", "e", "ê", "e", "è", "e", "ë", "e",
	"í", "i", "î", "i", "ì", "i", "ï", "i",
	"ó", "o", "ô", "o", "õ", "o", "ò", "o", "ö", "o",
	"ú", "u", "û", "u", "ù", "u", "ü", "u",
	"ç", "c", "ñ", "n",
)

func sortKeyPTBR(s string) string {
	return foldPTBR.Replace(strings.ToLower(s))
}

// GetMetaList devolve a lista com as colunas. Junta findListAndFields
// (título, tipo, rótulo, FK, desativada) com findColumnNames (o columnNameId,
// que é a chave real dos registros). A ligação é fieldSeq == seqNum.
// Lista inexistente ou removida responde ErrNotFound.
func (c *Client) GetMetaList(ctx context.Context, id int) (*MetaList, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	// findListById responde null para lista removida (findListAndFields
	// ainda a devolve) e HTTP 500 EJBException para id que nunca existiu.
	op := "findListById/" + strconv.Itoa(id)
	body, status, err := c.doJSON(ctx, http.MethodGet, c.url(restMetaListBase+op), nil)
	if err != nil {
		return nil, err
	}
	if status == http.StatusInternalServerError && strings.Contains(string(body), "EJBException") {
		return nil, fmt.Errorf("%w: lista %d", ErrNotFound, id)
	}
	if status != http.StatusOK {
		return nil, metaListError(op, body, status)
	}
	var head *struct {
		Deleted bool `json:"deleted"`
	}
	if err := json.Unmarshal(body, &head); err != nil {
		return nil, fmt.Errorf("resposta inesperada de findListById: %w", err)
	}
	if head == nil || head.Deleted {
		return nil, fmt.Errorf("%w: lista %d", ErrNotFound, id)
	}

	body, err = c.metaListDo(ctx, http.MethodPost, "findListAndFields/"+strconv.Itoa(id), nil)
	if err != nil {
		return nil, err
	}
	var lf struct {
		Name        *string `json:"name"`
		Description string  `json:"description"`
		ListID      int     `json:"listId"`
		FieldsVO    []struct {
			Title          string `json:"title"`
			Label          bool   `json:"label"`
			Type           int    `json:"type"`
			FieldSeq       int    `json:"fieldSeq"`
			MetalistFKID   int    `json:"metalistFKId"`
			MetalistFKName string `json:"metalistFKName"`
			Disabled       bool   `json:"disabled"`
		} `json:"fieldsVO"`
	}
	if err := json.Unmarshal(body, &lf); err != nil {
		return nil, fmt.Errorf("resposta inesperada de findListAndFields: %w", err)
	}
	if lf.Name == nil || lf.ListID == 0 {
		return nil, fmt.Errorf("%w: lista %d", ErrNotFound, id)
	}

	names, err := c.metaListColumnNames(ctx, id)
	if err != nil {
		return nil, err
	}
	bySeq := make(map[int]metaListColumnName, len(names))
	for _, n := range names {
		bySeq[n.SeqNum] = n
	}

	m := &MetaList{ID: id, Name: *lf.Name, Description: lf.Description}
	for _, f := range lf.FieldsVO {
		col := MetaListColumn{
			Title:       f.Title,
			Seq:         f.FieldSeq,
			Type:        f.Type,
			TypeName:    MetaListTypeName(f.Type),
			Label:       f.Label,
			RefListID:   f.MetalistFKID,
			RefListName: f.MetalistFKName,
			Disabled:    f.Disabled,
		}
		if n, ok := bySeq[f.FieldSeq]; ok {
			col.Name = n.ColumnNameID
		}
		m.Columns = append(m.Columns, col)
	}
	// findListAndFields às vezes volta sem fieldsVO (medido: lista com FK
	// para lista removida). Neste caso, as colunas ativas saem de
	// findColumnNames.
	if len(lf.FieldsVO) == 0 {
		m.FieldsUnavailable = len(names) > 0
		for _, n := range names {
			m.Columns = append(m.Columns, MetaListColumn{
				Name: n.ColumnNameID, Title: n.ColumnName, Seq: n.SeqNum,
				Type: n.Type, TypeName: MetaListTypeName(n.Type), RefListID: n.MetalistFKID,
			})
		}
	}
	sort.SliceStable(m.Columns, func(i, j int) bool { return m.Columns[i].Seq < m.Columns[j].Seq })
	return m, nil
}

type metaListColumnName struct {
	ColumnName   string `json:"columnName"`
	ColumnNameID string `json:"columnNameId"`
	SeqNum       int    `json:"seqNum"`
	Type         int    `json:"type"`
	MetalistFKID int    `json:"metalistFKId"`
}

func (c *Client) metaListColumnNames(ctx context.Context, id int) ([]metaListColumnName, error) {
	body, err := c.metaListGet(ctx, "findColumnNames/"+strconv.Itoa(id))
	if err != nil {
		return nil, err
	}
	var names []metaListColumnName
	if err := json.Unmarshal(body, &names); err != nil {
		return nil, fmt.Errorf("resposta inesperada de findColumnNames: %w", err)
	}
	return names, nil
}

// ListMetaListRecords devolve todas as linhas da lista, em ordem de id. A
// coluna FK traz o rótulo do registro referenciado.
func (c *Client) ListMetaListRecords(ctx context.Context, listID int) ([]MetaListRecord, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	body, err := c.metaListGet(ctx, "findAllListRecords/"+strconv.Itoa(listID))
	if err != nil {
		return nil, err
	}
	rows, err := decodeMetaListRows(body, "findAllListRecords")
	if err != nil {
		return nil, err
	}
	out := make([]MetaListRecord, 0, len(rows))
	for _, row := range rows {
		id, err := metaListRecordID(row["metaListRecordId"])
		if err != nil {
			return nil, fmt.Errorf("resposta inesperada de findAllListRecords: %w", err)
		}
		delete(row, "metaListRecordId")
		out = append(out, MetaListRecord{ID: id, Values: normalizeMetaListValues(row)})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// GetMetaListRecord devolve uma linha. A coluna FK traz o id cru do registro
// referenciado (é o valor que a escrita aceita). Id inexistente: ErrNotFound
// (o servidor responde 500 NullPointerException).
func (c *Client) GetMetaListRecord(ctx context.Context, listID, recordID int) (*MetaListRecord, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	op := fmt.Sprintf("findRecord/%d/%d", listID, recordID)
	body, status, err := c.doJSON(ctx, http.MethodGet, c.url(restMetaListBase+op), nil)
	if err != nil {
		return nil, err
	}
	if status == http.StatusInternalServerError && strings.Contains(string(body), "NullPointerException") {
		return nil, fmt.Errorf("%w: registro %d da lista %d", ErrNotFound, recordID, listID)
	}
	if status != http.StatusOK {
		return nil, metaListError(op, body, status)
	}
	rows, err := decodeMetaListRows(append(append([]byte("["), body...), ']'), "findRecord")
	if err != nil {
		return nil, err
	}
	if len(rows) != 1 || rows[0] == nil {
		return nil, fmt.Errorf("%w: registro %d da lista %d", ErrNotFound, recordID, listID)
	}
	delete(rows[0], "metaListRecordId")
	return &MetaListRecord{ID: recordID, Values: normalizeMetaListValues(rows[0])}, nil
}

// MetaListLabelRecord é uma opção válida para uma coluna FK: o id do
// registro na lista referenciada e os valores das colunas de rótulo.
type MetaListLabelRecord struct {
	ID     int
	Values map[string]any
}

// ListMetaListLabelRecords devolve os registros da lista referenciada com os
// campos de rótulo (findAllListLabelRecords). Serve para validar uma FK.
func (c *Client) ListMetaListLabelRecords(ctx context.Context, listID int) ([]MetaListLabelRecord, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	body, err := c.metaListGet(ctx, "findAllListLabelRecords/"+strconv.Itoa(listID))
	if err != nil {
		return nil, err
	}
	rows, err := decodeMetaListRows(body, "findAllListLabelRecords")
	if err != nil {
		return nil, err
	}
	out := make([]MetaListLabelRecord, 0, len(rows))
	for _, row := range rows {
		// Aqui o campo é "metalistRecordId" (l minúsculo), diferente das
		// outras rotas.
		id, err := metaListRecordID(row["metalistRecordId"])
		if err != nil {
			return nil, fmt.Errorf("resposta inesperada de findAllListLabelRecords: %w", err)
		}
		delete(row, "metalistRecordId")
		out = append(out, MetaListLabelRecord{ID: id, Values: normalizeMetaListValues(row)})
	}
	return out, nil
}

// MetaListValue é um valor já validado e normalizado para a escrita.
type MetaListValue struct {
	Column string // columnNameId
	Value  string
	Type   int
}

// CreateMetaListRecord grava uma linha nova. O servidor não devolve o id:
// quem chama descobre o id pela diferença da listagem (ver CLI).
func (c *Client) CreateMetaListRecord(ctx context.Context, listID int, values []MetaListValue) error {
	if err := c.EnsureSession(ctx); err != nil {
		return err
	}
	payload := map[string]any{
		"dataRecord": metaListDataRecord(values),
		"listId":     strconv.Itoa(listID),
	}
	return c.metaListWrite(ctx, http.MethodPost, "saveNewRecord", payload)
}

// UpdateMetaListRecord atualiza as colunas informadas. As colunas fora de
// values ficam como estão (medido). O servidor responde 200 também para id
// inexistente, então quem chama confere a existência antes.
func (c *Client) UpdateMetaListRecord(ctx context.Context, listID, recordID int, values []MetaListValue) error {
	if err := c.EnsureSession(ctx); err != nil {
		return err
	}
	payload := map[string]any{
		"dataRecord":       metaListDataRecord(values),
		"listId":           strconv.Itoa(listID),
		"metaListRecordId": strconv.Itoa(recordID),
	}
	return c.metaListWrite(ctx, http.MethodPost, "updateRecord", payload)
}

// DeleteMetaListRecords remove as linhas. Os ids vão como STRING: com número
// o servidor responde "removido" e não remove nada (medido). Id inexistente
// é ignorado pelo servidor, então quem chama confere a existência antes.
func (c *Client) DeleteMetaListRecords(ctx context.Context, listID int, recordIDs []int) error {
	if err := c.EnsureSession(ctx); err != nil {
		return err
	}
	ids := make([]string, 0, len(recordIDs))
	for _, id := range recordIDs {
		ids = append(ids, strconv.Itoa(id))
	}
	payload := map[string]any{"recordId": ids, "listId": strconv.Itoa(listID)}
	return c.metaListWrite(ctx, http.MethodDelete, "deleteRecord/", payload)
}

// MetaListNewColumn descreve uma coluna nova (CreateMetaList e
// UpdateMetaList).
type MetaListNewColumn struct {
	Title       string
	Type        int
	Label       bool
	RefListID   int    // só para MetaListTypeMetaList
	RefListName string // só para MetaListTypeMetaList
}

// MetaListColumnChange altera uma coluna existente. A API aceita mudar título
// e tipo, mas isso corrompe a lista (medido: coluna duplicada no banco e
// dado perdido). Por isso só rótulo e desativada mudam.
type MetaListColumnChange struct {
	Column   MetaListColumn // estado atual (de GetMetaList)
	Label    bool
	Disabled bool
}

// CreateMetaList cria uma lista (saveNewList). O servidor responde {} sem o
// id: quem chama acha a lista nova pelo nome (o nome pode repetir).
// ⚠️ Não é atômica em erro de DDL: um nome de coluna que o SQL recusa (ex.:
// com "/") responde 500 e deixa a lista criada e quebrada. Valide os títulos
// com MetaListColumnName antes.
func (c *Client) CreateMetaList(ctx context.Context, name, description string, columns []MetaListNewColumn) error {
	if err := c.EnsureSession(ctx); err != nil {
		return err
	}
	payload := map[string]any{"name": name, "description": description, "fieldsVO": metaListNewFields(columns, 0)}
	return c.metaListWrite(ctx, http.MethodPost, "saveNewList", payload)
}

// UpdateMetaList envia o updateList. O fieldsVO é parcial: as colunas fora
// de changes e added ficam como estão (medido).
// ⚠️ Não é atômica: com uma coluna recusada (palavra reservada), o nome e a
// descrição mudam mesmo assim. Quem chama separa as duas coisas: primeiro as
// colunas com o nome atual, depois o nome e a descrição sem colunas.
// ⚠️ Reativa uma lista removida: confira a existência antes.
// ⚠️ Coluna nova com nome igual ao de outra coluna é ignorada em silêncio.
func (c *Client) UpdateMetaList(ctx context.Context, id int, name, description string, changes []MetaListColumnChange, added []MetaListNewColumn, maxSeq int) error {
	if err := c.EnsureSession(ctx); err != nil {
		return err
	}
	fields := make([]map[string]any, 0, len(changes)+len(added))
	for _, ch := range changes {
		col := ch.Column
		var fkName any
		if col.RefListName != "" {
			fkName = col.RefListName
		}
		fields = append(fields, map[string]any{
			"title": col.Title, "label": ch.Label, "type": col.Type, "idRow": col.Seq, "fieldSeq": col.Seq,
			"metalistFKId": col.RefListID, "metalistFKName": fkName, "disabled": ch.Disabled, "newField": false,
		})
	}
	fields = append(fields, metaListNewFields(added, maxSeq)...)
	payload := map[string]any{"name": name, "description": description, "listId": id, "fieldsVO": fields}
	return c.metaListWrite(ctx, http.MethodPost, "updateList", payload)
}

func metaListNewFields(columns []MetaListNewColumn, maxSeq int) []map[string]any {
	fields := make([]map[string]any, 0, len(columns))
	for i, col := range columns {
		var fkName any
		if col.RefListName != "" {
			fkName = col.RefListName
		}
		fields = append(fields, map[string]any{
			"fieldSeq": -1, "idRow": maxSeq + i + 1, "title": col.Title, "type": strconv.Itoa(col.Type),
			"label": col.Label, "metalistFKName": fkName, "metalistFKId": col.RefListID,
			"disabled": false, "newField": true,
		})
	}
	return fields
}

// RemoveMetaList remove a lista (removeList). A remoção é lógica, e uma lista
// referenciada por FK de outra lista não pode ser removida (medido: mesmo
// depois de remover a lista que a referencia).
func (c *Client) RemoveMetaList(ctx context.Context, id int) error {
	if err := c.EnsureSession(ctx); err != nil {
		return err
	}
	payload := map[string]any{"listId": []string{strconv.Itoa(id)}}
	return c.metaListWrite(ctx, http.MethodDelete, "removeList/", payload)
}

// MetaListColumnName calcula o nome da coluna (columnNameId) que o servidor
// gera a partir do título. Regra medida em 2026-11-21: maiúsculas, sem
// acento, dígitos do início removidos, cada espaço vira "_", "-", "º" e "ª"
// somem. Outros símbolos são recusados aqui: o servidor manteria alguns
// (ex.: "/") no nome e o SQL quebraria a lista. O nome tem até 30
// caracteres. Palavras reservadas (ID, DATE, USER, VALUE, ORDER, SELECT...)
// são recusadas pelo servidor antes de gravar.
func MetaListColumnName(title string) (string, error) {
	var b strings.Builder
	for _, r := range foldPTBR.Replace(strings.ToLower(title)) {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r - 'a' + 'A')
		case r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('_')
		case r == '-', r == 'º', r == 'ª':
			// o servidor remove
		default:
			return "", fmt.Errorf("o título %q tem o caractere %q. Use só letras, dígitos, espaço, _ e -", title, r)
		}
	}
	name := strings.TrimLeft(b.String(), "0123456789")
	if strings.Trim(name, "_") == "" {
		return "", fmt.Errorf("o título %q não tem letras para formar o nome da coluna", title)
	}
	if len(name) > 30 {
		return "", fmt.Errorf("o título %q gera o nome %s, com mais de 30 caracteres", title, name)
	}
	return name, nil
}

// ParseMetaListType aceita o nome do tipo (texto, area-texto, inteiro, data,
// numerico, metalista) ou o código 1–6.
func ParseMetaListType(s string) (int, error) {
	v := foldPTBR.Replace(strings.ToLower(strings.TrimSpace(s)))
	switch v {
	case "texto", "text", "1":
		return MetaListTypeText, nil
	case "area-texto", "area", "areatexto", "textarea", "2":
		return MetaListTypeTextArea, nil
	case "inteiro", "integer", "int", "3":
		return MetaListTypeInteger, nil
	case "data", "date", "4":
		return MetaListTypeDate, nil
	case "numerico", "numeric", "decimal", "5":
		return MetaListTypeNumeric, nil
	case "metalista", "metalist", "lista", "6":
		return MetaListTypeMetaList, nil
	}
	return 0, fmt.Errorf("tipo %q inválido. Use texto, area-texto, inteiro, data, numerico ou metalista", s)
}

func metaListDataRecord(values []MetaListValue) []map[string]any {
	out := make([]map[string]any, 0, len(values))
	for _, v := range values {
		out = append(out, map[string]any{"columnNameId": v.Column, "value": v.Value, "type": v.Type})
	}
	return out
}

func (c *Client) metaListGet(ctx context.Context, op string) ([]byte, error) {
	return c.metaListDo(ctx, http.MethodGet, op, nil)
}

// metaListDo chama a rota e exige 200. O Content-Type leva charset=UTF-8,
// como o navegador envia: os valores costumam ter acento.
func (c *Client) metaListDo(ctx context.Context, method, op string, payload []byte) ([]byte, error) {
	var (
		body   []byte
		status int
		err    error
	)
	if payload == nil && method == http.MethodGet {
		body, status, err = c.doJSON(ctx, method, c.url(restMetaListBase+op), nil)
	} else {
		body, status, err = c.doWithContentType(ctx, method, c.url(restMetaListBase+op), payload, "application/json; charset=UTF-8")
	}
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, metaListError(op, body, status)
	}
	return body, nil
}

func (c *Client) metaListWrite(ctx context.Context, method, op string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = c.metaListDo(ctx, method, op, data)
	return err
}

// metaListError traduz a falha da rota. As rejeições de negócio vêm como HTTP
// 500 com texto puro (ex.: "Falha ao atualizar o registro. Verifique o tipo
// de dado..."). Elas viram errServerRejected com a frase do servidor.
func metaListError(op string, body []byte, status int) error {
	text := strings.TrimSpace(string(body))
	if status == http.StatusInternalServerError && text != "" && !strings.HasPrefix(text, "{") && !strings.HasPrefix(text, "<") &&
		!strings.HasPrefix(text, "java.") {
		return fmt.Errorf("%w: %s", errServerRejected, truncate(text, 512))
	}
	return &HTTPError{StatusCode: status, URL: op, Body: truncate(text, 512)}
}

func decodeMetaListRows(body []byte, op string) ([]map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var rows []map[string]any
	if err := dec.Decode(&rows); err != nil {
		return nil, fmt.Errorf("resposta inesperada de %s: %w", op, err)
	}
	return rows, nil
}

func metaListRecordID(v any) (int, error) {
	n, ok := v.(json.Number)
	if !ok {
		return 0, fmt.Errorf("id de registro ausente")
	}
	id, err := strconv.Atoi(n.String())
	if err != nil {
		return 0, fmt.Errorf("id de registro inválido %q", n.String())
	}
	return id, nil
}

// normalizeMetaListValues tira os zeros à direita do numérico
// (12.50000000000 → 12.5). O valor segue json.Number.
func normalizeMetaListValues(row map[string]any) map[string]any {
	for k, v := range row {
		if n, ok := v.(json.Number); ok {
			row[k] = json.Number(trimDecimal(n.String()))
		}
	}
	return row
}

func trimDecimal(s string) string {
	if !strings.Contains(s, ".") || strings.ContainsAny(s, "eE") {
		return s
	}
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}

var (
	reMetaListInt     = regexp.MustCompile(`^-?\d{1,10}$`)
	reMetaListNumeric = regexp.MustCompile(`^-?\d+(\.\d+)?$`)
)

// NormalizeMetaListValue valida o valor pelo tipo da coluna e o devolve no
// formato que o servidor grava sem perda. Valor vazio limpa a coluna.
//   - Inteiro: só dígitos (até 10).
//   - Numérico: aceita vírgula decimal e troca por ponto (o servidor dá 500
//     com vírgula).
//   - Data: aceita dd/MM/yyyy ou yyyy-MM-dd e devolve dd/MM/yyyy (o servidor
//     grava null, sem erro, para qualquer outro formato).
//   - Meta Lista: id numérico do registro na lista referenciada (a existência
//     é conferida por quem chama).
func NormalizeMetaListValue(colType int, value string) (string, error) {
	v := strings.TrimSpace(value)
	if v == "" {
		return "", nil
	}
	switch colType {
	case MetaListTypeInteger:
		if !reMetaListInt.MatchString(v) {
			return "", fmt.Errorf("valor %q não é um inteiro", value)
		}
		return v, nil
	case MetaListTypeNumeric:
		v = strings.Replace(v, ",", ".", 1)
		if !reMetaListNumeric.MatchString(v) {
			return "", fmt.Errorf("valor %q não é um número (use 12.5 ou 12,5)", value)
		}
		return v, nil
	case MetaListTypeDate:
		for _, layout := range []string{"02/01/2006", "2006-01-02"} {
			if t, err := time.Parse(layout, v); err == nil {
				return t.Format("02/01/2006"), nil
			}
		}
		return "", fmt.Errorf("valor %q não é uma data (use dd/mm/aaaa ou aaaa-mm-dd)", value)
	case MetaListTypeMetaList:
		if _, err := strconv.Atoi(v); err != nil {
			return "", fmt.Errorf("valor %q não é o id de um registro da lista referenciada", value)
		}
		return v, nil
	}
	// Texto e área de texto: o valor vai como veio (sem trim).
	return value, nil
}
