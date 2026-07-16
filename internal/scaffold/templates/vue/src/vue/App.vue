<script setup lang="ts">
// Exemplo de widget no padrão da casa: classes do Fluig Style Guide (o portal
// já carrega o CSS — dark mode vem de graça), dataset real via o kit e toast
// do FLUIGC. Apague o conteúdo e construa a sua a partir daqui.
import { computed, ref } from 'vue'
import { useDataset } from './composables/useDataset'
import { createConstraint } from './fluig/dataset'
import { toast } from './fluig/fluigc'
import { t } from './fluig/i18n'

const props = defineProps<{
	instanceId: string
	configs: Record<string, string>
}>()

// Preferência da instância (edit.ftl → widgetSettings → data-configs).
const title = computed(() => props.configs.customTitle || 'Widget [[.Code]]')

const search = ref('')

interface Colleague {
	colleagueId: string
	colleagueName: string
	mail: string
}

const { rows, loading, error } = useDataset<Colleague>({
	dataset: 'colleague',
	fields: ['colleagueId', 'colleagueName', 'mail'],
	constraints: [createConstraint('active', 'true')],
	order: ['colleagueName'],
})

const filtered = computed(() =>
	rows.value.filter((c) => (c.colleagueName || '').toLowerCase().includes(search.value.toLowerCase())),
)

function hello(): void {
	toast(t('toast') + props.instanceId, 'success')
}
</script>

<template>
	<div class="panel panel-default">
		<div class="panel-heading">
			<h3 class="panel-title">{{ title }}</h3>
		</div>
		<div class="panel-body">
			<p>{{ t('welcome') }}</p>
			<div class="form-group">
				<input v-model="search" type="text" class="form-control" :placeholder="t('searchPlaceholder')" />
			</div>

			<p v-if="loading">{{ t('loading') }}</p>
			<p v-else-if="error" class="text-danger">{{ error }}</p>
			<template v-else>
				<table class="table table-striped">
					<thead>
						<tr>
							<th>{{ t('colName') }}</th>
							<th>{{ t('colMail') }}</th>
						</tr>
					</thead>
					<tbody>
						<tr v-for="c in filtered" :key="c.colleagueId">
							<td>{{ c.colleagueName }}</td>
							<td>{{ c.mail }}</td>
						</tr>
					</tbody>
				</table>
				<p v-if="!filtered.length">{{ t('empty') }}</p>
			</template>

			<button type="button" class="btn btn-primary" @click="hello">{{ t('button') }}</button>
		</div>
	</div>
</template>

<style scoped>
/* Estilos próprios da widget. O <style scoped> do Vue já confina ao
   componente — sem risco de vazar para o portal. Prefira as classes e as
   variáveis CSS (--fs-color-*) do style guide antes de inventar cor. */
.table {
	margin-bottom: 12px;
}
</style>
