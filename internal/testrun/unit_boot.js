// fluigcli test unit — bootstrap do shell do Rhino (ROADMAP §5.18).
//
// Uso: java ... org.mozilla.javascript.tools.shell.Main -version 200 -opt -1
//      unit_boot.js <plano.json>
//
// O plano traz o prelúdio dos falsos, os scripts (fonte já lida pelo
// fluigcli) e os casos. Cada caso roda num escopo NOVO: o prelúdio, os scripts
// e a chamada. O resultado sai como uma linha JSON por caso no arquivo
// plano.out, gravado em UTF-8 e com flush a cada linha. Assim, um estouro de
// tempo preserva os casos que já terminaram.

(function (planPath) {
    var Files = java.nio.file.Files;
    var Paths = java.nio.file.Paths;
    var UTF8 = java.nio.charset.StandardCharsets.UTF_8;
    var plan = JSON.parse(String(new java.lang.String(Files.readAllBytes(Paths.get(planPath)), UTF8)));
    var out = new java.io.BufferedWriter(new java.io.OutputStreamWriter(new java.io.FileOutputStream(plan.out), UTF8));
    var cx = org.mozilla.javascript.Context.getCurrentContext();

    // O erro de compilação chega ao catch só como "Compilation produced N
    // syntax errors". O reporter guarda a primeira mensagem real e a linha.
    var compileErrors = [];
    cx.setErrorReporter(new org.mozilla.javascript.ErrorReporter({
        warning: function () {},
        error: function (msg, src, line) {
            compileErrors.push({ message: String(msg), script: src == null ? '' : String(src), line: line });
        },
        runtimeError: function (msg, src, line, lineSrc, off) {
            return new org.mozilla.javascript.EvaluatorException(msg, src, line, lineSrc, off);
        }
    }));

    var describe = function (e, phase, name) {
        var d = { phase: phase, script: name, message: String(e) };
        var je = e && (e.rhinoException || e.javaException);
        if (je instanceof org.mozilla.javascript.RhinoException) {
            d.message = String(je.details());
            d.line = je.lineNumber();
            if (je.sourceName()) {
                d.script = String(je.sourceName());
            }
        } else if (je) {
            d.message = String(je.getMessage ? je.getMessage() : je);
        } else if (e instanceof Error) {
            d.message = String(e.message);
            if (e.lineNumber) {
                d.line = Number(e.lineNumber);
            }
        }
        return d;
    };

    for (var i = 0; i < plan.cases.length; i++) {
        var c = plan.cases[i];
        var line;
        try {
            var scope = cx.initStandardObjects();
            cx.evaluateString(scope, plan.prelude, 'fluigcli-prelude.js', 1, null);
            scope.__fluigcliSetupJSON = JSON.stringify(c.setup);
            cx.evaluateString(scope, '__fluigcliSetup(__fluigcliSetupJSON)', 'fluigcli-setup.js', 1, null);
            var loadError = null;
            for (var s = 0; s < c.scripts.length && loadError == null; s++) {
                var sc = plan.scripts[c.scripts[s]];
                compileErrors = [];
                try {
                    cx.evaluateString(scope, sc.source, sc.name, 1, null);
                } catch (e) {
                    loadError = describe(e, 'load', sc.name);
                    if (compileErrors.length > 0) {
                        loadError.message = compileErrors[0].message;
                        loadError.line = compileErrors[0].line;
                        loadError.compile = true;
                    }
                }
            }
            if (loadError != null) {
                line = JSON.stringify({ file: c.file, 'case': c['case'], loadError: loadError });
            } else {
                var res = JSON.parse(String(cx.evaluateString(scope, '__fluigcliRun()', 'fluigcli-run.js', 1, null)));
                res.file = c.file;
                res['case'] = c['case'];
                line = JSON.stringify(res);
            }
        } catch (e) {
            line = JSON.stringify({ file: c.file, 'case': c['case'], loadError: describe(e, 'harness', '') });
        }
        out.write(line);
        out.newLine();
        out.flush();
    }
    out.close();
})(arguments[0]);
