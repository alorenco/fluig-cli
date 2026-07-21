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
select,input[type=text],input[type=datetime-local]{padding:8px 11px;border:1px solid var(--line);
  border-radius:8px;background:var(--bg);color:var(--txt);font-size:13.5px;outline:none}
select:focus,input[type=text]:focus,input[type=datetime-local]:focus{border-color:var(--accent)}
#file{max-width:260px}
#grep{flex:1;min-width:150px}
button{font:inherit}
.icobtn{background:transparent;border:1px solid var(--line);color:var(--sub);border-radius:8px;
  padding:7px 12px;cursor:pointer;font-size:13.5px;font-weight:600;white-space:nowrap;
  transition:border-color .08s,color .08s;text-decoration:none;display:inline-flex;
  gap:6px;align-items:center}
/* botão só-ícone, largura fixa (a informação vai no title) */
.icobtn.ib{width:38px;padding:7px 0;justify-content:center;font-size:16px}
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
.dot.range{background:var(--accent)}

/* popover de busca por intervalo de data/hora */
.rangebox{position:fixed;z-index:60;background:var(--card);border:1px solid var(--line);
  border-radius:10px;box-shadow:var(--shadow);padding:13px 14px;width:290px;font-size:13px}
.rangebox h4{margin:0 0 10px;font-size:13px;color:var(--txt)}
.rangebox .rgrow{display:flex;align-items:center;gap:8px;margin-bottom:8px}
.rangebox .rgrow label{color:var(--sub);width:46px;flex:0 0 auto}
.rangebox input{flex:1;min-width:0}
.rgnote{color:var(--sub);font-size:11.5px;margin:4px 0 11px;line-height:1.45}
.rgbtns{display:flex;gap:8px;justify-content:flex-end}
#banner.info{border-color:color-mix(in srgb,var(--accent) 45%,var(--line));
  background:color-mix(in srgb,var(--accent) 8%,var(--card))}

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
#log .fold{display:block;margin:2px 0 2px 14px;padding:1px 9px;border:1px solid #333b45;
  border-radius:6px;background:transparent;color:#8a97a3;cursor:pointer;
  font:11px ui-monospace,SFMono-Regular,Consolas,"Liberation Mono",monospace}
#log .fold:hover{color:#adbac7;border-color:#4a5763}
#log .err .fold:hover{color:#ff7b72;border-color:#ff7b72}
#log .wrn .fold:hover{color:#e3b341;border-color:#e3b341}
#log .cont{margin:2px 0 2px 14px;border-left:2px solid #333b45;padding-left:10px}
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
    <button class="icobtn ib" id="pause" title="pausar a rolagem (as linhas continuam chegando)">⏸</button>
    <button class="icobtn ib" id="clear" title="limpar a tela">🧹</button>
    <a class="icobtn ib" id="download" href="#" title="baixar o arquivo inteiro">⬇</a>
    <button class="icobtn ib" id="rangebtn" title="buscar um intervalo de data/hora" style="display:none">📅</button>
    <button class="icobtn" id="tz" title="alternar o fuso dos horários" style="display:none">🕓 servidor</button>
    <label class="check" title="rolar para o fim a cada linha nova">
      <input type="checkbox" id="autoscroll" checked> auto-rolagem</label>
    <div class="statline"><span class="dot" id="dot"></span><span id="stat">conectando…</span></div>
  </div>
  <div id="banner"></div>
  <div id="logwrap"><div id="log"><div class="empty">aguardando o log…</div></div></div>
</main>
<div id="rangebox" class="rangebox" style="display:none">
  <h4>Buscar intervalo</h4>
  <div class="rgrow"><label for="rgfrom">Início</label><input type="datetime-local" id="rgfrom" step="1"></div>
  <div class="rgrow"><label for="rgto">Fim</label><input type="datetime-local" id="rgto" step="1"></div>
  <div class="rgnote" id="rgnote"></div>
  <div class="rgbtns">
    <button class="icobtn" id="rgclear">Limpar</button>
    <button class="icobtn on" id="rgapply">Buscar</button>
  </div>
</div>
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

  // Fuso dos horários. O timestamp do log é hora-de-parede do servidor SEM
  // offset; com o fuso do servidor (helper >= 0.4.0) o painel converte para o
  // fuso do navegador. tz.mode = "server" (cru) | "browser" (convertido).
  var TZKEY = "fluigcli.logpanel.tz";
  var browserOff = -new Date().getTimezoneOffset(); // minutos a leste de UTC
  var tz = {srvOff: null, srvZone: "", mode: "server"};
  try { if (localStorage.getItem(TZKEY) === "browser") tz.mode = "browser"; } catch(e){}
  var rangeMode = false; // busca por intervalo (snapshot) em vez do ao vivo
  var TS = /^(\d{4})-(\d{2})-(\d{2})[ T](\d{2}):(\d{2}):(\d{2})([.,](\d{1,3}))?/;
  function pad(n, w){ n = "" + n; while (n.length < w) n = "0" + n; return n; }
  // Converte o prefixo de horário da linha (se ela começa com um) do fuso do
  // servidor para o do navegador; devolve a linha intacta fora do modo browser.
  function tsLine(line){
    if (tz.mode !== "browser" || tz.srvOff === null) return line;
    var m = TS.exec(line);
    if (!m) return line;
    var ms = m[8] ? parseInt((m[8] + "00").slice(0, 3), 10) : 0; // fração à direita: ".7" = 700ms
    var inst = Date.UTC(+m[1], +m[2]-1, +m[3], +m[4], +m[5], +m[6], ms) - tz.srvOff*60000;
    var d = new Date(inst);
    var out = d.getFullYear() + "-" + pad(d.getMonth()+1, 2) + "-" + pad(d.getDate(), 2) + " " +
              pad(d.getHours(), 2) + ":" + pad(d.getMinutes(), 2) + ":" + pad(d.getSeconds(), 2);
    if (m[7]) out += "," + pad(d.getMilliseconds(), 3);
    return out + line.slice(m[0].length);
  }
  function updateTzBtn(){
    var b = $("tz");
    var conv = tz.srvOff !== null && tz.srvOff !== browserOff;
    b.style.display = conv ? "" : "none";
    if (!conv) return;
    var br = tz.mode === "browser";
    b.textContent = br ? "🕓 navegador" : "🕓 servidor";
    b.classList.toggle("on", br);
    b.title = br
      ? "horários no fuso do NAVEGADOR (servidor: " + (tz.srvZone || "?") + ") — clique para ver o do servidor"
      : "horários no fuso do SERVIDOR (" + (tz.srvZone || "?") + ") — clique para converter ao do navegador";
  }
  // WARN/INFO com mais de FOLDBIG linhas de continuação recolhem a entrada
  // inteira (como o ERROR, que sempre recolhe); DEBUG/TRACE nunca recolhem.
  var FOLDBIG = 6;
  // decisão corrente da entrada (herdada pelas continuações do stack trace)
  var fs = freshEntry();
  function freshEntry(){ return {show:true, cls:"", rank:-1, foldAfter:Infinity,
                                 head:null, fold:null, btn:null, count:0, seen:[]}; }
  // A partir de quantas continuações a entrada é "grande" e recolhe (por nível):
  // ERROR/FATAL já na 1ª; WARN/INFO só se passar de FOLDBIG; demais nunca.
  function foldAfterFor(rank){
    if (rank >= 4) return 0;
    if (rank === 3 || rank === 2) return FOLDBIG;
    return Infinity;
  }

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
  // Decide e devolve o nó da linha (null = filtrada ou já recolhida no bloco).
  // A decisão é tomada na linha de cabeçalho e herdada pelas continuações
  // (mesma semântica do fluigcli log tail --follow). Entradas grandes recolhem
  // as continuações num bloco expansível junto do cabeçalho — só a mensagem
  // principal fica visível, o restante abre sob demanda: ERROR/FATAL sempre,
  // WARN/INFO só quando passam de FOLDBIG linhas, DEBUG/TRACE nunca.
  function lineNode(line, f){
    if (HEAD.test(line)) {
      var rank = lineLevel(line);
      fs.cls = levelClass(rank); fs.rank = rank;
      fs.foldAfter = foldAfterFor(rank);
      fs.head = null; fs.fold = null; fs.btn = null; fs.count = 0; fs.seen = [];
      fs.show = true;
      if (f.min >= 0 && rank < f.min) fs.show = false;
      if (fs.show && f.q && line.toLowerCase().indexOf(f.q) < 0) fs.show = false;
      if (!fs.show) return null;
      var div = document.createElement("div");
      if (fs.cls) div.className = fs.cls;
      div.textContent = tsLine(line);
      if (fs.foldAfter !== Infinity) fs.head = div; // níveis que podem recolher
      return div;
    }
    if (!fs.show) return null;
    if (!fs.head) return contDiv(line);             // nível que nunca recolhe (DEBUG/desconhecido)
    if (fs.fold) { appendFold(line); return null; } // entrada já recolhida
    fs.count++;
    if (fs.count <= fs.foldAfter) {                 // ainda dentro do limite: mostra e guarda
      var c = contDiv(line);
      fs.seen.push(c);
      return c;
    }
    startFold();                                    // passou do limite: recolhe a entrada inteira
    appendFold(line);
    return null;
  }
  function contDiv(line){
    var div = document.createElement("div");
    if (fs.cls) div.className = fs.cls;
    div.textContent = line;
    return div;
  }
  // Cria o bloco recolhido no cabeçalho e move para dentro dele as
  // continuações já mostradas (a entrada "cresceu demais" no meio do caminho).
  function startFold(){
    var fold = document.createElement("div");
    fold.className = "cont";
    fold.hidden = true;
    var btn = document.createElement("button");
    btn.className = "fold";
    btn.title = "detalhe da entrada (stack trace / linhas extras)";
    btn.onclick = function(){
      fold.hidden = !fold.hidden;
      foldLabel(btn, fold);
    };
    fs.head.appendChild(btn);
    fs.head.appendChild(fold);
    for (var i = 0; i < fs.seen.length; i++) fold.appendChild(fs.seen[i]);
    fs.seen = [];
    fs.fold = fold; fs.btn = btn;
  }
  function appendFold(line){
    fs.fold.appendChild(contDiv(line));
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
  function hideBanner(){ banner.className = ""; banner.style.display = "none"; }

  function connect(name){
    if (es) { es.close(); es = null; }
    file = name;
    rangeMode = false;
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
      if (d.serverTZ && typeof d.serverTZ.offsetMinutes === "number") {
        tz.srvOff = d.serverTZ.offsetMinutes;
        tz.srvZone = d.serverTZ.zoneId || "";
      }
      updateTzBtn();
      $("rangebtn").style.display = d.logRange ? "" : "none";
      // preferência salva = navegador e o fuso só chegou agora (fetch assíncrono):
      // reconverte o que já foi renderizado do stream.
      if (tz.mode === "browser" && tz.srvOff !== null && tz.srvOff !== browserOff && buf.length) rerender();
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

  // ---- busca por intervalo de data/hora (snapshot) ----
  // Formata um instante no fuso EXIBIDO como valor de datetime-local.
  function dtLocalValue(t){
    var srvView = tz.mode !== "browser" && tz.srvOff !== null;
    var d = srvView ? new Date(t + tz.srvOff*60000) : new Date(t);
    var y  = srvView ? d.getUTCFullYear() : d.getFullYear();
    var mo = (srvView ? d.getUTCMonth() : d.getMonth()) + 1;
    var da = srvView ? d.getUTCDate() : d.getDate();
    var h  = srvView ? d.getUTCHours() : d.getHours();
    var mi = srvView ? d.getUTCMinutes() : d.getMinutes();
    var s  = srvView ? d.getUTCSeconds() : d.getSeconds();
    return y+"-"+pad(mo,2)+"-"+pad(da,2)+"T"+pad(h,2)+":"+pad(mi,2)+":"+pad(s,2);
  }
  // Valor do input (no fuso exibido) -> hora LOCAL do servidor para a consulta.
  // end=true completa os segundos ausentes com :59 (minuto inclusivo).
  function boundValue(id, end){
    var v = $(id).value;
    if (!v) return "";
    if (v.length === 16) v += end ? ":59" : ":00";
    if (!(tz.mode === "browser" && tz.srvOff !== null)) return v; // já é hora do servidor
    var t = new Date(v).getTime();                                // navegador-local -> instante
    if (isNaN(t)) return "";
    var d = new Date(t + tz.srvOff*60000);                        // -> parede do servidor
    return d.getUTCFullYear()+"-"+pad(d.getUTCMonth()+1,2)+"-"+pad(d.getUTCDate(),2)+"T"+
           pad(d.getUTCHours(),2)+":"+pad(d.getUTCMinutes(),2)+":"+pad(d.getUTCSeconds(),2);
  }
  function openRange(){
    var box = $("rangebox");
    var zone = tz.mode === "browser" ? "navegador" : "servidor (" + (tz.srvZone || "?") + ")";
    $("rgnote").textContent = "Datas no fuso do " + zone + "; busca no arquivo " + file + ".";
    var now = Date.now();
    if (!$("rgfrom").value) $("rgfrom").value = dtLocalValue(now - 3600000);
    if (!$("rgto").value) $("rgto").value = dtLocalValue(now);
    box.style.display = "block";
    var b = $("rangebtn").getBoundingClientRect();
    var w = box.offsetWidth || 290;
    box.style.top = (b.bottom + 6) + "px";
    box.style.left = Math.max(8, Math.min(b.left, window.innerWidth - w - 8)) + "px";
  }
  function closeRange(){ $("rangebox").style.display = "none"; }
  function applyRange(){
    var from = boundValue("rgfrom", false), to = boundValue("rgto", true);
    if (!from && !to) { $("rgnote").textContent = "Informe ao menos um limite (início ou fim)."; return; }
    var dispFrom = $("rgfrom").value, dispTo = $("rgto").value;
    closeRange();
    if (es) { es.close(); es = null; }
    rangeMode = true;
    paused = false; pending = 0;
    $("pause").classList.remove("on"); $("pause").textContent = "⏸";
    setDot("range", "buscando o intervalo…");
    var qs = "file=" + encodeURIComponent(file);
    if (from) qs += "&from=" + encodeURIComponent(from);
    if (to) qs += "&to=" + encodeURIComponent(to);
    fetch("/_dev/api/log/range?" + qs).then(function(r){ return r.json(); }).then(function(d){
      if (d.error) { setDot("err", "parado"); showBanner(d.error); return; }
      var lines = [];
      (d.entries || []).forEach(function(e){ lines = lines.concat(e.split("\n")); });
      buf = lines;
      if (buf.length > BUFCAP) buf = buf.slice(buf.length - BUFCAP);
      rerender();
      if (buf.length === 0) note("nenhuma entrada no intervalo");
      setDot("range");
      rangeInfoBanner((d.entries || []).length, dispFrom, dispTo, d.truncated);
    }).catch(function(){ setDot("err", "parado"); showBanner("falha ao buscar o intervalo"); });
  }
  function rangeInfoBanner(n, from, to, truncated){
    banner.textContent = "";
    banner.className = "info";
    var span = document.createElement("span");
    span.textContent = "Intervalo · " + n + (n === 1 ? " entrada" : " entradas") +
      (truncated ? " (limite atingido — refine o intervalo)" : "") + "   " +
      (from || "início") + "  →  " + (to || "fim");
    banner.appendChild(span);
    var btn = document.createElement("button");
    btn.className = "icobtn";
    btn.textContent = "voltar ao vivo";
    btn.onclick = function(){ connect(file); };
    banner.appendChild(btn);
    banner.style.display = "block";
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
    this.textContent = paused ? "▶" : "⏸";
    this.title = paused ? "retomar a rolagem" : "pausar a rolagem (as linhas continuam chegando)";
    if (!paused) { pending = 0; rerender(); }
    updateStat();
  });
  $("clear").addEventListener("click", function(){
    buf = []; pending = 0;
    logEl.textContent = ""; empty = false;
    updateStat();
  });
  $("tz").addEventListener("click", function(){
    tz.mode = tz.mode === "browser" ? "server" : "browser";
    try { localStorage.setItem(TZKEY, tz.mode); } catch(e){}
    updateTzBtn();
    rerender();
  });
  $("rangebtn").addEventListener("click", function(ev){
    ev.stopPropagation();
    if ($("rangebox").style.display !== "none") { closeRange(); return; }
    openRange();
  });
  $("rgapply").addEventListener("click", applyRange);
  $("rgclear").addEventListener("click", function(){ $("rgfrom").value = ""; $("rgto").value = ""; });
  document.addEventListener("click", function(ev){
    var box = $("rangebox");
    if (box.style.display === "none") return;
    if (box.contains(ev.target) || ev.target === $("rangebtn")) return;
    closeRange();
  });
  document.addEventListener("keydown", function(ev){ if (ev.key === "Escape") closeRange(); });

  loadFiles();
  connect("server.log");
})();
</script>
</body>
</html>
`
