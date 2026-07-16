// Exemplo de widget no padrão da casa: classes do Fluig Style Guide (o portal
// já carrega o CSS — dark mode vem de graça), dataset real via o kit e toast
// do FLUIGC. Apague o conteúdo e construa a sua a partir daqui.
import { useMemo, useState } from 'react'
import { useDataset } from './hooks/useDataset'
import { createConstraint } from './fluig/dataset'
import { toast } from './fluig/fluigc'
import { t } from './fluig/i18n'

interface AppProps {
	instanceId: string
	configs: Record<string, string>
}

interface Colleague {
	colleagueId: string
	colleagueName: string
	mail: string
}

export function App({ instanceId, configs }: AppProps) {
	// Preferência da instância (edit.ftl → widgetSettings → data-configs).
	const title = configs.customTitle || 'Widget [[.Code]]'

	const [search, setSearch] = useState('')

	const { rows, loading, error } = useDataset<Colleague>({
		dataset: 'colleague',
		fields: ['colleagueId', 'colleagueName', 'mail'],
		constraints: [createConstraint('active', 'true')],
		order: ['colleagueName'],
	})

	const filtered = useMemo(
		() => rows.filter((c) => (c.colleagueName || '').toLowerCase().includes(search.toLowerCase())),
		[rows, search],
	)

	function hello(): void {
		toast(t('toast') + instanceId, 'success')
	}

	return (
		<div className="panel panel-default">
			<div className="panel-heading">
				<h3 className="panel-title">{title}</h3>
			</div>
			<div className="panel-body">
				<p>{t('welcome')}</p>
				<div className="form-group">
					<input
						type="text"
						className="form-control"
						placeholder={t('searchPlaceholder')}
						value={search}
						onChange={(e) => setSearch(e.target.value)}
					/>
				</div>

				{loading && <p>{t('loading')}</p>}
				{!loading && error && <p className="text-danger">{error}</p>}
				{!loading && !error && (
					<>
						<table className="table table-striped">
							<thead>
								<tr>
									<th>{t('colName')}</th>
									<th>{t('colMail')}</th>
								</tr>
							</thead>
							<tbody>
								{filtered.map((c) => (
									<tr key={c.colleagueId}>
										<td>{c.colleagueName}</td>
										<td>{c.mail}</td>
									</tr>
								))}
							</tbody>
						</table>
						{!filtered.length && <p>{t('empty')}</p>}
					</>
				)}

				<button type="button" className="btn btn-primary" onClick={hello}>
					{t('button')}
				</button>
			</div>
		</div>
	)
}
