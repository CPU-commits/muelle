import { resolve } from '$app/paths'
import { stageOwner } from '../../../domain'
import { EnvEditorViewModel } from '../../features/envEditor'
import { EnvImportViewModel } from '../../features/envImport'

export interface StageVariablesParams {
	readonly project: string
	readonly stage: string
}

// Las variables de un stage: las reciben todas sus apps (cada una puede pisarlas con las
// suyas). Cambiarlas deja con cambios pendientes a todas las apps del stage.
export class StageVariablesViewModel {
	readonly editor: EnvEditorViewModel
	readonly importer: EnvImportViewModel

	readonly #params: () => StageVariablesParams

	constructor(params: () => StageVariablesParams) {
		this.#params = params
		this.editor = new EnvEditorViewModel(() => {
			const { project, stage } = this.#params()
			return stageOwner(project, stage)
		})
		this.importer = new EnvImportViewModel(this.editor)
	}

	get stage(): string {
		return this.#params().stage
	}

	/** Cambió algo en esta visita: las apps del stage quedaron con cambios pendientes. */
	get hasPendingChanges(): boolean {
		return this.editor.savedChanges > 0
	}

	get homeHref(): string {
		return resolve('/')
	}
}
