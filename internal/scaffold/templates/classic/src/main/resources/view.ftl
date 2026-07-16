<#-- Modo visualização. Regra de ouro: a widget pode aparecer mais de uma vez
     na página — todo id de DOM leva o sufixo ${instanceId}. -->
<div id="[[.CamelCode]]_${instanceId}" class="super-widget wcm-widget-class fluig-style-guide" data-params="[[.CamelCode]].instance()">
	<div class="panel panel-default">
		<div class="panel-heading">
			<h3 class="panel-title">${i18n.getTranslation('application.title')}</h3>
		</div>
		<div class="panel-body">
			<p>${i18n.getTranslation('message.welcome')}</p>
			<button type="button" class="btn btn-primary" data-hello-button>
				${i18n.getTranslation('button.action')}
			</button>
		</div>
	</div>
</div>
