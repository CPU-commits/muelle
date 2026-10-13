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
import {
	dotenvIssueMessage,
	dotenvSwallowedMessage,
	importedNote,
	limitMessage,
} from '../../components/secretsMessages'
import type { EnvEditorViewModel } from '../envEditor'

// "Importar .env": el texto pegado se parsea mientras se escribe, se compara con las
// variables del dueño (el plan: nuevas, cambian, secretas que se reescriben, sin cambios
// y, si reemplaza, las que se borran) y se aplica en un solo PATCH. Borrar variables o
// cambiar la marca de secreta de una que ya existía se confirma antes. El texto puede
// tener secretos: se borra al cerrar el panel y después de importar.
export class EnvImportViewModel {
	open = $state(false)
	text = $state('')
	mode = $state<ImportMode>('merge')
	secret = $state(true)
	/** Lo que rechazó el backend (un 422, un límite que el panel no podía ver). */
	serverError = $state<string | null>(null)
	/**
	 * El plan que se está confirmando: el que vio la persona. Si mientras tanto la lista se
	 * vuelve a cargar, se aplica este y no uno recalculado (que podría borrar otra cosa).
	 */
	confirming = $state.raw<ImportPlan | null>(null)
	/** Cuántas veces se cerró el panel: la pantalla devuelve el foco al botón que lo abre. */
	closedCount = $state(0)

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

	/** Líneas que quedaron dentro de un valor de varias líneas y parecen otra variable. */
	get swallowedMessages(): string[] {
		return this.parsed.swallowed.map(dotenvSwallowedMessage)
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

	get isConfirming(): boolean {
		return this.confirming !== null
	}

	/** Lo más grave manda en el título: borrar, después mostrar, después ocultar. */
	get confirmTitle(): string {
		const plan = this.confirming
		if (!plan) return ''
		if (plan.removed.length > 0) return m.secrets_import_replace_title()
		if (keysBecoming(plan, 'to_plain').length > 0) return m.secrets_import_unmask_title()
		return m.secrets_import_conceal_title()
	}

	get confirmBody(): string {
		const plan = this.confirming
		if (!plan) return ''
		const revealed = keysBecoming(plan, 'to_plain')
		const concealed = keysBecoming(plan, 'to_secret')
		return [
			plan.removed.length > 0
				? m.secrets_import_replace_body({
						count: plan.removed.length,
						keys: plan.removed.join(', '),
					})
				: '',
			revealed.length > 0
				? m.secrets_import_unmask_body({
						count: revealed.length,
						keys: revealed.join(', '),
					})
				: '',
			concealed.length > 0
				? m.secrets_import_conceal_body({
						count: concealed.length,
						keys: concealed.join(', '),
					})
				: '',
		]
			.filter(Boolean)
			.join(' ')
	}

	get confirmLabel(): string {
		return this.confirming && this.confirming.removed.length > 0
			? m.secrets_import_replace_confirm()
			: m.secrets_import_submit()
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
		this.confirming = null
		this.closedCount += 1
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

	/**
	 * "Importar": lo que no se puede deshacer desde el panel se confirma antes, como en el
	 * menú de una fila: borrar variables, dejar a la vista una secreta o volver secreta una
	 * que se podía leer.
	 */
	submit = async (): Promise<boolean> => {
		if (!this.canSubmit) return false
		const plan = this.plan
		if (plan.removed.length > 0 || plan.changed.some((change) => change.reason !== 'value')) {
			this.confirming = plan
			return false
		}
		return this.#apply(plan)
	}

	confirmImport = async (): Promise<boolean> => {
		const plan = this.confirming
		if (!plan || this.#editor.isSaving) return false
		const applied = await this.#apply(plan)
		this.confirming = null
		return applied
	}

	cancelConfirm = (): void => {
		if (!this.isSubmitting) this.confirming = null
	}

	async #apply(plan: ImportPlan): Promise<boolean> {
		const patch = plan.patch
		if (!patch) return false
		this.serverError = null
		const outcome = await this.#editor.applyImport(patch, (message) => {
			this.serverError = message
			return true
		})
		if (outcome === 'failed') return false
		if (outcome === 'changed') {
			toast.success(m.secrets_import_success(), importedNote(this.#editor.kind))
		} else {
			// Solo secretas reescritas con el mismo valor: el backend no cambió nada.
			toast.info(m.secrets_nothing_changed(), m.secrets_nothing_changed_import())
		}
		this.close()
		return true
	}
}

/** Las claves que el import pasa a secretas (`to_secret`) o deja a la vista (`to_plain`). */
function keysBecoming(plan: ImportPlan, reason: 'to_secret' | 'to_plain'): string[] {
	return plan.changed.filter((change) => change.reason === reason).map((change) => change.key)
}
