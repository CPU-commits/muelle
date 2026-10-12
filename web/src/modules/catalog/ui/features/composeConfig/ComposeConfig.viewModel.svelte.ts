import { createMutation, useQueryClient } from '@tanstack/svelte-query'
import { untrack } from 'svelte'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { toast } from '@/shared/ui/stores'
import { ValidationError } from '@/shared/domain'
import { updateAppMutation } from '../../../data'
import {
	type AppRef,
	appRefOf,
	type ComposeApp,
	type ComposeServiceSummary,
	composeProblem,
	summarizeCompose,
} from '../../../domain'
import { catalogErrorMessage } from '../../components/catalogMessages'
import { lineCount, yamlSyntaxErrorLines } from '../../components/yamlEditor'

export type ComposeCheck =
	/** Todavía no se validó, o se descartó. */
	| { readonly kind: 'idle' }
	/** "Validar" o "Guardar" encontraron errores de sintaxis en esas líneas. */
	| { readonly kind: 'syntax'; readonly lines: readonly number[]; readonly onSave: boolean }
	/** El compose no se puede guardar (vacío, más de 256 KiB, un 422 del backend). */
	| { readonly kind: 'rejected'; readonly message: string }

// App·Compose (`App-Compose.dc.html`): el YAML en un editor, "Validar" (la sintaxis, en
// el navegador) y "Guardar" (PATCH `compose_yaml`). La validación completa del compose
// (puertos, build, redes) es de la Fase 2.
export class ComposeConfigViewModel {
	draft = $state('')
	check = $state.raw<ComposeCheck>({ kind: 'idle' })
	focusRequest = $state.raw<{ line: number; nonce: number } | null>(null)

	readonly #app: () => ComposeApp
	readonly #client = useQueryClient()
	readonly #update = createMutation(() => updateAppMutation(this.#client))

	constructor(app: () => ComposeApp) {
		this.#app = app
		this.draft = untrack(() => app().composeYaml)
	}

	get ref(): AppRef {
		return appRefOf(this.#app())
	}

	get isDirty(): boolean {
		return this.draft !== this.#app().composeYaml
	}

	get isSaving(): boolean {
		return this.#update.isPending
	}

	get lines(): number {
		return lineCount(this.draft)
	}

	get services(): ComposeServiceSummary[] {
		return summarizeCompose(this.draft)
	}

	get errorLines(): readonly number[] {
		return this.check.kind === 'syntax' ? this.check.lines : []
	}

	setDraft = (value: string): void => {
		this.draft = value
		// Con errores a la vista, se revalida mientras se corrigen (como un formulario).
		if (this.check.kind === 'syntax') this.#checkSyntax(this.check.onSave)
		else if (this.check.kind === 'rejected') this.check = { kind: 'idle' }
	}

	validate = (): void => {
		if (this.#checkSyntax(false)) {
			toast.success(m.catalog_compose_valid(), m.catalog_compose_valid_body())
		}
	}

	save = async (): Promise<void> => {
		if (this.isSaving) return
		const problem = composeProblem(this.draft)
		if (problem) {
			this.check = {
				kind: 'rejected',
				message:
					problem === 'empty'
						? m.catalog_error_compose_empty()
						: m.catalog_error_compose_too_large(),
			}
			return
		}
		if (!this.#checkSyntax(true)) return
		if (!this.isDirty) {
			toast.info(m.catalog_config_no_changes())
			return
		}
		try {
			await this.#update.mutateAsync({
				ref: this.ref,
				changes: { composeYaml: this.draft },
			})
			toast.success(m.catalog_compose_saved(), m.catalog_config_saved_body())
		} catch (error) {
			// Un 422 de `compose_yaml` trae el motivo del backend; lo demás, su mensaje.
			const message =
				error instanceof ValidationError && error.field === 'composeYaml'
					? error.message
					: catalogErrorMessage(error)
			this.check = { kind: 'rejected', message }
		}
	}

	discard = (): void => {
		this.draft = this.#app().composeYaml
		this.check = { kind: 'idle' }
	}

	goToLine = (line: number): void => {
		this.focusRequest = { line, nonce: (this.focusRequest?.nonce ?? 0) + 1 }
	}

	// true si la sintaxis es válida.
	#checkSyntax(onSave: boolean): boolean {
		const lines = yamlSyntaxErrorLines(this.draft)
		this.check = lines.length > 0 ? { kind: 'syntax', lines, onSave } : { kind: 'idle' }
		return lines.length === 0
	}
}
