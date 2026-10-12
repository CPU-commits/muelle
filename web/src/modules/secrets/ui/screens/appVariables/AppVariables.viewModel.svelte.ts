import { resolve } from '$app/paths'
import { createQuery, type CreateQueryResult } from '@tanstack/svelte-query'
import { envQuery } from '../../../data'
import { appOwner, type EnvVar, overlappingKeys, stageOwner } from '../../../domain'
import { secretsErrorMessage } from '../../components/secretsMessages'
import { EnvEditorViewModel } from '../../features/envEditor'
import { EnvImportViewModel } from '../../features/envImport'

export interface AppVariablesParams {
	readonly project: string
	readonly stage: string
	readonly app: string
}

// La pestaña Variables de una app: las suyas, editables (EnvEditorViewModel + "Importar
// .env"), y las heredadas del stage en solo lectura. Si una clave está en las dos, gana la
// de la app: la fila de la app dice "pisa al stage" y la del stage va tachada.
export class AppVariablesViewModel {
	readonly editor: EnvEditorViewModel
	readonly importer: EnvImportViewModel

	readonly #params: () => AppVariablesParams
	readonly #stageEnv: CreateQueryResult<EnvVar[], Error>

	readonly overriddenKeys: ReadonlySet<string> = $derived.by(() =>
		overlappingKeys(this.editor.vars, this.stageVars),
	)

	constructor(params: () => AppVariablesParams) {
		this.#params = params
		this.editor = new EnvEditorViewModel(
			() => {
				const { project, stage, app } = this.#params()
				return appOwner(project, stage, app)
			},
			() => this.overriddenKeys,
		)
		this.importer = new EnvImportViewModel(this.editor)
		this.#stageEnv = createQuery(() => {
			const { project, stage } = this.#params()
			return envQuery(stageOwner(project, stage))
		})
	}

	get stage(): string {
		return this.#params().stage
	}

	get stageVars(): readonly EnvVar[] {
		return this.#stageEnv.data ?? []
	}

	get stageLoading(): boolean {
		return this.#stageEnv.isPending
	}

	get stageError(): string | null {
		if (this.#stageEnv.data !== undefined || !this.#stageEnv.error) return null
		return secretsErrorMessage(this.#stageEnv.error, 'stage')
	}

	/** Cambió algo en esta visita: la app quedó con cambios pendientes. */
	get hasPendingChanges(): boolean {
		return this.editor.savedChanges > 0
	}

	retryStage = (): void => {
		void this.#stageEnv.refetch()
	}

	get stageHref(): string {
		const { project, stage } = this.#params()
		return resolve('/(app)/projects/[p]/stages/[s]/variables', { p: project, s: stage })
	}

	/** El Resumen, donde está "Desplegar". */
	get summaryHref(): string {
		const { project, stage, app } = this.#params()
		return resolve('/(app)/projects/[p]/stages/[s]/apps/[a]', { p: project, s: stage, a: app })
	}

	get homeHref(): string {
		return resolve('/')
	}
}
