import { ref, type Ref } from 'vue'
import { getDataset, type DatasetRequest } from '../fluig/dataset'

// Consulta um dataset com estado reativo: rows/loading/error prontos para o
// template; reload() re-executa (ex.: após mudar um filtro).
export function useDataset<T = Record<string, string>>(req: DatasetRequest) {
	const rows = ref([]) as Ref<T[]>
	const loading = ref(true)
	const error = ref<string | null>(null)

	async function reload(): Promise<void> {
		loading.value = true
		error.value = null
		try {
			rows.value = await getDataset<T>(req)
		} catch (err: any) {
			error.value = String(err?.message ?? err)
		} finally {
			loading.value = false
		}
	}

	void reload()
	return { rows, loading, error, reload }
}
