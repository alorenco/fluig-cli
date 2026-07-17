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
    // Na moldura do modo de tela o preview roda num iframe: parent vira a
    // página da moldura — espelha o ECM nela (mesma origem) para o
    // parent.ECM.* dos forms continuar funcionando.
    try {
      if (window.parent && window.parent !== window && !window.parent.ECM) {
        window.parent.ECM = window.ECM;
      }
    } catch (e) {}
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

  // makeEventEnv monta o ambiente compartilhado de execução dos eventos de
  // formulário (displayFields e validateForm): shims de getValue (sobre o
  // mapa vars), form, customHTML, DatasetFactory, java e importClass.
  function makeEventEnv(vars) {
    var env = {};
    env.getValue = function (name) {
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
      getMobile: function () { return cfg.mobile === true; },
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
    env.form = window.Proxy ? new Proxy(formImpl, {
      get: function (t, p) {
        if (p in t) return t[p];
        if (typeof p !== "string") return undefined;
        return function () { warn("form." + p + "() não é simulado (ignorado)"); };
      }
    }) : formImpl;
    env.parts = [];
    env.customHTML = { append: function (h) { env.parts.push(String(h)); } };
    var realDF = window.DatasetFactory;
    env.DatasetFactory = realDF ? {
      getDataset: function (a, b, c, d) { return wrapDataset(realDF.getDataset(a, b, c, d)); },
      createConstraint: function () { return realDF.createConstraint.apply(realDF, arguments); }
    } : realDF;
    env.java = makeJava();
    env.importClass = function (cls) {
      if (cls && cls.__simpleName) { window[cls.__simpleName] = cls; return; }
      warn("importClass: classe não simulada no preview (interop Java do Rhino)");
    };
    // log.* é a API oficial de log dos eventos server-side (escreve no log do
    // servidor) — no preview vai para o console do navegador e para o
    // relatório do painel.
    function makeLogFn(level) {
      return function (msg) {
        var text = "log." + level + ": " + String(msg);
        report.reads.push(text);
        var fn = level === "error" ? "error" : level === "warn" ? "warn" : "info";
        if (window.console && console[fn]) console[fn]("[fluigcli evento] " + text);
      };
    }
    env.log = {
      info: makeLogFn("info"),
      warn: makeLogFn("warn"),
      error: makeLogFn("error"),
      debug: makeLogFn("debug")
    };
    env.fluigAPI = makeFluigAPI();
    return env;
  }

  // fluigAPI é o SDK server-side dos eventos (validado no render real:
  // getUserService().getCurrent() preenche dados do usuário). O shim cobre o
  // usuário atual com dados REAIS (dataset colleague via proxy, síncrono);
  // serviço/método fora disso falha com o caminho claro no painel.
  var fluigAPIUserCache = null;
  function makeFluigAPI() {
    function proxyMissing(obj, path) {
      if (!window.Proxy) return obj;
      return new Proxy(obj, {
        get: function (t, p) {
          if (p in t) return t[p];
          if (typeof p !== "string") return undefined;
          return function () { throw new Error(path + "." + p + "() não é simulado no preview"); };
        }
      });
    }
    function currentUser() {
      if (fluigAPIUserCache) return fluigAPIUserCache;
      var code = cfg.wkUser || "";
      var name = "", mail = "", login = "";
      try {
        if (window.DatasetFactory && code) {
          // O tipo da constraint é o ENUM NUMÉRICO do cliente (string "MUST"
          // dá ClassCastException no servidor — visto em teste do mantenedor).
          var must = window.ConstraintType && window.ConstraintType.MUST !== undefined
            ? window.ConstraintType.MUST : 1;
          var c = window.DatasetFactory.createConstraint("colleagueId", code, code, must);
          var ds = window.DatasetFactory.getDataset("colleague",
            ["colleagueId", "colleagueName", "mail", "login"], [c], null);
          var row = ds && ds.values && ds.values[0];
          if (row) {
            name = row.colleagueName || "";
            mail = row.mail || "";
            login = row.login || "";
          }
        }
      } catch (e) {
        warn("fluigAPI.getUserService().getCurrent(): falha ao consultar o dataset colleague — " + ((e && e.message) || e));
      }
      if (!code) warn("fluigAPI.getUserService().getCurrent(): sem WKUser simulado — dados vazios (defina na Simulação)");
      var user = {
        getCode: function () { return code; },
        getEmail: function () { return mail; },
        getFullName: function () { return name; },
        getLogin: function () { return login || code; },
        getTenantId: function () { return boot.companyId || 1; },
        getUserTimeZone: function () { return Intl && Intl.DateTimeFormat ? Intl.DateTimeFormat().resolvedOptions().timeZone : ""; }
      };
      // Getter desconhecido do usuário: aviso + vazio (não derruba o evento).
      fluigAPIUserCache = window.Proxy ? new Proxy(user, {
        get: function (t, p) {
          if (p in t) return t[p];
          if (typeof p !== "string") return undefined;
          return function () {
            warn("fluigAPI.getUserService().getCurrent()." + p + "() não é simulado (devolve vazio)");
            return "";
          };
        }
      }) : user;
      return fluigAPIUserCache;
    }
    return proxyMissing({
      getUserService: function () {
        return proxyMissing({ getCurrent: currentUser }, "fluigAPI.getUserService()");
      }
    }, "fluigAPI");
  }

  // execEvent roda um fonte de evento no ambiente dado. Deixa o throw subir
  // — quem chama decide o que ele significa (erro no display, bloqueio na
  // validação).
  function execEvent(env, src) {
    new Function("getValue", "form", "customHTML", "DatasetFactory", "java", "importClass", "log", "fluigAPI", src)(
      env.getValue, env.form, env.customHTML, env.DatasetFactory, env.java, env.importClass, env.log, env.fluigAPI);
  }

  function runEvent() {
    if (!boot.event || cfg.enabled === false) return;
    var env = makeEventEnv(wkVars());
    try {
      execEvent(env, boot.event +
        "\n;if (typeof displayFields !== \"function\") throw new Error(\"events/displayFields.js não define displayFields(form, customHTML)\");" +
        "\ndisplayFields(form, customHTML);");
      report.ran = true;
      if (env.parts.length) document.body.insertAdjacentHTML("beforeend", env.parts.join(""));
    } catch (e) {
      report.error = String((e && e.message) || e);
    }
  }

  // runValidation simula os dois gatilhos do portal: Salvar (validateForm
  // com WKNextState nulo — a variável só existe no movimento) e Enviar
  // (beforeSendValidate client-side primeiro, como o portal faz, depois o
  // validateForm com o WKNextState escolhido). Nada é gravado.
  // Devolve: {ok:true} | {ok:false, runtime:bool, msg}. O throw de validação
  // é string (às vezes HTML) → msg renderizável; Error = defeito no evento.
  function runValidation(send, nextState) {
    var vars = wkVars();
    vars.WKNextState = send ? String(nextState) : null;
    var env = makeEventEnv(vars);
    try {
      if (send && typeof window.beforeSendValidate === "function") {
        window.beforeSendValidate(parseInt(cfg.wkNumState, 10) || 0, parseInt(nextState, 10) || 0);
      }
      if (boot.validate) {
        execEvent(env, boot.validate +
          "\n;if (typeof validateForm !== \"function\") throw new Error(\"events/validateForm.js não define validateForm(form)\");" +
          "\nvalidateForm(form);");
      }
      return { ok: true };
    } catch (e) {
      if (e instanceof Error) return { ok: false, runtime: true, msg: String(e.message || e) };
      return { ok: false, runtime: false, msg: String(e) };
    }
  }

  // --- publicação no servidor (🚀) ---

  var deployServers = null; // cache da lista por carga de página

  // deployInfo é a linha de status do diálogo — só aparece quando há algo a
  // dizer (erro ou progresso); vazio esconde.
  function deployInfo(msg, isErr) {
    els.depinfo.className = isErr ? "status err" : "status";
    els.depinfo.textContent = msg || "";
    els.depinfo.style.display = msg ? "" : "none";
  }

  function selectedDeployServer() {
    if (!deployServers) return null;
    for (var i = 0; i < deployServers.length; i++) {
      if (deployServers[i].name === els.depserver.value) return deployServers[i];
    }
    return null;
  }

  function openDeploy() {
    if (deployServers) return; // já carregado nesta página
    api("/_dev/api/formsim/deploy/servers", function (err, data) {
      if (err || !data || !Array.isArray(data.servers)) {
        deployInfo("Servidores indisponíveis: " + (err || "resposta inesperada"), true);
        return;
      }
      deployServers = data.servers;
      var sel = els.depserver;
      sel.innerHTML = "";
      var pick = "";
      deployServers.forEach(function (srv) {
        var o = document.createElement("option");
        o.value = srv.name;
        var marks = [];
        if (srv.env) marks.push(srv.env === "prod" ? "PRODUÇÃO" : srv.env);
        if (srv.current) marks.push("conectado");
        else if (srv.default) marks.push("padrão");
        o.textContent = srv.name + (marks.length ? " · " + marks.join(" · ") : "");
        o.title = srv.url;
        sel.appendChild(o);
        if (srv.current) pick = srv.name;
        if (!pick && srv.default) pick = srv.name;
      });
      if (pick) sel.value = pick;
      onDeployServerChange();
      loadDeployForms();
    });
  }

  function onDeployServerChange() {
    var srv = selectedDeployServer();
    els.depprod.style.display = srv && srv.env === "prod" ? "" : "none";
    els.depconfirm.value = "";
    folderStack = [];
    foldersLoadedFor = ""; // pastas são por servidor
    updateDeployButton();
  }

  // updateDeployButton habilita o Publicar só com tudo OK: busca de
  // formulários concluída, obrigatórios preenchidos (nome/dataset/descritor/
  // pasta na criação; dataset/descritor no update) e, em produção, o nome do
  // servidor confirmado.
  function updateDeployButton() {
    var ok = !els.depform.disabled && !!selectedDeployServer();
    if (ok) {
      var create = els.depform.value === "0" || els.depform.value === "";
      if (create) {
        ok = !!(els.depname.value.trim() &&
          els.depdataset.value.trim() &&
          els.depcard.value &&
          (parseInt(els.depparent.value, 10) || 0) > 0);
      } else {
        ok = !!(els.depdataset.value.trim() && els.depcard.value);
      }
    }
    if (ok) {
      var srv = selectedDeployServer();
      if (srv && srv.env === "prod" && els.depconfirm.value.trim() !== srv.name) ok = false;
    }
    els.depgo.disabled = !ok;
  }

  // Campos candidatos a descritor: os inputs nomeados do form local (sem os
  // ___N das tabelas pai×filho).
  function formFieldNames() {
    var seen = {}, out = [];
    var els2 = document.querySelectorAll("input[name],select[name],textarea[name]");
    for (var i = 0; i < els2.length; i++) {
      var n = els2[i].getAttribute("name");
      if (!n || /___\d+$/.test(n) || seen[n]) continue;
      seen[n] = true;
      out.push(n);
    }
    return out;
  }

  // fillDescriptor popula o select do campo descritor com os campos do form;
  // valor atual do servidor fora da lista vira opção "(atual)".
  function fillDescriptor(current) {
    var sel = els.depcard;
    sel.innerHTML = "<option value=\"\">— escolha o campo —</option>";
    var found = false;
    formFieldNames().forEach(function (n) {
      var o = document.createElement("option");
      o.value = n;
      o.textContent = n;
      sel.appendChild(o);
      if (n === current) found = true;
    });
    if (current && !found) {
      var o = document.createElement("option");
      o.value = current;
      o.textContent = current + " (atual)";
      sel.appendChild(o);
    }
    sel.value = current || "";
  }

  // Sugestão de dataset na criação: ds_{{nome_formulario}} — segue o nome
  // digitado até o dev editar o campo do dataset à mão.
  var dsTouched = false;
  function dsSuggest(name) {
    return "ds_" + String(name).toLowerCase().replace(/[^a-z0-9_]+/g, "_").replace(/^_+|_+$/g, "");
  }
  function applyDsSuggestion() {
    if (dsTouched) return;
    els.depdataset.value = dsSuggest(els.depname.value || boot.folder);
  }

  // --- navegador de pastas do GED (por servidor) ---

  var folderStack = [];      // trilha: [{id, name}]
  var foldersLoadedFor = ""; // servidor cujas pastas estão carregadas

  function renderFolderPath() {
    if (!folderStack.length) {
      els.deppath.textContent = "";
      return;
    }
    els.deppath.textContent = "Publicar em: " +
      folderStack.map(function (f) { return f.name; }).join(" / ");
  }

  function loadDeployFolders(parentId) {
    var sel = els.depfolder;
    sel.disabled = true;
    sel.innerHTML = "<option value=\"\">— carregando… —</option>";
    apiPost("/_dev/api/formsim/deploy/folders",
      { server: els.depserver.value, password: els.deppass.value, parentId: parentId },
      function (err, data) {
        if (err) {
          // Fallback: sem a navegação, o id pode ser digitado à mão.
          sel.innerHTML = "<option value=\"\">— pastas indisponíveis —</option>";
          els.depparent.style.display = "";
          deployInfo("Pastas do GED indisponíveis (" + err + ") — digite o id da pasta abaixo.", true);
          updateDeployButton();
          return;
        }
        els.depparent.style.display = "none";
        foldersLoadedFor = els.depserver.value;
        sel.disabled = false;
        sel.innerHTML = "";
        var o0 = document.createElement("option");
        o0.value = "";
        o0.textContent = folderStack.length ? "— escolher subpasta —" : "— escolher pasta —";
        sel.appendChild(o0);
        if (folderStack.length) {
          var up = document.createElement("option");
          up.value = "__up";
          up.textContent = "⬅ voltar";
          sel.appendChild(up);
        }
        (data.folders || []).forEach(function (f) {
          var o = document.createElement("option");
          o.value = String(f.id);
          o.textContent = f.name;
          sel.appendChild(o);
        });
        renderFolderPath();
        updateDeployButton();
      });
  }

  function onDeployFolderPick() {
    var v = els.depfolder.value;
    if (v === "") return;
    if (v === "__up") {
      folderStack.pop();
      var last = folderStack.length ? folderStack[folderStack.length - 1] : null;
      els.depparent.value = last ? last.id : "";
      loadDeployFolders(last ? last.id : 0);
      return;
    }
    var opt = els.depfolder.selectedOptions[0];
    folderStack.push({ id: parseInt(v, 10), name: opt.textContent });
    els.depparent.value = v;
    loadDeployFolders(parseInt(v, 10)); // desce para permitir refinar
  }

  function loadDeployForms() {
    var sel = els.depform;
    sel.disabled = true;
    sel.innerHTML = "<option value=\"\">— carregando… —</option>";
    updateDeployButton(); // busca em andamento → Publicar desabilitado
    apiPost("/_dev/api/formsim/deploy/forms",
      { server: els.depserver.value, password: els.deppass.value, folder: boot.folder },
      function (err, data) {
        if (data && data.needsPassword) {
          els.deppassrow.style.display = "";
          deployInfo("Sem credencial salva para este servidor — digite a senha e Enter.", true);
          sel.innerHTML = "<option value=\"\">— aguardando a senha —</option>";
          els.deppass.focus();
          updateDeployButton();
          return;
        }
        if (err) {
          deployInfo("Formulários indisponíveis: " + err, true);
          updateDeployButton();
          return;
        }
        els.deppassrow.style.display = "none";
        sel.disabled = false;
        sel.innerHTML = "";
        var oNew = document.createElement("option");
        oNew.value = "0";
        oNew.textContent = "— criar novo formulário —";
        sel.appendChild(oNew);
        var byName = "";
        (data.forms || []).forEach(function (f) {
          var o = document.createElement("option");
          o.value = String(f.documentId);
          o.textContent = f.name + " (" + f.documentId + ")";
          o.setAttribute("data-dataset", f.datasetName || "");
          o.setAttribute("data-card", f.cardDescription || "");
          if (f.datasetName) o.title = "dataset: " + f.datasetName;
          sel.appendChild(o);
          if (f.name === boot.folder) byName = String(f.documentId);
        });
        // Pré-seleção: vínculo do forms.json > nome igual ao da pasta > criar.
        var linked = data.linkedDocumentId ? String(data.linkedDocumentId) : "";
        if (linked && sel.querySelector("option[value=\"" + linked + "\"]")) sel.value = linked;
        else if (byName) sel.value = byName;
        else sel.value = "0";
        var dl = document.getElementById("fluigcli-dep-ds");
        dl.innerHTML = "";
        (data.datasets || []).forEach(function (name) {
          var o = document.createElement("option");
          o.value = name;
          dl.appendChild(o);
        });
        deployInfo("");
        onDeployFormChange();
        updateDeployButton();
      });
  }

  function onDeployFormChange() {
    var create = els.depform.value === "0" || els.depform.value === "";
    els.depcreate.style.display = create ? "" : "none";
    els.depcreate2.style.display = create ? "" : "none";
    els.depexisting.style.display = create ? "none" : "";
    if (create) {
      if (!els.depname.value) els.depname.value = boot.folder;
      dsTouched = false;
      applyDsSuggestion();
      fillDescriptor("");
      // Pastas carregam sob demanda, uma vez por servidor.
      if (foldersLoadedFor !== els.depserver.value) {
        folderStack = [];
        loadDeployFolders(0);
      }
    } else {
      // Padrão Fluig: dataset e descritor do formulário aparecem no update.
      var opt = els.depform.selectedOptions[0];
      els.depdataset.value = opt ? opt.getAttribute("data-dataset") || "" : "";
      fillDescriptor(opt ? opt.getAttribute("data-card") || "" : "");
    }
  }

  function doDeploy() {
    var srv = selectedDeployServer();
    if (!srv) { deployInfo("Escolha o servidor.", true); return; }
    var docId = parseInt(els.depform.value, 10) || 0;
    var req = {
      server: srv.name,
      password: els.deppass.value,
      folder: boot.folder,
      documentId: docId,
      versionMode: els.depversion.value,
      confirm: els.depconfirm.value.trim(),
      datasetName: els.depdataset.value.trim(),
      cardDescription: els.depcard.value.trim()
    };
    if (srv.env === "prod" && req.confirm !== srv.name) {
      deployInfo("PRODUÇÃO: digite o nome exato do servidor (" + srv.name + ") para confirmar.", true);
      return;
    }
    if (docId === 0) {
      if (els.depform.disabled) { deployInfo("Aguarde a lista de formulários (ou informe a senha).", true); return; }
      req.create = {
        name: els.depname.value.trim() || boot.folder,
        datasetName: req.datasetName,
        parentId: parseInt(els.depparent.value, 10) || 0,
        persistenceType: els.deppersist.value,
        cardDescription: req.cardDescription
      };
      if (!req.create.datasetName) { deployInfo("O nome do dataset é obrigatório na criação.", true); return; }
      if (!req.create.cardDescription) { deployInfo("Escolha o campo descritor.", true); return; }
      if (req.create.parentId <= 0) { deployInfo("Escolha a pasta do GED onde salvar.", true); return; }
    }
    deployInfo("Publicando em " + srv.name + "…");
    apiPost("/_dev/api/formsim/deploy", req, function (err, data) {
      if (data && data.needsPassword) {
        els.deppassrow.style.display = "";
        deployInfo("Sem credencial salva — digite a senha e publique de novo.", true);
        els.deppass.focus();
        return;
      }
      if (err) {
        deployInfo("Falha ao publicar: " + err, true);
        return;
      }
      root.classList.remove("open-deploy");
      showDialog("ok", "Formulário publicado",
        (data.action === "created" ? "Criado" : "Atualizado") + " no servidor " + data.server +
        ": " + data.name + " (documentId " + data.documentId + "). O vínculo local (forms.json) foi atualizado.",
        false);
    });
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

  // apiPost envia JSON e devolve (err, data) — em erro o data vem junto,
  // para o chamador ler flags como needsPassword.
  function apiPost(path, body, cb) {
    var x = new XMLHttpRequest();
    x.open("POST", path, true);
    x.setRequestHeader("Content-Type", "application/json");
    x.onreadystatechange = function () {
      if (x.readyState !== 4) return;
      var data = null;
      try { data = JSON.parse(x.responseText); } catch (e) {}
      if (x.status >= 200 && x.status < 300) cb(null, data);
      else cb((data && data.error) || ("HTTP " + x.status), data);
    };
    x.send(JSON.stringify(body));
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
    "#fluigcli-sim{position:fixed;bottom:16px;z-index:2147483000;display:flex;" +
    "flex-direction:column;gap:10px;" +
    "font:13px/1.45 system-ui,-apple-system,Segoe UI,Roboto,sans-serif;color:#1d2b36}" +
    // Posição da barra: sem transform (transform quebraria o position:fixed
    // do overlay dos diálogos) — o centro usa left+right+margin auto.
    "#fluigcli-sim.pos-right{right:16px;align-items:flex-end}" +
    "#fluigcli-sim.pos-left{left:16px;align-items:flex-start}" +
    "#fluigcli-sim.pos-center{left:0;right:0;margin:0 auto;width:fit-content;align-items:center}" +
    "#fluigcli-sim *{box-sizing:border-box;font:inherit;color:inherit}" +
    "#fluigcli-sim .bar{display:flex;gap:2px;align-items:center;border:1px solid #d5dde5;" +
    "background:#fff;border-radius:999px;padding:5px 8px;box-shadow:0 2px 8px rgba(16,36,54,.18);" +
    "max-width:min(400px,94vw)}" +
    "#fluigcli-sim .bar button{position:relative;border:0;background:none;cursor:pointer;" +
    "min-width:34px;height:34px;border-radius:999px;font-size:16px;line-height:1;padding:0 6px}" +
    "#fluigcli-sim .bar button:hover{background:#eef2f5}" +
    // Tela estreita (modo celular do preview): botões mais juntos para os 10
    // ícones caberem em 375px sem cortar o último.
    "@media (max-width:430px){#fluigcli-sim .bar{gap:0;padding:5px 6px}" +
    "#fluigcli-sim .bar button{min-width:29px;padding:0 3px;font-size:14px}}" +
    "#fluigcli-sim .bar button[data-act=screen]{font-size:12.5px;font-weight:650}" +
    "#fluigcli-sim .dot{position:absolute;top:2px;right:2px;width:8px;height:8px;" +
    "border-radius:50%;background:#9aa7b2}" +
    "#fluigcli-sim .dot.ok{background:#25b26e}#fluigcli-sim .dot.err{background:#e2574c}" +
    "#fluigcli-sim .card{display:none;width:340px;max-width:92vw;max-height:76vh;overflow:auto;background:#fff;" +
    "border:1px solid #d5dde5;border-radius:12px;box-shadow:0 8px 28px rgba(16,36,54,.25);padding:14px 16px 16px}" +
    "#fluigcli-sim.open-sim .card.simcard{display:block}" +
    "#fluigcli-sim.open-send .card.sendcard{display:block}" +
    "#fluigcli-sim.open-deploy .card.deploycard{display:block}" +
    "#fluigcli-sim.open-audit .card.auditcard{display:block}" +
    "#fluigcli-sim .dot.warn{background:#e5a50a}" +
    "#fluigcli-sim .afind{border-left:3px solid #9aa7b2;padding:5px 8px;margin:7px 0;background:#f6f8fa;" +
    "border-radius:0 7px 7px 0;font-size:11.5px;line-height:1.45}" +
    "#fluigcli-sim .afind.err{border-left-color:#e2574c}" +
    "#fluigcli-sim .afind.warn{border-left-color:#e5a50a}" +
    "#fluigcli-sim .afind .aloc{font-family:ui-monospace,Consolas,monospace;color:#5a6b7b;font-size:10.5px}" +
    "#fluigcli-sim .afind .asug{color:#1b6e53;margin-top:2px}" +
    "#fluigcli-sim .prodwarn{color:#b3352b;font-weight:650}" +
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
    "#fluigcli-sim .btn:disabled{opacity:.45;cursor:not-allowed}" +
    "#fluigcli-sim .btn.sec{background:#eef2f5;color:#1d2b36;margin-top:6px}" +
    "#fluigcli-sim .status{margin:8px 0 0;padding:7px 9px;border-radius:7px;font-size:12px;background:#f2f6f9}" +
    "#fluigcli-sim .status.err{background:#fdecea;color:#8c2f28;white-space:pre-wrap}" +
    "#fluigcli-sim details{margin-top:8px;font-size:12px}" +
    "#fluigcli-sim details ul{margin:4px 0 6px;padding-left:18px}" +
    "#fluigcli-sim .muted{color:#5a6b7b}" +
    "#fluigcli-sim .toggle{display:flex;gap:6px;align-items:center;margin-top:10px;font-size:12px}" +
    "#fluigcli-sim .toggle input{width:auto}" +
    "#fluigcli-sim .sep{margin:14px 0 2px;padding-top:10px;border-top:1px solid #e3e8ee;" +
    "font-weight:650;font-size:12.5px}" +
    "#fluigcli-sim .dlg-overlay{position:fixed;inset:0;background:rgba(16,36,54,.45);" +
    "z-index:2147483001;display:flex;align-items:center;justify-content:center}" +
    "#fluigcli-sim .dlg{background:#fff;border-radius:12px;box-shadow:0 12px 40px rgba(0,0,0,.35);" +
    "width:min(480px,90vw);max-height:70vh;overflow:auto;padding:18px 20px}" +
    "#fluigcli-sim .dlg h4{margin:0 0 10px;font-size:15px}" +
    "#fluigcli-sim .dlg.ok h4{color:#1d7a4f}#fluigcli-sim .dlg.block h4{color:#b3352b}" +
    "#fluigcli-sim .dlg.crash h4{color:#9a6700}" +
    "#fluigcli-sim .dlg .body{font-size:13.5px;line-height:1.55;word-break:break-word}" +
    "#fluigcli-sim .dlg .hint{margin-top:10px;color:#5a6b7b;font-size:12px}" +
    "#fluigcli-sim .dlg button{margin-top:14px;width:100%;padding:8px;border:0;border-radius:8px;" +
    "cursor:pointer;background:#eef2f5;font-weight:650}" +
    "@media (prefers-color-scheme: dark){" +
    "#fluigcli-sim{color:#e6edf3}" +
    "#fluigcli-sim .bar,#fluigcli-sim .card{background:#1b232d;border-color:#2b3742}" +
    "#fluigcli-sim .bar button:hover{background:#2b3742}" +
    "#fluigcli-sim select,#fluigcli-sim input[type=text],#fluigcli-sim input[type=number],#fluigcli-sim textarea{" +
    "background:#12181f;border-color:#2b3742}" +
    "#fluigcli-sim .btn.sec{background:#2b3742;color:#e6edf3}" +
    "#fluigcli-sim .status{background:#232d38}" +
    "#fluigcli-sim .status.err{background:#4a2320;color:#f3b0aa}" +
    "#fluigcli-sim .sub,#fluigcli-sim .muted{color:#93a4b4}" +
    "#fluigcli-sim .sep{border-top-color:#2b3742}" +
    "#fluigcli-sim .dlg{background:#1b232d}" +
    "#fluigcli-sim .dlg button{background:#2b3742;color:#e6edf3}" +
    "#fluigcli-sim .dlg .hint{color:#93a4b4}" +
    // Achados da auditoria no dark: fundo escuro (o claro fixo deixava a
    // mensagem ilegível com o texto herdado #e6edf3) e acentos mais claros.
    "#fluigcli-sim .afind{background:#12181f}" +
    "#fluigcli-sim .afind .aloc{color:#93a4b4}" +
    "#fluigcli-sim .afind .asug{color:#5fd3a6}}";

  function esc(s) {
    return String(s).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");
  }

  var root, els = {};

  // Preferências da barra (posição) — globais do dev server, não por form.
  var UIKEY = "fluigcli.formsim.ui";
  var ui = { pos: "right" };
  try { ui = Object.assign(ui, JSON.parse(localStorage.getItem(UIKEY) || "{}")); } catch (e) {}
  function saveUI() { try { localStorage.setItem(UIKEY, JSON.stringify(ui)); } catch (e) {} }

  function applyPos() {
    root.classList.remove("pos-right", "pos-center", "pos-left");
    root.classList.add("pos-" + (ui.pos === "center" || ui.pos === "left" ? ui.pos : "right"));
  }
  function cyclePos() {
    ui.pos = ui.pos === "right" ? "center" : ui.pos === "center" ? "left" : "right";
    saveUI();
    applyPos();
  }

  // Modo de tela: livre → celular (375) → tablet (768). Limitar a largura
  // por container NÃO dispara as media queries do grid (elas olham o
  // viewport) — por isso o modo navega para a moldura do dev server
  // (?screen=…), que põe o preview num IFRAME com viewport próprio. Este
  // runtime roda DENTRO do iframe (boot.screen diz o modo) e troca o modo
  // navegando o window.top.
  function applyScreen() {
    var btn = root.querySelector("[data-act=screen]");
    if (btn) btn.textContent = boot.screen === "phone" ? "📱375" : boot.screen === "tablet" ? "📱768" : "🖥";
  }
  function cycleScreen() {
    var next = boot.screen === "phone" ? "tablet" : boot.screen === "tablet" ? "" : "phone";
    var url = location.pathname + (next ? "?screen=" + next : "");
    try { (window.top || window).location.href = url; } catch (e) { location.href = url; }
  }

  // openPortal abre o render REAL do formulário (streamcontrol) numa aba,
  // pela mesma origem do proxy: a URL vem do getDefinitionProcess do
  // processo escolhido na Simulação. A aba abre em branco JÁ no clique
  // (gesto do usuário — senão o navegador bloqueia o popup) e navega quando
  // a resposta chega.
  function openPortal() {
    if (!cfg.processId) {
      showDialog("crash", "Sem processo vinculado",
        "Escolha o processo do formulário na Simulação (auto-detectado com fluigcli form link) para abrir o render real no Fluig.", false);
      return;
    }
    var tab = window.open("about:blank", "_blank");
    api("/ecm/api/rest/ecm/workflowView/getDefinitionProcess?processId=" + encodeURIComponent(cfg.processId), function (err, data) {
      var formHtml = data && data.content && data.content.formHtml;
      if (err || !formHtml) {
        if (tab) tab.close();
        showDialog("crash", "Não consegui obter o render real", String(err || "resposta sem formHtml"), false);
        return;
      }
      var path = String(formHtml).replace(/^https?:\/\/[^\/]+/, "");
      if (tab) tab.location = path; else window.open(path, "_blank");
    });
  }

  // --- Auditoria de Style Guide (fluigcli audit) ---
  // Roda no dev server sobre a pasta deste formulário; como o preview
  // recarrega a cada salvamento, o resultado acompanha a edição.
  var auditData = null;
  function runAudit() {
    api("/_dev/api/audit?form=" + encodeURIComponent(boot.folder), function (err, data) {
      if (err || !data) {
        if (els.auditdot) els.auditdot.className = "dot";
        if (els.auditsummary) els.auditsummary.textContent = "fluigcli audit · falhou: " + err;
        return;
      }
      auditData = data;
      renderAudit();
    });
  }
  function renderAudit() {
    var e = auditData.counts.error, w = auditData.counts.warning;
    els.auditdot.className = "dot " + (e ? "err" : (w ? "warn" : "ok"));
    els.auditsummary.textContent = "fluigcli audit · " +
      (e + w === 0 ? "nenhuma pendência ✓" : e + " erro(s) e " + w + " aviso(s)") +
      " · " + auditData.scanned + " arquivo(s)";
    var list = els.auditlist;
    list.innerHTML = "";
    var prefix = "forms/" + boot.folder + "/";
    var max = 80;
    auditData.findings.slice(0, max).forEach(function (f) {
      var div = document.createElement("div");
      div.className = "afind " + (f.severity === "error" ? "err" : "warn");
      var loc = document.createElement("div");
      loc.className = "aloc";
      var file = f.file.indexOf(prefix) === 0 ? f.file.slice(prefix.length) : f.file;
      loc.textContent = f.rule + " · " + file + ":" + f.line;
      var msg = document.createElement("div");
      msg.textContent = f.message;
      div.appendChild(loc);
      div.appendChild(msg);
      if (f.suggestion) {
        var sug = document.createElement("div");
        sug.className = "asug";
        sug.textContent = "→ " + f.suggestion + (f.fix ? " (corrigível com --fix)" : "");
        div.appendChild(sug);
      }
      list.appendChild(div);
    });
    if (auditData.findings.length > max) {
      var more = document.createElement("p");
      more.className = "sub";
      more.textContent = "… e mais " + (auditData.findings.length - max) +
        " achado(s) — veja tudo com fluigcli audit forms/" + boot.folder;
      list.appendChild(more);
    }
  }

  function buildPanel() {
    var style = document.createElement("style");
    style.textContent = CSS;
    document.head.appendChild(style);

    root = document.createElement("div");
    root.id = "fluigcli-sim";
    root.innerHTML = "" +
      "<div class=\"card simcard\">" +
      "<h3>Simulação de processo <button type=\"button\" title=\"Fechar\" data-act=\"close\">×</button></h3>" +
      "<p class=\"sub\">fluigcli dev · " + esc(boot.folder) + "</p>" +
      "<div class=\"status\" data-el=\"status\" style=\"display:none\"></div>" +
      "<label>Processo</label>" +
      "<div class=\"row\"><select data-el=\"process\"><option value=\"\">— sem processo —</option></select>" +
      "<button type=\"button\" class=\"btn sec\" style=\"flex:0 0 34px;margin:0\" title=\"Recarregar do servidor\" data-act=\"refresh\">↻</button></div>" +
      "<label>Etapa (WKNumState)</label>" +
      "<select data-el=\"state\" style=\"margin-bottom:6px\"><option value=\"\">— número manual —</option></select>" +
      "<input type=\"number\" data-el=\"statenum\" min=\"0\" step=\"1\">" +
      "<label>Modo do formulário</label>" +
      "<select data-el=\"mode\"><option>ADD</option><option>MOD</option><option>VIEW</option></select>" +
      "<div class=\"toggle\"><input type=\"checkbox\" data-el=\"mobile\" id=\"fluigcli-sim-mob\">" +
      "<label for=\"fluigcli-sim-mob\" style=\"margin:0;font-weight:600\" title=\"O displayFields recebe form.getMobile() = true, como no app\">getMobile() = celular</label></div>" +
      "<label>Usuário (WKUser)</label>" +
      "<select data-el=\"user\"></select>" +
      "<label>Outras variáveis (CHAVE=valor, uma por linha)</label>" +
      "<textarea data-el=\"vars\" placeholder=\"WKNumProces=1234\"></textarea>" +
      "<div class=\"toggle\"><input type=\"checkbox\" data-el=\"enabled\" id=\"fluigcli-sim-on\">" +
      "<label for=\"fluigcli-sim-on\" style=\"margin:0;font-weight:600\">Simulação ligada</label></div>" +
      "<button type=\"button\" class=\"btn\" data-act=\"apply\">Aplicar e recarregar</button>" +
      "<details><summary>Execução do displayFields</summary><div data-el=\"detail\" class=\"muted\"></div></details>" +
      "</div>" +
      "<div class=\"card sendcard\">" +
      "<h3>Enviar etapa <button type=\"button\" title=\"Fechar\" data-act=\"closesend\">×</button></h3>" +
      "<div class=\"status\" data-el=\"testinfo\"></div>" +
      "<label>Próxima etapa (WKNextState)</label>" +
      "<select data-el=\"nextstate\" style=\"margin-bottom:6px\"><option value=\"\">— número manual —</option></select>" +
      "<input type=\"number\" data-el=\"nextstatenum\" min=\"0\" step=\"1\">" +
      "<button type=\"button\" class=\"btn\" data-act=\"sendgo\">Validar envio</button>" +
      "<p class=\"sub\" style=\"margin-top:10px\">Roda o events/validateForm.js local — nada é gravado no servidor.</p>" +
      "</div>" +
      "<div class=\"card deploycard\">" +
      "<h3>Publicar no servidor <button type=\"button\" title=\"Fechar\" data-act=\"closedeploy\">×</button></h3>" +
      "<div class=\"status\" data-el=\"depinfo\" style=\"display:none\"></div>" +
      "<label>Servidor</label>" +
      "<select data-el=\"depserver\"></select>" +
      "<div data-el=\"deppassrow\" style=\"display:none\">" +
      "<label>Senha (sem credencial salva para este servidor)</label>" +
      "<input type=\"password\" data-el=\"deppass\" autocomplete=\"off\">" +
      "</div>" +
      "<label>Formulário no servidor</label>" +
      "<select data-el=\"depform\" disabled><option value=\"\">— carregando… —</option></select>" +
      "<div data-el=\"depcreate\" style=\"display:none\">" +
      "<label>Nome do formulário</label><input type=\"text\" data-el=\"depname\">" +
      "</div>" +
      "<label>Nome do dataset</label>" +
      "<input type=\"text\" data-el=\"depdataset\" list=\"fluigcli-dep-ds\"><datalist id=\"fluigcli-dep-ds\"></datalist>" +
      "<label>Campo descritor</label>" +
      "<select data-el=\"depcard\"><option value=\"\">— escolha o campo —</option></select>" +
      "<div data-el=\"depexisting\">" +
      "<label>Versão</label>" +
      "<select data-el=\"depversion\"><option value=\"keep\">Manter a atual (padrão)</option>" +
      "<option value=\"new\">Criar uma nova</option></select>" +
      "</div>" +
      "<div data-el=\"depcreate2\" style=\"display:none\">" +
      "<label>Pasta do GED onde salvar</label>" +
      "<select data-el=\"depfolder\" disabled><option value=\"\">— carregando… —</option></select>" +
      "<div class=\"sub\" data-el=\"deppath\" style=\"margin:4px 0 0\"></div>" +
      "<input type=\"number\" data-el=\"depparent\" min=\"1\" style=\"display:none\" placeholder=\"id da pasta do GED\">" +
      "<label>Armazenamento</label>" +
      "<select data-el=\"deppersist\"><option value=\"db\">Tabelas de banco de dados (recomendado)</option>" +
      "<option value=\"single\">Numa única tabela (pequena quantidade de registros)</option></select>" +
      "</div>" +
      "<div data-el=\"depprod\" style=\"display:none\">" +
      "<label class=\"prodwarn\">⚠ PRODUÇÃO — digite o nome do servidor para confirmar</label>" +
      "<input type=\"text\" data-el=\"depconfirm\" autocomplete=\"off\">" +
      "</div>" +
      "<button type=\"button\" class=\"btn\" data-act=\"deploygo\" data-el=\"depgo\" disabled>Publicar</button>" +
      "</div>" +
      "<div class=\"card auditcard\">" +
      "<h3>Style Guide 2.0 <button type=\"button\" title=\"Fechar\" data-act=\"auditclose\">×</button></h3>" +
      "<p class=\"sub\" data-el=\"auditsummary\">fluigcli audit · executando…</p>" +
      "<div data-el=\"auditlist\"></div>" +
      "<p class=\"sub\" style=\"margin-top:10px\">Correções determinísticas: <code>fluigcli audit forms/" +
      esc(boot.folder) + " --fix</code>. A auditoria reexecuta a cada salvamento.</p>" +
      "</div>" +
      "<div class=\"bar\">" +
      "<button type=\"button\" data-act=\"open\" title=\"Simulação de processo (etapa, modo, usuário, variáveis)\">⚙<span class=\"dot\"></span></button>" +
      "<button type=\"button\" data-act=\"audit\" title=\"Style Guide 2.0: auditoria deste formulário (fluigcli audit)\">🎨<span class=\"dot\" data-el=\"auditdot\"></span></button>" +
      "<button type=\"button\" data-act=\"save\" title=\"Salvar: valida o formulário agora (validateForm) — nada é gravado\">💾</button>" +
      "<button type=\"button\" data-act=\"send\" title=\"Enviar etapa: pergunta a próxima etapa e valida o envio\">▶</button>" +
      "<button type=\"button\" data-act=\"deploy\" title=\"Publicar o formulário no servidor (atualiza ou cria, como o form export)\">🚀</button>" +
      "<button type=\"button\" data-act=\"screen\" title=\"Alternar largura da tela: livre → celular (375) → tablet (768). getMobile() simula na Simulação\">🖥</button>" +
      "<button type=\"button\" data-act=\"portal\" title=\"Abrir o render real deste formulário no Fluig (nova aba, via proxy)\">↗</button>" +
      "<button type=\"button\" data-act=\"index\" title=\"Voltar ao índice de formulários\">⌂</button>" +
      "<button type=\"button\" data-act=\"clean\" title=\"Limpar os campos e recarregar o preview\">🧹</button>" +
      "<button type=\"button\" data-act=\"pos\" title=\"Posição da barra: direita → centro → esquerda\">⇔</button>" +
      "</div>";
    document.body.appendChild(root);

    root.querySelectorAll("[data-el]").forEach(function (el) { els[el.getAttribute("data-el")] = el; });

    root.addEventListener("click", function (ev) {
      var actEl = ev.target.closest ? ev.target.closest("[data-act]") : ev.target;
      var act = actEl && actEl.getAttribute && actEl.getAttribute("data-act");
      if (act === "open") {
        var was = root.classList.contains("open-sim");
        root.classList.remove("open-sim", "open-send", "open-deploy", "open-audit");
        if (!was) { root.classList.add("open-sim"); onOpen(false); }
      }
      if (act === "send") {
        var wasS = root.classList.contains("open-send");
        root.classList.remove("open-sim", "open-send", "open-deploy", "open-audit");
        if (!wasS) { root.classList.add("open-send"); updateTestInfo(); onOpen(false); }
      }
      if (act === "deploy") {
        var wasD = root.classList.contains("open-deploy");
        root.classList.remove("open-sim", "open-send", "open-deploy", "open-audit");
        if (!wasD) { root.classList.add("open-deploy"); openDeploy(); }
      }
      if (act === "audit") {
        var wasA = root.classList.contains("open-audit");
        root.classList.remove("open-sim", "open-send", "open-deploy", "open-audit");
        if (!wasA) { root.classList.add("open-audit"); if (!auditData) runAudit(); }
      }
      if (act === "auditclose") { root.classList.remove("open-audit"); }
      if (act === "close") { root.classList.remove("open-sim"); }
      if (act === "closesend") { root.classList.remove("open-send"); }
      if (act === "closedeploy") { root.classList.remove("open-deploy"); }
      if (act === "deploygo") { doDeploy(); }
      if (act === "refresh") { onOpen(true); }
      if (act === "apply") { apply(); }
      if (act === "save") { doValidate(false); }
      if (act === "sendgo") { doValidate(true); }
      if (act === "screen") { cycleScreen(); }
      if (act === "portal") { openPortal(); }
      if (act === "index") { location.href = "/_dev/forms/"; }
      if (act === "clean") { location.reload(); }
      if (act === "pos") { cyclePos(); }
      if (act === "dlgclose") { closeDialog(); }
    });
    els.process.addEventListener("change", function () {
      var opt = els.process.selectedOptions[0];
      loadStates(els.process.value, opt ? parseInt(opt.getAttribute("data-version") || "0", 10) : 0, false);
    });
    els.state.addEventListener("change", function () {
      if (els.state.value !== "") els.statenum.value = els.state.value;
    });
    els.nextstate.addEventListener("change", function () {
      if (els.nextstate.value !== "") els.nextstatenum.value = els.nextstate.value;
    });
    els.depserver.addEventListener("change", function () {
      els.deppass.value = "";
      onDeployServerChange();
      loadDeployForms();
    });
    els.deppass.addEventListener("keydown", function (ev) {
      if (ev.key === "Enter") loadDeployForms();
    });
    els.depform.addEventListener("change", onDeployFormChange);
    els.depname.addEventListener("input", applyDsSuggestion);
    els.depdataset.addEventListener("input", function () { dsTouched = true; });
    els.depfolder.addEventListener("change", onDeployFolderPick);
    // Qualquer edição no cartão reavalia o botão Publicar (os listeners
    // específicos acima rodam antes — o estado já está atualizado aqui).
    var depCardEl = root.querySelector(".card.deploycard");
    depCardEl.addEventListener("input", updateDeployButton);
    depCardEl.addEventListener("change", updateDeployButton);

    els.statenum.value = cfg.wkNumState == null ? "0" : cfg.wkNumState;
    els.nextstatenum.value = cfg.wkNextState == null ? "" : cfg.wkNextState;
    els.mode.value = cfg.formMode || "ADD";
    els.mobile.checked = cfg.mobile === true;
    // Antes da lista de usuários carregar (no primeiro abrir), o select tem
    // só o valor atual — o Aplicar continua funcionando offline.
    fillUsers(null);
    els.enabled.checked = cfg.enabled !== false;
    var lines = [];
    var extra = cfg.vars || {};
    for (var k in extra) { if (Object.prototype.hasOwnProperty.call(extra, k)) lines.push(k + "=" + extra[k]); }
    els.vars.value = lines.join("\n");
    renderStatus();
    applyPos();
    applyScreen();
  }

  // renderStatus: a linha de status só aparece para ERRO (a janela fica
  // limpa; o pontinho do botão ⚙ já indica ok/erro/desligado).
  function renderStatus() {
    var dot = root.querySelector(".dot");
    var st = els.status;
    if (boot.event && cfg.enabled !== false && report.error) {
      st.className = "status err";
      st.textContent = "displayFields falhou: " + report.error;
      st.style.display = "";
      dot.className = "dot err";
    } else {
      st.textContent = "";
      st.style.display = "none";
      dot.className = boot.event && cfg.enabled !== false && report.ran ? "dot ok" : "dot";
    }
    var d = [];
    if (report.unknown.length) d.push("<b>getValue não simulado:</b><ul><li>" + report.unknown.map(esc).join("</li><li>") + "</li></ul>");
    if (report.reads.length) d.push("<b>Leituras:</b><ul><li>" + report.reads.map(esc).join("</li><li>") + "</li></ul>");
    if (report.sets.length) d.push("<b>form.setValue:</b><ul><li>" + report.sets.map(esc).join("</li><li>") + "</li></ul>");
    if (report.warns.length) d.push("<b>Avisos:</b><ul><li>" + report.warns.map(esc).join("</li><li>") + "</li></ul>");
    els.detail.innerHTML = d.join("") || "Nada executado.";
  }

  // fillUsers popula o seletor de WKUser: ativos primeiro (a lista já vem
  // ordenada por nome do servidor), inativos marcados; o valor atual sempre
  // vira opção mesmo fora da lista. Com list=null monta só o valor atual
  // (estado antes da lista carregar).
  function fillUsers(list) {
    var sel = els.user;
    var current = sel.value || cfg.wkUser || "";
    sel.innerHTML = "";
    sel.appendChild(document.createElement("option")); // vazio = sem usuário
    var found = false;
    var ordered = (list || []).filter(function (u) { return u.active; })
      .concat((list || []).filter(function (u) { return !u.active; }));
    ordered.forEach(function (u) {
      var o = document.createElement("option");
      o.value = u.code;
      o.textContent = u.name + (u.active ? "" : " (inativo)");
      o.title = u.code;
      sel.appendChild(o);
      if (u.code === current) found = true;
    });
    if (current && !found) {
      var cur = document.createElement("option");
      cur.value = current;
      cur.textContent = current + " (atual)";
      sel.appendChild(cur);
    }
    sel.value = current;
  }

  var opened = false;
  function onOpen(force) {
    if (opened && !force) return;
    opened = true;
    api("/_dev/api/formsim/context?folder=" + encodeURIComponent(boot.folder) + (force ? "&force=1" : ""), function (err, ctx) {
      if (err) { statusNote("Contexto indisponível: " + err); }
      else if (ctx && !els.user.value && ctx.userCode) { cfg.wkUser = cfg.wkUser || ctx.userCode; fillUsers(null); }
      api("/_dev/api/formsim/processes" + (force ? "?force=1" : ""), function (perr, procs) {
        if (perr) { statusNote("Processos indisponíveis: " + perr); return; }
        fillProcesses(Array.isArray(procs) ? procs : [],
          ctx && Array.isArray(ctx.processes) ? ctx.processes : []);
      });
      api("/_dev/api/formsim/users" + (force ? "?force=1" : ""), function (uerr, users) {
        if (uerr) { statusNote("Usuários indisponíveis: " + uerr); return; }
        fillUsers(Array.isArray(users) ? users : []);
      });
    });
  }

  function statusNote(msg) {
    els.status.className = "status err";
    els.status.textContent = msg;
    els.status.style.display = "";
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
    var sels = [els.state, els.nextstate];
    sels.forEach(function (sel) { sel.innerHTML = "<option value=\"\">— número manual —</option>"; });
    if (!processId) return;
    sels.forEach(function (sel) { sel.disabled = true; });
    api("/_dev/api/formsim/states?process=" + encodeURIComponent(processId) + "&version=" + (version || 0), function (err, data) {
      sels.forEach(function (sel) { sel.disabled = false; });
      if (err) { statusNote("Etapas indisponíveis: " + err); return; }
      (data && Array.isArray(data.states) ? data.states : []).forEach(function (st) {
        sels.forEach(function (sel) {
          var o = document.createElement("option");
          o.value = String(st.sequence);
          var kind = st.bpmnType || st.stateType || "";
          o.textContent = st.sequence + " — " + (st.stateName || "(sem nome)") + (kind ? " · " + kind : "");
          if (st.stateDescription) o.title = st.stateDescription;
          sel.appendChild(o);
        });
      });
      els.process.selectedOptions[0] && els.process.selectedOptions[0].setAttribute("data-version", String(data.version || version || 0));
      var cur = keepCurrent ? String(cfg.wkNumState) : els.statenum.value;
      if (cur !== "" && els.state.querySelector("option[value=\"" + cur + "\"]")) els.state.value = cur;
      var next = els.nextstatenum.value || (cfg.wkNextState == null ? "" : String(cfg.wkNextState));
      if (next !== "" && els.nextstate.querySelector("option[value=\"" + next + "\"]")) els.nextstate.value = next;
    });
  }

  function apply() {
    cfg.enabled = els.enabled.checked;
    cfg.wkNumState = els.statenum.value === "" ? "0" : els.statenum.value;
    cfg.formMode = els.mode.value;
    cfg.mobile = els.mobile.checked;
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

  // --- teste de gravação (Salvar / Enviar etapa) ---

  function closeDialog() {
    var d = root.querySelector(".dlg-overlay");
    if (d) d.parentNode.removeChild(d);
  }

  // showDialog imita o diálogo do portal. asHTML=true renderiza a msg como
  // HTML (o throw de validação costuma trazer <b style=…> — conteúdo local
  // do próprio projeto); caso contrário vai como texto.
  function showDialog(kind, title, msg, asHTML, hint) {
    closeDialog();
    var ov = document.createElement("div");
    ov.className = "dlg-overlay";
    var dlg = document.createElement("div");
    dlg.className = "dlg " + kind;
    var h = document.createElement("h4");
    h.textContent = title;
    var body = document.createElement("div");
    body.className = "body";
    if (asHTML) body.innerHTML = msg; else body.textContent = msg;
    dlg.appendChild(h);
    dlg.appendChild(body);
    if (hint) {
      var p = document.createElement("div");
      p.className = "hint";
      p.textContent = hint;
      dlg.appendChild(p);
    }
    var btn = document.createElement("button");
    btn.type = "button";
    btn.setAttribute("data-act", "dlgclose");
    btn.textContent = "Fechar";
    dlg.appendChild(btn);
    ov.appendChild(dlg);
    ov.addEventListener("click", function (ev) { if (ev.target === ov) closeDialog(); });
    root.appendChild(ov);
  }

  function stateLabel(sel, num) {
    var opt = num !== "" && sel.querySelector("option[value=\"" + num + "\"]");
    return opt ? opt.textContent : ("etapa " + num);
  }

  // updateTestInfo resume o contexto que o teste de gravação vai usar (vem
  // da simulação aplicada — o cartão de gravação não altera etapa/modo).
  function updateTestInfo() {
    var cur = String(cfg.wkNumState == null ? "0" : cfg.wkNumState);
    var opt = els.state.querySelector("option[value=\"" + cur + "\"]");
    var info = "Valida com WKNumState=" + cur + (opt ? " (" + opt.textContent + ")" : "") +
      ", modo " + (cfg.formMode || "ADD") + " — ajuste na Simulação.";
    if (!boot.validate) {
      info = "Este formulário não tem events/validateForm.js — o Fluig gravaria sem validar. " + info;
    }
    els.testinfo.textContent = info;
  }

  function doValidate(send) {
    var nextState = "";
    if (send) {
      nextState = els.nextstatenum.value;
      if (nextState === "") {
        showDialog("crash", "Escolha a próxima etapa",
          "Para simular o Enviar, selecione a próxima etapa (WKNextState) ou digite o número dela.", false);
        return;
      }
      cfg.wkNextState = nextState; // lembra a escolha entre reloads
      saveCfg(cfg);
    }
    if (!boot.validate && !(send && typeof window.beforeSendValidate === "function")) {
      showDialog("ok", "Sem validação", "Este formulário não tem events/validateForm.js — o Fluig " +
        (send ? "avançaria a etapa" : "salvaria") + " sem validar.", false);
      return;
    }
    var r = runValidation(send, nextState);
    if (r.ok) {
      if (send) root.classList.remove("open-send");
      showDialog("ok", "Validação passou",
        send ? "O Fluig salvaria o formulário e avançaria para " + stateLabel(els.nextstate, nextState) + "."
          : "O Fluig salvaria o formulário.",
        false, "Simulação: nada foi gravado no servidor.");
    } else if (r.runtime) {
      showDialog("crash", "Erro no evento de validação", r.msg, false,
        "Isso é um defeito no script (não uma mensagem de validação) — no portal apareceria um erro genérico.");
    } else {
      showDialog("block", send ? "Validação bloqueou o envio" : "Validação bloqueou a gravação", r.msg, true,
        "Mensagem do throw do validateForm/beforeSendValidate, como o portal exibiria.");
    }
  }

  // Ordem: os stubs do ambiente do portal entram antes de tudo (o form usa
  // no document.ready e nos onclick); o evento roda JÁ (antes do load do
  // formulário, que lê os campos preenchidos por ele); o painel em seguida.
  installPortalStub();
  installWdkMachine();
  populateDatasetSelects(); // antes do evento: displayFields pode selecionar valor
  runEvent();
  buildPanel();
  runAudit(); // o 🎨 já abre com o veredito do style guide
  if (firstVisit) autodetect();
})();
`
