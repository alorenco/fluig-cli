package devserver

// dashboardHTML é a página do dashboard (rota /). Self-contained e no mesmo
// design system do índice de formulários (claro/escuro, accent ciano); os
// dados vêm de /_dev/api/dash (15s), /_dev/api/status (60s, saúde do servidor
// via server status) e /_dev/api/audit/project (60s, resumo do linter).
//
// Layout (reorganizado a pedido do mantenedor, 2026-07-17): 1) status do
// servidor conectado com o ambiente em destaque (produção em cor de alerta);
// 2) tiles de acesso; 3) widgets SPA (só o acionável; some sem SPA);
// 4) resumo do audit; 5) watch integrado; 6) configurações. A antiga seção
// "Widgets servidas do disco" foi removida (o map-local segue igual).
const dashboardHTML = `<!doctype html>
<html lang="pt-BR">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>fluigcli dev</title>
<style>
:root{--bg:#f4f6f8;--card:#fff;--txt:#1d2b36;--sub:#5a6b7b;--line:#e3e8ee;
  --accent:#0c9abe;--accent-txt:#fff;--ok:#25b26e;--warn:#b3352b;--amber:#a06a00;
  --chipbg:#f7f9fb;--shadow:0 1px 2px rgba(16,36,54,.08)}
@media(prefers-color-scheme:dark){:root{--bg:#12181f;--card:#1b232d;
  --txt:#e6edf3;--sub:#93a4b4;--line:#2b3742;--chipbg:#161d26;--amber:#d9a03c;
  --shadow:0 1px 2px rgba(0,0,0,.4)}}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--txt);
  font:15px/1.5 system-ui,-apple-system,"Segoe UI",Roboto,sans-serif}
header{padding:26px 32px 18px;border-bottom:1px solid var(--line)}
header h1{margin:0;font-size:22px;font-weight:650}
header h1 small{color:var(--accent);font-weight:650}
header p{margin:7px 0 0;color:var(--sub);font-size:13.5px}
main{max-width:1080px;margin:0 auto;padding:26px 32px 48px}
section{margin-top:28px}
section:first-child{margin-top:0}
section>h2{font-size:12.5px;font-weight:650;letter-spacing:.06em;text-transform:uppercase;
  color:var(--sub);margin:0 0 10px}

.panel{background:var(--card);border:1px solid var(--line);border-radius:12px;
  padding:18px 20px;box-shadow:var(--shadow)}
.panel .row{display:flex;gap:12px;align-items:center;flex-wrap:wrap}
.panel label{font-size:13.5px;display:flex;gap:7px;align-items:center;cursor:pointer}
.panel .muted{color:var(--sub);font-size:12.5px}
.panel input[type=number]{width:90px;padding:5px 8px;border:1px solid var(--line);
  border-radius:7px;background:var(--bg);color:var(--txt)}
.panel button{padding:7px 14px;border:0;border-radius:8px;cursor:pointer;
  background:var(--accent);color:var(--accent-txt);font-weight:650;font-size:13px}
.panel button.sec{background:color-mix(in srgb,var(--txt) 8%,transparent);color:var(--txt)}
.panel button:disabled{opacity:.5;cursor:not-allowed}

/* status do servidor: uma linha de identificação + uma de números com hints
   no hover — compacto de propósito (pedido do mantenedor, 2026-07-17) */
.status{border-left:5px solid var(--accent);padding:14px 20px}
.status.prod{border-left-color:var(--warn)}
.envtag{display:inline-block;border-radius:7px;padding:2px 10px;font-size:12px;
  font-weight:750;letter-spacing:.05em;text-transform:uppercase;
  background:color-mix(in srgb,var(--accent) 14%,transparent);color:var(--accent)}
.status.prod .envtag{background:color-mix(in srgb,var(--warn) 16%,transparent);color:var(--warn)}
.srvhead{display:flex;gap:10px;align-items:baseline;flex-wrap:wrap}
.srvhead .name{font-weight:700;font-size:16px}
.srvhead .meta{color:var(--sub);font-size:12.5px}
.srvhead .ver{margin-left:auto;color:var(--sub);font-size:12.5px;cursor:default}
.srvhead .ver b{color:var(--txt);font-weight:650}
.statline{display:flex;gap:4px 18px;flex-wrap:wrap;align-items:center;
  margin-top:9px;font-size:13px;color:var(--sub)}
.statline:empty{display:none}
.statline .it{cursor:default;white-space:nowrap}
.statline .it b{color:var(--txt);font-weight:650}
.dot{display:inline-block;width:9px;height:9px;border-radius:50%;
  background:var(--line);margin-right:4px;cursor:default}
.dot.ok{background:var(--ok)}
.dot.fail{background:var(--warn)}
.monfail{color:var(--warn);font-weight:650;cursor:default}

/* ações principais: tiles horizontais compactos */
.tiles{display:grid;grid-template-columns:repeat(auto-fit,minmax(320px,1fr));gap:14px}
a.tile{display:flex;gap:16px;align-items:center;background:var(--card);
  border:1px solid var(--line);border-radius:12px;padding:18px 20px;
  text-decoration:none;color:var(--txt);box-shadow:var(--shadow);
  transition:transform .08s,border-color .08s}
a.tile:hover{transform:translateY(-2px);border-color:var(--accent)}
a.tile .icon{flex:0 0 46px;height:46px;border-radius:12px;display:flex;
  align-items:center;justify-content:center;font-size:22px;
  background:color-mix(in srgb,var(--accent) 10%,transparent)}
a.tile .name{font-weight:650;font-size:15.5px}
a.tile .meta{margin-top:2px;color:var(--sub);font-size:13px}
a.tile .go{margin-left:auto;color:var(--sub);font-size:18px;flex:0 0 auto;
  transition:transform .08s,color .08s}
a.tile:hover .go{color:var(--accent);transform:translateX(3px)}

/* widgets SPA e audit: cards lado a lado */
.projgrid{display:grid;grid-template-columns:repeat(auto-fit,minmax(380px,1fr));gap:14px}
.spaw{display:flex;gap:10px;align-items:baseline;flex-wrap:wrap;padding:7px 0;
  border-top:1px solid var(--line)}
.spaw:first-of-type{border-top:0;padding-top:0}
.spaw .code{font:600 13px/1.4 ui-monospace,Consolas,monospace;color:var(--accent)}
.pill{border-radius:999px;padding:2px 10px;font-size:11.5px;font-weight:600}
.pill.ok{background:color-mix(in srgb,var(--ok) 12%,transparent);color:var(--ok)}
.pill.warn{background:color-mix(in srgb,var(--warn) 12%,transparent);color:var(--warn)}
.pill.dim{background:color-mix(in srgb,var(--txt) 7%,transparent);color:var(--sub)}
.spaw .npm{font-size:12px;color:var(--sub)}
.auditnums{display:flex;gap:18px;align-items:baseline}
.auditnums .n{font-size:26px;font-weight:750}
.auditnums .n.err{color:var(--warn)}
.auditnums .n.warn{color:var(--amber)}
.auditnums .lbl{font-size:12px;color:var(--sub)}
.rules{margin:12px 0 0;padding:0;list-style:none;font-size:13px}
.rules li{display:flex;justify-content:space-between;gap:10px;padding:4px 0;
  border-top:1px solid var(--line)}
.rules li:first-child{border-top:0}
.rules .rc{font-weight:650;font-variant-numeric:tabular-nums}
.empty{color:var(--sub);font-size:13px}

.types{display:flex;gap:10px;flex-wrap:wrap;margin:14px 0 0}
.types label{border:1px solid var(--line);border-radius:999px;padding:7px 14px;
  transition:border-color .08s,background .08s}
.types label:hover{border-color:var(--accent)}
.types label.on{border-color:var(--accent);background:color-mix(in srgb,var(--accent) 8%,transparent);
  color:var(--accent);font-weight:600}
.recent{margin:14px 0 0;padding:12px 14px;list-style:none;border-radius:8px;
  background:var(--chipbg);border:1px solid var(--line);
  font:12.5px/1.8 ui-monospace,Consolas,monospace;color:var(--sub)}
.recent:empty{display:none}
.recent li{white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.note{margin-top:10px;font-size:12.5px;color:var(--sub)}
.note.err{color:var(--warn)}
.note:empty{display:none}

.switch{position:relative;width:38px;height:22px;flex:0 0 38px}
.switch input{opacity:0;width:0;height:0}
.switch span{position:absolute;inset:0;border-radius:999px;background:var(--line);transition:.15s}
.switch span:before{content:"";position:absolute;width:16px;height:16px;border-radius:50%;
  background:#fff;top:3px;left:3px;transition:.15s;box-shadow:0 1px 2px rgba(0,0,0,.3)}
.switch input:checked+span{background:var(--ok)}
.switch input:checked+span:before{left:19px}
</style>
</head>
<body>
<header>
  <h1>fluigcli <small>dev</small></h1>
  <p id="hdr">carregando…</p>
</header>
<main>
  <section>
    <h2>Servidor conectado</h2>
    <div class="panel status" id="statusPanel">
      <div class="srvhead">
        <span class="envtag" id="envTag">…</span>
        <span class="name" id="srvName"></span>
        <span class="meta" id="srvMeta"></span>
        <span class="ver" id="srvVer"></span>
      </div>
      <div class="statline" id="statLine"></div>
      <div class="note" id="statusNote"></div>
    </div>
  </section>

  <section>
    <h2>Acessos</h2>
    <div class="tiles">
      <a class="tile" id="portalTile" href="/portal/p/1/home">
        <div class="icon">🌐</div>
        <div><div class="name">Portal Fluig</div>
          <div class="meta">o portal real, pelo proxy autenticado</div></div>
        <div class="go">→</div>
      </a>
      <a class="tile" href="/_dev/forms/">
        <div class="icon">📄</div>
        <div><div class="name">Formulários</div>
          <div class="meta" id="formsMeta">preview com simulação de processo</div></div>
        <div class="go">→</div>
      </a>
      <a class="tile" href="/_dev/datasets/">
        <div class="icon">🗃️</div>
        <div><div class="name">Datasets</div>
          <div class="meta">consultar datasets e ver o resultado</div></div>
        <div class="go">→</div>
      </a>
      <a class="tile" href="/_dev/logs/">
        <div class="icon">📜</div>
        <div><div class="name">Logs do servidor</div>
          <div class="meta">server.log ao vivo, com filtros</div></div>
        <div class="go">→</div>
      </a>
    </div>
  </section>

  <section>
    <h2>Projeto</h2>
    <div class="projgrid">
      <div class="panel" id="spaPanel" style="display:none">
        <div class="row" style="margin-bottom:10px">
          <span style="font-weight:650;font-size:14px">Widgets SPA</span>
          <span style="flex:1"></span>
          <label class="switch" title="Compila o bundle a cada save (npm run watch por widget) — liga e desliga sem reiniciar o dev"><input type="checkbox" id="npmOn"><span></span></label>
          <span class="muted">npm watch</span>
        </div>
        <div id="spaList"></div>
        <div class="note" id="spaNote"></div>
      </div>
      <div class="panel" id="auditPanel">
        <div style="font-weight:650;font-size:14px;margin-bottom:10px">Style guide (fluigcli audit)</div>
        <div id="auditBody" class="empty">auditando o projeto…</div>
      </div>
    </div>
  </section>

  <section id="watchSection" style="display:none">
    <h2>Watch integrado — publicar ao salvar</h2>
    <div class="panel">
      <div class="row">
        <label class="switch"><input type="checkbox" id="watchOn"><span></span></label>
        <div>
          <div style="font-weight:650;font-size:14px">Publicar no servidor conectado ao salvar</div>
          <div class="muted">Só atualiza o que já existe (nunca cria); formulários com a versão mantida; scripts de processo cirúrgicos.</div>
        </div>
      </div>
      <div class="types" id="watchTypes"></div>
      <div class="note" id="watchNote"></div>
      <ul class="recent" id="watchRecent"></ul>
    </div>
  </section>

  <section>
    <h2>Configurações</h2>
    <div class="panel">
      <div class="row">
        <label class="switch"><input type="checkbox" id="reloadOn" checked><span></span></label>
        <div style="font-weight:650;font-size:14px">Live reload</div>
        <label class="muted" style="cursor:default">debounce
          <input type="number" id="debounce" min="50" max="10000" step="50"> ms
        </label>
        <button class="sec" id="reloadApply">Aplicar</button>
        <span style="flex:1"></span>
        <button class="sec" id="clearCaches" title="Zera os caches do painel de simulação (contexto, processos, etapas, usuários), o resumo do audit e as conexões de publicação">Limpar caches do painel</button>
      </div>
      <div class="note" id="cfgNote"></div>
    </div>
  </section>
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
  function note(id, msg, isErr) {
    el(id).textContent = msg || "";
    el(id).className = isErr ? "note err" : "note";
  }

  var state = null;

  function fmtUptime(s) {
    if (s < 90) return s + "s";
    var m = Math.round(s / 60);
    if (m < 90) return m + " min";
    var h = Math.round(m / 6) / 10;
    if (h < 48) return h + " h";
    return Math.round(h / 24) + " dias";
  }
  function fmtGB(b) {
    if (!b && b !== 0) return "–";
    return (Math.round(b / (1 << 30) * 10) / 10) + " GB";
  }

  // --- status do servidor (/_dev/api/status, refresh próprio de 60s) ---
  // Compacto de propósito: uma linha de identificação + uma de números;
  // o detalhe fica nos hints (title) de cada item.
  var envLabels = { prod: "Produção", hml: "Homologação", dev: "Desenvolvimento" };

  function esc(s) {
    return String(s == null ? "" : s).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/"/g, "&quot;");
  }
  function it(html, tip) {
    return "<span class=\"it\" title=\"" + esc(tip) + "\">" + html + "</span>";
  }

  function renderStatus(st) {
    var srv = st.server || {};
    var env = (srv.env || "").toLowerCase();
    el("statusPanel").className = "panel status" + (env === "prod" ? " prod" : "");
    el("envTag").textContent = envLabels[env] || env || "ambiente?";
    el("srvName").textContent = srv.name || "?";
    el("srvMeta").textContent = (srv.url || "") + " · " + (srv.user || "?");
    // "TOTVS Fluig Plataforma - Voyager 2.0.0-260707" → mostra só o codinome
    // e a versão; a string completa fica no hint.
    var ver = el("srvVer");
    if (st.version) {
      ver.innerHTML = "Fluig <b>" + esc(st.version.replace(/^.*?- /, "")) + "</b>";
      ver.title = st.version;
    } else {
      ver.textContent = st.versionError ? "versão indisponível" : "";
      ver.title = st.versionError || "";
    }

    var s = st.stats, parts = [];
    if (s) {
      parts.push(
        it("no ar há <b>" + fmtUptime(Math.round((s.uptimeMillis || 0) / 1000)) + "</b>", "uptime do servidor Fluig"),
        it("<b>" + s.connectedUsers + "</b> usuários", "usuários conectados agora"),
        it("<b>" + s.threadCount + "</b> threads", "threads ativas (pico " + s.threadPeak + ")"),
        it("JVM <b>" + fmtGB(s.heapUsedBytes) + "</b>", "memória JVM: " + fmtGB(s.heapUsedBytes) + " heap + " + fmtGB(s.nonHeapUsedBytes) + " non-heap"),
        it("SO <b>" + fmtGB(s.osMemoryFreeBytes) + "</b> livres", "memória do sistema operacional: " + fmtGB(s.osMemoryFreeBytes) + " livres de " + fmtGB(s.osMemoryTotalBytes)),
        it("<b>" + esc(s.databaseName) + "</b> " + fmtGB(s.databaseSizeBytes), "banco " + s.databaseName + " " + s.databaseVersion + " · tamanho " + fmtGB(s.databaseSizeBytes)));
    }
    // Monitores viram pontos coloridos (hint = nome/status); só FAILURE
    // ganha o nome à mostra.
    var monHTML = "", fails = [];
    (st.monitors || []).forEach(function (m) {
      var cls = m.status === "OK" ? " ok" : m.status === "FAILURE" ? " fail" : "";
      monHTML += "<span class=\"dot" + cls + "\" title=\"" + esc(m.name + " — " + m.status +
        (m.status === "NONE" ? " (não configurado)" : " · sucesso " + Math.round(m.successRate) + "%")) + "\"></span>";
      if (m.status === "FAILURE") fails.push(m.name);
    });
    if (monHTML) {
      parts.push("<span class=\"it\" title=\"monitores de serviço do servidor — passe o mouse em cada ponto\">" + monHTML + "</span>");
      fails.forEach(function (n) { parts.push("<span class=\"monfail\" title=\"monitor em falha\">" + esc(n) + " ✗</span>"); });
    }
    // Estado do fluigcliHelper (componente auxiliar): versão quando instalado;
    // ausente/desatualizado vira destaque com a orientação no hint.
    if (st.helper) {
      if (st.helper.installed && st.helper.version) {
        parts.push(it("helper <b>v" + esc(st.helper.version) + "</b>",
          "componente auxiliar fluigcliHelper v" + st.helper.version + " instalado no servidor"));
      } else if (st.helper.installed) {
        parts.push("<span class=\"monfail\" title=\"fluigcliHelper instalado, mas sem versão conhecida — reinstale com: fluigcli server install-helper --force\">helper desatualizado ✗</span>");
      } else {
        parts.push("<span class=\"monfail\" title=\"componente auxiliar fluigcliHelper não instalado — widget import e workflow export dependem dele; rode: fluigcli server install-helper\">helper ausente ✗</span>");
      }
    }
    el("statLine").innerHTML = parts.join("");

    var problems = [];
    if (st.unavailable) problems.push(st.unavailable);
    if (st.statsError) problems.push("estatísticas: " + st.statsError);
    if (st.monitorsError && st.monitorsError !== st.statsError) problems.push("monitores: " + st.monitorsError);
    note("statusNote", problems.join(" · "), problems.length > 0);
  }

  function loadStatus() {
    api("GET", "/_dev/api/status", null, function (err, data) {
      if (err) { note("statusNote", "status indisponível: " + err, true); return; }
      renderStatus(data);
    });
  }

  // --- resumo do audit (/_dev/api/audit/project) ---
  function renderAudit(a) {
    var c = a.counts || {};
    var html = "<div class=\"auditnums\">" +
      "<span><span class=\"n" + (c.error ? " err" : "") + "\">" + (c.error || 0) + "</span> <span class=\"lbl\">erros</span></span>" +
      "<span><span class=\"n" + (c.warning ? " warn" : "") + "\">" + (c.warning || 0) + "</span> <span class=\"lbl\">avisos</span></span>" +
      "<span class=\"lbl\" style=\"margin-left:auto\">" + a.scanned + " arquivo(s) auditado(s)" +
      (a.ignored ? " · " + a.ignored + " ignorado(s)" : "") + "</span></div>";
    var rules = (a.rules || []).slice(0, 5);
    if (rules.length) {
      html += "<ul class=\"rules\">";
      rules.forEach(function (r) {
        // O hint (hover) explica a regra — a explicação vem do linter.
        html += "<li title=\"" + esc(r.title || "") + "\"><span>" + esc(r.rule) +
          " <span class=\"lbl\">(" + (r.severity === "error" ? "erro" : "aviso") + ")</span></span>" +
          "<span class=\"rc\">" + r.count + "</span></li>";
      });
      html += "</ul>";
    } else {
      html += "<div class=\"empty\" style=\"margin-top:10px\">Nenhum achado — projeto em dia com o style guide. ✓</div>";
    }
    html += "<div class=\"note\">Detalhes por arquivo: <code>fluigcli audit</code> (ou o botão 🎨 no preview de cada formulário).</div>";
    el("auditBody").className = "";
    el("auditBody").innerHTML = html;
  }

  function loadAudit() {
    api("GET", "/_dev/api/audit/project", null, function (err, data) {
      if (err) {
        el("auditBody").className = "empty";
        el("auditBody").textContent = "audit indisponível: " + err;
        return;
      }
      renderAudit(data);
    });
  }

  // --- widgets SPA (payload do dash) ---
  function renderSPA() {
    var spa = state.spa || [];
    var panel = el("spaPanel");
    if (!spa.length) { panel.style.display = "none"; return; }
    panel.style.display = "";
    el("npmOn").checked = !!state.npmWatch;
    var box = el("spaList");
    box.innerHTML = "";
    spa.forEach(function (w) {
      var row = document.createElement("div");
      row.className = "spaw";
      var code = document.createElement("span");
      code.className = "code";
      code.textContent = w.code;
      row.appendChild(code);
      var pill = document.createElement("span");
      pill.className = "pill " + (w.stale ? "warn" : "ok");
      pill.textContent = w.stale ? "bundle desatualizado" : "bundle em dia";
      if (w.stale) pill.title = w.stale;
      row.appendChild(pill);
      var npm = document.createElement("span");
      npm.className = "npm";
      npm.textContent = "npm watch: " + (w.npm || "?");
      row.appendChild(npm);
      box.appendChild(row);
    });
  }

  function renderWatch() {
    var wsec = el("watchSection");
    if (!state.watch) { wsec.style.display = "none"; return; }
    wsec.style.display = "";
    var w = state.watch;
    el("watchOn").checked = !!w.enabled;
    var box = el("watchTypes");
    box.innerHTML = "";
    (state.watchTypes || []).forEach(function (t) {
      var on = (w.types || []).indexOf(t.id) >= 0;
      var lab = document.createElement("label");
      if (on) lab.className = "on";
      var chk = document.createElement("input");
      chk.type = "checkbox";
      chk.checked = on;
      chk.setAttribute("data-type", t.id);
      lab.appendChild(chk);
      lab.appendChild(document.createTextNode(t.label));
      box.appendChild(lab);
    });
    if (!w.available) {
      note("watchNote", "Nenhuma pasta da convenção (datasets/, events/, mechanisms/, forms/, workflow/scripts/) no projeto.", true);
    } else if (w.enabled && (!w.types || !w.types.length)) {
      note("watchNote", "Escolha ao menos um tipo de artefato para o watch publicar.", true);
    } else {
      note("watchNote", "");
    }
    var rec = el("watchRecent");
    rec.innerHTML = "";
    (w.recent || []).forEach(function (m) {
      var li = document.createElement("li");
      li.textContent = m;
      li.title = m;
      rec.appendChild(li);
    });
  }

  function render() {
    el("hdr").textContent = "dev server no ar há " + fmtUptime(state.uptimeSeconds || 0) +
      " · live reload, preview de formulários e proxy autenticado";
    el("portalTile").setAttribute("href", state.portalPath || "/");
    el("formsMeta").textContent = (state.formsCount || 0) +
      " formulário(s) no projeto, com preview e simulação de processo";
    renderSPA();
    renderWatch();
    el("reloadOn").checked = !!(state.reload && state.reload.enabled);
    el("debounce").value = state.reload ? state.reload.debounceMs : 500;
  }

  function load() {
    api("GET", "/_dev/api/dash", null, function (err, data) {
      if (err) { el("hdr").textContent = "dashboard indisponível: " + err; return; }
      state = data;
      render();
    });
  }

  function saveWatch() {
    var types = [];
    el("watchTypes").querySelectorAll("input[data-type]").forEach(function (c) {
      if (c.checked) types.push(c.getAttribute("data-type"));
    });
    api("POST", "/_dev/api/dash/watch",
      { enabled: el("watchOn").checked, types: types },
      function (err, data) {
        if (err) { note("watchNote", "Falha ao salvar: " + err, true); return; }
        state.watch = data;
        renderWatch();
      });
  }

  document.addEventListener("change", function (ev) {
    var t = ev.target;
    if (t === el("watchOn") || (t.getAttribute && t.getAttribute("data-type"))) saveWatch();
  });
  el("npmOn").addEventListener("change", function () {
    api("POST", "/_dev/api/dash/npm-watch", { enabled: el("npmOn").checked }, function (err, data) {
      if (err) {
        note("spaNote", "Falha: " + err, true);
        el("npmOn").checked = !!state.npmWatch;
        return;
      }
      state.npmWatch = data.enabled;
      note("spaNote", data.enabled ? "npm watch ligado — compilando a cada save." : "npm watch desligado (processos encerrados).");
      setTimeout(load, 1500); // o estado dos spawns aparece no próximo dash
    });
  });
  el("reloadApply").addEventListener("click", function () {
    api("POST", "/_dev/api/dash/reload",
      { enabled: el("reloadOn").checked, debounceMs: parseInt(el("debounce").value, 10) || 500 },
      function (err, data) {
        if (err) { note("cfgNote", "Falha: " + err, true); return; }
        state.reload = data;
        note("cfgNote", "Live reload " + (data.enabled ? "ativo" : "pausado") + " · debounce " + data.debounceMs + "ms");
      });
  });
  el("clearCaches").addEventListener("click", function () {
    api("POST", "/_dev/api/dash/clear-caches", {}, function (err) {
      note("cfgNote", err ? "Falha: " + err : "Caches do painel limpos.", !!err);
      if (!err) { loadStatus(); loadAudit(); }
    });
  });

  load();
  loadStatus();
  loadAudit();
  setInterval(load, 15000);        // feed do watch, SPA e uptime frescos
  setInterval(loadStatus, 60000);  // saúde do servidor (cadência própria)
  setInterval(loadAudit, 60000);   // resumo do audit (cache invalida no save)
})();
</script>
</body>
</html>
`
