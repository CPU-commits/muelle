import { m } from '@/shared/ui/i18n/paraglide/messages'
import type { ChoiceOption } from '@/shared/ui'
import { toast } from '@/shared/ui/stores'
import {
	type ImportMode,
	type ImportPlan,
	type ParsedDotenv,
	parseDotenv,
	patchLimitProblem,
	planImport,
} from '../../../domain'
import { dotenvIssueMessage, limitMessage, savedNote } from '../../components/secretsMessages'
import type { EnvEditorViewModel } from '../envEditor'

// "Importar .env": el texto pegado se parsea mientras se escribe, se compara con las
// variables del dueño (el plan: nuevas, cambian, secretas que se reescriben, sin cambios
// y, si reemplaza, las que se borran) y se aplica en un solo PATCH. El texto puede tener
// secretos: se borra al cerrar el panel y después de importar.
export class EnvImportViewModel {
	open = $state(false)
	text = $state('')
	mode = $state<ImportMode>('merge')
	secret = $state(true)
	/** Lo que rechazó el backend (un 422, un límite que el panel no podía ver). */
	serverError = $state<string | null>(null)
	confirmingReplace = $state(false)

	readonly #editor: EnvEditorViewModel

	readonly parsed: ParsedDotenv = $derived.by(() => parseDotenv(this.text))
	readonly plan: ImportPlan = $derived.by(() =>
		planImport(this.#editor.vars, this.parsed.entries, {
			mode: this.mode,
			secret: this.secret,
		}),
	)

	constructor(editor: EnvEditorViewModel) {
		this.#editor = editor
	}

	get modeOptions(): ChoiceOption<ImportMode>[] {
		return [
			{ value: 'merge', label: m.secrets_import_mode_merge() },
			{ value: 'replace', label: m.secrets_import_mode_replace() },
		]
	}

	get hasText(): boolean {
		return this.text.trim().length > 0
	}

	get entryCount(): number {
		return this.parsed.entries.length
	}

	get issueMessages(): string[] {
		return this.parsed.issues.map(dotenvIssueMessage)
	}

	get duplicateMessages(): string[] {
		return this.parsed.duplicates.map((duplicate) =>
			m.secrets_import_duplicate({
				key: duplicate.key,
				first: duplicate.firstLine,
				line: duplicate.line,
			}),
		)
	}

	/** El plan se muestra cuando el archivo se leyó entero y hay algo que comparar. */
	get showPlan(): boolean {
		return this.#editor.hasLoaded && this.parsed.issues.length === 0 && this.entryCount > 0
	}

	get limitError(): string | null {
		const patch = this.plan.patch
		if (!patch) return null
		const limit = patchLimitProblem(this.#editor.vars, patch)
		return limit ? limitMessage(limit) : null
	}

	get isSubmitting(): boolean {
		return this.#editor.pendingAction === 'import'
	}

	get canSubmit(): boolean {
		return (
			this.showPlan &&
			this.plan.patch !== null &&
			this.limitError === null &&
			!this.#editor.isSaving
		)
	}

	get removedKeysText(): string {
		return this.plan.removed.join(', ')
	}

	toggle = (): void => {
		if (this.open) this.close()
		else this.open = true
	}

	close = (): void => {
		if (this.isSubmitting) return
		this.open = false
		this.text = ''
		this.mode = 'merge'
		this.secret = true
		this.serverError = null
		this.confirmingReplace = false
	}

	setText = (value: string): void => {
		this.text = value
		this.serverError = null
	}

	setMode = (value: ImportMode): void => {
		this.mode = value
		this.serverError = null
	}

	setSecret = (value: boolean): void => {
		this.secret = value
		this.serverError = null
	}

	/** "Importar": si va a borrar variables, primero lo confirma. */
	submit = async (): Promise<boolean> => {
		if (!this.canSubmit) return false
		if (this.plan.removed.length > 0) {
			this.confirmingReplace = true
			return false
		}
		return this.#apply()
	}

	confirmReplace = async (): Promise<boolean> => {
		if (!this.canSubmit) return false
		const applied = await this.#apply()
		this.confirmingReplace = false
		return applied
	}

	cancelReplace = (): void => {
		if (!this.isSubmitting) this.confirmingReplace = false
	}

	async #apply(): Promise<boolean> {
		const patch = this.plan.patch
		if (!patch) return false
		this.serverError = null
		const applied = await this.#editor.applyImport(patch, (message) => {
			this.serverError = message
			return true
		})
		if (!applied) return false
		toast.success(m.secrets_import_success(), savedNote(this.#editor.kind))
		this.close()
		return true
	}
}
