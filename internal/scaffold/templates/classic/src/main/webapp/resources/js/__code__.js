var [[.CamelCode]] = SuperWidget.extend({

	// Roda quando a instância monta. Aqui já existem this.instanceId,
	// this.isEditMode e this.DOM (o container jQuery da widget).
	init: function () {
	},

	// Bindings declarativos: o atributo data-hello-button no HTML vira o
	// evento 'click' ligado ao método helloAction.
	bindings: {
		local: {
			'hello-button': ['click_helloAction']
		},
		global: {}
	},

	helloAction: function () {
		FLUIGC.toast({
			title: '',
			message: 'Widget [[.Code]] funcionando! (instância ' + this.instanceId + ')',
			type: 'success'
		});
	}
});
