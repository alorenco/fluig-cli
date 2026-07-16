<#-- Ponte da SPA: o FreeMarker só injeta as preferências da instância
     (widgetSettings) e a SPA compilada monta no div raiz — o template da
     aplicação vive em src/, não aqui. -->
<div id="[[.CamelCode]]_${instanceId}" class="super-widget wcm-widget-class fluig-style-guide" data-params="[[.CamelCode]].instance({mode: 'view'})">
	<div id="[[.Code]]-root-${instanceId}" data-configs="${(widgetSettings!'{}')?html}"></div>
</div>
