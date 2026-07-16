// Consulta de datasets pela API pública do Fluig (a autenticação é a sessão
// do usuário logado no portal; no npm run dev, a sessão vem do fluigcli dev
// via proxy do Vite).

export enum ConstraintType {
	MUST = 1,
	SHOULD = 2,
	MUST_NOT = 3,
}

export interface DatasetConstraint {
	_field: string
	_initialValue: string
	_finalValue: string
	_type: ConstraintType
	_likeSearch: boolean
}

export interface DatasetRequest {
	dataset: string
	fields?: string[]
	constraints?: DatasetConstraint[]
	order?: string[]
}

/** Monta um filtro de dataset (intervalo = initial/final diferentes). */
export function createConstraint(
	field: string,
	initialValue: string,
	finalValue: string = initialValue,
	type: ConstraintType = ConstraintType.MUST,
	likeSearch = false,
): DatasetConstraint {
	return {
		_field: field,
		_initialValue: initialValue,
		_finalValue: finalValue,
		_type: type,
		_likeSearch: likeSearch,
	}
}

/** Executa a consulta e devolve as linhas como objetos campo→valor. */
export async function getDataset<T = Record<string, string>>(req: DatasetRequest): Promise<T[]> {
	const res = await fetch('/api/public/ecm/dataset/datasets', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({
			name: req.dataset,
			fields: req.fields ?? [],
			constraints: req.constraints ?? [],
			order: req.order ?? [],
		}),
	})
	if (!res.ok) {
		throw new Error(`dataset ${req.dataset}: HTTP ${res.status}`)
	}
	const data = await res.json()
	if (data.message && String(data.message).trim() !== 'OK') {
		throw new Error(String(data.message))
	}
	// O shape da resposta varia entre versões do Fluig.
	if (Array.isArray(data.content)) return data.content
	if (Array.isArray(data.content?.values)) return data.content.values
	if (Array.isArray(data.values)) return data.values
	return []
}
