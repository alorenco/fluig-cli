<script setup lang="ts">
// Exemplo de widget com Vuetify 3 (via npm, tree-shaken) + dataset real via o
// kit. Ícones `mdi-*` funcionam por string (@mdi/font, como nas widgets
// Vuetify antigas). Apague o conteúdo e construa a sua a partir daqui.
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
	<v-card :title="title">
		<v-card-text>
			<p class="mb-4">{{ t('welcome') }}</p>
			<v-text-field
				v-model="search"
				:placeholder="t('searchPlaceholder')"
				prepend-inner-icon="mdi-magnify"
				density="compact"
				variant="outlined"
				hide-details
				class="mb-4"
			/>

			<v-progress-linear v-if="loading" indeterminate />
			<v-alert v-else-if="error" type="error" :text="error" />
			<template v-else>
				<v-table density="compact">
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
				</v-table>
				<p v-if="!filtered.length">{{ t('empty') }}</p>
			</template>
		</v-card-text>
		<v-card-actions>
			<v-btn color="primary" variant="flat" @click="hello">{{ t('button') }}</v-btn>
		</v-card-actions>
	</v-card>
</template>
