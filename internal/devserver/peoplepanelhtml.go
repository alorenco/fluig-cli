package devserver

// peoplePanelHTML é a página da subtela Pessoas (/_dev/people/). Self-contained
// e no mesmo design system do Explorador de Processos / Dataset Lab (claro/
// escuro, accent ciano). Dados via /_dev/api/people/*. Sem template literals no
// JS: a página é uma raw string Go delimitada por backticks — o JS concatena
// strings.
const peoplePanelHTML = `<!doctype html>
<html lang="pt-BR">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>fluigcli dev — pessoas</title>
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
.btn:disabled{opacity:.5;cursor:default;filter:none}
.btn.ghost{background:transparent;border:1px solid var(--line);color:var(--txt)}
.btn.ghost:hover{border-color:var(--accent);color:var(--accent)}
.btn.danger{background:var(--warn)}
.icobtn{background:transparent;border:1px solid var(--line);color:var(--sub);border-radius:8px;
  width:36px;height:36px;cursor:pointer;font-size:15px;transition:border-color .08s,color .08s}
.icobtn:hover{border-color:var(--accent);color:var(--accent)}

/* abas */
.tabs{display:flex;gap:8px;flex-wrap:wrap;margin-bottom:2px}
.tab{padding:9px 18px;border:1px solid var(--line);border-radius:999px;background:var(--card);
  color:var(--sub);cursor:pointer;font-size:13.5px;font-weight:600;transition:all .08s}
.tab:hover{border-color:var(--accent);color:var(--accent)}
.tab.on{background:var(--accent);color:var(--accent-txt);border-color:var(--accent)}
.tab .n{opacity:.7;font-weight:400;margin-left:6px}

/* barra de busca / filtros */
.bar{display:flex;gap:10px;align-items:center;flex-wrap:wrap;margin:18px 0 4px}
.bar input[type=search]{flex:1;min-width:260px;max-width:460px;padding:10px 14px;border:1px solid var(--line);
  border-radius:9px;background:var(--card);color:var(--txt);font-size:14.5px;outline:none}
.bar input[type=search]:focus{border-color:var(--accent)}
.bar .count{color:var(--sub);font-size:13px}
.bar .grow{flex:1}
.chk{display:inline-flex;align-items:center;gap:7px;color:var(--sub);font-size:13px;cursor:pointer;user-select:none}
.chk input{width:15px;height:15px;accent-color:var(--accent)}

/* tabela */
.tablewrap{overflow:auto;border:1px solid var(--line);border-radius:12px;margin-top:12px}
table{border-collapse:collapse;width:100%;font-size:13px}
thead th{position:sticky;top:0;background:var(--card);text-align:left;font-weight:700;
  padding:9px 12px;border-bottom:2px solid var(--line);white-space:nowrap;z-index:1}
tbody td{padding:8px 12px;border-bottom:1px solid var(--line);vertical-align:middle}
tbody tr.row{cursor:pointer}
tbody tr.row:hover{background:color-mix(in srgb,var(--accent) 7%,transparent)}
tbody tr:nth-child(even of .row){background:var(--zebra)}
.mono{font-family:ui-monospace,Consolas,monospace}
.tbadge{font-size:10.5px;font-weight:700;letter-spacing:.03em;padding:1px 7px;border-radius:5px;white-space:nowrap}
.tbadge.on{background:color-mix(in srgb,var(--ok) 16%,transparent);color:var(--ok)}
.tbadge.off{background:color-mix(in srgb,var(--warn) 14%,transparent);color:var(--warn)}
.tbadge.you{background:color-mix(in srgb,var(--accent) 15%,transparent);color:var(--accent)}
.tbadge.type{background:color-mix(in srgb,var(--sub) 16%,transparent);color:var(--sub)}
.sub{color:var(--sub);font-size:11.5px}
td.blocked{color:var(--warn)}
/* links dentro das tabelas (ex.: processos do "onde é usado?") — accent, não o
   azul padrão do navegador que some no fundo escuro */
.tablewrap a{color:var(--accent);text-decoration:none;font-weight:600}
.tablewrap a:hover{text-decoration:underline}
/* célula de código (userCode) copiável na lista de usuários */
.codecell{white-space:nowrap;cursor:copy}
.codecell:hover{color:var(--accent)}
.cpyic{opacity:.45;font-size:11px;margin-left:2px}
.codecell:hover .cpyic{opacity:1}

/* card de detalhe */
.card{background:var(--card);border:1px solid var(--line);border-radius:12px;
  padding:18px 20px;box-shadow:var(--shadow);margin-top:18px}
.card h2{margin:0 0 6px;font-size:18px;font-weight:650;display:flex;align-items:center;gap:10px;flex-wrap:wrap}
.card .code{font:600 13px/1.4 ui-monospace,Consolas,monospace;color:var(--accent);
  border:1px solid var(--line);border-radius:6px;padding:2px 8px;background:var(--chipbg);cursor:pointer}
.card .code:hover{border-color:var(--accent)}
.card .actions{display:flex;gap:10px;align-items:center;flex-wrap:wrap;margin:14px 0}
.card .close{margin-left:auto;background:transparent;border:0;color:var(--sub);cursor:pointer;font-size:20px;line-height:1}
.card .close:hover{color:var(--accent)}
.chips{display:flex;flex-wrap:wrap;gap:6px;margin-top:4px}
.chip{border:1px solid var(--line);border-radius:999px;padding:3px 12px;font-size:12.5px;
  background:var(--chipbg);cursor:pointer;transition:border-color .08s,color .08s}
.chip:hover{border-color:var(--accent);color:var(--accent)}
.sectitle{font-size:11.5px;letter-spacing:.05em;text-transform:uppercase;color:var(--sub);
  font-weight:700;margin:16px 0 6px}

.state-msg{padding:50px 0;text-align:center;color:var(--sub)}
.state-msg.err{color:var(--warn)}
.state-msg code{background:var(--chipbg);border:1px solid var(--line);border-radius:5px;padding:2px 7px}
.banner{background:color-mix(in srgb,var(--amber) 14%,transparent);border:1px solid var(--amber);
  color:var(--txt);border-radius:10px;padding:14px 18px;margin-top:18px;font-size:13.5px}
.banner b{color:var(--amber)}
.spin{display:inline-block;width:15px;height:15px;border:2px solid color-mix(in srgb,var(--accent) 30%,transparent);
  border-top-color:var(--accent);border-radius:50%;animation:sp .7s linear infinite;vertical-align:-2px}
@keyframes sp{to{transform:rotate(360deg)}}
.toast{position:fixed;bottom:24px;left:50%;transform:translateX(-50%) translateY(20px);
  background:var(--txt);color:var(--bg);padding:9px 18px;border-radius:999px;font-size:13px;font-weight:600;
  opacity:0;pointer-events:none;transition:opacity .18s,transform .18s;z-index:50}
.toast.show{opacity:1;transform:translateX(-50%) translateY(0)}

/* modal de confirmação */
.mask{position:fixed;inset:0;background:rgba(8,14,20,.5);display:none;align-items:center;justify-content:center;z-index:60}
.mask.open{display:flex}
.modal{background:var(--card);border:1px solid var(--line);border-radius:14px;max-width:440px;width:90%;
  padding:22px 24px;box-shadow:0 18px 50px rgba(8,14,20,.4)}
.modal h3{margin:0 0 10px;font-size:16px}
.modal p{margin:0 0 18px;color:var(--sub);font-size:13.5px}
.modal .mrow{display:flex;gap:10px;justify-content:flex-end}
.hidden{display:none!important}
</style>
</head>
<body>
<header>
  <div class="hrow">
    <div>
      <h1>fluigcli <small>dev</small> · Pessoas</h1>
      <p>Usuários, grupos e papéis da plataforma — e quem participa de cada um. Inclua/remova você mesmo dos grupos/papéis e veja onde eles atuam nos processos.</p>
    </div>
    <a class="back" href="/">← Dashboard</a>
  </div>
</header>
<main>
  <div class="tabs">
    <button class="tab on" data-tab="users">👤 Usuários<span class="n" id="nUsers"></span></button>
    <button class="tab" data-tab="groups">👥 Grupos<span class="n" id="nGroups"></span></button>
    <button class="tab" data-tab="roles">🎭 Papéis<span class="n" id="nRoles"></span></button>
  </div>

  <div class="bar">
    <input id="q" type="search" placeholder="Filtrar…" autocomplete="off" spellcheck="false">
    <label class="chk hidden" id="commWrap"><input type="checkbox" id="showComm"> mostrar grupos de comunidade</label>
    <label class="chk hidden" id="blockedWrap"><input type="checkbox" id="showBlocked"> mostrar bloqueados</label>
    <span class="grow"></span>
    <span class="count" id="count"></span>
    <button class="icobtn" id="reload" title="Recarregar (ignora cache)">↻</button>
  </div>

  <div id="banner"></div>
  <div id="list"><div class="state-msg">Carregando… <span class="spin"></span></div></div>
  <div id="detail"></div>
</main>

<div class="mask" id="mask">
  <div class="modal">
    <h3 id="mTitle">Confirmar</h3>
    <p id="mBody"></p>
    <div class="mrow">
      <button class="btn ghost" id="mCancel">Cancelar</button>
      <button class="btn" id="mOk">Confirmar</button>
    </div>
  </div>
</div>
<div class="toast" id="toast"></div>
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
  function esc(s) { return String(s == null ? "" : s).replace(/[&<>"]/g, function (c) {
    return { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]; }); }
  function toast(msg) {
    var t = el("toast"); t.textContent = msg; t.classList.add("show");
    clearTimeout(t._t); t._t = setTimeout(function () { t.classList.remove("show"); }, 1500);
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
  // confirm() modal (retorna via callback; confirm nativo não combina com o design).
  function confirmModal(title, bodyHTML, okLabel, danger, cb) {
    el("mTitle").textContent = title;
    el("mBody").innerHTML = bodyHTML;
    var ok = el("mOk");
    ok.textContent = okLabel || "Confirmar";
    ok.className = "btn" + (danger ? " danger" : "");
    el("mask").classList.add("open");
    function close() { el("mask").classList.remove("open"); ok.onclick = null; el("mCancel").onclick = null; }
    ok.onclick = function () { close(); cb(true); };
    el("mCancel").onclick = function () { close(); cb(false); };
  }
  el("mask").addEventListener("click", function (e) { if (e.target === this) this.classList.remove("open"); });

  var TABS = ["users", "groups", "roles"];
  var state = {
    tab: "users", me: "",
    users: null, groups: null, roles: null,      // caches por aba (null = não carregado)
    needsAdmin: false,
    detailKind: "", detailCode: "", detailLogin: ""
  };

  // ---- roteamento por aba + deep-link ----
  function setTab(tab, skipLoad) {
    if (TABS.indexOf(tab) < 0) tab = "users";
    state.tab = tab;
    document.querySelectorAll(".tab").forEach(function (b) { b.classList.toggle("on", b.getAttribute("data-tab") === tab); });
    el("commWrap").classList.toggle("hidden", tab !== "groups");
    el("blockedWrap").classList.toggle("hidden", tab !== "users");
    el("q").placeholder = { users: "Filtrar por nome, login, código ou e-mail…", groups: "Filtrar por código ou descrição…", roles: "Filtrar por código ou descrição…" }[tab];
    el("q").value = "";
    if (!skipLoad) { clearDetail(); load(false); }
  }
  document.querySelectorAll(".tab").forEach(function (b) {
    b.addEventListener("click", function () { setTab(this.getAttribute("data-tab")); pushLink(); });
  });
  el("q").addEventListener("input", function () { render(); });
  el("showComm").addEventListener("change", function () { render(); });
  el("showBlocked").addEventListener("change", function () { render(); });
  el("reload").addEventListener("click", function () { load(true); });

  function pushLink(extra) {
    if (!history.replaceState) return;
    var q = extra || "";
    history.replaceState(null, "", q ? ("?" + q) : location.pathname);
  }

  // ---- carga de listas ----
  function load(force) {
    var tab = state.tab;
    if (state[tab] && !force) { render(); return; }
    el("list").innerHTML = '<div class="state-msg">Carregando… <span class="spin"></span></div>';
    el("banner").innerHTML = "";
    api("GET", "/_dev/api/people/" + tab + (force ? "?force=1" : ""), null, function (err, data) {
      if (err) { el("list").innerHTML = '<div class="state-msg err">' + esc(err) + '</div>'; return; }
      if (data && data.needsAdmin) { state.needsAdmin = true; showAdminBanner(data.error); return; }
      state.me = (data && data.me) || state.me;
      state[tab] = (data && (data.users || data.groups || data.roles)) || [];
      render();
    });
  }
  function showAdminBanner(msg) {
    el("list").innerHTML = "";
    el("banner").innerHTML = '<div class="banner"><b>Sem privilégio administrativo.</b> A subtela Pessoas usa as APIs <code>/admin/api/v1</code>, que ' +
      'exigem um usuário com o papel admin no servidor. ' + (msg ? "(" + esc(msg) + ") " : "") +
      'Peça o papel ao administrador do Fluig e recarregue.</div>';
  }

  // ---- render da aba ----
  function matchText(hay, q) { return hay.toLowerCase().indexOf(q) >= 0; }
  function render() {
    if (state.needsAdmin) return;
    var tab = state.tab, q = el("q").value.trim().toLowerCase();
    var rows = state[tab] || [];
    var html = "";
    if (tab === "users") {
      // Por padrão só ativos; o checkbox "mostrar bloqueados" inclui os demais.
      var showBlk = el("showBlocked").checked;
      var base = rows.filter(function (u) { return showBlk || (u.state || "").toUpperCase() === "ACTIVE"; });
      var items = base.filter(function (u) {
        return !q || matchText(u.fullName || "", q) || matchText(u.login || "", q) || matchText(u.email || "", q) || matchText(u.code || "", q);
      });
      html += '<div class="tablewrap"><table><thead><tr><th>Nome</th><th>Login</th><th>Código (userCode)</th><th>E-mail</th><th>Estado</th></tr></thead><tbody>';
      items.forEach(function (u) {
        var you = u.login === state.me;
        var codeCell = u.code
          ? '<td class="mono codecell" data-copy="' + esc(u.code) + '" title="Copiar o código (userCode)">' + esc(u.code) + '<span class="cpyic">📋</span></td>'
          : '<td><span class="sub">—</span></td>';
        html += '<tr class="row" data-login="' + esc(u.login) + '"><td>' + esc(u.fullName || u.login) +
          (you ? ' <span class="tbadge you">você</span>' : "") + '</td>' +
          '<td class="mono">' + esc(u.login) + "</td>" + codeCell + "<td>" + esc(u.email || "") + "</td>" +
          "<td>" + stateBadge(u.state) + "</td></tr>";
      });
      html += "</tbody></table></div>";
      setCount(items.length, base.length, "usuário");
    } else if (tab === "groups") {
      var showComm = el("showComm").checked;
      var g1 = rows.filter(function (g) { return showComm || (g.type || "").toLowerCase() !== "community"; });
      var items2 = g1.filter(function (g) { return !q || matchText(g.code || "", q) || matchText(g.description || "", q); });
      html += '<div class="tablewrap"><table><thead><tr><th>Código</th><th>Descrição</th><th>Tipo</th></tr></thead><tbody>';
      items2.forEach(function (g) {
        html += '<tr class="row" data-code="' + esc(g.code) + '"><td class="mono">' + esc(g.code) + "</td><td>" +
          esc(g.description || "") + '</td><td><span class="tbadge type">' + esc(g.type || "user") + "</span></td></tr>";
      });
      html += "</tbody></table></div>";
      setCount(items2.length, g1.length, "grupo");
    } else {
      var items3 = rows.filter(function (r) { return !q || matchText(r.code || "", q) || matchText(r.description || "", q); });
      html += '<div class="tablewrap"><table><thead><tr><th>Código</th><th>Descrição</th></tr></thead><tbody>';
      items3.forEach(function (r) {
        html += '<tr class="row" data-code="' + esc(r.code) + '"><td class="mono">' + esc(r.code) + "</td><td>" +
          esc(r.description || "") + "</td></tr>";
      });
      html += "</tbody></table></div>";
      setCount(items3.length, rows.length, "papel", "papéis");
    }
    el("list").innerHTML = html;
    wireRows();
  }
  function setCount(shown, total, sing, plur) {
    var word = shown === 1 ? sing : (plur || sing + "s");
    el("count").textContent = shown === total ? (shown + " " + word) : (shown + " de " + total + " " + word);
    var idmap = { users: "nUsers", groups: "nGroups", roles: "nRoles" };
    // Contadores das abas (total, sem filtro): setados na primeira carga de cada aba.
    var counts = { users: state.users, groups: state.groups, roles: state.roles };
    TABS.forEach(function (t) { if (counts[t]) el(idmap[t]).textContent = counts[t].length; });
  }
  function stateBadge(st) {
    if ((st || "").toUpperCase() === "ACTIVE") return '<span class="tbadge on">ativo</span>';
    return '<span class="tbadge off">' + esc((st || "?").toLowerCase()) + "</span>";
  }
  function wireRows() {
    document.querySelectorAll("tr.row").forEach(function (tr) {
      tr.addEventListener("click", function () {
        if (state.tab === "users") openUser(this.getAttribute("data-login"));
        else openGroupOrRole(state.tab === "groups" ? "group" : "role", this.getAttribute("data-code"));
      });
    });
    // Copiar o código sem abrir o detalhe da linha (stopPropagation).
    document.querySelectorAll("#list [data-copy]").forEach(function (n) {
      n.addEventListener("click", function (e) { e.stopPropagation(); copy(this.getAttribute("data-copy"), "Código copiado"); });
    });
  }

  // ---- detalhe de usuário (visão reversa) ----
  function clearDetail() { state.detailKind = ""; state.detailCode = ""; state.detailLogin = ""; el("detail").innerHTML = ""; }
  function openUser(login) {
    if (!login) return;
    state.detailKind = "user"; state.detailLogin = login;
    el("detail").innerHTML = '<div class="card"><div class="state-msg">Carregando <b>' + esc(login) + '</b>… <span class="spin"></span></div></div>';
    scrollDetail();
    api("GET", "/_dev/api/people/user?login=" + encodeURIComponent(login), null, function (err, data) {
      if (err) { el("detail").innerHTML = '<div class="card"><div class="state-msg err">' + esc(err) + "</div></div>"; return; }
      if (data && data.needsAdmin) { showAdminBanner(data.error); el("detail").innerHTML = ""; return; }
      renderUser(data.user);
    });
  }
  function renderUser(u) {
    if (!u) { el("detail").innerHTML = ""; return; }
    var you = u.login === state.me;
    var h = '<div class="card"><h2>' + esc(u.fullName || u.login) + (you ? ' <span class="tbadge you">você</span>' : "") +
      " " + stateBadge(u.state) + '<button class="close" title="Fechar">×</button></h2>';
    h += '<div class="chips"><span class="chip" data-copy="' + esc(u.login) + '" title="Copiar o login">login: ' + esc(u.login) + "</span>";
    if (u.code) h += '<span class="chip" data-copy="' + esc(u.code) + '" title="Copiar o userCode">código: ' + esc(u.code) + "</span>";
    if (u.email) h += '<span class="chip" data-copy="' + esc(u.email) + '">' + esc(u.email) + "</span>";
    h += "</div>";
    h += '<div class="sectitle">Grupos (' + ((u.groups || []).length) + ")</div>";
    h += chipsFor("group", u.groups);
    h += '<div class="sectitle">Papéis (' + ((u.roles || []).length) + ")</div>";
    h += chipsFor("role", u.roles);
    h += "</div>";
    el("detail").innerHTML = h;
    wireDetail();
  }
  function chipsFor(kind, arr) {
    if (!arr || !arr.length) return '<div class="sub">nenhum</div>';
    return '<div class="chips">' + arr.map(function (c) {
      return '<span class="chip" data-go="' + kind + "|" + esc(c) + '" title="Ver participantes">' + esc(c) + "</span>";
    }).join("") + "</div>";
  }

  // ---- detalhe de grupo/papel (membros + incluir-me + onde é usado) ----
  function openGroupOrRole(kind, code) {
    if (!code) return;
    state.detailKind = kind; state.detailCode = code;
    var label = kind === "group" ? "grupo" : "papel";
    el("detail").innerHTML = '<div class="card"><div class="state-msg">Carregando o ' + label + ' <b>' + esc(code) + '</b>… <span class="spin"></span></div></div>';
    scrollDetail();
    api("GET", "/_dev/api/people/members?kind=" + kind + "&code=" + encodeURIComponent(code), null, function (err, data) {
      if (err) { el("detail").innerHTML = '<div class="card"><div class="state-msg err">' + esc(err) + "</div></div>"; return; }
      if (data && data.needsAdmin) { showAdminBanner(data.error); el("detail").innerHTML = ""; return; }
      state.me = data.me || state.me;
      renderMembers(kind, code, data.members || []);
    });
  }
  function descOf(kind, code) {
    var arr = kind === "group" ? state.groups : state.roles;
    if (!arr) return "";
    for (var i = 0; i < arr.length; i++) if (arr[i].code === code) return arr[i].description || "";
    return "";
  }
  function renderMembers(kind, code, members) {
    var label = kind === "group" ? "Grupo" : "Papel";
    var desc = descOf(kind, code);
    var iAmMember = members.some(function (m) { return m.login === state.me; });
    var h = '<div class="card"><h2>' + esc(desc || code) + " " +
      '<span class="tbadge type">' + label + "</span>" +
      (iAmMember ? ' <span class="tbadge you">você participa</span>' : "") +
      '<button class="close" title="Fechar">×</button></h2>';
    h += '<div class="chips"><span class="code" data-copy="' + esc(code) + '" title="Copiar o código">' + esc(code) + "</span></div>";
    // ações
    h += '<div class="actions">';
    if (state.me) {
      if (iAmMember) h += '<button class="btn danger" id="leaveBtn">Remover-me deste ' + (kind === "group" ? "grupo" : "papel") + "</button>";
      else h += '<button class="btn" id="joinBtn">Incluir-me neste ' + (kind === "group" ? "grupo" : "papel") + "</button>";
    } else {
      h += '<span class="sub">identidade do dev não resolvida — sem incluir/remover</span>';
    }
    h += '<button class="btn ghost" id="copyMembers">Copiar membros (TSV)</button>';
    h += '<button class="btn ghost" id="usageBtn">Onde é usado?</button>';
    h += "</div>";
    // membros
    h += '<div class="sectitle">Membros (' + members.length + ")</div>";
    if (!members.length) h += '<div class="sub">nenhum membro.</div>';
    else {
      h += '<div class="tablewrap"><table><thead><tr><th>Nome</th><th>Login</th><th>E-mail</th><th>Estado</th></tr></thead><tbody>';
      members.forEach(function (m) {
        var you = m.login === state.me;
        var blocked = (m.state || "").toUpperCase() !== "ACTIVE";
        h += '<tr class="row" data-login="' + esc(m.login) + '"><td' + (blocked ? ' class="blocked"' : "") + ">" + esc(m.fullName || m.login) +
          (you ? ' <span class="tbadge you">você</span>' : "") + "</td><td class=\"mono\">" + esc(m.login) + "</td><td>" +
          esc(m.email || "") + "</td><td>" + stateBadge(m.state) + "</td></tr>";
      });
      h += "</tbody></table></div>";
    }
    h += '<div id="usage"></div>';
    h += "</div>";
    el("detail").innerHTML = h;
    state._members = members;
    wireDetail();
    wireMemberActions(kind, code, iAmMember);
  }
  function wireMemberActions(kind, code, iAmMember) {
    var join = el("joinBtn"), leave = el("leaveBtn");
    if (join) join.addEventListener("click", function () { membership(kind, code, "add"); });
    if (leave) leave.addEventListener("click", function () { membership(kind, code, "remove"); });
    var cm = el("copyMembers");
    if (cm) cm.addEventListener("click", function () {
      var lines = ["Nome\tLogin\tE-mail\tEstado"];
      (state._members || []).forEach(function (m) { lines.push([m.fullName || "", m.login || "", m.email || "", m.state || ""].join("\t")); });
      copy(lines.join("\n"), (state._members || []).length + " membro(s) copiados");
    });
    var ub = el("usageBtn");
    if (ub) ub.addEventListener("click", function () { loadUsage(kind, code); });
  }
  function membership(kind, code, action) {
    var label = kind === "group" ? "grupo" : "papel";
    var verb = action === "add" ? "incluir-se em" : "remover-se de";
    confirmModal(
      (action === "add" ? "Incluir-me" : "Remover-me"),
      "Confirma <b>" + verb + "</b> o " + label + ' <b>' + esc(code) + "</b> com o seu usuário (<b>" + esc(state.me) + "</b>)?",
      (action === "add" ? "Incluir-me" : "Remover-me"), action === "remove",
      function (ok) {
        if (!ok) return;
        api("POST", "/_dev/api/people/membership", { kind: kind, code: code, action: action }, function (err, data) {
          if (err) { toast(err); return; }
          if (data && data.needsAdmin) { toast("sem privilégio administrativo"); return; }
          toast(action === "add" ? "Incluído com sucesso" : "Removido com sucesso");
          openGroupOrRole(kind, code); // recarrega os membros (cache já invalidado no servidor)
        });
      });
  }
  function loadUsage(kind, code) {
    var box = el("usage"); if (!box) return;
    box.innerHTML = '<div class="sectitle">Onde é usado?</div><div class="sub">Varrendo os processos… <span class="spin"></span></div>';
    api("GET", "/_dev/api/people/usage?kind=" + kind + "&code=" + encodeURIComponent(code), null, function (err, data) {
      if (err) { box.innerHTML = '<div class="sectitle">Onde é usado?</div><div class="state-msg err">' + esc(err) + "</div>"; return; }
      if (data && data.needsAdmin) { box.innerHTML = ""; showAdminBanner(data.error); return; }
      var h = '<div class="sectitle">Onde é usado? <span class="sub">(' + data.scanned + " processo(s) varridos" +
        (data.failed ? ", " + data.failed + " pulado(s)" : "") + ")</span></div>";
      if (!data.hits || !data.hits.length) { h += '<div class="sub">Não encontrei este ' + (kind === "group" ? "grupo" : "papel") + " na atribuição de nenhuma etapa.</div>"; box.innerHTML = h; return; }
      h += '<div class="tablewrap"><table><thead><tr><th>Processo</th><th>Etapa</th><th style="text-align:right">Nº</th><th>Mecanismo</th></tr></thead><tbody>';
      data.hits.forEach(function (hit) {
        h += "<tr><td><a href=\"/_dev/processes/?process=" + encodeURIComponent(hit.processId) + "\" target=\"_blank\" rel=\"noopener\">" +
          esc(hit.processName || hit.processId) + " ↗</a><div class=\"sub mono\">" + esc(hit.processId) + "</div></td>" +
          "<td>" + esc(hit.stateName) + '</td><td class="mono" style="text-align:right">' + hit.sequence + "</td>" +
          '<td class="sub">' + esc(hit.mechanism || "") + "</td></tr>";
      });
      h += "</tbody></table></div>";
      box.innerHTML = h;
    });
  }

  function wireDetail() {
    document.querySelectorAll("#detail [data-copy]").forEach(function (n) {
      n.addEventListener("click", function () { copy(this.getAttribute("data-copy"), "Copiado"); });
    });
    document.querySelectorAll("#detail [data-go]").forEach(function (n) {
      n.addEventListener("click", function () {
        var parts = this.getAttribute("data-go").split("|");
        var kind = parts[0], code = parts.slice(1).join("|");
        setTab(kind === "group" ? "groups" : "roles", true);
        document.querySelectorAll(".tab").forEach(function (b) { b.classList.toggle("on", b.getAttribute("data-tab") === state.tab); });
        el("commWrap").classList.toggle("hidden", state.tab !== "groups");
        el("blockedWrap").classList.toggle("hidden", state.tab !== "users");
        if (state[state.tab]) render(); else load(false);
        openGroupOrRole(kind, code);
      });
    });
    document.querySelectorAll("#detail tr.row").forEach(function (tr) {
      tr.addEventListener("click", function () { openUser(this.getAttribute("data-login")); });
    });
    var cl = document.querySelector("#detail .close");
    if (cl) cl.addEventListener("click", clearDetail);
  }
  function scrollDetail() {
    setTimeout(function () { var d = el("detail"); if (d && d.firstChild) d.scrollIntoView({ behavior: "smooth", block: "start" }); }, 30);
  }

  // ---- boot: deep-link ?user= / ?group= / ?role= ----
  function boot() {
    var p = new URLSearchParams(location.search);
    var deepUser = p.get("user"), deepGroup = p.get("group"), deepRole = p.get("role");
    if (deepGroup) { setTab("groups", true); afterLoad(function () { openGroupOrRole("group", deepGroup); }); }
    else if (deepRole) { setTab("roles", true); afterLoad(function () { openGroupOrRole("role", deepRole); }); }
    else if (deepUser) { setTab("users", true); afterLoad(function () { openUser(deepUser); }); }
    else { setTab("users", true); }
    document.querySelectorAll(".tab").forEach(function (b) { b.classList.toggle("on", b.getAttribute("data-tab") === state.tab); });
    el("commWrap").classList.toggle("hidden", state.tab !== "groups");
    el("blockedWrap").classList.toggle("hidden", state.tab !== "users");
    el("q").placeholder = { users: "Filtrar por nome, login, código ou e-mail…", groups: "Filtrar por código ou descrição…", roles: "Filtrar por código ou descrição…" }[state.tab];
    load(false);
    // Pré-carrega os contadores das outras abas (barato; melhora as chips).
    TABS.forEach(function (t) { if (t !== state.tab) api("GET", "/_dev/api/people/" + t, null, function (e, d) {
      if (!e && d && !d.needsAdmin) { state[t] = d.users || d.groups || d.roles || []; if (!state.needsAdmin) refreshTabCounts(); }
    }); });
  }
  function afterLoad(fn) { state._afterLoad = fn; }
  function refreshTabCounts() {
    var idmap = { users: "nUsers", groups: "nGroups", roles: "nRoles" };
    var counts = { users: state.users, groups: state.groups, roles: state.roles };
    TABS.forEach(function (t) { if (counts[t]) el(idmap[t]).textContent = counts[t].length; });
  }
  // hook após a carga da aba corrente: dispara o deep-link de detalhe
  var _origRender = render;
  render = function () { _origRender(); if (state._afterLoad) { var f = state._afterLoad; state._afterLoad = null; f(); } };

  boot();
})();
</script>
</body>
</html>`
