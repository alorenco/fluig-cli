# Fixtures de teste

As fixtures devem ser **gravadas do servidor de homologação real**, com dados
sanitizados (nomes, e-mails e logins trocados por valores fictícios, listas internas
reduzidas — a estrutura e os tipos são os reais).

| Arquivo | Origem | Status |
|---|---|---|
| `findUserByLogin.json` | `GET /portal/api/rest/wcmservice/rest/user/findUserByLogin` | ✅ gravada da homologação (Fluig 1.8.2) em 2026-07-07, sanitizada |
| `ECMDatasetService.wsdl` | `GET /webdesk/ECMDatasetService?wsdl` | ✅ capturado da homologação (referência de estrutura RPC/literal) |
| `soap_findAllDatasets.xml` | resposta de `findAllFormulariesDatasets` | ✅ formato confirmado na integração (341 datasets reais parseados em 2026-07-07) |
| `soap_getDataset.xml` | resposta de `getDataset` | ⚠️ construída do WSDL — confirmar via `dataset query` no gate |
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
> script/evento — só processo inteiro. Update cirúrgico só via fluiggersWidget.
> Ver a referência das APIs nativas no CLAUDE.md.

## workflow list (ROADMAP 2026-07-09)

| Arquivo | Origem | Status |
|---|---|---|
| `rest_processes_page1.json` / `page2.json` | `GET /process-management/api/v2/processes` | ✅ gravadas da homologação (Voyager 2.0.0) em 2026-07-09, sanitizadas; envelope `{items, hasNext}` |

> Descobertas: processo sem categoria vem **sem a chave** `categoryId` (ex.:
> FLUIGADHOCPROCESS); o parâmetro `fields` devolve itens vazios neste endpoint;
> `expand=versions` infla a resposta ~25× (13 KB → 334 KB em 31 processos).

## Fase 6 — Widgets

Sem fixtures novas (empacotamento/desempacotamento do WAR é testado com zips
sintéticos in-memory). Investigação: APIs nativas de widget (`/wcm/api/v2/widgets`,
`/api/public/wcm/widget`) respondem `NotFoundException`; listagem/download só via
`GET /fluiggersWidget/api/widgets[/{filename}]`. Export/deploy é nativo (uploadfile).

---

> `TestIntegrationFormListAndDownload` (`-tags=integration`) exercita list +
> download + eventos read-only.
