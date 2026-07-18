# Fixtures de teste

As fixtures devem ser **gravadas do servidor de homologação real**, com dados
sanitizados (nomes, e-mails e logins trocados por valores fictícios, listas internas
reduzidas — a estrutura e os tipos são os reais).

| Arquivo | Origem | Status |
|---|---|---|
| `findUserByLogin.json` | `GET /portal/api/rest/wcmservice/rest/user/findUserByLogin` | ✅ gravada da homologação (Fluig 1.8.2) em 2026-07-07, sanitizada |
| `ECMDatasetService.wsdl` | `GET /webdesk/ECMDatasetService?wsdl` | ✅ capturado da homologação (referência de estrutura RPC/literal) |
| `soap_findAllDatasets.xml` | resposta de `findAllFormulariesDatasets` | ✅ formato confirmado na integração (341 datasets reais parseados em 2026-07-07) |
| ~~`soap_getDataset.xml`~~ | resposta de `getDataset` | ❌ removida em 2026-07-09: a operação SOAP respondia EOF na homologação (nunca foi confirmada); o `dataset query` migrou para a REST v2 |
| `soap_fault.xml` | `soap:Fault` genérico | sintética (formato SOAP padrão) |
| `loadDataset.json` | `GET .../dataset/loadDataset` | ✅ estrutura confirmada na integração (ciclo create→update→reload) |

> O teste `TestIntegrationDatasetCycle` (`-tags=integration`) rodou contra a
> homologação e validou list + create/update/reload. Descoberta: loadDataset de
> dataset inexistente responde HTTP 500 (não 404).

## Fase 2 — Eventos globais

| Arquivo | Origem | Status |
|---|---|---|
| `getEventList.json` | `GET .../globalevent/getEventList` | ✅ formato confirmado na integração (2026-07-07); saveEventList (Content-Type form) e o merge também validados no ciclo de escrita |

> O ciclo de escrita da integração (`TestIntegrationGlobalEventWriteCycle`) só roda
> com `FLUIGCLI_TEST_EVENTS_WRITE=1` e trava se a lista vier vazia — proteção contra
> apagar os eventos globais reais (saveEventList substitui o conjunto inteiro).

## Fase 3 — Mecanismos de atribuição

| Arquivo | Origem | Status |
|---|---|---|
| `getMechanismList.json` | `GET .../mechanism/getCustomAttributionMechanismList` | ✅ formato confirmado na integração (2026-07-07): código em `attributionMecanismDescription`, `assignmentType`=1, `controlClass`=CustomAssignmentImpl |

## Fase 4 — Formulários (ECMCardIndexService)

| Arquivo | Origem | Status |
|---|---|---|
| `ECMCardIndexService.wsdl` | `GET /webdesk/ECMCardIndexService?wsdl` | ✅ capturado da homologação |
| `soap_listForms.xml` | `getCardIndexesWithoutApprover` | ⚠️ construída do WSDL — confirmar no gate |
| `soap_attachmentsList.xml` | `getAttachmentsList` | ⚠️ construída do WSDL |
| `soap_cardContent.xml` | `getCardIndexContent` (base64) | ⚠️ construída do WSDL |
| `soap_customEvents.xml` | `getCustomizationEvents` | ⚠️ construída do WSDL |
| `soap_writeForm.xml` | create/update (`webServiceMessage`) | ⚠️ construída do WSDL |

## Fase 5 — Scripts de processo (workflow)

| Arquivo | Origem | Status |
|---|---|---|
| `ECMWorkflowEngineService.wsdl` | `GET /webdesk/ECMWorkflowEngineService?wsdl` | ✅ capturado; `workflow version` nativo validado (v57) |

> Descoberta: nem SOAP nem a REST v2 (`/process-management`) têm endpoint de
> script/evento — só processo inteiro. Update cirúrgico só via componente
> auxiliar (fluigcliHelper; a fluiggersWidget da comunidade também serve).
> Ver a referência das APIs nativas no CLAUDE.md.

## workflow list (ROADMAP 2026-07-09)

| Arquivo | Origem | Status |
|---|---|---|
| `rest_processes_page1.json` / `page2.json` | `GET /process-management/api/v2/processes` | ✅ gravadas da homologação (Voyager 2.0.0) em 2026-07-09, sanitizadas; envelope `{items, hasNext}` |

> Descobertas: processo sem categoria vem **sem a chave** `categoryId` (ex.:
> FLUIGADHOCPROCESS); o parâmetro `fields` devolve itens vazios neste endpoint;
> `expand=versions` infla a resposta ~25× (13 KB → 334 KB em 31 processos).

## workflow publish (ROADMAP 2026-07-09)

| Arquivo | Origem | Status |
|---|---|---|
| `rest_process_export.xml` | `GET /process-management/api/v2/processes/{id}/export/xml` | ✅ gravada da homologação em 2026-07-09 (processo de teste zz_fluigcli_test_pub, criado e apagado durante a investigação) |

> Descobertas (ciclo validado com processos `zz_fluigcli_test_*`, removidos ao
> final): o export REST vem em **UTF-8**, raiz `<list>`, só a última versão
> (≠ zip SOAP, ISO-8859-1); **todo import cria versão nova em edição** (as PKs
> do corpo são renumeradas); `release=true` no import **não é atômico** (a
> versão fica criada mesmo se a liberação falhar) — por isso o publish libera
> pelo endpoint dedicado; o release desativa a versão anterior e o withdraw
> reverte, mas **withdraw exige que a versão anterior tenha histórico de
> liberação** (senão HTTP 500 EJBTransactionRolledback — beco sem saída via
> REST); `DELETE /processes/{id}` exige uma única versão não liberada
> (apagar `process-versions/latest` uma a uma antes).

## Fase 6 — Widgets

Sem fixtures novas (empacotamento/desempacotamento do WAR é testado com zips
sintéticos in-memory). Investigação: APIs nativas de widget (`/wcm/api/v2/widgets`,
`/api/public/wcm/widget`) respondem `NotFoundException`; listagem/download só via
`GET /<helper>/api/widgets[/{filename}]` (fluigcliHelper ou fluiggersWidget).
Export/deploy é nativo (uploadfile). ⚠️ O download exige Accept ≠ application/json
(406 do RESTEasy — visto ao vivo em 2026-07-18 nos dois helpers).

## dataset REST v2 (ROADMAP 2026-07-09)

| Arquivo | Origem | Status |
|---|---|---|
| `rest_datasets_page1.json` / `page2.json` | `GET /dataset/api/v2/datasets` | ✅ gravadas da homologação em 2026-07-09, sanitizadas (exemplares reais de BUILTIN/CUSTOM/GENERATED) |
| `rest_dataset_handle.json` | `GET /dataset/api/v2/dataset-handle/search` | ✅ gravada da homologação em 2026-07-09, valores fictícios |

> Descobertas: o SOAP `getDataset` respondia **EOF** na homologação (o
> `dataset query` estava quebrado — a REST v2 o corrigiu); o handle/search
> aplica **limit default de 300** no servidor (a CLI pagina por offset quando
> `--limit 0`); **um único** `orderby` (dois fazem a resposta vir nula);
> dataset inexistente/consulta inválida responde **200 com columns/values
> null** (dataset vazio responde arrays vazios) → é assim que se detecta o
> "não encontrado". A listagem REST não expõe `version` (o campo saiu do
> contrato do `dataset list`).

## widget list nativo (ROADMAP 2026-07-09)

| Arquivo | Origem | Status |
|---|---|---|
| `rest_applications_page1.json` / `page2.json` | `GET /page-management/api/v2/applications?internal=false` | ✅ gravadas da homologação em 2026-07-09, sanitizadas; envelope `{items, hasNext}` |

> Descobertas: a rota funciona sem o componente auxiliar, mas **omite widgets**
> (3 de 28 na homologação — fluiggersWidget, repositorio, sbi_global — mesmo
> respondendo 200 no `GET applications/{code}`; critério do filtro não
> identificado) e não traz o nome do `.war` (necessário ao `widget import`).
> Por isso é o **fallback** do `widget list`, não a fonte primária.

---

> `TestIntegrationFormListAndDownload` (`-tags=integration`) exercita list +
> download + eventos read-only.
