// Wrappers finos dos componentes imperativos do FLUIGC (Fluig Style Guide),
// com fallback para o console quando a página não os tem (npm run dev).

type ToastType = 'success' | 'info' | 'warning' | 'danger'

export function toast(message: string, type: ToastType = 'info', title = ''): void {
	const F = (window as any).FLUIGC
	if (F?.toast) {
		F.toast({ title, message, type })
	} else {
		console.log(`[toast:${type}]`, message)
	}
}

/** Indicador de carregamento do style guide sobre um seletor/elemento. */
export function loading(target: string | Element | Window = window): { show(): void; hide(): void } {
	const F = (window as any).FLUIGC
	if (F?.loading) {
		return F.loading(target)
	}
	return {
		show: () => console.log('[loading] show'),
		hide: () => console.log('[loading] hide'),
	}
}
