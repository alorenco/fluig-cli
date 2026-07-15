package devserver

// datasetLabHTML é a página do Dataset Lab (rota /_dev/datasets/).
// Self-contained e no mesmo design system do dashboard/índice de formulários
// (claro/escuro, accent ciano). Os dados vêm de /_dev/api/dataset/*.
//
// Sem template literals (backticks) no JS de propósito: a página inteira é uma
// raw string Go delimitada por backticks — o JS usa concatenação de string.
const datasetLabHTML = `<!doctype html>
<html lang="pt-BR">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>fluigcli dev — datasets</title>
<style>
:root{--bg:#f4f6f8;--card:#fff;--txt:#1d2b36;--sub:#5a6b7b;--line:#e3e8ee;
  --accent:#0c9abe;--accent-txt:#fff;--ok:#25b26e;--warn:#b3352b;
  --chipbg:#f7f9fb;--zebra:#f9fbfc;--shadow:0 1px 2px rgba(16,36,54,.08)}
@media(prefers-color-scheme:dark){:root{--bg:#12181f;--card:#1b232d;
  --txt:#e6edf3;--sub:#93a4b4;--line:#2b3742;--chipbg:#161d26;--zebra:#19212b;
  --shadow:0 1px 2px rgba(0,0,0,.4)}}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--txt);
  font:15px/1.5 system-ui,-apple-system,"Segoe UI",Roboto,sans-serif}
header{padding:26px 32px 18px;border-bottom:1px solid var(--line)}
header .hrow{display:flex;align-items:center;justify-content:space-between;gap:14px;flex-wrap:wrap}
header h1{margin:0;font-size:22px;font-weight:650}
header h1 small{color:var(--accent);font-weight:650}
header p{margin:7px 0 0;color:var(--sub);font-size:13.5px}
header .back{display:inline-block;padding:7px 14px;border:1px solid var(--line);border-radius:999px;
  background:var(--card);color:var(--txt);text-decoration:none;font-size:13px;font-weight:600;
  box-shadow:var(--shadow);transition:border-color .08s,color .08s}
header .back:hover{border-color:var(--accent);color:var(--accent)}
main{max-width:1180px;margin:0 auto;padding:22px 32px 48px}

.panel{background:var(--card);border:1px solid var(--line);border-radius:12px;
  padding:16px 18px;box-shadow:var(--shadow)}
button{font:inherit}
.btn{padding:8px 16px;border:0;border-radius:8px;cursor:pointer;
  background:var(--accent);color:var(--accent-txt);font-weight:650;font-size:13.5px;
  transition:filter .08s}
.btn:hover{filter:brightness(1.06)}
.btn:disabled{opacity:.5;cursor:not-allowed}
.btn.sec{background:color-mix(in srgb,var(--txt) 8%,transparent);color:var(--txt)}
.btn.ghost{background:transparent;border:1px solid var(--line);color:var(--txt)}
.btn.ghost:hover{border-color:var(--accent);color:var(--accent);filter:none}
.icobtn{background:transparent;border:1px solid var(--line);color:var(--sub);border-radius:8px;
  width:36px;height:36px;cursor:pointer;font-size:15px;transition:border-color .08s,color .08s}
.icobtn:hover{border-color:var(--accent);color:var(--accent)}

/* barra de consulta */
.querybar{display:flex;gap:10px;align-items:center;flex-wrap:wrap}
.combo{position:relative;flex:1;min-width:280px}
.combo>input{width:100%;padding:10px 14px;border:1px solid var(--line);border-radius:9px;
  background:var(--bg);color:var(--txt);font-size:14.5px;outline:none}
.combo>input:focus{border-color:var(--accent)}
.combo-list{position:absolute;z-index:20;top:calc(100% + 4px);left:0;right:0;max-height:340px;
  overflow:auto;background:var(--card);border:1px solid var(--line);border-radius:10px;
  box-shadow:0 10px 34px rgba(16,36,54,.18);display:none}
.combo-list.open{display:block}
.combo-item{padding:9px 14px;cursor:pointer;display:flex;gap:10px;align-items:center;
  border-bottom:1px solid var(--line)}
.combo-item:last-child{border-bottom:0}
.combo-item:hover,.combo-item.hi{background:color-mix(in srgb,var(--accent) 9%,transparent)}
.combo-item .cid{font:600 13.5px/1.3 ui-monospace,Consolas,monospace}
.combo-item .cdesc{color:var(--sub);font-size:12.5px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;flex:1}
.combo-item .empty{color:var(--sub);padding:10px 4px}
.tbadge{font-size:10.5px;font-weight:700;letter-spacing:.03em;padding:1px 7px;border-radius:5px;
  background:color-mix(in srgb,var(--sub) 16%,transparent);color:var(--sub);white-space:nowrap}
.tbadge.custom{background:color-mix(in srgb,var(--ok) 16%,transparent);color:var(--ok)}
.tbadge.off{background:color-mix(in srgb,var(--warn) 14%,transparent);color:var(--warn)}

/* painel de parâmetros */
.params{margin-top:14px;display:none}
.params.open{display:block}
.params .grp{margin-top:16px}
.params .grp:first-child{margin-top:2px}
.params .grp>label.h,.params .grp>.h{display:block;font-size:12px;font-weight:700;letter-spacing:.05em;
  text-transform:uppercase;color:var(--sub);margin-bottom:8px}
.chips{display:flex;flex-wrap:wrap;gap:7px}
.chip{border:1px solid var(--line);border-radius:999px;padding:5px 12px;font-size:12.5px;cursor:pointer;
  background:var(--chipbg);color:var(--txt);user-select:none;transition:border-color .08s,background .08s}
.chip:hover{border-color:var(--accent)}
.chip.on{border-color:var(--accent);background:color-mix(in srgb,var(--accent) 12%,transparent);
  color:var(--accent);font-weight:600}
.chip.mini{font-size:11.5px;padding:3px 9px}
.rowline{display:flex;gap:10px;align-items:center;flex-wrap:wrap}
.rowline .lbl{font-size:13px;color:var(--sub);min-width:56px}
input[type=text],input[type=number],select{padding:7px 10px;border:1px solid var(--line);
  border-radius:8px;background:var(--bg);color:var(--txt);font-size:13.5px;outline:none}
input[type=text]:focus,input[type=number]:focus,select:focus{border-color:var(--accent)}
input[type=number]{width:100px}
.seg{display:inline-flex;border:1px solid var(--line);border-radius:8px;overflow:hidden}
.seg button{padding:7px 12px;border:0;background:var(--bg);color:var(--sub);cursor:pointer;font-size:12.5px}
.seg button.on{background:var(--accent);color:var(--accent-txt);font-weight:650}
.check{display:inline-flex;gap:6px;align-items:center;font-size:13px;cursor:pointer;color:var(--txt)}

/* filtros */
.filters{display:flex;flex-direction:column;gap:8px}
.filter{display:grid;grid-template-columns:1.3fr 1fr 1fr auto auto auto;gap:8px;align-items:center}
.filter .rm{color:var(--warn);border-color:color-mix(in srgb,var(--warn) 40%,var(--line))}
@media(max-width:760px){.filter{grid-template-columns:1fr 1fr}}
.sqlrow{display:none;margin-top:10px;padding-top:12px;border-top:1px dashed var(--line)}
.sqlrow.open{display:block}

/* resultado */
.result{margin-top:22px}
.resbar{display:flex;gap:10px;align-items:center;flex-wrap:wrap;margin-bottom:10px}
.resbar .stat{font-size:13px;color:var(--sub)}
.resbar .stat b{color:var(--txt)}
.resbar .grow{flex:1}
.resbar .dur{font:12px/1 ui-monospace,Consolas,monospace;color:var(--sub);
  border:1px solid var(--line);border-radius:6px;padding:4px 8px}
.tablewrap{overflow:auto;max-height:66vh;border:1px solid var(--line);border-radius:12px;background:var(--card)}
table{border-collapse:collapse;width:100%;font-size:13px}
thead th{position:sticky;top:0;background:var(--card);text-align:left;font-weight:700;
  padding:9px 12px;border-bottom:2px solid var(--line);white-space:nowrap;z-index:1;
  box-shadow:0 1px 0 var(--line)}
tbody td{padding:7px 12px;border-bottom:1px solid var(--line);vertical-align:top;
  max-width:420px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
tbody td:hover{white-space:normal;word-break:break-word}
tbody tr:nth-child(even){background:var(--zebra)}
tbody td .nul{color:var(--sub);font-style:italic;opacity:.7}
th .rownum,td.rownum{color:var(--sub);font:11.5px/1 ui-monospace,Consolas,monospace;
  text-align:right;user-select:none;background:var(--chipbg)}
.jsonwrap{margin:0;padding:14px 16px;border:1px solid var(--line);border-radius:12px;background:var(--card);
  overflow:auto;max-height:66vh;font:12.5px/1.55 ui-monospace,Consolas,monospace;white-space:pre}
.state{padding:44px 0;text-align:center;color:var(--sub)}
.state.err{color:var(--warn)}
.state code{background:var(--chipbg);border:1px solid var(--line);border-radius:5px;padding:2px 7px}
.note{margin-top:10px;font-size:12.5px;color:var(--sub)}
.note.warn{color:var(--warn)}
.note:empty{display:none}
.spin{display:inline-block;width:14px;height:14px;border:2px solid color-mix(in srgb,var(--accent) 30%,transparent);
  border-top-color:var(--accent);border-radius:50%;animation:sp .7s linear infinite;vertical-align:-2px}
@keyframes sp{to{transform:rotate(360deg)}}
.hidden{display:none!important}
</style>
</head>
<body>
<header>
  <div class="hrow">
    <div>
      <h1>fluigcli <small>dev</small> · Datasets</h1>
      <p>Monte e execute consultas de dataset e veja o resultado — só leitura, direto no servidor conectado pelo proxy.</p>
    </div>
    <a class="back" href="/">← Dashboard</a>
  </div>
</header>
<main>
  <div class="panel">
    <div class="querybar">
      <div class="combo">
        <input id="dsInput" type="text" autocomplete="off" spellcheck="false"
          placeholder="Escolha ou digite o id do dataset…">
        <div class="combo-list" id="dsList"></div>
      </div>
      <button class="btn" id="runBtn" disabled>Consultar</button>
      <button class="btn ghost" id="paramsBtn" aria-expanded="false">Configurar parâmetros</button>
      <button class="icobtn" id="reloadDs" title="Recarregar a lista de datasets">↻</button>
    </div>

    <div class="params" id="params">
      <div class="grp">
        <div class="h">Campos <span class="chip mini" id="fldsAll" style="cursor:pointer">todas</span>
          <span class="chip mini" id="fldsNone" style="cursor:pointer">nenhuma</span></div>
        <div class="chips" id="fields"></div>
        <div class="note" id="fieldsNote"></div>
      </div>

      <div class="grp">
        <div class="rowline">
          <span class="h" style="margin:0">Ordenar</span>
          <select id="orderBy"><option value="">(sem ordenação)</option></select>
          <div class="seg" id="orderDir">
            <button data-dir="asc" class="on">↑ Asc</button>
            <button data-dir="desc">↓ Desc</button>
          </div>
        </div>
      </div>

      <div class="grp">
        <div class="rowline">
          <span class="h" style="margin:0">Limite</span>
          <input type="number" id="limit" min="0" step="50" value="100">
          <span class="note" style="margin:0">linhas no resultado (0 = sem limite; cuidado com datasets grandes)</span>
        </div>
      </div>

      <div class="grp">
        <div class="rowline" style="justify-content:space-between">
          <span class="h" style="margin:0">Filtros</span>
          <span>
            <label class="check" style="margin-right:14px"><input type="checkbox" id="sqlOn"> sqlLimit (datasets SQL)</label>
            <button class="btn sec" id="addFilter">+ Adicionar filtro</button>
          </span>
        </div>
        <div class="sqlrow" id="sqlRow">
          <div class="rowline">
            <span class="lbl">sqlLimit</span>
            <label class="check">inicial <input type="number" id="sqlIni" value="1" style="width:80px"></label>
            <label class="check">final <input type="number" id="sqlFim" value="100" style="width:80px"></label>
            <select id="sqlType"><option value="MUST">Must</option><option value="MUST_NOT">Must Not</option><option value="SHOULD">Should</option></select>
            <label class="check"><input type="checkbox" id="sqlLike"> usa Like</label>
          </div>
        </div>
        <div class="filters" id="filters" style="margin-top:10px"></div>
      </div>
    </div>
  </div>

  <div class="result" id="result">
    <div class="resbar hidden" id="resbar">
      <span class="stat" id="resStat"></span>
      <span class="dur hidden" id="resDur"></span>
      <span class="grow"></span>
      <div class="seg" id="viewSeg">
        <button data-view="table" class="on">Tabela</button>
        <button data-view="json">JSON</button>
      </div>
      <button class="btn ghost" id="copyBtn">Copiar</button>
      <button class="btn ghost" id="csvBtn">Exportar CSV</button>
    </div>
    <div id="resBody">
      <div class="state">Escolha um dataset e clique em <b>Consultar</b>.</div>
    </div>
  </div>
</main>
<script>
(function () {
  "use strict";
  function api(method, path, body, cb) {
    var x = new XMLHttpRequest();
    x.open(method, path, true);
    if (body) x.setRequestHeader("Content-Type", "application/json");
    x.onreadystatechange = function () {
      if (x.readyState !== 4) return;
      var data = null;
      try { data = JSON.parse(x.responseText); } catch (e) {}
      if (x.status >= 200 && x.status < 300) cb(null, data);
      else cb((data && data.error) || ("HTTP " + x.status), data);
    };
    x.send(body ? JSON.stringify(body) : null);
  }
  function el(id) { return document.getElementById(id); }
  function esc(s) { return String(s).replace(/[&<>"]/g, function (c) {
    return { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]; }); }

  var datasets = [];        // {id, description, type, custom, active}
  var dsLoading = true;     // lista de datasets ainda carregando
  var dsError = "";         // falha ao carregar a lista (mensagem)
  var currentId = "";       // dataset selecionado
  var columns = [];         // colunas conhecidas do dataset (da sonda/resultado)
  var selectedFields = {};  // coluna -> bool
  var orderDir = "asc";
  var lastResult = null;    // {columns, rows} do último Consultar

  // ---- persistência (localStorage) ----
  var UIKEY = "fluigcli.datasetlab.ui";
  function cfgKey(id) { return "fluigcli.datasetlab.cfg:" + id; }
  function readJSON(k, dflt) { try { return JSON.parse(localStorage.getItem(k)) || dflt; } catch (e) { return dflt; } }
  function writeJSON(k, v) { try { localStorage.setItem(k, JSON.stringify(v)); } catch (e) {} }
  function ui() { return readJSON(UIKEY, {}); }
  function saveUI(patch) { var u = ui(); for (var k in patch) u[k] = patch[k]; writeJSON(UIKEY, u); }

  // ---- combobox de datasets ----
  var hiIdx = -1;
  function renderList(filter) {
    var box = el("dsList"), f = (filter || "").toLowerCase();
    var items = datasets.filter(function (d) {
      return !f || d.id.toLowerCase().indexOf(f) >= 0 ||
        (d.description || "").toLowerCase().indexOf(f) >= 0;
    });
    box.innerHTML = "";
    hiIdx = -1;
    if (dsLoading) {
      box.innerHTML = '<div class="combo-item"><span class="empty">Carregando datasets do servidor… <span class="spin"></span></span></div>';
    } else if (dsError) {
      box.innerHTML = '<div class="combo-item"><span class="empty" style="color:var(--warn)">Falha ao carregar datasets: ' + esc(dsError) + ' — clique em ↻ para tentar de novo.</span></div>';
    } else if (!datasets.length) {
      box.innerHTML = '<div class="combo-item"><span class="empty">Nenhum dataset encontrado no servidor.</span></div>';
    } else if (!items.length) {
      box.innerHTML = '<div class="combo-item"><span class="empty">Nada casa com o filtro.</span></div>';
    } else {
      items.slice(0, 200).forEach(function (d) {
        var it = document.createElement("div");
        it.className = "combo-item";
        it.setAttribute("data-id", d.id);
        var badge = d.custom ? '<span class="tbadge custom">CUSTOM</span>'
          : '<span class="tbadge">' + esc(d.type || "?") + "</span>";
        if (!d.active) badge += '<span class="tbadge off">inativo</span>';
        it.innerHTML = '<span class="cid">' + esc(d.id) + "</span>" +
          '<span class="cdesc">' + esc(d.description || "") + "</span>" + badge;
        it.addEventListener("mousedown", function (ev) { ev.preventDefault(); choose(d.id); });
        box.appendChild(it);
      });
    }
    box.classList.add("open");
  }
  function closeList() { el("dsList").classList.remove("open"); }
  function moveHi(delta) {
    var its = el("dsList").querySelectorAll(".combo-item[data-id]");
    if (!its.length) return;
    if (hiIdx >= 0 && its[hiIdx]) its[hiIdx].classList.remove("hi");
    hiIdx = (hiIdx + delta + its.length) % its.length;
    its[hiIdx].classList.add("hi");
    its[hiIdx].scrollIntoView({ block: "nearest" });
  }
  function choose(id) {
    currentId = id;
    el("dsInput").value = id;
    closeList();
    el("runBtn").disabled = false;
    saveUI({ lastId: id });
    onSelectDataset(id);
  }

  el("dsInput").addEventListener("input", function () { renderList(this.value); el("runBtn").disabled = !this.value; });
  el("dsInput").addEventListener("focus", function () { renderList(this.value); });
  el("dsInput").addEventListener("blur", function () { setTimeout(closeList, 120); });
  el("dsInput").addEventListener("keydown", function (ev) {
    if (ev.key === "ArrowDown") { ev.preventDefault(); if (!el("dsList").classList.contains("open")) renderList(this.value); moveHi(1); }
    else if (ev.key === "ArrowUp") { ev.preventDefault(); moveHi(-1); }
    else if (ev.key === "Enter") {
      var its = el("dsList").querySelectorAll(".combo-item[data-id]");
      if (el("dsList").classList.contains("open") && hiIdx >= 0 && its[hiIdx]) { choose(its[hiIdx].getAttribute("data-id")); }
      else if (this.value) { currentId = this.value.trim(); closeList(); runQuery(); }
    } else if (ev.key === "Escape") { closeList(); }
  });

  function loadDatasets(force) {
    dsLoading = true; dsError = "";
    el("dsInput").placeholder = "Carregando datasets…";
    if (el("dsList").classList.contains("open")) renderList(el("dsInput").value);
    api("GET", "/_dev/api/dataset/list" + (force ? "?force=1" : ""), null, function (err, data) {
      dsLoading = false;
      if (err) {
        dsError = err;
        el("dsInput").placeholder = "Falha ao carregar — clique em ↻ para tentar de novo";
        if (el("dsList").classList.contains("open")) renderList(el("dsInput").value);
        return;
      }
      datasets = (data && data.datasets) || [];
      el("dsInput").placeholder = "Escolha ou digite o id do dataset… (" + datasets.length + " no servidor)";
      if (el("dsList").classList.contains("open")) renderList(el("dsInput").value);
      var last = ui().lastId;
      if (last && datasets.some(function (d) { return d.id === last; })) choose(last);
    });
  }

  // ---- painel de parâmetros ----
  function onSelectDataset(id) {
    var cfg = readJSON(cfgKey(id), null);
    // limite / ordenação / sqlLimit vêm da config salva (se houver)
    if (cfg) {
      el("limit").value = (cfg.limit != null ? cfg.limit : 100);
      el("orderBy").value = ""; // preenchido quando as colunas chegarem
      orderDir = cfg.orderDir || "asc";
      applyOrderDir();
      el("sqlOn").checked = !!(cfg.sql && cfg.sql.on);
      if (cfg.sql) {
        el("sqlIni").value = cfg.sql.ini != null ? cfg.sql.ini : 1;
        el("sqlFim").value = cfg.sql.fim != null ? cfg.sql.fim : 100;
        el("sqlType").value = cfg.sql.type || "MUST";
        el("sqlLike").checked = !!cfg.sql.like;
      }
      toggleSql();
    } else {
      el("limit").value = 100; orderDir = "asc"; applyOrderDir();
      el("sqlOn").checked = false; toggleSql();
    }
    // colunas: sonda
    el("fields").innerHTML = '<span class="note">carregando colunas… <span class="spin"></span></span>';
    el("fieldsNote").textContent = "";
    api("GET", "/_dev/api/dataset/fields?id=" + encodeURIComponent(id), null, function (err, data) {
      if (err) { setColumns([], cfg); el("fieldsNote").textContent = "Não consegui descobrir as colunas:" + err; el("fieldsNote").className = "note warn"; return; }
      if (data && data.probeError) {
        el("fieldsNote").textContent = "As colunas se revelam após consultar (o dataset pode exigir um filtro, ex.: sqlLimit).";
        el("fieldsNote").className = "note";
      } else { el("fieldsNote").textContent = ""; }
      setColumns((data && data.columns) || [], cfg);
    });
    // filtros salvos
    el("filters").innerHTML = "";
    if (cfg && cfg.filters) cfg.filters.forEach(function (fl) { addFilterRow(fl); });
  }

  function setColumns(cols, cfg) {
    columns = cols.slice();
    // seleção de campos: da config, senão todas
    selectedFields = {};
    var sel = cfg && cfg.fields;
    columns.forEach(function (c) { selectedFields[c] = sel ? (sel.indexOf(c) >= 0) : true; });
    renderFields();
    // ordenação
    var ob = el("orderBy"), keep = cfg && cfg.orderBy || "";
    ob.innerHTML = '<option value="">(sem ordenação)</option>';
    columns.forEach(function (c) {
      var o = document.createElement("option"); o.value = c; o.textContent = c;
      if (c === keep) o.selected = true; ob.appendChild(o);
    });
    // filtros: atualiza os selects de campo já existentes
    refreshFilterFieldOptions();
  }

  function renderFields() {
    var box = el("fields"); box.innerHTML = "";
    if (!columns.length) { box.innerHTML = '<span class="note">colunas desconhecidas — a consulta traz todas.</span>'; return; }
    columns.forEach(function (c) {
      var chip = document.createElement("span");
      chip.className = "chip" + (selectedFields[c] ? " on" : "");
      chip.textContent = c;
      chip.addEventListener("click", function () { selectedFields[c] = !selectedFields[c]; chip.classList.toggle("on"); });
      box.appendChild(chip);
    });
  }
  el("fldsAll").addEventListener("click", function () { columns.forEach(function (c) { selectedFields[c] = true; }); renderFields(); });
  el("fldsNone").addEventListener("click", function () { columns.forEach(function (c) { selectedFields[c] = false; }); renderFields(); });

  function applyOrderDir() {
    el("orderDir").querySelectorAll("button").forEach(function (b) {
      b.classList.toggle("on", b.getAttribute("data-dir") === orderDir);
    });
  }
  el("orderDir").addEventListener("click", function (ev) {
    var b = ev.target.closest("button"); if (!b) return;
    orderDir = b.getAttribute("data-dir"); applyOrderDir();
  });

  function toggleSql() { el("sqlRow").classList.toggle("open", el("sqlOn").checked); }
  el("sqlOn").addEventListener("change", toggleSql);

  var typeOpts = '<option value="MUST">Must</option><option value="MUST_NOT">Must Not</option><option value="SHOULD">Should</option>';
  function addFilterRow(fl) {
    fl = fl || {};
    var row = document.createElement("div");
    row.className = "filter";
    var fieldCtl = columns.length
      ? '<select class="f-field">' + fieldOptions(fl.field) + "</select>"
      : '<input type="text" class="f-field" placeholder="campo" value="' + esc(fl.field || "") + '">';
    row.innerHTML = fieldCtl +
      '<input type="text" class="f-ini" placeholder="valor inicial" value="' + esc(fl.initial || "") + '">' +
      '<input type="text" class="f-fim" placeholder="valor final (opc.)" value="' + esc(fl.final || "") + '">' +
      '<select class="f-type">' + typeOpts + "</select>" +
      '<label class="check"><input type="checkbox" class="f-like"> Like</label>' +
      '<button class="icobtn rm" title="Remover filtro">✕</button>';
    el("filters").appendChild(row);
    if (fl.type) row.querySelector(".f-type").value = fl.type;
    if (fl.like) row.querySelector(".f-like").checked = true;
    row.querySelector(".rm").addEventListener("click", function () { row.remove(); });
  }
  function fieldOptions(sel) {
    return '<option value="">campo…</option>' + columns.map(function (c) {
      return '<option value="' + esc(c) + '"' + (c === sel ? " selected" : "") + ">" + esc(c) + "</option>";
    }).join("");
  }
  function refreshFilterFieldOptions() {
    el("filters").querySelectorAll(".filter").forEach(function (row) {
      var cur = row.querySelector(".f-field"), val = cur.value;
      if (!columns.length) return;
      if (cur.tagName === "SELECT") { cur.innerHTML = fieldOptions(val); }
      else { // troca input por select agora que temos colunas
        var sel = document.createElement("select"); sel.className = "f-field";
        sel.innerHTML = fieldOptions(val); cur.replaceWith(sel);
      }
    });
  }
  el("addFilter").addEventListener("click", function () { addFilterRow(); });

  el("paramsBtn").addEventListener("click", function () {
    var open = el("params").classList.toggle("open");
    this.setAttribute("aria-expanded", open ? "true" : "false");
    saveUI({ paramsOpen: open });
  });

  // ---- montar payload + consultar ----
  function collectFilters() {
    var out = [];
    el("filters").querySelectorAll(".filter").forEach(function (row) {
      var field = row.querySelector(".f-field").value.trim();
      if (!field) return;
      out.push({
        field: field,
        initial: row.querySelector(".f-ini").value,
        final: row.querySelector(".f-fim").value,
        type: row.querySelector(".f-type").value,
        like: row.querySelector(".f-like").checked
      });
    });
    return out;
  }
  function selectedFieldList() {
    var sel = columns.filter(function (c) { return selectedFields[c]; });
    // todas ou nenhuma selecionada => [] (o servidor devolve todas as colunas)
    if (!columns.length || sel.length === 0 || sel.length === columns.length) return [];
    return sel;
  }
  function orderByValue() {
    var col = el("orderBy").value;
    if (!col) return "";
    return orderDir === "desc" ? col + "_DESC" : col;
  }
  function buildPayload() {
    var cons = collectFilters();
    if (el("sqlOn").checked) {
      cons.push({ field: "sqlLimit", initial: el("sqlIni").value, final: el("sqlFim").value,
        type: el("sqlType").value, like: el("sqlLike").checked });
    }
    return {
      id: currentId,
      fields: selectedFieldList(),
      constraints: cons,
      orderBy: orderByValue(),
      limit: parseInt(el("limit").value, 10) || 0
    };
  }
  function persistCfg() {
    if (!currentId) return;
    writeJSON(cfgKey(currentId), {
      fields: columns.filter(function (c) { return selectedFields[c]; }),
      orderBy: el("orderBy").value, orderDir: orderDir,
      limit: parseInt(el("limit").value, 10) || 0,
      sql: { on: el("sqlOn").checked, ini: el("sqlIni").value, fim: el("sqlFim").value,
        type: el("sqlType").value, like: el("sqlLike").checked },
      filters: collectFilters()
    });
  }

  function runQuery() {
    if (!currentId) return;
    persistCfg();
    var btn = el("runBtn"), lbl = btn.textContent;
    btn.disabled = true; btn.innerHTML = 'Consultando <span class="spin"></span>';
    el("resbar").classList.add("hidden");
    el("resBody").innerHTML = '<div class="state">Consultando o servidor… <span class="spin"></span></div>';
    api("POST", "/_dev/api/dataset/query", buildPayload(), function (err, data) {
      btn.disabled = false; btn.textContent = lbl;
      if (err) {
        el("resBody").innerHTML = '<div class="state err">' + esc(err) + "</div>";
        return;
      }
      lastResult = data;
      // colunas reveladas pelo resultado alimentam os seletores
      if (data.columns && data.columns.length && (!columns.length || columns.join() !== data.columns.join())) {
        setColumns(data.columns, readJSON(cfgKey(currentId), null));
      }
      renderResult(data);
    });
  }
  el("runBtn").addEventListener("click", runQuery);

  // ---- render do resultado ----
  var view = "table";
  function renderResult(data) {
    el("resbar").classList.remove("hidden");
    var truncNote = data.truncated ? ' · <span style="color:var(--warn)">limite atingido — pode haver mais</span>' : "";
    el("resStat").innerHTML = "<b>" + data.count + "</b> linha(s) · <b>" + (data.columns || []).length + "</b> coluna(s)" + truncNote;
    var dur = el("resDur");
    dur.classList.remove("hidden"); dur.textContent = data.durationMs + " ms";
    renderView();
  }
  function renderView() {
    if (!lastResult) return;
    if (view === "json") { renderJSON(lastResult); return; }
    renderTable(lastResult);
  }
  function renderTable(data) {
    var cols = data.columns || [], rows = data.rows || [];
    if (!rows.length) { el("resBody").innerHTML = '<div class="state">Nenhuma linha retornada.</div>'; return; }
    var h = '<div class="tablewrap"><table><thead><tr><th class="rownum">#</th>';
    cols.forEach(function (c) { h += "<th>" + esc(c) + "</th>"; });
    h += "</tr></thead><tbody>";
    rows.forEach(function (row, i) {
      h += '<tr><td class="rownum">' + (i + 1) + "</td>";
      cols.forEach(function (c) {
        var v = row[c];
        if (v === null || v === undefined) h += '<td><span class="nul">null</span></td>';
        else h += "<td title=\"" + esc(v) + "\">" + esc(v) + "</td>";
      });
      h += "</tr>";
    });
    h += "</tbody></table></div>";
    el("resBody").innerHTML = h;
  }
  function renderJSON(data) {
    var cols = data.columns || [], rows = data.rows || [];
    var objs = rows.map(function (row) {
      var o = {}; cols.forEach(function (c) { o[c] = (row[c] === undefined ? null : row[c]); }); return o;
    });
    el("resBody").innerHTML = '<pre class="jsonwrap"></pre>';
    el("resBody").querySelector("pre").textContent = JSON.stringify(objs, null, 2);
  }
  el("viewSeg").addEventListener("click", function (ev) {
    var b = ev.target.closest("button"); if (!b) return;
    view = b.getAttribute("data-view");
    el("viewSeg").querySelectorAll("button").forEach(function (x) { x.classList.toggle("on", x === b); });
    saveUI({ view: view });
    renderView();
  });

  // ---- copiar / exportar ----
  function cellStr(v) { return v === null || v === undefined ? "" : String(v); }
  function copyBtnFeedback(ok) {
    var b = el("copyBtn"), t = b.textContent;
    b.textContent = ok ? "Copiado ✓" : "Falhou"; setTimeout(function () { b.textContent = t; }, 1400);
  }
  el("copyBtn").addEventListener("click", function () {
    if (!lastResult) return;
    var cols = lastResult.columns || [], rows = lastResult.rows || [];
    var lines = [cols.join("\t")];
    rows.forEach(function (row) { lines.push(cols.map(function (c) { return cellStr(row[c]).replace(/[\t\n\r]/g, " "); }).join("\t")); });
    var text = lines.join("\n");
    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(text).then(function () { copyBtnFeedback(true); }, function () { copyBtnFeedback(false); });
    } else {
      var ta = document.createElement("textarea"); ta.value = text; document.body.appendChild(ta); ta.select();
      var ok = false; try { ok = document.execCommand("copy"); } catch (e) {}
      document.body.removeChild(ta); copyBtnFeedback(ok);
    }
  });
  function csvCell(v) {
    var s = cellStr(v);
    if (/[",\n\r]/.test(s)) s = '"' + s.replace(/"/g, '""') + '"';
    return s;
  }
  el("csvBtn").addEventListener("click", function () {
    if (!lastResult) return;
    var cols = lastResult.columns || [], rows = lastResult.rows || [];
    var lines = [cols.map(csvCell).join(",")];
    rows.forEach(function (row) { lines.push(cols.map(function (c) { return csvCell(row[c]); }).join(",")); });
    var blob = new Blob([String.fromCharCode(0xFEFF) + lines.join("\r\n")], { type: "text/csv;charset=utf-8;" });
    var url = URL.createObjectURL(blob), a = document.createElement("a");
    var stamp = new Date().toISOString().slice(0, 19).replace(/[:T]/g, "-");
    a.href = url; a.download = (currentId || "dataset") + "-" + stamp + ".csv";
    document.body.appendChild(a); a.click(); document.body.removeChild(a);
    setTimeout(function () { URL.revokeObjectURL(url); }, 2000);
  });

  el("reloadDs").addEventListener("click", function () { loadDatasets(true); });

  // ---- boot ----
  (function boot() {
    var u = ui();
    if (u.paramsOpen) { el("params").classList.add("open"); el("paramsBtn").setAttribute("aria-expanded", "true"); }
    if (u.view === "json") { view = "json"; el("viewSeg").querySelectorAll("button").forEach(function (x) { x.classList.toggle("on", x.getAttribute("data-view") === "json"); }); }
    loadDatasets(false);
    el("dsInput").focus();
  })();
})();
</script>
</body>
</html>
`
