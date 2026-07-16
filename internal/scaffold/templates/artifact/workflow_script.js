/**
 * Processo: [[.ProcessID]] — evento [[.Event]]
 * Roda no servidor (Rhino) [[.Doc]].
 *
 * Publicação: fluigcli workflow export [[.ProcessID]]   (cirúrgica, sem nova versão; exige a fluiggersWidget)
 *         ou: fluigcli workflow publish [[.ProcessID]]  (nativa; cria e libera nova versão)
 */
function [[.Event]]([[.Params]]) {
    // APIs disponíveis nos eventos de processo:
    //   hAPI.getCardValue("campo") / hAPI.setCardValue("campo", "valor")
    //   getValue("WKUser"), getValue("WKNumState"), getValue("WKDef")...
    //   DatasetFactory.getDataset(nome, fields, constraints, order)
    //   log.info("mensagem")
}
