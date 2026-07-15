package devserver

// dashboardHTML é a página do dashboard (rota /). Self-contained e no mesmo
// design system do índice de formulários (claro/escuro, accent ciano); os
// dados vêm de /_dev/api/dash e os controles usam os POSTs de /_dev/api/dash/*.
//
// Layout (revisado com screenshots via Playwright em 2026-07-11): ações em
// tiles compactos (Portal primeiro), widgets num painel próprio com chips —
// cards de alturas independentes, nada estica junto.
const dashboardHTML = `<!doctype html>
<html lang="pt-BR">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>fluigcli dev</title>
<style>
:root{--bg:#f4f6f8;--card:#fff;--txt:#1d2b36;--sub:#5a6b7b;--line:#e3e8ee;
  --accent:#0c9abe;--accent-txt:#fff;--ok:#25b26e;--warn:#b3352b;
  --chipbg:#f7f9fb;--shadow:0 1px 2px rgba(16,36,54,.08)}
@media(prefers-color-scheme:dark){:root{--bg:#12181f;--card:#1b232d;
  --txt:#e6edf3;--sub:#93a4b4;--line:#2b3742;--chipbg:#161d26;
  --shadow:0 1px 2px rgba(0,0,0,.4)}}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--txt);
  font:15px/1.5 system-ui,-apple-system,"Segoe UI",Roboto,sans-serif}
header{padding:26px 32px 18px;border-bottom:1px solid var(--line)}
header h1{margin:0;font-size:22px;font-weight:650}
header h1 small{color:var(--accent);font-weight:650}
header p{margin:7px 0 0;color:var(--sub);font-size:13.5px}
.badge{display:inline-block;background:color-mix(in srgb,var(--accent) 12%,transparent);
  color:var(--accent);border-radius:5px;padding:1px 7px;font-size:11.5px;font-weight:600;
  vertical-align:1px}
.badge.prod{background:color-mix(in srgb,var(--warn) 14%,transparent);color:var(--warn)}
main{max-width:1080px;margin:0 auto;padding:26px 32px 48px}
section{margin-top:28px}
section:first-child{margin-top:0}
section>h2{font-size:12.5px;font-weight:650;letter-spacing:.06em;text-transform:uppercase;
  color:var(--sub);margin:0 0 10px}

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

/* widgets do disco: chips em grade, sem quebra feia */
.mounts{display:grid;grid-template-columns:repeat(auto-fill,minmax(300px,1fr));
  gap:8px;margin-top:2px;max-height:220px;overflow:auto;padding-right:4px}
.mount{border:1px solid var(--line);background:var(--chipbg);border-radius:8px;
  padding:8px 12px;min-width:0}
.mount .root{font:600 12.5px/1.4 ui-monospace,Consolas,monospace;color:var(--accent);
  white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.mount .path{font:12px/1.4 ui-monospace,Consolas,monospace;color:var(--sub);
  white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
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
    </div>
  </section>

  <section>
    <h2>Widgets servidas do disco <span class="badge" id="mountsBadge" style="display:none"></span></h2>
    <div class="panel">
      <div class="muted" id="mountsMeta" style="margin-bottom:10px"></div>
      <div class="mounts" id="mounts"></div>
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
        <button class="sec" id="clearCaches" title="Zera os caches do painel de simulação (contexto, processos, etapas, usuários) e as conexões de publicação">Limpar caches do painel</button>
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
    return (Math.round(m / 6) / 10) + " h";
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

  function renderMounts() {
    var mounts = state.mounts || [];
    var badge = el("mountsBadge");
    badge.textContent = mounts.length;
    badge.style.display = mounts.length ? "" : "none";
    el("mountsMeta").textContent = mounts.length
      ? "JS/CSS e view.ftl servidos da sua máquina, sem deploy:"
      : "";
    var box = el("mounts");
    box.innerHTML = "";
    if (!mounts.length) {
      var d = document.createElement("div");
      d.className = "empty";
      d.textContent = "Nenhuma widget local em wcm/widget/ — o proxy segue servindo tudo do servidor.";
      box.appendChild(d);
      return;
    }
    mounts.forEach(function (m) {
      var i = m.indexOf(" → ");
      var root = i >= 0 ? m.slice(0, i) : m;
      var path = i >= 0 ? m.slice(i + 3) : "";
      var chip = document.createElement("div");
      chip.className = "mount";
      chip.title = m;
      var r = document.createElement("div");
      r.className = "root";
      r.textContent = root;
      chip.appendChild(r);
      if (path) {
        var p = document.createElement("div");
        p.className = "path";
        p.textContent = path;
        chip.appendChild(p);
      }
      box.appendChild(chip);
    });
  }

  function render() {
    var srv = state.server || {};
    var envBadge = srv.env ? " <span class=\"badge" + (srv.env === "prod" ? " prod" : "") + "\">" + srv.env + "</span>" : "";
    el("hdr").innerHTML = "Servidor <strong>" + (srv.name || "?") + "</strong>" + envBadge +
      " · " + (srv.url || "") + " · usuário " + (srv.user || "?") +
      " · no ar há " + fmtUptime(state.uptimeSeconds || 0);
    el("portalTile").setAttribute("href", state.portalPath || "/");
    el("formsMeta").textContent = (state.formsCount || 0) +
      " formulário(s) no projeto, com preview e simulação de processo";
    renderMounts();
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
    });
  });

  load();
  setInterval(load, 15000); // feed do watch e uptime frescos
})();
</script>
</body>
</html>
`
