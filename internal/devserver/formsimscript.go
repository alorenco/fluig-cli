package devserver

// formSimJS é o runtime injetado no preview de formulários: executa o
// events/displayFields.js local com a API server-side emulada e monta o
// painel flutuante de simulação (WKNumState/formMode/WKUser/vars extras).
// Sem dependências — o próprio formulário carrega jQuery/DatasetFactory, mas
// o runtime só usa DOM e XMLHttpRequest.
//
// Mantido como const Go (sem template literal/backtick no JS) e injetado via
// /_dev/formsim.js. Mensagens em pt-BR (regra do projeto).
const formSimJS = `(function () {
  "use strict";
  var boot = window.__fluigcliFormSim;
  if (!boot || window.__fluigcliFormSimLoaded) return;
  window.__fluigcliFormSimLoaded = true;

  var KEY = "fluigcli.formsim." + boot.folder;

  function loadCfg() {
    try { return JSON.parse(localStorage.getItem(KEY) || "null"); } catch (e) { return null; }
  }
  function saveCfg(c) { try { localStorage.setItem(KEY, JSON.stringify(c)); } catch (e) {} }

  var cfg = loadCfg();
  var firstVisit = !cfg;
  if (!cfg) cfg = { enabled: true, formMode: "ADD", wkNumState: "0", wkUser: "", processId: "", processVersion: 0, vars: {} };

  // Relatório da execução, exibido no painel (avisos tardios — ex.: stub do
  // portal usado no document.ready do form — re-renderizam os detalhes).
  var report = { ran: false, error: null, reads: [], sets: [], warns: [], unknown: [] };
  function warn(msg) {
    if (report.warns.indexOf(msg) < 0) {
      report.warns.push(msg);
      if (root) renderStatus();
    }
  }

  // Stub do frame pai do portal: numa solicitação real o formulário roda num
  // iframe e o pai expõe ECM.attachmentTable (anexos). No preview parent ===
  // window, e o ECM que existir veio dos scripts do portal carregados pelo
  // próprio form (namespace SEM attachmentTable) — por isso o stub MESCLA:
  // completa só o que falta, sem sobrescrever nada da página.
  function installPortalStub() {
    var ecm = window.ECM || (window.ECM = {});
    if (!ecm.attachmentTable) {
      ecm.attachmentTable = {
        getData: function () {
          warn("parent.ECM.attachmentTable.getData() emulado — no preview a solicitação não tem anexos");
          return [];
        }
      };
    }
  }

  // Tabelas pai×filho (wdkAddChild/fnWdkRemoveChild): o render do Fluig
  // marca a primeira linha do tbody de cada <table tablename="..."> como
  // linha-modelo (detail="true" detailname="<tabela>", escondida), injeta a
  // máquina REAL (/ecm_resources/resources/assets/forms/wdkdetail.js) e
  // semeia o contador com WdksetNewId('{"tabela":N}'). O preview replica a
  // marcação aqui e o dev server injeta o wdkdetail.js do próprio servidor
  // (quando existe) — a emulação local só entra sem a máquina real.
  function installWdkMachine() {
    var tables = document.querySelectorAll("table[tablename]");
    if (!tables.length) return;
    var seeds = {};
    tables.forEach(function (table) {
      var name = table.getAttribute("tablename");
      var tb = table.tBodies && table.tBodies[0];
      if (!name || !tb || !tb.rows.length) return;
      var model = tb.rows[0];
      model.setAttribute("detail", "true");
      model.setAttribute("detailname", name);
      model.style.display = "none";
      var max = 0;
      table.querySelectorAll("[name]").forEach(function (el) {
        var m = /___(\d+)$/.exec(el.getAttribute("name") || "");
        if (m) max = Math.max(max, parseInt(m[1], 10));
      });
      seeds[name] = max;
    });
    if (typeof window.wdkAddChild === "function" && typeof window.WdksetNewId === "function") {
      window.WdksetNewId(JSON.stringify(seeds)); // máquina real do servidor
    } else {
      installWdkStub();
    }
  }

  // Emulação local da máquina, fiel à semântica do wdkdetail.js: clona a
  // linha-modelo renumerando name/id/for para ___N e devolve N.
  function installWdkStub() {
    if (typeof window.wdkAddChild === "function") return;

    function suffix(v, n) { return v.replace(/___\d+$/, "") + "___" + n; }

    window.wdkAddChild = function (tableName) {
      var model = document.querySelector("[detail=\"true\"][detailname=\"" + tableName + "\"]");
      var table = document.getElementById(tableName) ||
        document.querySelector("table[tablename=\"" + tableName + "\"]");
      if (!table || !model) {
        warn("wdkAddChild: tabela \"" + tableName + "\" sem linha-modelo no preview");
        return 0;
      }
      var max = 0;
      table.querySelectorAll("[name]").forEach(function (el) {
        var m = /___(\d+)$/.exec(el.getAttribute("name") || "");
        if (m) max = Math.max(max, parseInt(m[1], 10));
      });
      var n = max + 1;
      var clone = model.cloneNode(true);
      clone.removeAttribute("detail");
      clone.removeAttribute("detailname");
      clone.style.display = "";
      var all = clone.querySelectorAll("*");
      for (var i = 0; i < all.length; i++) {
        var el = all[i];
        if (el.getAttribute("name")) el.setAttribute("name", suffix(el.getAttribute("name"), n));
        if (el.id) el.id = suffix(el.id, n);
        if (el.getAttribute("for")) el.setAttribute("for", suffix(el.getAttribute("for"), n));
      }
      model.parentNode.appendChild(clone);
      return n;
    };

    window.fnWdkRemoveChild = function (el) {
      var tr = el && el.closest ? el.closest("tr") : null;
      if (tr && tr.parentNode) tr.parentNode.removeChild(tr);
      else warn("fnWdkRemoveChild: não achei a linha para remover");
    };
  }

  // --- shims da API server-side de formulário ---

  function findFields(name) {
    var els = document.getElementsByName(name);
    if (els && els.length) return Array.prototype.slice.call(els);
    var el = document.getElementById(name);
    return el ? [el] : [];
  }
  function setField(name, value) {
    var els = findFields(name);
    if (!els.length) { warn("form.setValue: campo \"" + name + "\" não existe no HTML"); return; }
    var v = value == null ? "" : String(value);
    els.forEach(function (el) {
      var t = (el.type || "").toLowerCase();
      if (t === "radio" || t === "checkbox") el.checked = el.value === v;
      else if ("value" in el) el.value = v;
      else el.textContent = v;
    });
    report.sets.push(name + " = " + JSON.stringify(v));
  }
  function getField(name) {
    var els = findFields(name);
    if (!els.length) return "";
    if ((els[0].type || "").toLowerCase() === "radio") {
      for (var i = 0; i < els.length; i++) if (els[i].checked) return els[i].value;
      return "";
    }
    return els[0].value != null ? els[0].value : "";
  }

  function wkVars() {
    var vars = {
      WKNumState: cfg.wkNumState == null || cfg.wkNumState === "" ? "0" : String(cfg.wkNumState),
      WKUser: cfg.wkUser || "",
      WKCompany: String(boot.companyId || 1)
    };
    if (cfg.processId) { vars.WKDef = cfg.processId; }
    if (cfg.processVersion) { vars.WKVersDef = String(cfg.processVersion); }
    var extra = cfg.vars || {};
    for (var k in extra) { if (Object.prototype.hasOwnProperty.call(extra, k)) vars[k] = extra[k]; }
    return vars;
  }

  // --- emulação mínima do Java (Rhino) que os eventos costumam usar ---
  // Cobre o padrão "new java.util.HashMap() + form.getCardData() + iterator".
  // Classe não simulada falha com o caminho claro (java.x.Y) no painel.

  function javaIterator(arr) {
    var i = 0;
    return {
      hasNext: function () { return i < arr.length; },
      next: function () { return arr[i++]; }
    };
  }
  function javaCollection(arr) {
    return {
      iterator: function () { return javaIterator(arr); },
      size: function () { return arr.length; },
      isEmpty: function () { return arr.length === 0; },
      contains: function (v) { return arr.indexOf(v) >= 0; },
      toArray: function () { return arr.slice(); }
    };
  }
  function javaList(arr) {
    var c = javaCollection(arr);
    c.get = function (i) { return arr[i]; };
    c.add = function (v) { arr.push(v); return true; };
    return c;
  }
  function javaMap(obj) {
    function keys() { return Object.keys(obj); }
    return {
      get: function (k) { return Object.prototype.hasOwnProperty.call(obj, k) ? obj[k] : null; },
      put: function (k, v) { var old = obj[k]; obj[k] = v; return old == null ? null : old; },
      containsKey: function (k) { return Object.prototype.hasOwnProperty.call(obj, k); },
      remove: function (k) { var old = obj[k]; delete obj[k]; return old == null ? null : old; },
      size: function () { return keys().length; },
      isEmpty: function () { return keys().length === 0; },
      keySet: function () { return javaCollection(keys()); },
      values: function () { return javaList(keys().map(function (k) { return obj[k]; })); },
      entrySet: function () {
        return javaCollection(keys().map(function (k) {
          return { getKey: function () { return k; }, getValue: function () { return obj[k]; } };
        }));
      }
    };
  }
  function javaMissing(path) {
    var msg = path + " não é simulado no preview (interop Java do Rhino)";
    var f = function () { throw new Error(msg); };
    if (!window.Proxy) return f;
    return new Proxy(f, {
      get: function (t, p) {
        if (typeof p !== "string" || p === "__simpleName") return undefined;
        return javaMissing(path + "." + p);
      },
      construct: function () { throw new Error(msg); },
      apply: function () { throw new Error(msg); }
    });
  }
  function makeJava() {
    function cls(name, ctor) { ctor.__simpleName = name; return ctor; }
    var pkgs = {
      util: {
        HashMap: cls("HashMap", function () { return javaMap({}); }),
        LinkedHashMap: cls("LinkedHashMap", function () { return javaMap({}); }),
        ArrayList: cls("ArrayList", function () { return javaList([]); })
      },
      lang: { String: cls("String", String) }
    };
    if (!window.Proxy) return pkgs;
    function wrapPkg(name, pkg) {
      return new Proxy(pkg, {
        get: function (t, p) {
          if (p in t) return t[p];
          if (typeof p !== "string" || p === "__simpleName") return undefined;
          return javaMissing("java." + name + "." + p);
        }
      });
    }
    return new Proxy(pkgs, {
      get: function (t, p) {
        if (p in t) return wrapPkg(p, t[p]);
        if (typeof p !== "string") return undefined;
        return javaMissing("java." + p);
      }
    });
  }

  // getCardData server-side = mapa campo → valor do card; no preview, o
  // retrato atual dos campos nomeados do DOM (os controles do painel não
  // têm atributo name, então ficam de fora).
  function cardDataObj() {
    var obj = {};
    var els = document.querySelectorAll("input[name],select[name],textarea[name]");
    for (var i = 0; i < els.length; i++) {
      var n = els[i].getAttribute("name");
      if (n && !Object.prototype.hasOwnProperty.call(obj, n)) obj[n] = getField(n);
    }
    return obj;
  }

  // getChildrenIndexes server-side = índices dos filhos no card; no preview,
  // os sufixos ___N presentes dentro da tabela.
  function childrenIndexes(tableName) {
    var scope = document.getElementById(tableName);
    if (!scope) {
      warn("form.getChildrenIndexes: tabela \"" + tableName + "\" não existe no HTML — devolvendo []");
      return [];
    }
    var seen = {}, out = [];
    var els = scope.querySelectorAll("[name]");
    for (var i = 0; i < els.length; i++) {
      var m = /___(\d+)$/.exec(els[i].getAttribute("name") || "");
      if (m && !seen[m[1]]) {
        seen[m[1]] = true;
        out.push(parseInt(m[1], 10));
      }
    }
    out.sort(function (a, b) { return a - b; });
    return out;
  }

  // Selects declarativos (dataset= / datasetkey= / datasetvalue=): o render
  // do servidor popula os <option> consultando o dataset — validado no render
  // real (2026-07-10): opção vazia primeiro quando addblankline="true",
  // depois um option por linha (value=datasetkey, texto=datasetvalue), na
  // ordem do dataset, mantendo os atributos. Replicado com o DatasetFactory
  // cliente da página — dados reais via proxy.
  function populateDatasetSelects() {
    var sels = document.querySelectorAll("select[dataset]");
    if (!sels.length) return;
    if (!window.DatasetFactory) {
      warn("selects com dataset= não populados: DatasetFactory indisponível na página");
      return;
    }
    sels.forEach(function (sel) {
      var ds = sel.getAttribute("dataset");
      var key = sel.getAttribute("datasetkey");
      var val = sel.getAttribute("datasetvalue");
      if (!ds || !key || !val || sel.hasAttribute("data-fluigcli-populated")) return;
      try {
        var result = window.DatasetFactory.getDataset(ds, null, null, null);
        var values = (result && result.values) || [];
        if ((sel.getAttribute("addblankline") || "").toLowerCase() === "true") {
          sel.appendChild(document.createElement("option"));
        }
        values.forEach(function (row) {
          var o = document.createElement("option");
          o.value = row[key] == null ? "" : String(row[key]);
          o.textContent = row[val] == null ? "" : String(row[val]);
          sel.appendChild(o);
        });
        sel.setAttribute("data-fluigcli-populated", "true");
      } catch (e) {
        warn("select " + (sel.name || sel.id || "?") + ": falha ao consultar o dataset " + ds + " — " + ((e && e.message) || e));
      }
    });
  }

  // O DatasetFactory server-side devolve getValue(row, col)/rowsCount; o
  // cliente devolve {columns, values} — o wrapper serve as duas interfaces.
  function wrapDataset(ds) {
    if (ds && typeof ds.getValue !== "function") {
      var values = ds.values || [];
      ds.rowsCount = values.length;
      ds.getValue = function (row, col) {
        var r = values[row];
        return r && Object.prototype.hasOwnProperty.call(r, col) ? r[col] : null;
      };
    }
    return ds;
  }

  function runEvent() {
    if (!boot.event || cfg.enabled === false) return;
    var vars = wkVars();
    var getValue = function (name) {
      var known = Object.prototype.hasOwnProperty.call(vars, name);
      var v = known ? vars[name] : null;
      report.reads.push(name + " = " + (known ? JSON.stringify(v) : "(não simulado)"));
      if (!known && report.unknown.indexOf(name) < 0) report.unknown.push(name);
      return v;
    };
    var formImpl = {
      setValue: setField,
      getValue: getField,
      getFormMode: function () { return cfg.formMode || "ADD"; },
      setEnabled: function (name, enabled) { findFields(name).forEach(function (el) { el.disabled = !enabled; }); },
      getMobile: function () { return false; },
      getCompanyId: function () { return boot.companyId || 1; },
      getCardData: function () { return javaMap(cardDataObj()); },
      getChildrenIndexes: childrenIndexes,
      setVisibleById: function (id, visible) {
        var el = document.getElementById(id);
        if (!el) { warn("form.setVisibleById: elemento \"" + id + "\" não existe no HTML"); return; }
        el.style.display = visible === false || visible === "false" ? "none" : "";
      },
      setShowDisabledFields: function () {},
      setHidePrintLink: function () {},
      setHideDeleteButton: function () {},
      setEnhancedSecurityHiddenInputs: function () {}
    };
    var form = window.Proxy ? new Proxy(formImpl, {
      get: function (t, p) {
        if (p in t) return t[p];
        if (typeof p !== "string") return undefined;
        return function () { warn("form." + p + "() não é simulado (ignorado)"); };
      }
    }) : formImpl;
    var parts = [];
    var customHTML = { append: function (h) { parts.push(String(h)); } };
    var realDF = window.DatasetFactory;
    var DF = realDF ? {
      getDataset: function (a, b, c, d) { return wrapDataset(realDF.getDataset(a, b, c, d)); },
      createConstraint: function () { return realDF.createConstraint.apply(realDF, arguments); }
    } : undefined;
    var javaObj = makeJava();
    var importClassShim = function (cls) {
      if (cls && cls.__simpleName) { window[cls.__simpleName] = cls; return; }
      warn("importClass: classe não simulada no preview (interop Java do Rhino)");
    };
    try {
      var src = boot.event +
        "\n;if (typeof displayFields !== \"function\") throw new Error(\"events/displayFields.js não define displayFields(form, customHTML)\");" +
        "\ndisplayFields(form, customHTML);";
      new Function("getValue", "form", "customHTML", "DatasetFactory", "java", "importClass", src)(
        getValue, form, customHTML, DF || realDF, javaObj, importClassShim);
      report.ran = true;
      if (parts.length) document.body.insertAdjacentHTML("beforeend", parts.join(""));
    } catch (e) {
      report.error = String((e && e.message) || e);
    }
  }

  // --- API local do dev server ---

  function api(path, cb) {
    var x = new XMLHttpRequest();
    x.open("GET", path, true);
    x.onreadystatechange = function () {
      if (x.readyState !== 4) return;
      var data = null;
      try { data = JSON.parse(x.responseText); } catch (e) {}
      if (x.status >= 200 && x.status < 300) cb(null, data);
      else cb((data && data.error) || ("HTTP " + x.status), null);
    };
    x.send();
  }

  // Primeira visita: detecta usuário e processo vinculados e recarrega uma vez
  // com os padrões preenchidos (o evento já rodou com WKNumState=0).
  function autodetect() {
    api("/_dev/api/formsim/context?folder=" + encodeURIComponent(boot.folder), function (err, ctx) {
      var gained = false;
      if (!err && ctx) {
        if (!cfg.wkUser && ctx.userCode) { cfg.wkUser = ctx.userCode; gained = true; }
        if (!cfg.processId && ctx.processes && ctx.processes.length) {
          cfg.processId = ctx.processes[0].processId;
          cfg.processVersion = ctx.processes[0].version || 0;
          gained = true;
        }
      }
      saveCfg(cfg);
      if (gained && boot.event && cfg.enabled !== false) location.reload();
    });
  }

  // --- painel flutuante ---

  var CSS = "" +
    "#fluigcli-sim{position:fixed;right:16px;bottom:16px;z-index:2147483000;" +
    "font:13px/1.45 system-ui,-apple-system,Segoe UI,Roboto,sans-serif;color:#1d2b36}" +
    "#fluigcli-sim *{box-sizing:border-box;font:inherit;color:inherit}" +
    "#fluigcli-sim .chip{display:flex;align-items:center;gap:8px;cursor:pointer;border:1px solid #d5dde5;" +
    "background:#fff;border-radius:999px;padding:7px 14px;box-shadow:0 2px 8px rgba(16,36,54,.18);font-weight:600}" +
    "#fluigcli-sim .dot{width:9px;height:9px;border-radius:50%;background:#9aa7b2}" +
    "#fluigcli-sim .dot.ok{background:#25b26e}#fluigcli-sim .dot.err{background:#e2574c}" +
    "#fluigcli-sim .card{display:none;width:340px;max-height:76vh;overflow:auto;background:#fff;" +
    "border:1px solid #d5dde5;border-radius:12px;box-shadow:0 8px 28px rgba(16,36,54,.25);padding:14px 16px 16px}" +
    "#fluigcli-sim.open .card{display:block}#fluigcli-sim.open .chip{display:none}" +
    "#fluigcli-sim h3{margin:0 0 2px;font-size:14px;display:flex;justify-content:space-between;align-items:center}" +
    "#fluigcli-sim h3 button{border:0;background:none;cursor:pointer;font-size:16px;line-height:1;padding:2px}" +
    "#fluigcli-sim .sub{color:#5a6b7b;font-size:11.5px;margin:0 0 10px}" +
    "#fluigcli-sim label{display:block;margin:9px 0 3px;font-weight:600;font-size:12px}" +
    "#fluigcli-sim select,#fluigcli-sim input[type=text],#fluigcli-sim input[type=number],#fluigcli-sim textarea{" +
    "width:100%;padding:6px 8px;border:1px solid #c9d3dc;border-radius:7px;background:#fff}" +
    "#fluigcli-sim textarea{height:52px;resize:vertical;font-family:ui-monospace,Consolas,monospace;font-size:12px}" +
    "#fluigcli-sim .row{display:flex;gap:8px;align-items:center}" +
    "#fluigcli-sim .row>*{flex:1}" +
    "#fluigcli-sim .btn{margin-top:12px;width:100%;padding:8px 10px;border:0;border-radius:8px;cursor:pointer;" +
    "background:#0c9abe;color:#fff;font-weight:650}" +
    "#fluigcli-sim .btn.sec{background:#eef2f5;color:#1d2b36;margin-top:6px}" +
    "#fluigcli-sim .status{margin:8px 0 0;padding:7px 9px;border-radius:7px;font-size:12px;background:#f2f6f9}" +
    "#fluigcli-sim .status.err{background:#fdecea;color:#8c2f28;white-space:pre-wrap}" +
    "#fluigcli-sim details{margin-top:8px;font-size:12px}" +
    "#fluigcli-sim details ul{margin:4px 0 6px;padding-left:18px}" +
    "#fluigcli-sim .muted{color:#5a6b7b}" +
    "#fluigcli-sim .toggle{display:flex;gap:6px;align-items:center;margin-top:10px;font-size:12px}" +
    "#fluigcli-sim .toggle input{width:auto}" +
    "@media (prefers-color-scheme: dark){" +
    "#fluigcli-sim{color:#e6edf3}" +
    "#fluigcli-sim .chip,#fluigcli-sim .card{background:#1b232d;border-color:#2b3742}" +
    "#fluigcli-sim select,#fluigcli-sim input[type=text],#fluigcli-sim input[type=number],#fluigcli-sim textarea{" +
    "background:#12181f;border-color:#2b3742}" +
    "#fluigcli-sim .btn.sec{background:#2b3742;color:#e6edf3}" +
    "#fluigcli-sim .status{background:#232d38}" +
    "#fluigcli-sim .status.err{background:#4a2320;color:#f3b0aa}" +
    "#fluigcli-sim .sub,#fluigcli-sim .muted{color:#93a4b4}}";

  function esc(s) {
    return String(s).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");
  }

  var root, els = {};

  function buildPanel() {
    var style = document.createElement("style");
    style.textContent = CSS;
    document.head.appendChild(style);

    root = document.createElement("div");
    root.id = "fluigcli-sim";
    root.innerHTML = "" +
      "<div class=\"chip\" data-act=\"open\"><span class=\"dot\"></span>Simulação</div>" +
      "<div class=\"card\">" +
      "<h3>Simulação de processo <button type=\"button\" title=\"Fechar\" data-act=\"close\">×</button></h3>" +
      "<p class=\"sub\">fluigcli dev · " + esc(boot.folder) + "</p>" +
      "<div class=\"status\" data-el=\"status\"></div>" +
      "<label>Processo</label>" +
      "<div class=\"row\"><select data-el=\"process\"><option value=\"\">— sem processo —</option></select>" +
      "<button type=\"button\" class=\"btn sec\" style=\"flex:0 0 34px;margin:0\" title=\"Recarregar do servidor\" data-act=\"refresh\">↻</button></div>" +
      "<label>Etapa (WKNumState)</label>" +
      "<select data-el=\"state\" style=\"margin-bottom:6px\"><option value=\"\">— número manual —</option></select>" +
      "<input type=\"number\" data-el=\"statenum\" min=\"0\" step=\"1\">" +
      "<label>Modo do formulário</label>" +
      "<select data-el=\"mode\"><option>ADD</option><option>MOD</option><option>VIEW</option></select>" +
      "<label>Usuário (WKUser)</label>" +
      "<input type=\"text\" data-el=\"user\" placeholder=\"userCode do colaborador\">" +
      "<label>Outras variáveis (CHAVE=valor, uma por linha)</label>" +
      "<textarea data-el=\"vars\" placeholder=\"WKNumProces=1234\"></textarea>" +
      "<div class=\"toggle\"><input type=\"checkbox\" data-el=\"enabled\" id=\"fluigcli-sim-on\">" +
      "<label for=\"fluigcli-sim-on\" style=\"margin:0;font-weight:600\">Simulação ligada</label></div>" +
      "<button type=\"button\" class=\"btn\" data-act=\"apply\">Aplicar e recarregar</button>" +
      "<details><summary>Execução do displayFields</summary><div data-el=\"detail\" class=\"muted\"></div></details>" +
      "</div>";
    document.body.appendChild(root);

    root.querySelectorAll("[data-el]").forEach(function (el) { els[el.getAttribute("data-el")] = el; });

    root.addEventListener("click", function (ev) {
      var act = ev.target.getAttribute && ev.target.getAttribute("data-act");
      if (act === "open") { root.classList.add("open"); onOpen(false); }
      if (act === "close") { root.classList.remove("open"); }
      if (act === "refresh") { onOpen(true); }
      if (act === "apply") { apply(); }
    });
    els.process.addEventListener("change", function () {
      var opt = els.process.selectedOptions[0];
      loadStates(els.process.value, opt ? parseInt(opt.getAttribute("data-version") || "0", 10) : 0, false);
    });
    els.state.addEventListener("change", function () {
      if (els.state.value !== "") els.statenum.value = els.state.value;
    });

    els.statenum.value = cfg.wkNumState == null ? "0" : cfg.wkNumState;
    els.mode.value = cfg.formMode || "ADD";
    els.user.value = cfg.wkUser || "";
    els.enabled.checked = cfg.enabled !== false;
    var lines = [];
    var extra = cfg.vars || {};
    for (var k in extra) { if (Object.prototype.hasOwnProperty.call(extra, k)) lines.push(k + "=" + extra[k]); }
    els.vars.value = lines.join("\n");
    renderStatus();
  }

  function renderStatus() {
    var dot = root.querySelector(".dot");
    var st = els.status;
    if (!boot.event) {
      st.textContent = "Este formulário não tem events/displayFields.js — as variáveis simuladas só têm efeito através dele.";
      dot.className = "dot";
    } else if (cfg.enabled === false) {
      st.textContent = "Simulação desligada — o formulário está no preview cru.";
      dot.className = "dot";
    } else if (report.error) {
      st.className = "status err";
      st.textContent = "displayFields falhou: " + report.error;
      dot.className = "dot err";
    } else if (report.ran) {
      st.textContent = "displayFields executado com WKNumState=" + wkVars().WKNumState +
        " e modo " + (cfg.formMode || "ADD") + ".";
      dot.className = "dot ok";
    } else {
      st.textContent = "displayFields ainda não executou.";
      dot.className = "dot";
    }
    var d = [];
    if (report.unknown.length) d.push("<b>getValue não simulado:</b><ul><li>" + report.unknown.map(esc).join("</li><li>") + "</li></ul>");
    if (report.reads.length) d.push("<b>Leituras:</b><ul><li>" + report.reads.map(esc).join("</li><li>") + "</li></ul>");
    if (report.sets.length) d.push("<b>form.setValue:</b><ul><li>" + report.sets.map(esc).join("</li><li>") + "</li></ul>");
    if (report.warns.length) d.push("<b>Avisos:</b><ul><li>" + report.warns.map(esc).join("</li><li>") + "</li></ul>");
    els.detail.innerHTML = d.join("") || "Nada executado.";
  }

  var opened = false;
  function onOpen(force) {
    if (opened && !force) return;
    opened = true;
    api("/_dev/api/formsim/context?folder=" + encodeURIComponent(boot.folder) + (force ? "&force=1" : ""), function (err, ctx) {
      if (err) { statusNote("Contexto indisponível: " + err); }
      else if (ctx && !els.user.value && ctx.userCode) { els.user.value = ctx.userCode; }
      api("/_dev/api/formsim/processes" + (force ? "?force=1" : ""), function (perr, procs) {
        if (perr) { statusNote("Processos indisponíveis: " + perr); return; }
        fillProcesses(procs || [], (ctx && ctx.processes) || []);
      });
    });
  }

  function statusNote(msg) {
    els.status.className = "status err";
    els.status.textContent = msg;
  }

  function fillProcesses(all, detected) {
    var sel = els.process;
    sel.innerHTML = "<option value=\"\">— sem processo —</option>";
    var seen = {};
    detected.forEach(function (p) {
      seen[p.processId] = true;
      var o = document.createElement("option");
      o.value = p.processId;
      o.setAttribute("data-version", String(p.version || 0));
      o.textContent = "★ " + (p.description || p.processId) + " (vinculado ao formulário)";
      sel.appendChild(o);
    });
    all.forEach(function (p) {
      if (seen[p.id]) return;
      var o = document.createElement("option");
      o.value = p.id;
      o.textContent = (p.description || p.id) + (p.active ? "" : " (inativo)");
      sel.appendChild(o);
    });
    sel.value = cfg.processId || "";
    if (sel.value) {
      var opt = sel.selectedOptions[0];
      loadStates(sel.value, cfg.processVersion || (opt ? parseInt(opt.getAttribute("data-version") || "0", 10) : 0), true);
    }
  }

  function loadStates(processId, version, keepCurrent) {
    var sel = els.state;
    sel.innerHTML = "<option value=\"\">— número manual —</option>";
    if (!processId) return;
    sel.disabled = true;
    api("/_dev/api/formsim/states?process=" + encodeURIComponent(processId) + "&version=" + (version || 0), function (err, data) {
      sel.disabled = false;
      if (err) { statusNote("Etapas indisponíveis: " + err); return; }
      (data.states || []).forEach(function (st) {
        var o = document.createElement("option");
        o.value = String(st.sequence);
        var kind = st.bpmnType || st.stateType || "";
        o.textContent = st.sequence + " — " + (st.stateName || "(sem nome)") + (kind ? " · " + kind : "");
        if (st.stateDescription) o.title = st.stateDescription;
        sel.appendChild(o);
      });
      els.process.selectedOptions[0] && els.process.selectedOptions[0].setAttribute("data-version", String(data.version || version || 0));
      var cur = keepCurrent ? String(cfg.wkNumState) : els.statenum.value;
      if (cur !== "" && sel.querySelector("option[value=\"" + cur + "\"]")) sel.value = cur;
    });
  }

  function apply() {
    cfg.enabled = els.enabled.checked;
    cfg.wkNumState = els.statenum.value === "" ? "0" : els.statenum.value;
    cfg.formMode = els.mode.value;
    cfg.wkUser = els.user.value.trim();
    cfg.processId = els.process.value;
    var opt = els.process.selectedOptions[0];
    cfg.processVersion = opt ? parseInt(opt.getAttribute("data-version") || "0", 10) : 0;
    cfg.vars = {};
    els.vars.value.split("\n").forEach(function (line) {
      var i = line.indexOf("=");
      if (i > 0) cfg.vars[line.slice(0, i).trim()] = line.slice(i + 1).trim();
    });
    saveCfg(cfg);
    location.reload();
  }

  // Ordem: os stubs do ambiente do portal entram antes de tudo (o form usa
  // no document.ready e nos onclick); o evento roda JÁ (antes do load do
  // formulário, que lê os campos preenchidos por ele); o painel em seguida.
  installPortalStub();
  installWdkMachine();
  populateDatasetSelects(); // antes do evento: displayFields pode selecionar valor
  runEvent();
  buildPanel();
  if (firstVisit) autodetect();
})();
`
