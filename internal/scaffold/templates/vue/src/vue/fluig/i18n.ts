// Traduções da SPA, escolhidas pelo idioma do usuário no portal
// (WCMAPI.getLocale()). Os .properties de src/main/resources continuam
// valendo para o que o SERVIDOR renderiza (título na galeria e edit.ftl);
// aqui fica o que a SPA mostra.

const messages: Record<string, Record<string, string>> = {
	pt_BR: {
		welcome: 'Widget [[.Code]] criada com fluigcli — edite src/vue/App.vue e salve para ver o reload.',
		searchPlaceholder: 'Filtrar por nome…',
		loading: 'Carregando usuários…',
		colName: 'Nome',
		colMail: 'E-mail',
		button: 'Testar toast',
		toast: 'Widget funcionando! Instância: ',
		empty: 'Nenhum usuário encontrado.',
	},
	en_US: {
		welcome: 'Widget [[.Code]] created with fluigcli — edit src/vue/App.vue and save to reload.',
		searchPlaceholder: 'Filter by name…',
		loading: 'Loading users…',
		colName: 'Name',
		colMail: 'E-mail',
		button: 'Try toast',
		toast: 'Widget working! Instance: ',
		empty: 'No users found.',
	},
	es: {
		welcome: 'Widget [[.Code]] creada con fluigcli — edite src/vue/App.vue y guarde para recargar.',
		searchPlaceholder: 'Filtrar por nombre…',
		loading: 'Cargando usuarios…',
		colName: 'Nombre',
		colMail: 'E-mail',
		button: 'Probar toast',
		toast: '¡Widget funcionando! Instancia: ',
		empty: 'Ningún usuario encontrado.',
	},
}

function locale(): string {
	try {
		const loc = (window as any).WCMAPI?.getLocale?.()
		if (loc && messages[loc]) return loc
	} catch {
		// fora do portal não há WCMAPI
	}
	return 'pt_BR'
}

/** Traduz uma chave no idioma do usuário (fallback: pt_BR e a própria chave). */
export function t(key: string): string {
	return messages[locale()][key] ?? messages.pt_BR[key] ?? key
}
