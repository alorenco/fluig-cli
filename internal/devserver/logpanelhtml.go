package devserver

// logPanelHTML é a página do painel de logs (rota /_dev/logs/).
// Self-contained, no mesmo design system do dashboard/Dataset Lab. As linhas
// chegam pelo SSE /_dev/api/log/stream; filtro de nível/palavra, pausa e
// colorização são locais (cada aba filtra por conta própria). O console é
// escuro nos dois temas (leitura de log é terminal).
//
// Sem template literals (backticks) no JS: a página é uma raw string Go.
const logPanelHTML = `<!doctype html>
<html lang="pt-BR">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>fluigcli dev — logs</title>
<style>
:root{--bg:#f4f6f8;--card:#fff;--txt:#1d2b36;--sub:#5a6b7b;--line:#e3e8ee;
  --accent:#0c9abe;--accent-txt:#fff;--ok:#25b26e;--warn:#b3352b;
  --shadow:0 1px 2px rgba(16,36,54,.08)}
@media(prefers-color-scheme:dark){:root{--bg:#12181f;--card:#1b232d;
  --txt:#e6edf3;--sub:#93a4b4;--line:#2b3742;--shadow:0 1px 2px rgba(0,0,0,.4)}}
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
main{max-width:1280px;margin:0 auto;padding:22px 32px 40px}

.panel{background:var(--card);border:1px solid var(--line);border-radius:12px;
  padding:12px 14px;box-shadow:var(--shadow)}
.toolbar{display:flex;gap:10px;align-items:center;flex-wrap:wrap}
select,input[type=text]{padding:8px 11px;border:1px solid var(--line);
  border-radius:8px;background:var(--bg);color:var(--txt);font-size:13.5px;outline:none}
select:focus,input[type=text]:focus{border-color:var(--accent)}
#file{max-width:340px}
#grep{flex:1;min-width:180px}
button{font:inherit}
.icobtn{background:transparent;border:1px solid var(--line);color:var(--sub);border-radius:8px;
  padding:7px 12px;cursor:pointer;font-size:13.5px;font-weight:600;white-space:nowrap;
  transition:border-color .08s,color .08s;text-decoration:none;display:inline-flex;
  gap:6px;align-items:center}
.icobtn:hover{border-color:var(--accent);color:var(--accent)}
.icobtn.on{border-color:var(--accent);color:var(--accent);
  background:color-mix(in srgb,var(--accent) 10%,transparent)}
.check{display:inline-flex;gap:6px;align-items:center;font-size:13px;cursor:pointer;
  color:var(--sub);white-space:nowrap;user-select:none}
.statline{display:flex;gap:10px;align-items:center;margin-left:auto;font-size:12.5px;color:var(--sub)}
.dot{width:9px;height:9px;border-radius:50%;background:#8a97a3;flex:0 0 auto}
.dot.ok{background:var(--ok)}
.dot.warn{background:#e3b341}
.dot.err{background:var(--warn)}

#banner{display:none;margin:14px 0 0;padding:12px 15px;border:1px solid
  color-mix(in srgb,var(--warn) 45%,var(--line));border-radius:10px;font-size:13.5px;
  background:color-mix(in srgb,var(--warn) 8%,var(--card));color:var(--txt)}
#banner code{font:12px ui-monospace,Consolas,monospace;background:color-mix(in srgb,var(--txt) 8%,transparent);
  padding:1px 6px;border-radius:5px}
#banner button{margin-left:10px}

/* console: escuro nos dois temas */
#logwrap{margin-top:14px;background:#10161d;border:1px solid #232f3a;border-radius:12px;
  overflow:hidden;box-shadow:var(--shadow)}
#log{height:calc(100vh - 285px);min-height:280px;overflow:auto;padding:12px 14px;
  font:12px/1.55 ui-monospace,SFMono-Regular,Consolas,"Liberation Mono",monospace;
  color:#d7e0e8;overscroll-behavior:contain}
#log div{white-space:pre-wrap;word-break:break-word}
#log .err{color:#ff7b72}
#log .wrn{color:#e3b341}
#log .dbg{color:#7d8b99}
#log .note{color:#79c0ff;font-style:italic;margin:4px 0}
#log .empty{color:#7d8b99;font-style:italic}
#log .fold{display:block;margin:2px 0 2px 14px;padding:1px 9px;border:1px solid #3a2a2b;
  border-radius:6px;background:transparent;color:#8a97a3;cursor:pointer;
  font:11px ui-monospace,SFMono-Regular,Consolas,"Liberation Mono",monospace}
#log .fold:hover{color:#ff7b72;border-color:#ff7b72}
#log .cont{margin:2px 0 2px 14px;border-left:2px solid #3a2a2b;padding-left:10px}
</style>
</head>
<body>
<header>
  <div class="hrow">
    <div>
      <h1>fluigcli <small>dev</small> — logs do servidor</h1>
      <p>server.log ao vivo, direto do servidor conectado (via fluigcliHelper) — sem SSH.</p>
    </div>
    <a class="back" href="/">← dashboard</a>
  </div>
</header>
<main>
  <div class="panel toolbar">
    <select id="file" title="arquivo do diretório de log do servidor"></select>
    <select id="level" title="severidade mínima">
      <option value="">todos os níveis</option>
      <option value="1">DEBUG+</option>
      <option value="2">INFO+</option>
      <option value="3">WARN+</option>
      <option value="4">ERROR+</option>
    </select>
    <input type="text" id="grep" placeholder="filtrar por palavra…" title="substring, sem diferenciar maiúsculas">
    <button class="icobtn" id="pause" title="pausar a rolagem (as linhas continuam chegando)">⏸ pausar</button>
    <button class="icobtn" id="clear" title="limpar a tela">🧹 limpar</button>
    <a class="icobtn" id="download" href="#" title="baixar o arquivo inteiro">⬇ baixar</a>
    <label class="check" title="rolar para o fim a cada linha nova">
      <input type="checkbox" id="autoscroll" checked> auto-rolagem</label>
    <div class="statline"><span class="dot" id="dot"></span><span id="stat">conectando…</span></div>
  </div>
  <div id="banner"></div>
  <div id="logwrap"><div id="log"><div class="empty">aguardando o log…</div></div></div>
</main>
<script>
(function(){
  "use strict";
  var $ = function(id){ return document.getElementById(id); };
  var logEl = $("log"), dot = $("dot"), stat = $("stat"), banner = $("banner");

  var LEVELS = {TRACE:0,FINEST:0,FINER:0,DEBUG:1,FINE:1,INFO:2,CONFIG:2,
                WARN:3,WARNING:3,ERROR:4,SEVERE:4,FATAL:5};
  var HEAD = /^\d{4}-\d{2}-\d{2}[ T]/;

  var buf = [];              // todas as linhas recebidas (cap BUFCAP)
  var BUFCAP = 5000, DOMCAP = 3000;
  var es = null, file = "server.log";
  var paused = false, pending = 0, empty = true;
  // decisão corrente da entrada (herdada pelas continuações do stack trace)
  var fs = freshEntry();
  function freshEntry(){ return {show:true, cls:"", head:null, fold:null, btn:null}; }

  function lineLevel(line){
    var toks = line.split(/\s+/, 5);
    for (var i = 0; i < toks.length && i < 4; i++) {
      var r = LEVELS[toks[i].toUpperCase()];
      if (r !== undefined) return r;
    }
    return -1;
  }
  function levelClass(rank){
    if (rank >= 4) return "err";
    if (rank === 3) return "wrn";
    if (rank <= 1 && rank >= 0) return "dbg";
    return "";
  }
  function filterState(){
    return {min: $("level").value === "" ? -1 : parseInt($("level").value, 10),
            q: $("grep").value.trim().toLowerCase()};
  }
  // Decide e devolve o nó da linha (null = filtrada ou anexada ao dobrável).
  // A decisão é tomada na linha de cabeçalho e herdada pelas continuações — o
  // stack trace acompanha o ERROR que o abriu (mesma semântica do fluigcli
  // log tail --follow). Continuações de uma entrada ERROR não viram linhas
  // soltas: ficam recolhidas num bloco expansível junto do cabeçalho (a
  // mensagem principal fica visível; o stack trace abre sob demanda).
  function lineNode(line, f){
    if (HEAD.test(line)) {
      var rank = lineLevel(line);
      fs.cls = levelClass(rank);
      fs.head = null; fs.fold = null; fs.btn = null;
      fs.show = true;
      if (f.min >= 0 && rank < f.min) fs.show = false;
      if (fs.show && f.q && line.toLowerCase().indexOf(f.q) < 0) fs.show = false;
      if (!fs.show) return null;
      var div = document.createElement("div");
      if (fs.cls) div.className = fs.cls;
      div.textContent = line;
      if (fs.cls === "err") fs.head = div;
      return div;
    }
    if (!fs.show) return null;
    if (fs.head) { foldLine(line); return null; }
    var cont = document.createElement("div");
    if (fs.cls) cont.className = fs.cls;
    cont.textContent = line;
    return cont;
  }
  // Anexa a continuação ao bloco recolhido da entrada ERROR corrente,
  // criando o botão de expandir na primeira continuação.
  function foldLine(line){
    if (!fs.fold) {
      var fold = document.createElement("div");
      fold.className = "cont";
      fold.hidden = true;
      var btn = document.createElement("button");
      btn.className = "fold";
      btn.title = "stack trace / detalhe da entrada";
      btn.onclick = function(){
        fold.hidden = !fold.hidden;
        foldLabel(btn, fold);
      };
      fs.head.appendChild(btn);
      fs.head.appendChild(fold);
      fs.fold = fold; fs.btn = btn;
    }
    var div = document.createElement("div");
    div.textContent = line;
    fs.fold.appendChild(div);
    foldLabel(fs.btn, fs.fold);
  }
  function foldLabel(btn, fold){
    var n = fold.childElementCount;
    var s = n + (n === 1 ? " linha" : " linhas");
    btn.textContent = fold.hidden ? "▸ mostrar +" + s : "▾ ocultar " + s;
  }
  function appendLines(lines){
    if (empty) { logEl.textContent = ""; empty = false; }
    var f = filterState();
    var frag = document.createDocumentFragment();
    for (var i = 0; i < lines.length; i++) {
      var node = lineNode(lines[i], f);
      if (node) frag.appendChild(node);
    }
    logEl.appendChild(frag);
    while (logEl.children.length > DOMCAP) logEl.removeChild(logEl.firstChild);
    if ($("autoscroll").checked) logEl.scrollTop = logEl.scrollHeight;
    updateStat();
  }
  function note(msg){
    if (empty) { logEl.textContent = ""; empty = false; }
    var div = document.createElement("div");
    div.className = "note";
    div.textContent = "— " + msg + " —";
    logEl.appendChild(div);
  }
  function rerender(){
    logEl.textContent = "";
    empty = false;
    fs = freshEntry();
    var start = Math.max(0, buf.length - BUFCAP);
    appendLines(buf.slice(start));
  }
  function updateStat(){
    var s = buf.length + " linhas";
    if (paused && pending > 0) s += " · " + pending + " novas em pausa";
    stat.textContent = s;
  }
  function setDot(cls, label){
    dot.className = "dot" + (cls ? " " + cls : "");
    if (label) stat.textContent = label;
  }
  function showBanner(msg){
    banner.textContent = msg;
    var btn = document.createElement("button");
    btn.className = "icobtn";
    btn.textContent = "tentar de novo";
    btn.onclick = function(){ connect(file); };
    banner.appendChild(btn);
    banner.style.display = "block";
  }
  function hideBanner(){ banner.style.display = "none"; }

  function connect(name){
    if (es) { es.close(); es = null; }
    file = name;
    buf = []; pending = 0;
    logEl.innerHTML = "<div class=\"empty\">aguardando o log…</div>";
    empty = true;
    fs = freshEntry();
    hideBanner();
    $("download").href = "/fluigcliHelper/api/logs/" + encodeURIComponent(name) + "/download";
    setDot("", "conectando…");
    es = new EventSource("/_dev/api/log/stream?file=" + encodeURIComponent(name));
    es.onopen = function(){ setDot("ok"); updateStat(); };
    es.onerror = function(){ setDot("warn", "reconectando…"); };
    es.onmessage = function(m){
      var ev;
      try { ev = JSON.parse(m.data); } catch (e) { return; }
      if (ev.error) {
        setDot("err", "parado");
        showBanner(ev.error);
        if (es) { es.close(); es = null; }
        return;
      }
      if (ev.info) { if (!paused) note(ev.info); return; }
      if (ev.lines && ev.lines.length) {
        buf = buf.concat(ev.lines);
        if (buf.length > BUFCAP) buf = buf.slice(buf.length - BUFCAP);
        if (paused) { pending += ev.lines.length; updateStat(); }
        else appendLines(ev.lines);
      }
      setDot("ok");
      updateStat();
    };
  }

  function loadFiles(){
    fetch("/_dev/api/log/files").then(function(r){ return r.json(); }).then(function(d){
      if (d.error) { showBanner(d.error); setDot("err", "parado"); return; }
      var sel = $("file");
      sel.innerHTML = "";
      var files = d.files || [];
      // server.log primeiro; o resto por data de modificação (mais novo antes)
      files.sort(function(a, b){
        if (a.name === "server.log") return -1;
        if (b.name === "server.log") return 1;
        return (b.lastModified || "") < (a.lastModified || "") ? -1 : 1;
      });
      for (var i = 0; i < files.length; i++) {
        var o = document.createElement("option");
        o.value = files[i].name;
        o.textContent = files[i].name + " (" + fmtSize(files[i].size) + ")";
        sel.appendChild(o);
      }
      if (!files.length) {
        var oo = document.createElement("option");
        oo.value = "server.log"; oo.textContent = "server.log";
        sel.appendChild(oo);
      }
      sel.value = file;
      if (sel.value !== file) { sel.selectedIndex = 0; }
    }).catch(function(){ /* o stream reporta o erro */ });
  }
  function fmtSize(n){
    if (n >= 1073741824) return (n / 1073741824).toFixed(1) + " GB";
    if (n >= 1048576) return (n / 1048576).toFixed(1) + " MB";
    if (n >= 1024) return (n / 1024).toFixed(1) + " KB";
    return n + " B";
  }

  $("file").addEventListener("change", function(){ connect(this.value); });
  $("level").addEventListener("change", rerender);
  var grepTimer = null;
  $("grep").addEventListener("input", function(){
    if (grepTimer) clearTimeout(grepTimer);
    grepTimer = setTimeout(rerender, 200);
  });
  $("pause").addEventListener("click", function(){
    paused = !paused;
    this.classList.toggle("on", paused);
    this.innerHTML = paused ? "▶ retomar" : "⏸ pausar";
    if (!paused) { pending = 0; rerender(); }
    updateStat();
  });
  $("clear").addEventListener("click", function(){
    buf = []; pending = 0;
    logEl.textContent = ""; empty = false;
    updateStat();
  });

  loadFiles();
  connect("server.log");
})();
</script>
</body>
</html>
`
