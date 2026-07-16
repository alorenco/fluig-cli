/**
 * Mecanismo de atribuição customizado: [[.Name]]
 *
 * Publicação: fluigcli mechanism export mechanisms/[[.Name]].js --name "[[.Name]]"
 *
 * Deve devolver a lista de usuários aptos a receber a tarefa — sempre por
 * userCode (matrícula), NUNCA por login.
 */
function resolve(process, colleague) {
    var users = new java.util.ArrayList();

    // Exemplo: decidir os responsáveis consultando um dataset
    // var ds = DatasetFactory.getDataset("colleague", null, null, null);
    // for (var i = 0; i < ds.rowsCount; i++) {
    //     users.add(ds.getValue(i, "colleagueId"));
    // }

    users.add(colleague.getColleagueId()); // devolve ao próprio usuário
    return users;
}
