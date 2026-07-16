/**
 * Roda NO SERVIDOR no render do formulário — e no preview do `fluigcli dev`,
 * que o executa localmente com o painel de simulação (WKNumState, WKUser...).
 * Use para mostrar/esconder seções conforme a etapa do processo e o modo.
 */
function displayFields(form, customHTML) {
    var numState = parseInt(getValue("WKNumState"), 10) || 0;
    // var mode = form.getFormMode(); // ADD | MOD | VIEW

    // Exemplo: mostrar a seção de aprovação só na etapa 5
    // form.setVisibleById("secao_aprovacao", numState == 5);
}
