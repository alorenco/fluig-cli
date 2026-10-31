// fluigcli test unit — falsos das APIs do Fluig (ROADMAP §5.18).
//
// Este arquivo roda no ESCOPO DE CADA CASO, antes dos scripts do usuário. Os
// nomes internos começam com __fluigcli para não colidir com os do usuário.
// Os textos devolvidos pelos falsos são java.lang.String, como no servidor:
// assim os defeitos do Rhino (=== falso, String vazia truthy) aparecem aqui.

var __fluigcliGlobal = this;
var __fluigcliCase = null;
var __fluigcliObs = null;

function __fluigcliSetup(json) {
    var C = JSON.parse(json);
    __fluigcliCase = C;
    var obs = { form: {}, card: {}, logs: [], datasetCalls: [], calls: [], html: [], missing: [] };
    __fluigcliObs = obs;
    var G = __fluigcliGlobal;
    var has = function (o, k) { return o != null && Object.prototype.hasOwnProperty.call(o, k); };
    var J = function (v) { return v == null ? null : new java.lang.String(String(v)); };
    var str = function (v) { return v == null ? null : String(v); };
    var ints = function (ids) {
        var arr = java.lang.reflect.Array.newInstance(java.lang.Integer, ids.length);
        for (var i = 0; i < ids.length; i++) {
            arr[i] = new java.lang.Integer(ids[i]);
        }
        return arr;
    };
    var argList = function (a) {
        var out = [];
        for (var i = 0; i < a.length; i++) {
            out.push(__fluigcliToJSON(a[i]));
        }
        return out;
    };
    var record = function (target, method) {
        return function () {
            obs.calls.push({ target: target, method: method, args: argList(arguments) });
            var ret = C.returns && has(C.returns, target + '.' + method) ? C.returns[target + '.' + method] : null;
            return __fluigcliFromJSON(ret);
        };
    };
    var fields = C.fields || {};
    var children = C.children || {};
    var cardValue = function (over, k) {
        k = String(k);
        if (has(over, k)) {
            return J(over[k]);
        }
        return has(fields, k) ? J(fields[k]) : null;
    };

    // Relógio fixo: new Date() e Date.now() devolvem o instante do "today".
    if (C.today) {
        var RealDate = Date;
        var t = C.today;
        var fixed = new RealDate(t[0], t[1] - 1, t[2], t[3], t[4], t[5]).getTime();
        var FakeDate = function () {
            if (!(this instanceof FakeDate)) {
                return new RealDate(fixed).toString();
            }
            if (arguments.length === 0) {
                return new RealDate(fixed);
            }
            var a = [null];
            for (var i = 0; i < arguments.length; i++) {
                a.push(arguments[i]);
            }
            return new (Function.prototype.bind.apply(RealDate, a))();
        };
        FakeDate.prototype = RealDate.prototype;
        FakeDate.now = function () { return fixed; };
        FakeDate.UTC = RealDate.UTC;
        FakeDate.parse = RealDate.parse;
        G.Date = FakeDate;
    }

    // form: o objeto dos eventos de formulário.
    var form = {};
    var formMembers = C.formMembers || [];
    for (var i = 0; i < formMembers.length; i++) {
        form[formMembers[i]] = record('form', formMembers[i]);
    }
    form.getValue = function (k) { return cardValue(obs.form, k); };
    form.setValue = function (k, v) { obs.form[String(k)] = str(v); };
    form.getChildrenIndexes = function (t) { return ints(children[String(t)] || []); };
    form.getFormMode = function () { return J(C.formMode || 'ADD'); };
    form.getMobile = function () { return C.mobile === true; };
    form.getCompanyId = function () { return C.companyId == null ? 1 : C.companyId; };
    __fluigcliGlobal.__fluigcliForm = form;

    // customHTML: o segundo argumento do displayFields.
    __fluigcliGlobal.__fluigcliCustomHTML = {
        append: function (s) { obs.html.push(String(s)); }
    };

    // hAPI: os scripts de processo. Lê o mesmo card do form.
    var hAPI = {};
    var hapiMembers = C.hapiMembers || [];
    for (var j = 0; j < hapiMembers.length; j++) {
        hAPI[hapiMembers[j]] = record('hAPI', hapiMembers[j]);
    }
    hAPI.getCardValue = function (k) { return cardValue(obs.card, k); };
    hAPI.setCardValue = function (k, v) { obs.card[String(k)] = str(v); };
    hAPI.getChildrenIndexes = function (t) { return ints(children[String(t)] || []); };
    hAPI.getCardData = function () {
        var m = new java.util.HashMap();
        for (var k in fields) {
            if (has(fields, k)) {
                m.put(k, J(fields[k]));
            }
        }
        for (var k2 in obs.card) {
            if (has(obs.card, k2)) {
                m.put(k2, J(obs.card[k2]));
            }
        }
        return m;
    };
    G.hAPI = hAPI;

    // getValue("WK…"): variáveis do processo.
    // O tipo segue o JSON: texto = java.lang.String, true/false =
    // java.lang.Boolean (WKMobile e as outras WK booleanas do fluig.d.ts),
    // número inteiro = java.lang.Integer. Ausente = null.
    var wk = C.wk || {};
    G.getValue = function (k) {
        k = String(k);
        if (!has(wk, k) || wk[k] == null) {
            return null;
        }
        var v = wk[k];
        if (typeof v === 'boolean') {
            return new java.lang.Boolean(v);
        }
        if (typeof v === 'number') {
            if (v % 1 !== 0) {
                return new java.lang.Double(v);
            }
            return Math.abs(v) > 2147483647 ? new java.lang.Long(v) : new java.lang.Integer(v);
        }
        return J(v);
    };

    // log
    var logger = function (level) {
        return function (m) { obs.logs.push({ level: level, message: String(m) }); };
    };
    G.log = { info: logger('info'), warn: logger('warn'), error: logger('error'), debug: logger('debug'), fatal: logger('fatal') };

    // DatasetFactory
    G.ConstraintType = { MUST: 'MUST', SHOULD: 'SHOULD', MUST_NOT: 'MUST_NOT' };
    var datasets = C.datasets || {};
    var makeDataset = function (rule) {
        var cols = rule.columns || [];
        var rows = rule.rows || [];
        var values = java.lang.reflect.Array.newInstance(java.lang.Object, rows.length);
        for (var r = 0; r < rows.length; r++) {
            var row = java.lang.reflect.Array.newInstance(java.lang.Object, cols.length);
            for (var c = 0; c < cols.length; c++) {
                var v = rows[r][c];
                row[c] = v == null ? null : new java.lang.String(String(v));
            }
            values[r] = row;
        }
        var names = java.lang.reflect.Array.newInstance(java.lang.String, cols.length);
        for (var n = 0; n < cols.length; n++) {
            names[n] = new java.lang.String(cols[n]);
        }
        var colIndex = function (col) {
            if (typeof col === 'number') {
                return col;
            }
            col = String(col);
            for (var x = 0; x < cols.length; x++) {
                if (cols[x] === col) {
                    return x;
                }
            }
            for (var y = 0; y < cols.length; y++) {
                if (cols[y].toLowerCase() === col.toLowerCase()) {
                    return y;
                }
            }
            return -1;
        };
        return {
            values: values,
            rowsCount: rows.length,
            columnsCount: cols.length,
            columnsName: names,
            getRowsCount: function () { return rows.length; },
            getColumnsCount: function () { return cols.length; },
            getColumnsName: function () { return names; },
            getValue: function (r, col) {
                var ix = colIndex(col);
                if (r < 0 || r >= rows.length || ix < 0 || ix >= cols.length) {
                    return null;
                }
                return values[r][ix];
            }
        };
    };
    var matches = function (when, cs) {
        if (!when) {
            return true;
        }
        for (var f in when) {
            if (!has(when, f)) {
                continue;
            }
            var ok = false;
            for (var i = 0; i < cs.length; i++) {
                if (cs[i].field === f && cs[i].initial === String(when[f])) {
                    ok = true;
                }
            }
            if (!ok) {
                return false;
            }
        }
        return true;
    };
    G.DatasetFactory = {
        createConstraint: function (field, initial, fin, type) {
            var c = {
                fieldName: J(field), initialValue: J(initial), finalValue: J(fin), constraintType: type, likeSearch: false,
                getFieldName: function () { return J(field); },
                getInitialValue: function () { return J(initial); },
                getFinalValue: function () { return J(fin); },
                getConstraintType: function () { return type; },
                setLikeSearch: function (b) { c.likeSearch = !!b; }
            };
            c.__fluigcli = { field: str(field), initial: str(initial), 'final': str(fin), type: str(type) };
            return c;
        },
        getDataset: function (name, fieldList, constraints, order) {
            name = String(name);
            var cs = [];
            if (constraints != null) {
                for (var i = 0; i < constraints.length; i++) {
                    var c = constraints[i];
                    if (c != null && c.__fluigcli) {
                        cs.push(c.__fluigcli);
                    }
                }
            }
            obs.datasetCalls.push({ name: name, constraints: cs });
            if (!has(datasets, name)) {
                obs.missing.push('dataset ' + name);
                throw new Error('fluigcli: o dataset "' + name + '" não tem falso no caso de teste');
            }
            var spec = datasets[name];
            if (spec === null) {
                return null;
            }
            var rules = Array.isArray(spec) ? spec : [spec];
            for (var k = 0; k < rules.length; k++) {
                var rule = rules[k];
                if (!matches(rule.when, cs)) {
                    continue;
                }
                if (rule.error != null) {
                    throw new Error(String(rule.error));
                }
                if (rule['null'] === true) {
                    return null;
                }
                return makeDataset(rule);
            }
            obs.missing.push('dataset ' + name);
            throw new Error('fluigcli: nenhuma regra do falso do dataset "' + name + '" casa com as constraints ' + JSON.stringify(cs));
        },
        getAvailableDatasets: record('DatasetFactory', 'getAvailableDatasets')
    };

    // fluigAPI: só o usuário corrente.
    var u = C.user || {};
    var user = {
        getLogin: function () { return J(u.login); },
        getCode: function () { return J(u.code); },
        getFullName: function () { return J(u.fullName); },
        getEmail: function () { return J(u.email); },
        getTenantId: function () { return C.companyId == null ? 1 : C.companyId; }
    };
    G.fluigAPI = { getUserService: function () { return { getCurrent: function () { return user; } }; } };
}

// __fluigcliFromJSON converte um valor do JSON do caso para o script: texto vira
// java.lang.String; {"$java": "x"} também; listas e objetos recursivos.
function __fluigcliFromJSON(v) {
    if (v == null) {
        return null;
    }
    if (typeof v === 'string') {
        return new java.lang.String(v);
    }
    if (Array.isArray(v)) {
        return v.map(__fluigcliFromJSON);
    }
    if (typeof v === 'object') {
        if (Object.prototype.hasOwnProperty.call(v, '$java')) {
            return new java.lang.String(String(v.$java));
        }
        var o = {};
        for (var k in v) {
            if (Object.prototype.hasOwnProperty.call(v, k)) {
                o[k] = __fluigcliFromJSON(v[k]);
            }
        }
        return o;
    }
    return v;
}

// __fluigcliArg resolve um argumento de chamada: tokens "$form", "$hAPI",
// "$customHTML" e "$children:<tabela>"; "$$x" é o texto "$x"; {"$java": "x"} é
// java.lang.String. Os demais valores JSON viram valores JS.
function __fluigcliArg(v) {
    if (typeof v === 'string') {
        if (v === '$form') {
            return __fluigcliGlobal.__fluigcliForm;
        }
        if (v === '$hAPI') {
            return __fluigcliGlobal.hAPI;
        }
        if (v === '$customHTML') {
            return __fluigcliGlobal.__fluigcliCustomHTML;
        }
        if (v.indexOf('$children:') === 0) {
            return __fluigcliGlobal.__fluigcliForm.getChildrenIndexes(v.substring(10));
        }
        if (v.indexOf('$$') === 0) {
            return v.substring(1);
        }
        return v;
    }
    if (v != null && typeof v === 'object' && !Array.isArray(v) && Object.prototype.hasOwnProperty.call(v, '$java')) {
        return new java.lang.String(String(v.$java));
    }
    return v;
}

function __fluigcliIsJava(v, cls) {
    try {
        return v instanceof cls;
    } catch (e) {
        return false;
    }
}

function __fluigcliTypeOf(v) {
    if (v === undefined) {
        return 'undefined';
    }
    if (v === null) {
        return 'null';
    }
    if (__fluigcliIsJava(v, java.lang.String)) {
        return 'java-string';
    }
    if (typeof v === 'string') {
        return 'js-string';
    }
    if (typeof v === 'number' || __fluigcliIsJava(v, java.lang.Number)) {
        return 'number';
    }
    if (typeof v === 'boolean' || __fluigcliIsJava(v, java.lang.Boolean)) {
        return 'boolean';
    }
    if (Array.isArray(v)) {
        return 'array';
    }
    if (__fluigcliIsJavaArray(v)) {
        return 'java-array';
    }
    if (typeof v === 'function') {
        return 'function';
    }
    if (v instanceof Date) {
        return 'date';
    }
    if (__fluigcliIsJava(v, java.lang.Object)) {
        return 'java-object';
    }
    return 'object';
}

function __fluigcliIsJavaArray(v) {
    return v != null && typeof v === 'object' &&
        String(Object.prototype.toString.call(v)) === '[object JavaArray]';
}

// __fluigcliToJSON converte o valor devolvido pela função para JSON.
function __fluigcliToJSON(v) {
    var t = __fluigcliTypeOf(v);
    switch (t) {
    case 'undefined':
    case 'null':
        return null;
    case 'java-string':
    case 'js-string':
        return String(v);
    case 'number':
        var n = Number(v);
        return isFinite(n) ? n : String(n);
    case 'boolean':
        return v == true;
    case 'array':
    case 'java-array':
        var out = [];
        for (var i = 0; i < v.length; i++) {
            out.push(__fluigcliToJSON(v[i]));
        }
        return out;
    case 'date':
        return isNaN(v.getTime()) ? 'Invalid Date' : v.toISOString();
    case 'function':
        return '[function]';
    case 'java-object':
        return String(v.toString());
    }
    var o = {};
    for (var k in v) {
        if (Object.prototype.hasOwnProperty.call(v, k) && typeof v[k] !== 'function') {
            o[k] = __fluigcliToJSON(v[k]);
        }
    }
    return o;
}

function __fluigcliDescribe(e) {
    var d = { message: '', kind: 'value' };
    if (e != null && e.javaException) {
        d.kind = 'java';
        d.message = String(e.javaException.getMessage ? e.javaException.getMessage() : e.javaException);
        d.javaClass = String(e.javaException.getClass().getName());
    } else if (e instanceof Error) {
        d.kind = 'error';
        d.name = String(e.name);
        d.message = String(e.message);
        if (e.fileName) {
            d.file = String(e.fileName);
        }
        if (e.lineNumber) {
            d.line = Number(e.lineNumber);
        }
    } else {
        d.message = String(e);
    }
    return d;
}

// __fluigcliRun chama a função do caso e devolve as observações em JSON.
function __fluigcliRun() {
    var C = __fluigcliCase;
    var obs = __fluigcliObs;
    var res = {};
    var fn = __fluigcliGlobal[C.call];
    if (typeof fn !== 'function') {
        res.setupError = 'a função "' + C.call + '" não existe nos scripts carregados';
    } else {
        var args = [];
        var raw = C.args || [];
        for (var i = 0; i < raw.length; i++) {
            args.push(__fluigcliArg(raw[i]));
        }
        var t0 = java.lang.System.nanoTime();
        try {
            var r = fn.apply(__fluigcliGlobal, args);
            res.returnType = __fluigcliTypeOf(r);
            res['return'] = __fluigcliToJSON(r);
        } catch (e) {
            res.thrown = __fluigcliDescribe(e);
        }
        res.durationMs = (java.lang.System.nanoTime() - t0) / 1e6;
    }
    res.form = obs.form;
    res.card = obs.card;
    res.logs = obs.logs;
    res.datasetCalls = obs.datasetCalls;
    res.calls = obs.calls;
    res.html = obs.html;
    res.missing = obs.missing;
    return JSON.stringify(res);
}
