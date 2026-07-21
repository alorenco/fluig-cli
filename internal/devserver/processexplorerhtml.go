package devserver

// processExplorerHTML é a página do Explorador de Processos (/_dev/processes/).
// Self-contained e no mesmo design system do Dataset Lab (claro/escuro, accent
// ciano). Dados via /_dev/api/process/*. Sem template literals no JS: a página
// é uma raw string Go delimitada por backticks — o JS concatena strings.
const processExplorerHTML = `<!doctype html>
<html lang="pt-BR">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>fluigcli dev — processos</title>
<style>
:root{--bg:#f4f6f8;--card:#fff;--txt:#1d2b36;--sub:#5a6b7b;--line:#e3e8ee;
  --accent:#0c9abe;--accent-txt:#fff;--ok:#25b26e;--warn:#b3352b;--amber:#b7791f;
  --chipbg:#f7f9fb;--zebra:#f9fbfc;--shadow:0 1px 2px rgba(16,36,54,.08)}
@media(prefers-color-scheme:dark){:root{--bg:#12181f;--card:#1b232d;
  --txt:#e6edf3;--sub:#93a4b4;--line:#2b3742;--chipbg:#161d26;--zebra:#19212b;
  --amber:#d8a54a;--shadow:0 1px 2px rgba(0,0,0,.4)}}
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
button{font:inherit}
.btn{padding:8px 16px;border:0;border-radius:8px;cursor:pointer;background:var(--accent);
  color:var(--accent-txt);font-weight:650;font-size:13.5px;transition:filter .08s}
.btn:hover{filter:brightness(1.06)}
.btn.ghost{background:transparent;border:1px solid var(--line);color:var(--txt)}
.btn.ghost:hover{border-color:var(--accent);color:var(--accent)}
.icobtn{background:transparent;border:1px solid var(--line);color:var(--sub);border-radius:8px;
  width:36px;height:36px;cursor:pointer;font-size:15px;transition:border-color .08s,color .08s}
.icobtn:hover{border-color:var(--accent);color:var(--accent)}

/* barra de seleção */
.querybar{display:flex;gap:10px;align-items:center;flex-wrap:wrap}
.combo{position:relative;flex:1;min-width:300px}
.combo>input{width:100%;padding:10px 14px;border:1px solid var(--line);border-radius:9px;
  background:var(--card);color:var(--txt);font-size:14.5px;outline:none}
.combo>input:focus{border-color:var(--accent)}
.combo-list{position:absolute;z-index:20;top:calc(100% + 4px);left:0;right:0;max-height:360px;
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
select{padding:9px 12px;border:1px solid var(--line);border-radius:9px;background:var(--card);
  color:var(--txt);font-size:13.5px;outline:none}
select:focus{border-color:var(--accent)}

.tbadge{font-size:10.5px;font-weight:700;letter-spacing:.03em;padding:1px 7px;border-radius:5px;
  background:color-mix(in srgb,var(--sub) 16%,transparent);color:var(--sub);white-space:nowrap}
.tbadge.off{background:color-mix(in srgb,var(--warn) 14%,transparent);color:var(--warn)}
.tbadge.on{background:color-mix(in srgb,var(--ok) 16%,transparent);color:var(--ok)}

/* cards */
.card{background:var(--card);border:1px solid var(--line);border-radius:12px;
  padding:18px 20px;box-shadow:var(--shadow);margin-top:20px}
.card h2{margin:0 0 14px;font-size:13px;font-weight:700;letter-spacing:.05em;
  text-transform:uppercase;color:var(--sub);display:flex;align-items:center;gap:10px}
.card h2 .rt{margin-left:auto;font:inherit;text-transform:none;letter-spacing:0}

/* cabeçalho do processo */
.phead{display:flex;flex-wrap:wrap;gap:22px 40px;align-items:flex-start}
.phead .desc{font-size:20px;font-weight:650;margin:0}
.pid{font:600 13px/1.4 ui-monospace,Consolas,monospace;color:var(--accent);cursor:pointer;
  border:1px solid var(--line);border-radius:6px;padding:2px 8px;background:var(--chipbg)}
.pid:hover{border-color:var(--accent)}
.kv{display:flex;flex-direction:column;gap:2px}
.kv .k{font-size:11.5px;letter-spacing:.04em;text-transform:uppercase;color:var(--sub)}
.kv .v{font-size:14px;font-weight:600}
.kv .v a{color:var(--accent);text-decoration:none}
.kv .v a:hover{text-decoration:underline}

/* tabela de etapas */
.filterbar{display:flex;gap:10px;align-items:center;margin-bottom:12px;flex-wrap:wrap}
.filterbar input{flex:1;max-width:300px;padding:8px 12px;border:1px solid var(--line);
  border-radius:8px;background:var(--bg);color:var(--txt);font-size:13.5px;outline:none}
.filterbar input:focus{border-color:var(--accent)}
.filterbar .hint{color:var(--sub);font-size:12.5px}
.grow{flex:1}
.seg{display:inline-flex;border:1px solid var(--line);border-radius:8px;overflow:hidden}
.seg button{padding:7px 12px;border:0;background:var(--bg);color:var(--sub);cursor:pointer;font-size:12.5px}
.seg button.on{background:var(--accent);color:var(--accent-txt);font-weight:650}
.tablewrap{overflow:auto;border:1px solid var(--line);border-radius:12px}
table{border-collapse:collapse;width:100%;font-size:13px}
thead th{position:sticky;top:0;background:var(--card);text-align:left;font-weight:700;
  padding:9px 12px;border-bottom:2px solid var(--line);white-space:nowrap;z-index:1}
tbody td{padding:8px 12px;border-bottom:1px solid var(--line);vertical-align:top}
tbody tr.state{cursor:pointer}
tbody tr.state:hover{background:color-mix(in srgb,var(--accent) 7%,transparent)}
tbody tr:nth-child(even of .state){background:var(--zebra)}
td.seq{font:700 15px/1 ui-monospace,Consolas,monospace;color:var(--accent);text-align:right;white-space:nowrap}
td.seq small{display:block;color:var(--sub);font-weight:400;font-size:10px;margin-top:3px}
.kind{font-size:10.5px;font-weight:700;letter-spacing:.03em;padding:2px 8px;border-radius:5px;white-space:nowrap}
.kind.start{background:color-mix(in srgb,var(--ok) 16%,transparent);color:var(--ok)}
.kind.end{background:color-mix(in srgb,var(--warn) 14%,transparent);color:var(--warn)}
.kind.task{background:color-mix(in srgb,var(--accent) 15%,transparent);color:var(--accent)}
.kind.service{background:color-mix(in srgb,var(--amber) 20%,transparent);color:var(--amber)}
.kind.gateway{background:color-mix(in srgb,var(--sub) 20%,transparent);color:var(--txt)}
.kind.event,.kind.unknown{background:color-mix(in srgb,var(--sub) 16%,transparent);color:var(--sub)}
.who{display:flex;flex-wrap:wrap;gap:5px}
.chip{border:1px solid var(--line);border-radius:999px;padding:2px 10px;font-size:12px;
  background:var(--chipbg);white-space:nowrap}
.chip b{font-family:ui-monospace,Consolas,monospace;font-weight:700}
.chip b.hl{color:var(--accent)}
.chip.mech{color:var(--sub)}
.arrows{color:var(--sub);font-size:12px;font-family:ui-monospace,Consolas,monospace}
.subrow td{background:var(--chipbg);font-size:12.5px;color:var(--sub);padding:6px 12px 12px}
.subrow .rule{font-family:ui-monospace,Consolas,monospace;color:var(--txt)}
.subrow .cond{margin:4px 0}

/* scripts */
.scripts{display:grid;grid-template-columns:1fr 1fr;gap:20px}
@media(max-width:820px){.scripts{grid-template-columns:1fr}}
.scol h3{margin:0 0 8px;font-size:13px;color:var(--sub)}
.script{display:flex;align-items:center;gap:10px;padding:7px 10px;border:1px solid var(--line);
  border-radius:8px;margin-bottom:7px;background:var(--bg)}
.script .nm{font:600 13px/1.3 ui-monospace,Consolas,monospace;flex:1;word-break:break-all}
.script .sz{color:var(--sub);font-size:11.5px;white-space:nowrap}
.lbadge{font-size:10.5px;font-weight:700;padding:1px 7px;border-radius:5px;white-space:nowrap}
.lbadge.local{background:color-mix(in srgb,var(--ok) 16%,transparent);color:var(--ok)}
.lbadge.server{background:color-mix(in srgb,var(--sub) 16%,transparent);color:var(--sub)}

/* diagrama */
.diagram{overflow:auto;border:1px solid var(--line);border-radius:10px;background:#fff;padding:10px;
  max-height:80vh;cursor:grab}
.diagram.grabbing{cursor:grabbing}
.diaginner{width:100%;margin:0 auto;transition:width .12s ease}
.diagram svg{width:100%;height:auto;display:block}
.zlbl{font:12px/1 ui-monospace,Consolas,monospace;color:var(--sub);min-width:44px;text-align:center;
  display:inline-block}
.collapsed{display:none}

.state-msg{padding:60px 0;text-align:center;color:var(--sub)}
.state-msg.err{color:var(--warn)}
.state-msg code{background:var(--chipbg);border:1px solid var(--line);border-radius:5px;padding:2px 7px}
.spin{display:inline-block;width:15px;height:15px;border:2px solid color-mix(in srgb,var(--accent) 30%,transparent);
  border-top-color:var(--accent);border-radius:50%;animation:sp .7s linear infinite;vertical-align:-2px}
@keyframes sp{to{transform:rotate(360deg)}}
.toast{position:fixed;bottom:24px;left:50%;transform:translateX(-50%) translateY(20px);
  background:var(--txt);color:var(--bg);padding:9px 18px;border-radius:999px;font-size:13px;font-weight:600;
  opacity:0;pointer-events:none;transition:opacity .18s,transform .18s;z-index:50}
.toast.show{opacity:1;transform:translateX(-50%) translateY(0)}
.hidden{display:none!important}
</style>
</head>
<body>
<header>
  <div class="hrow">
    <div>
      <h1>fluigcli <small>dev</small> · Processos</h1>
      <p>Tudo do processo num lugar — códigos das etapas (WKNumState), quem atua, scripts e diagrama. Só leitura.</p>
    </div>
    <a class="back" href="/">← Dashboard</a>
  </div>
</header>
<main>
  <div class="card" style="margin-top:0">
    <div class="querybar">
      <div class="combo">
        <input id="pInput" type="text" autocomplete="off" spellcheck="false"
          placeholder="Escolha ou digite o id/nome do processo…">
        <div class="combo-list" id="pList"></div>
      </div>
      <select id="verSel" class="hidden" title="Versão do processo"></select>
      <button class="icobtn" id="reloadP" title="Recarregar (ignora cache)">↻</button>
    </div>
  </div>

  <div id="detail">
    <div class="state-msg">Escolha um processo para ver os detalhes.</div>
  </div>
</main>
<div class="toast" id="toast"></div>
<script>
(function () {
  "use strict";
  function api(method, path, cb) {
    var x = new XMLHttpRequest();
    x.open(method, path, true);
    x.onreadystatechange = function () {
      if (x.readyState !== 4) return;
      var data = null;
      try { data = JSON.parse(x.responseText); } catch (e) {}
      if (x.status >= 200 && x.status < 300) cb(null, data);
      else cb((data && data.error) || ("HTTP " + x.status), data);
    };
    x.send(null);
  }
  function el(id) { return document.getElementById(id); }
  function esc(s) { return String(s == null ? "" : s).replace(/[&<>"]/g, function (c) {
    return { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]; }); }
  function toast(msg) {
    var t = el("toast"); t.textContent = msg; t.classList.add("show");
    clearTimeout(t._t); t._t = setTimeout(function () { t.classList.remove("show"); }, 1400);
  }
  function copy(text, note) {
    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(text).then(function () { toast(note || "Copiado"); },
        function () { toast("Não consegui copiar"); });
    } else {
      var ta = document.createElement("textarea"); ta.value = text; document.body.appendChild(ta);
      ta.select(); try { document.execCommand("copy"); toast(note || "Copiado"); } catch (e) {}
      document.body.removeChild(ta);
    }
  }

  var UIKEY = "fluigcli.procexplorer.ui";
  function ui() { try { return JSON.parse(localStorage.getItem(UIKEY)) || {}; } catch (e) { return {}; } }
  function saveUI(patch) { var u = ui(); for (var k in patch) u[k] = patch[k]; try { localStorage.setItem(UIKEY, JSON.stringify(u)); } catch (e) {} }

  var processes = [], pLoading = true, pError = "";
  var currentId = "", currentVersion = 0, detail = null;

  // ---- combobox ----
  var hiIdx = -1;
  function renderList(filter) {
    var box = el("pList"), f = (filter || "").toLowerCase();
    var items = processes.filter(function (p) {
      return !f || p.id.toLowerCase().indexOf(f) >= 0 || (p.description || "").toLowerCase().indexOf(f) >= 0;
    });
    box.innerHTML = ""; hiIdx = -1;
    if (pLoading) box.innerHTML = '<div class="combo-item"><span class="empty">Carregando processos… <span class="spin"></span></span></div>';
    else if (pError) box.innerHTML = '<div class="combo-item"><span class="empty" style="color:var(--warn)">Falha: ' + esc(pError) + ' — clique em ↻.</span></div>';
    else if (!processes.length) box.innerHTML = '<div class="combo-item"><span class="empty">Nenhum processo no servidor.</span></div>';
    else if (!items.length) box.innerHTML = '<div class="combo-item"><span class="empty">Nada casa com o filtro.</span></div>';
    else items.slice(0, 200).forEach(function (p) {
      var it = document.createElement("div"); it.className = "combo-item"; it.setAttribute("data-id", p.id);
      var badge = p.active ? "" : '<span class="tbadge off">inativo</span>';
      it.innerHTML = '<span class="cid">' + esc(p.id) + "</span><span class=\"cdesc\">" + esc(p.description || "") + "</span>" + badge;
      it.addEventListener("mousedown", function (ev) { ev.preventDefault(); choose(p.id); });
      box.appendChild(it);
    });
    box.classList.add("open");
  }
  function closeList() { el("pList").classList.remove("open"); }
  function moveHi(d) {
    var its = el("pList").querySelectorAll(".combo-item[data-id]"); if (!its.length) return;
    if (hiIdx >= 0 && its[hiIdx]) its[hiIdx].classList.remove("hi");
    hiIdx = (hiIdx + d + its.length) % its.length; its[hiIdx].classList.add("hi");
    its[hiIdx].scrollIntoView({ block: "nearest" });
  }
  function choose(id) {
    currentId = id; currentVersion = 0; el("pInput").value = id; closeList();
    saveUI({ lastId: id }); setDeepLink(id); loadDetail(false);
  }
  el("pInput").addEventListener("input", function () { renderList(this.value); });
  el("pInput").addEventListener("focus", function () { renderList(this.value); });
  el("pInput").addEventListener("blur", function () { setTimeout(closeList, 120); });
  el("pInput").addEventListener("keydown", function (ev) {
    if (ev.key === "ArrowDown") { ev.preventDefault(); if (!el("pList").classList.contains("open")) renderList(this.value); moveHi(1); }
    else if (ev.key === "ArrowUp") { ev.preventDefault(); moveHi(-1); }
    else if (ev.key === "Enter") {
      var its = el("pList").querySelectorAll(".combo-item[data-id]");
      if (el("pList").classList.contains("open") && hiIdx >= 0 && its[hiIdx]) choose(its[hiIdx].getAttribute("data-id"));
      else if (this.value) choose(this.value.trim());
    } else if (ev.key === "Escape") closeList();
  });
  el("reloadP").addEventListener("click", function () {
    if (currentId) loadDetail(true); else loadProcesses(true);
  });
  el("verSel").addEventListener("change", function () {
    currentVersion = parseInt(this.value, 10) || 0; loadDetail(false);
  });

  function setDeepLink(id) {
    if (history.replaceState) history.replaceState(null, "", id ? ("?process=" + encodeURIComponent(id)) : location.pathname);
  }

  function loadProcesses(force) {
    pLoading = true; pError = "";
    if (el("pList").classList.contains("open")) renderList(el("pInput").value);
    api("GET", "/_dev/api/process/list" + (force ? "?force=1" : ""), function (err, data) {
      pLoading = false;
      if (err) { pError = err; if (el("pList").classList.contains("open")) renderList(el("pInput").value); return; }
      processes = (data && data.processes) || [];
      el("pInput").placeholder = "Escolha ou digite o id/nome do processo… (" + processes.length + " no servidor)";
      if (el("pList").classList.contains("open")) renderList(el("pInput").value);
      // Deep-link tem prioridade sobre o último visto.
      var q = new URLSearchParams(location.search).get("process");
      var pick = q || ui().lastId;
      if (pick && processes.some(function (p) { return p.id === pick; })) { el("pInput").value = pick; choose(pick); }
    });
  }

  // ---- detalhe ----
  function loadDetail(force) {
    if (!currentId) return;
    el("detail").innerHTML = '<div class="state-msg">Carregando <b>' + esc(currentId) + '</b>… <span class="spin"></span></div>';
    var url = "/_dev/api/process/detail?id=" + encodeURIComponent(currentId) +
      (currentVersion ? "&version=" + currentVersion : "") + (force ? "&force=1" : "");
    api("GET", url, function (err, data) {
      if (err) {
        el("detail").innerHTML = '<div class="state-msg err">' + esc(err) + '</div>';
        el("verSel").classList.add("hidden");
        return;
      }
      detail = data; currentVersion = data.version || 0;
      renderVersions(data); renderDetail(data);
    });
  }

  function renderVersions(d) {
    var sel = el("verSel");
    if (!d.versions || !d.versions.length) { sel.classList.add("hidden"); return; }
    sel.innerHTML = "";
    d.versions.forEach(function (v) {
      var o = document.createElement("option"); o.value = v.version;
      o.textContent = "v" + v.version + (v.active ? " (ativa)" : "");
      if (v.version === d.version) o.selected = true;
      sel.appendChild(o);
    });
    sel.classList.remove("hidden");
  }

  function whoChips(a) {
    if (!a) return '<span class="chip mech">—</span>';
    var out = '<span class="chip mech">' + esc(a.mechanism || "?") + "</span>";
    var kindLabel = { role: "papel", group: "grupo", formField: "campo" };
    if (a.kind === "baseActivity" && a.value) {
      // executor de uma atividade anterior → nome da etapa (código no hover).
      var st = stBySeq[a.value];
      var nm = st ? st.name : null;
      var tip = "executor da atividade #" + a.value + (a.extra ? " · " + a.extra : "");
      out += '<span class="chip" title="' + esc(tip) + '">executor de <b class="hl">' + esc(nm || ("#" + a.value)) + "</b></span>";
    } else if ((a.kind === "user" || a.kind === "colleague") && a.value) {
      // usuário específico → nome (userCode no hover).
      var label = a.kind === "user" ? "usuário" : "colaborador";
      out += '<span class="chip" title="' + esc(a.value) + '">' + label + ' <b class="hl">' + esc(a.name || a.value) + "</b></span>";
    } else if (a.value) {
      out += '<span class="chip">' + esc(kindLabel[a.kind] || a.kind || "") + ' <b>' + esc(a.value) + "</b></span>";
      if (a.extra) out += '<span class="chip mech">' + esc(a.extra) + "</span>";
    }
    return '<div class="who">' + out + "</div>";
  }

  function transText(st) {
    if (!st.transitions || !st.transitions.length) return "";
    return "→ " + st.transitions.map(function (t) { return t.to; }).join(", ");
  }

  function fmtDeadline(sec) {
    if (!sec) return "";
    if (sec % 3600 === 0) return (sec / 3600) + "h";
    if (sec % 60 === 0) return (sec / 60) + "min";
    return sec + "s";
  }

  function renderDetail(d) {
    var h = "";

    // --- cabeçalho ---
    h += '<div class="card"><h2>Processo</h2><div class="phead">';
    h += '<div><p class="desc">' + esc(d.description || d.id) + "</p>";
    h += '<div style="margin-top:8px;display:flex;gap:8px;align-items:center;flex-wrap:wrap">';
    h += '<span class="pid" data-copy="' + esc(d.id) + '" title="Copiar o id">' + esc(d.id) + "</span>";
    h += d.active ? '<span class="tbadge on">ativo</span>' : '<span class="tbadge off">inativo</span>';
    h += "</div></div>";
    h += '<div class="kv"><span class="k">Versão</span><span class="v">v' + d.version +
      (d.versions && d.versions.length ? ' <span style="color:var(--sub);font-weight:400">de ' + d.versions.length + "</span>" : "") + "</span></div>";
    if (d.author) h += '<div class="kv"><span class="k">Autor</span><span class="v">' + esc(d.author) + "</span></div>";
    // formulário
    if (d.formId) {
      var fv = "";
      if (d.form && d.form.folder) fv = '<a href="/_dev/forms/' + encodeURIComponent(d.form.folder) + '/" target="_blank" rel="noopener" title="Abrir o preview do formulário em nova aba">' + esc(d.form.name || d.form.folder) + " ↗</a>";
      else if (d.form && d.form.name) fv = esc(d.form.name);
      else fv = '<span style="color:var(--sub)">(sem vínculo local)</span>';
      h += '<div class="kv"><span class="k">Formulário · id ' + d.formId + '</span><span class="v">' + fv + "</span></div>";
    } else {
      h += '<div class="kv"><span class="k">Formulário</span><span class="v" style="color:var(--sub);font-weight:400">nenhum</span></div>';
    }
    h += "</div></div>";

    // --- etapas ---
    var nStates = d.states.filter(function (s) { return s.kind !== "event"; }).length;
    h += '<div class="card"><h2>Etapas <span class="rt" style="color:var(--sub)">' + nStates + ' · clique numa linha para copiar o snippet de WKNumState</span></h2>';
    h += '<div class="filterbar"><input id="stFilter" type="search" placeholder="Filtrar etapas por nome…">' +
      '<span class="hint" id="stCount"></span><span class="grow"></span>' +
      '<div class="seg" id="ordSeg" title="Ordem das etapas"><button data-ord="flow" class="on">Fluxo</button><button data-ord="num">Nº</button></div>' +
      '<button class="btn ghost" id="copyConsts" title="Copiar as constantes de WKNumState de todas as etapas">Copiar constantes</button>' +
      "</div>";
    h += '<div class="tablewrap"><table><thead><tr>' +
      '<th style="text-align:right">Nº</th><th>Etapa</th><th>Tipo</th><th>Quem atua</th><th>Prazo</th><th>Fluxo</th>' +
      "</tr></thead><tbody id=\"stBody\"></tbody></table></div></div>";

    // --- diagrama ---
    if (d.diagramSvg) {
      h += '<div class="card"><h2>Diagrama <span class="rt">' +
        '<button class="icobtn" id="zOut" title="Reduzir">−</button>' +
        '<span class="zlbl" id="zLbl">ajustado</span>' +
        '<button class="icobtn" id="zIn" title="Aproximar">+</button>' +
        '<button class="btn ghost" id="zFit" style="margin-left:8px">ajustar</button>' +
        '<button class="btn ghost" id="diagToggle">ocultar</button></span></h2>' +
        '<div class="diagram" id="diagBox"><div class="diaginner" id="diagInner">' + d.diagramSvg + "</div></div></div>";
    }

    // --- scripts ---
    var globals = d.events.filter(function (e) { return !e.service; });
    var services = d.events.filter(function (e) { return e.service; });
    h += '<div class="card"><h2>Scripts <span class="rt" style="color:var(--sub)">' + d.events.length + " no servidor</span></h2>";
    if (!d.events.length) h += '<div style="color:var(--sub);font-size:13px">Este processo não tem scripts de evento.</div>';
    else {
      h += '<div class="scripts">';
      h += '<div class="scol"><h3>Eventos globais (' + globals.length + ")</h3>" + scriptsHTML(d.id, globals) + "</div>";
      h += '<div class="scol"><h3>Service tasks (' + services.length + ")</h3>" + (services.length ? scriptsHTML(d.id, services) : '<div style="color:var(--sub);font-size:12.5px">nenhuma</div>') + "</div>";
      h += "</div>";
      h += '<div style="margin-top:12px;color:var(--sub);font-size:12.5px">Baixe do servidor com <code>fluigcli workflow import ' + esc(d.id) + "</code>.</div>";
    }

    el("detail").innerHTML = h;
    renderStates(d.states);
    wireDetail(d);
  }

  function scriptsHTML(pid, evs) {
    if (!evs.length) return "";
    return evs.map(function (e) {
      var badge = e.localPath
        ? '<span class="lbadge local" title="' + esc(e.localPath) + '">local ✓</span>'
        : '<span class="lbadge server">só no servidor</span>';
      var fname = esc(pid + "." + e.event + ".js");
      return '<div class="script"><span class="nm">' + fname + '</span><span class="sz">' +
        e.codeChars.toLocaleString("pt-BR") + " car.</span>" + badge + "</div>";
    }).join("");
  }

  var stateRows = [], stOrder = "flow", stBySeq = {};
  function renderStates(states) {
    stateRows = states;
    stBySeq = {};
    states.forEach(function (s) { stBySeq[s.sequence] = s; });
    stOrder = ui().order === "num" ? "num" : "flow";
    var seg = el("ordSeg");
    if (seg) seg.querySelectorAll("button").forEach(function (b) { b.classList.toggle("on", b.getAttribute("data-ord") === stOrder); });
    drawStates(el("stFilter") ? el("stFilter").value : "");
  }
  // orderedStates: por número, ou na ordem APROXIMADA do fluxo (BFS a partir
  // dos inícios seguindo as transitions; ilhas e não visitados vão ao fim por
  // número). O fluxo tem ciclos (retornos) — o BFS os visita uma vez só.
  function orderedStates() {
    if (stOrder === "num") return stateRows.slice().sort(function (a, b) { return a.sequence - b.sequence; });
    var bySeq = {}; stateRows.forEach(function (s) { bySeq[s.sequence] = s; });
    var seen = {}, out = [];
    var queue = stateRows.filter(function (s) { return s.kind === "start"; }).map(function (s) { return s.sequence; });
    if (!queue.length && stateRows.length) queue = [stateRows.slice().sort(function (a, b) { return a.sequence - b.sequence; })[0].sequence];
    while (queue.length) {
      var seq = queue.shift();
      if (seen[seq] || !bySeq[seq]) continue;
      seen[seq] = true; out.push(bySeq[seq]);
      (bySeq[seq].transitions || []).forEach(function (t) { if (!seen[t.to]) queue.push(t.to); });
    }
    stateRows.filter(function (s) { return !seen[s.sequence]; })
      .sort(function (a, b) { return a.sequence - b.sequence; })
      .forEach(function (s) { out.push(s); });
    return out;
  }
  function drawStates(filter) {
    var body = el("stBody"); if (!body) return;
    var f = (filter || "").toLowerCase();
    var shown = 0, html = "";
    orderedStates().forEach(function (st) {
      // Eventos intermediários (boundary de erro etc.) não são etapas úteis
      // ao dev do formulário — ficam fora da lista (mas seguem no fluxo).
      if (st.kind === "event") return;
      if (f && st.name.toLowerCase().indexOf(f) < 0) return;
      shown++;
      var snippet = 'getValue("WKNumState") == ' + st.sequence + " // " + st.name;
      html += '<tr class="state" data-copy="' + esc(snippet) + '">' +
        '<td class="seq">' + st.sequence + "</td>" +
        "<td>" + esc(st.name) + (st.description && st.description !== st.name ? '<br><span style="color:var(--sub);font-size:11.5px">' + esc(st.description) + "</span>" : "") + "</td>" +
        '<td><span class="kind ' + esc(st.kind) + '">' + kindLabel(st.kind) + "</span></td>" +
        "<td>" + (st.kind === "task" ? whoChips(st.assignment) : (st.kind === "service" ? svcChip(st.service) : '<span style="color:var(--sub)">—</span>')) + "</td>" +
        '<td class="arrows">' + esc(fmtDeadline(st.deadlineTime)) + "</td>" +
        '<td class="arrows">' + esc(transText(st)) + "</td></tr>";
      // sub-linha para gateway com condições
      if (st.kind === "gateway" && st.conditions && st.conditions.length) {
        html += '<tr class="subrow"><td></td><td colspan="5">' + condHTML(st.conditions) + "</td></tr>";
      }
    });
    body.innerHTML = html || '<tr><td colspan="6" style="color:var(--sub);padding:16px">Nenhuma etapa casa com o filtro.</td></tr>';
    var total = stateRows.filter(function (s) { return s.kind !== "event"; }).length;
    if (el("stCount")) el("stCount").textContent = shown + " de " + total;
  }
  function kindLabel(k) {
    return { start: "Início", end: "Fim", task: "Atividade", service: "Automação", gateway: "Gateway", event: "Evento" }[k] || k;
  }
  function svcChip(s) {
    if (!s) return '<span class="chip mech">automação</span>';
    var t = "tentativas " + (s.attempts || 1);
    if (s.frequency) t += " · a cada " + s.frequency + "min";
    return '<div class="who"><span class="chip mech">' + esc(t) + "</span></div>";
  }
  function condHTML(conds) {
    return conds.map(function (c) {
      var rules = (c.rules || []).map(function (r) {
        return '<span class="rule">' + esc(r.field) + " " + esc(r.operator) + (r.value ? ' "' + esc(r.value) + '"' : "") + "</span>";
      }).join(' <b>e</b> ');
      return '<div class="cond">→ etapa <b>' + c.to + "</b>" + (rules ? " quando " + rules : " (senão)") + "</div>";
    }).join("");
  }

  // constName deriva um nome de constante do nome da etapa (sem acento,
  // MAIÚSCULAS_COM_UNDERSCORE, prefixo ETAPA_).
  function constName(name) {
    var s = String(name || "").normalize("NFD").replace(/[\u0300-\u036f]/g, "");
    s = s.toUpperCase().replace(/[^A-Z0-9]+/g, "_").replace(/^_+|_+$/g, "");
    return "ETAPA_" + (s || "ETAPA");
  }
  // genConsts monta o bloco de constantes de WKNumState de todas as etapas
  // (sempre por número). Nomes repetidos (ex.: várias "Corrigir Integração")
  // ganham o sufixo do sequence. Inclui ETAPA_NOVO = 0 (registro recém-criado).
  function genConsts() {
    var ordered = stateRows.filter(function (s) { return s.kind !== "event"; })
      .sort(function (a, b) { return a.sequence - b.sequence; });
    var count = {};
    ordered.forEach(function (s) { var n = constName(s.name); count[n] = (count[n] || 0) + 1; });
    var entries = ordered.map(function (s) {
      var n = constName(s.name); if (count[n] > 1) n += "_" + s.sequence;
      return { decl: "const " + n + " = " + s.sequence + ";", label: s.name };
    });
    entries.unshift({ decl: "const ETAPA_NOVO = 0;", label: "registro recém-criado (antes de gravar)" });
    var w = 0; entries.forEach(function (e) { if (e.decl.length > w) w = e.decl.length; });
    var lines = ["// Etapas de " + currentId + (detail && detail.version ? " (v" + detail.version + ")" : "") + " — WKNumState"];
    entries.forEach(function (e) {
      var pad = ""; for (var i = e.decl.length; i < w + 2; i++) pad += " ";
      lines.push(e.decl + pad + "// " + e.label);
    });
    return lines.join("\n");
  }

  var zoom = 1;
  function applyZoom() {
    var inner = el("diagInner"); if (!inner) return;
    inner.style.width = Math.round(zoom * 100) + "%";
    if (el("zLbl")) el("zLbl").textContent = zoom === 1 ? "ajustado" : Math.round(zoom * 100) + "%";
  }
  function prepSVG() {
    var box = el("diagBox"); if (!box) return;
    var svg = box.querySelector("svg"); if (!svg) return;
    // Sem viewBox o SVG não escala ao mudar a largura — derivamos do width/height.
    if (!svg.getAttribute("viewBox")) {
      var w = parseInt(svg.getAttribute("width"), 10), h = parseInt(svg.getAttribute("height"), 10);
      if (w && h) svg.setAttribute("viewBox", "0 0 " + w + " " + h);
    }
    svg.removeAttribute("width"); svg.removeAttribute("height");
    zoom = 1; applyZoom();
  }

  function wireDetail(d) {
    // copiar id / snippets de etapa
    document.querySelectorAll("[data-copy]").forEach(function (n) {
      n.addEventListener("click", function () { copy(this.getAttribute("data-copy"), "Copiado para o clipboard"); });
    });
    var flt = el("stFilter");
    if (flt) flt.addEventListener("input", function () { drawStates(this.value); });
    // ordenação fluxo/número
    var seg = el("ordSeg");
    if (seg) seg.querySelectorAll("button").forEach(function (b) {
      b.addEventListener("click", function () {
        stOrder = this.getAttribute("data-ord"); saveUI({ order: stOrder });
        seg.querySelectorAll("button").forEach(function (x) { x.classList.toggle("on", x === b); });
        drawStates(flt ? flt.value : "");
      });
    });
    // copiar constantes
    var cc = el("copyConsts");
    if (cc) cc.addEventListener("click", function () { copy(genConsts(), stateRows.length + " constantes copiadas"); });
    // diagrama: zoom, ajustar, ocultar, arrastar para navegar
    prepSVG();
    var zi = el("zIn"), zo = el("zOut"), zf = el("zFit");
    if (zi) zi.addEventListener("click", function () { zoom = Math.min(4, Math.round((zoom + 0.25) * 100) / 100); applyZoom(); });
    if (zo) zo.addEventListener("click", function () { zoom = Math.max(0.5, Math.round((zoom - 0.25) * 100) / 100); applyZoom(); });
    if (zf) zf.addEventListener("click", function () { zoom = 1; applyZoom(); });
    var dt = el("diagToggle");
    if (dt) dt.addEventListener("click", function () {
      var box = el("diagBox"); var hidden = box.classList.toggle("collapsed");
      this.textContent = hidden ? "mostrar" : "ocultar";
    });
    var box = el("diagBox");
    if (box) {
      var pan = { on: false, x: 0, y: 0, sl: 0, st: 0 };
      box.addEventListener("mousedown", function (e) {
        pan.on = true; pan.x = e.clientX; pan.y = e.clientY; pan.sl = box.scrollLeft; pan.st = box.scrollTop;
        box.classList.add("grabbing");
      });
      window.addEventListener("mouseup", function () { pan.on = false; box.classList.remove("grabbing"); });
      box.addEventListener("mousemove", function (e) {
        if (!pan.on) return;
        box.scrollLeft = pan.sl - (e.clientX - pan.x); box.scrollTop = pan.st - (e.clientY - pan.y);
      });
    }
  }

  loadProcesses(false);
})();
</script>
</body>
</html>`
