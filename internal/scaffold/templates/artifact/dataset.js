/**
 * Dataset customizado: [[.Name]]
 *
 * Publicação: fluigcli dataset export datasets/[[.Name]].js --new
 * Consulta:   fluigcli dataset query [[.Name]] --limit 10
 */

// Estrutura do dataset (colunas, chave e índices de sincronização).
function defineStructure() {
    addColumn("id");
    addColumn("descricao");

    // Chave principal e índices — só para dataset com sincronização/cache:
    // setKey(["id"]);
    // addIndex(["descricao"]);
}

// Consulta — roda a cada chamada (portal, REST, hAPI, widgets).
// fields:      colunas pedidas (null = todas)
// constraints: filtros recebidos (fieldName, initialValue, finalValue)
// sortFields:  ordenação pedida
function createDataset(fields, constraints, sortFields) {
    var dataset = DatasetBuilder.newDataset();
    dataset.addColumn("id");
    dataset.addColumn("descricao");

    // Exemplo: reagir a uma constraint recebida
    // if (constraints != null) {
    //     for (var i = 0; i < constraints.length; i++) {
    //         var c = constraints[i];
    //         if (c.fieldName == "id") { /* filtrar pela c.initialValue */ }
    //     }
    // }

    dataset.addRow([1, "Exemplo"]);
    return dataset;
}

// Sincronização com base externa (opcional — apague se não usar).
// function onSync(lastSyncDate) {
//     return DatasetBuilder.newDataset();
// }

// Sincronização mobile (opcional).
// function onMobileSync(user) {
// }
