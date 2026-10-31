// fluigcli test — ajudante injetado na página da tarefa (ROADMAP §5.19).
//
// Roda na janela PAI do pageworkflowview, antes dos scripts do Fluig. Registra
// o POST do workflowView/send (corpo e resposta) e lê/escreve o formulário, que
// mora no iframe #workflowView-cardViewer (mesma origem). Medições do Gate E:
// - campo bloqueado pelo setEnabled(false) em modo edição = input hidden com o
//   nome original (o valor enviado) + elemento visível renomeado "_campo",
//   readonly ou disabled;
// - a mensagem de erro (beforeSendValidate ou recusa do servidor) é um
//   FLUIGC.message: div[id^=fluig-message-].modal.in.
(function () {
    if (window.top !== window || window.__fluigcli) {
        return;
    }
    var sends = [];
    var open = XMLHttpRequest.prototype.open;
    var send = XMLHttpRequest.prototype.send;
    XMLHttpRequest.prototype.open = function (m, u) {
        this.__fluigcliURL = String(u || '');
        return open.apply(this, arguments);
    };
    XMLHttpRequest.prototype.send = function (body) {
        var x = this;
        if (/workflowView\/send/.test(x.__fluigcliURL)) {
            var rec = { body: typeof body === 'string' ? body.slice(0, 500000) : '', status: 0, response: '', done: false };
            sends.push(rec);
            x.addEventListener('loadend', function () {
                rec.status = x.status;
                rec.response = String(x.responseText || '').slice(0, 20000);
                rec.done = true;
            });
        }
        return send.apply(this, arguments);
    };

    function doc() {
        var f = document.getElementById('workflowView-cardViewer');
        try {
            return f && f.contentDocument;
        } catch (e) {
            return null;
        }
    }
    function vis(e) {
        return !!(e && (e.offsetWidth || e.offsetHeight || e.getClientRects().length));
    }
    function list(d, sel) {
        return Array.prototype.slice.call(d.querySelectorAll(sel));
    }
    function esc(s) {
        return String(s).replace(/["\\]/g, '\\$&');
    }
    // resolve acha o que a tela mostra para uma chave: nome de campo, id ou
    // "css:<seletor>".
    function resolve(d, key) {
        if (key.indexOf('css:') === 0) {
            var c = list(d, key.slice(4));
            return { all: c, field: c.length > 0 && c[0].matches('input,select,textarea') };
        }
        var named = list(d, '[name="' + esc(key) + '"]');
        var shown = named.filter(function (e) { return e.type !== 'hidden'; });
        if (shown.length) {
            return { all: shown, field: true };
        }
        var under = list(d, '[name="_' + esc(key) + '"]');
        if (under.length) {
            return { all: under, field: true, locked: true };
        }
        var byId = d.getElementById(key);
        if (byId) {
            return { all: [byId], field: byId.matches('input,select,textarea') };
        }
        if (named.length) {
            return { all: named, field: true, hiddenOnly: true };
        }
        return { all: [] };
    }
    function valueOf(r) {
        var e = r.all[0];
        if (!e) {
            return null;
        }
        if (!r.field) {
            return (e.innerText || e.textContent || '').trim();
        }
        if (e.type === 'radio') {
            var ck = r.all.filter(function (x) { return x.checked; });
            return ck.length ? ck[0].value : '';
        }
        if (e.type === 'checkbox') {
            return r.all.filter(function (x) { return x.checked; }).map(function (x) { return x.value; }).join(',');
        }
        return e.value;
    }
    function messages() {
        var out = [];
        [document, doc()].forEach(function (d) {
            if (!d) {
                return;
            }
            list(d, '[id^="fluig-message-"].modal.in, [id^="fluig-message-"].modal.show').forEach(function (m) {
                if (!vis(m) && getComputedStyle(m).display === 'none') {
                    return;
                }
                var body = m.querySelector('.modal-body') || m;
                var t = (body.innerText || body.textContent || '').replace(/Ver detalhes técnicos[\s\S]*$/, '').trim();
                if (t) {
                    out.push(t);
                }
            });
        });
        return out;
    }

    window.__fluigcli = {
        ready: function () {
            var d = doc();
            return {
                form: !!(d && d.readyState === 'complete' && d.querySelector('form')),
                messages: messages(),
                tenant: (location.pathname.match(/\/portal\/p\/([^/]+)\//) || [])[1] || ''
            };
        },
        inspect: function (q) {
            var d = doc();
            var out = { elems: {}, css: {} };
            if (!d) {
                return out;
            }
            (q.names || []).forEach(function (k) {
                var r = resolve(d, k);
                var shown = r.all.filter(vis);
                out.elems[k] = {
                    exists: r.all.length > 0,
                    visible: shown.length > 0,
                    editable: !!r.field && !r.locked && !r.hiddenOnly && shown.some(function (e) { return !e.disabled && !e.readOnly; }),
                    locked: !!r.locked,
                    value: valueOf(r),
                    text: r.all[0] ? ((r.field ? valueOf(r) : (r.all[0].innerText || r.all[0].textContent || '')) || '').trim() : null
                };
            });
            Object.keys(q.css || {}).forEach(function (k) {
                var r = resolve(d, k);
                if (!r.all[0]) {
                    out.css[k] = null;
                    return;
                }
                var cs = getComputedStyle(r.all[0]);
                var m = {};
                q.css[k].forEach(function (p) { m[p] = cs.getPropertyValue(p); });
                out.css[k] = m;
            });
            return out;
        },
        hidden: function (names) {
            var d = doc();
            var out = {};
            names.forEach(function (n) {
                var e = d && d.querySelector('[name="' + esc(n) + '"]');
                out[n] = e ? e.value : null;
            });
            return out;
        },
        fill: function (pairs) {
            var d = doc();
            if (!d) {
                return ['o formulário não está carregado'];
            }
            var errs = [];
            pairs.forEach(function (p) {
                var k = p[0], v = p[1];
                var r = resolve(d, k);
                if (!r.all.length) {
                    errs.push(k + ': campo não existe no formulário');
                    return;
                }
                if (r.locked || r.hiddenOnly) {
                    errs.push(k + ': campo bloqueado nesta etapa (displayFields)');
                    return;
                }
                var e = r.all[0];
                if (e.type === 'radio' || e.type === 'checkbox') {
                    var alvo = r.all.filter(function (x) { return x.value === v; })[0];
                    if (!alvo) {
                        errs.push(k + ': nenhuma opção com o valor ' + JSON.stringify(v) + ' (opções: ' + r.all.map(function (x) { return x.value; }).join(', ') + ')');
                        return;
                    }
                    if (alvo.disabled) {
                        errs.push(k + ': a opção ' + JSON.stringify(v) + ' está desabilitada');
                        return;
                    }
                    if (e.type === 'checkbox' && alvo.checked) {
                        return;
                    }
                    // No Style Guide do Fluig o label cobre o input: o clique vai no label.
                    var lb = alvo.id ? d.querySelector('label[for="' + esc(alvo.id) + '"]') : null;
                    (lb || alvo).click();
                    return;
                }
                if (e.disabled || e.readOnly) {
                    errs.push(k + ': campo somente leitura');
                    return;
                }
                e.focus();
                if (e.tagName === 'SELECT') {
                    e.value = v;
                    if (e.value !== v) {
                        errs.push(k + ': nenhuma opção com o valor ' + JSON.stringify(v));
                        return;
                    }
                } else {
                    e.value = v;
                    e.dispatchEvent(new Event('input', { bubbles: true }));
                }
                e.dispatchEvent(new Event('change', { bubbles: true }));
                e.dispatchEvent(new Event('blur', { bubbles: true }));
                var w = d.defaultView;
                if (w && w.jQuery) {
                    w.jQuery(e).trigger('change');
                }
            });
            return errs;
        },
        click: function (key) {
            var d = doc();
            var r = d ? resolve(d, key) : { all: [] };
            if (!r.all.length) {
                return key + ': elemento não existe no formulário';
            }
            r.all[0].click();
            return '';
        },
        options: function () {
            var s = document.querySelector('#fluigRightbar #nextActivity');
            return s ? Array.prototype.slice.call(s.options).map(function (o) { return o.value + '=' + o.text.trim(); }) : null;
        },
        choose: function (state) {
            var s = document.querySelector('#fluigRightbar #nextActivity');
            if (!s) {
                return 'a janela de envio não abriu';
            }
            s.value = String(state);
            if (s.value !== String(state)) {
                return 'a etapa ' + state + ' não está entre as opções de envio';
            }
            s.dispatchEvent(new Event('change', { bubbles: true }));
            if (window.jQuery) {
                window.jQuery(s).trigger('change');
            }
            return '';
        },
        status: function () {
            return { sends: sends, messages: messages() };
        },
        reset: function () {
            list(document, '[id^="fluig-message-"].modal.in button, [id^="fluig-message-"].modal.show button').forEach(function (b) { b.click(); });
            var d = doc();
            if (d) {
                list(d, '[id^="fluig-message-"].modal.in button').forEach(function (b) { b.click(); });
            }
            var c = document.querySelector('#fluigRightbar #cancel-button');
            if (c && vis(c)) {
                c.click();
            }
        }
    };
})();
