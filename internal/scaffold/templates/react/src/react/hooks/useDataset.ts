import { useCallback, useEffect, useRef, useState } from 'react'
import { getDataset, type DatasetRequest } from '../fluig/dataset'

// Consulta um dataset com estado reativo: rows/loading/error prontos para o
// JSX; reload() re-executa (ex.: após mudar um filtro).
export function useDataset<T = Record<string, string>>(req: DatasetRequest) {
	const [rows, setRows] = useState<T[]>([])
	const [loading, setLoading] = useState(true)
	const [error, setError] = useState<string | null>(null)

	// A requisição é congelada na montagem — mudar o objeto inline a cada
	// render não dispara nova consulta (chame reload() explicitamente).
	const reqRef = useRef(req)

	const reload = useCallback(async (): Promise<void> => {
		setLoading(true)
		setError(null)
		try {
			setRows(await getDataset<T>(reqRef.current))
		} catch (err: any) {
			setError(String(err?.message ?? err))
		} finally {
			setLoading(false)
		}
	}, [])

	useEffect(() => {
		void reload()
	}, [reload])

	return { rows, loading, error, reload }
}
