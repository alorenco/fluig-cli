// Ponte entre a widget Fluig (SuperWidget) e a SPA React.
//
// O portal carrega este bundle (application.resource.js.1), lê o
// data-params dos FTLs e chama [[.CamelCode]].instance({mode}) para cada
// instância na página — o init monta um root React por instância no div
// #[[.Code]]-root-<instanceId> do view.ftl. Modo edição fica no edit.ftl
// (formulário clássico + UPDATEPREFERENCES), sem React.
import { createRoot } from 'react-dom/client'
import { App } from './App'
import './app.css'

const win = window as any

// Fora do portal (npm run dev) não existe SuperWidget — simula o mínimo.
if (!win.SuperWidget) {
	win.SuperWidget = {
		extend: (def: Record<string, unknown>) => ({
			instance: (opts: Record<string, unknown>) => {
				const inst: any = { ...def, ...opts, DOM: win.$ ? win.$(document.body) : null }
				inst.init?.()
				return inst
			},
		}),
	}
}

win.[[.CamelCode]] = win.SuperWidget.extend({
	init(this: any) {
		if (this.mode !== 'edit') {
			mount(this.instanceId)
		}
	},

	// data-save-settings no edit.ftl → clique → saveSettings.
	bindings: {
		local: { 'save-settings': ['click_saveSettings'] },
		global: {},
	},

	// Grava as preferências da instância (widgetSettings): coleta os campos
	// nomeados do formulário do edit.ftl e envia pelo mecanismo oficial.
	saveSettings(this: any) {
		const fields: Record<string, string> = {}
		this.DOM.find('input, select, textarea').each(function (this: any) {
			if (this.name) fields[this.name] = win.$(this).val()
		})
		try {
			const ok = win.WCMSpaceAPI.PageService.UPDATEPREFERENCES(
				{ async: false },
				this.instanceId,
				{ widgetSettings: JSON.stringify(fields) },
			)
			win.FLUIGC.toast({
				title: '',
				message: ok ? 'Preferências salvas.' : 'Não foi possível salvar.',
				type: ok ? 'success' : 'danger',
			})
		} catch (err: any) {
			win.FLUIGC.toast({ title: '', message: String(err?.message ?? err), type: 'danger' })
		}
	},
})

// Monta a SPA no div do view.ftl. O portal pode rodar o init antes do div
// estar no DOM — insiste por até ~0,5 s.
function mount(instanceId: string | number, attempt = 0) {
	const el = document.getElementById(`[[.Code]]-root-${instanceId}`)
	if (!el) {
		if (attempt < 10) setTimeout(() => mount(instanceId, attempt + 1), 50)
		return
	}
	let configs: Record<string, string> = {}
	try {
		configs = JSON.parse(el.getAttribute('data-configs') || '{}')
	} catch {
		// preferências corrompidas viram objeto vazio
	}
	createRoot(el).render(<App instanceId={String(instanceId)} configs={configs} />)
}

// npm run dev: instancia direto, com o id "local" do index.html.
if (import.meta.env.DEV) {
	win.[[.CamelCode]].instance({ mode: 'view', instanceId: 'local' })
}
