<#-- Modo edição: preferências por instância (mecanismo oficial do Fluig).
     O botão dispara saveSettings (ver main.ts), que grava via
     WCMSpaceAPI.PageService.UPDATEPREFERENCES; o view.ftl recebe o JSON
     de volta em ${widgetSettings}. -->
<#assign settingsJSON = widgetSettings!'{}'>
<#attempt><#assign settings = settingsJSON?eval><#recover><#assign settings = {}></#attempt>
<div id="[[.CamelCode]]_${instanceId}" class="super-widget wcm-widget-class fluig-style-guide" data-params="[[.CamelCode]].instance({mode: 'edit'})">
	<div class="panel panel-default">
		<div class="panel-heading">
			<h3 class="panel-title">${i18n.getTranslation('application.title')}</h3>
		</div>
		<div class="panel-body">
			<form role="form" class="fs-prevent-default">
				<div class="form-group">
					<label for="[[.Code]]-customtitle-${instanceId}">${i18n.getTranslation('edit.customTitle')}</label>
					<input type="text" class="form-control" id="[[.Code]]-customtitle-${instanceId}" name="customTitle" value="${(settings.customTitle)!''}">
				</div>
				<button type="button" class="btn btn-primary" data-save-settings>${i18n.getTranslation('edit.save')}</button>
			</form>
		</div>
	</div>
</div>
