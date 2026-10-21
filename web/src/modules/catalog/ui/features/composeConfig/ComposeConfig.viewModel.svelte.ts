import { createMutation, useQueryClient } from '@tanstack/svelte-query'
import { untrack } from 'svelte'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { toast } from '@/shared/ui/stores'
import { catalogKeys, updateAppMutation } from '../../../data'
import {
	type AppRef,
	appRefOf,
	type ComposeApp,
	type ComposeServiceSummary,
	composeProblem,
	KindMismatch,
	summarizeCompose,
} from '../../../domain'
import { catalogErrorMessage } from '../../components/catalogMessages'
import { guardUnsavedChanges } from '../../components/unsavedGuard'
import { lineCount, loadYamlEditor, loadYamlSyntax } from '../../components/yamlEditor'

/** Un problema que impide guardar, en una línea del YAML. */
export type ComposeLineProblem =
	| { readonly line: number; readonly kind: 'syntax' }
	| { readonly line: number; readonly kind: 'duplicate_service'; readonly service: string }

export type ComposeCheck =
	/** Todavía no se validó, o se descartó. */
	| { readonly kind: 'idle' }
	/** "Validar" o "Guardar" encontraron problemas en esas líneas. */
	| {
			readonly kind: 'problems'
			readonly problems: readonly ComposeLineProblem[]
			readonly onSave: boolean
	  }
	/** El compose no se puede guardar (vacío, más de 256 KiB, un 422 del backend). */
	| { readonly kind: 'rejected'; readonly message: string }

// App·Compose (`App-Compose.dc.html`): el YAML en un editor, "Validar" (sintaxis y
// servicios repetidos, en el navegador) y "Guardar" (PATCH `compose_yaml`). El resto
// (puertos, build, volúmenes, redes, nombres de servicio en el stage) lo valida el backend
// al guardar (contrato §2.12): su motivo llega como `rejected`.
export class ComposeConfigViewModel {
	draft = $state('')
	check = $state.raw<ComposeCheck>({ kind: 'idle' })
	focusRequest = $state.raw<{ line: number; nonce: number } | null>(null)

	/** El componente del editor: CodeMirror se baja recién acá, no con el resto del panel. */
	readonly editor = loadYamlEditor()
	// Si el chunk no baja (sin red, un deploy que lo borró), el YAML se edita en el textarea
	// y se guarda igual, sin la revisión de sintaxis: el backend solo pide que no esté vacío.
	readonly #syntax = loadYamlSyntax().catch(() => null)

	readonly #app: () => ComposeApp
	readonly #client = useQueryClient()
	readonly #update = createMutation(() => updateAppMutation(this.#client))

	constructor(app: () => ComposeApp) {
		this.#app = app
		this.draft = untrack(() => app().composeYaml)
		guardUnsavedChanges(() => this.isDirty)
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
		return this.check.kind === 'problems'
			? this.check.problems.map((problem) => problem.line)
			: []
	}

	setDraft = (value: string): void => {
		this.draft = value
		// Con errores a la vista, se revalida mientras se corrigen (como un formulario).
		if (this.check.kind === 'problems') void this.#recheck(value, this.check.onSave)
		else if (this.check.kind === 'rejected') this.check = { kind: 'idle' }
	}

	validate = async (): Promise<void> => {
		const problems = await this.#problems(this.draft)
		this.check =
			problems.length > 0 ? { kind: 'problems', problems, onSave: false } : { kind: 'idle' }
		if (problems.length > 0) return
		// Sin el chunk no se revisó la sintaxis: decir "válido" sería mentir.
		if ((await this.#syntax) === null) {
			toast.info(
				m.catalog_compose_syntax_unavailable(),
				m.catalog_compose_syntax_unavailable_body(),
			)
		} else {
			toast.success(m.catalog_compose_valid(), m.catalog_compose_valid_body())
		}
	}

	save = async (): Promise<void> => {
		if (this.isSaving) return
		const yaml = this.draft
		const problem = composeProblem(yaml)
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
		const problems = await this.#problems(yaml)
		if (problems.length > 0) {
			this.check = { kind: 'problems', problems, onSave: true }
			return
		}
		this.check = { kind: 'idle' }
		if (yaml === this.#app().composeYaml) {
			toast.info(m.catalog_config_no_changes())
			return
		}
		try {
			await this.#update.mutateAsync({ ref: this.ref, changes: { composeYaml: yaml } })
			toast.success(m.catalog_compose_saved(), m.catalog_config_saved_body())
		} catch (error) {
			// La app es de formulario ahora (otra pestaña): se vuelve a cargar y la pantalla
			// cambia sola de editor. La explicación va en un toast: este editor se desmonta con
			// la recarga y un error en él no se llegaría a leer.
			if (error instanceof KindMismatch) {
				toast.info(m.catalog_error_kind_mismatch())
				void this.#client.invalidateQueries({ queryKey: catalogKeys.app(this.ref) })
				return
			}
			this.check = { kind: 'rejected', message: catalogErrorMessage(error) }
		}
	}

	discard = (): void => {
		this.draft = this.#app().composeYaml
		this.check = { kind: 'idle' }
	}

	goToLine = (line: number): void => {
		this.focusRequest = { line, nonce: (this.focusRequest?.nonce ?? 0) + 1 }
	}

	async #problems(yaml: string): Promise<ComposeLineProblem[]> {
		const checker = await this.#syntax
		const syntax: ComposeLineProblem[] = (checker?.yamlSyntaxErrorLines(yaml) ?? []).map(
			(line) => ({ line, kind: 'syntax' }),
		)
		const duplicates: ComposeLineProblem[] = summarizeCompose(yaml)
			.filter((service) => service.duplicate)
			.map((service) => ({
				line: service.line,
				kind: 'duplicate_service',
				service: service.name,
			}))
		return [...syntax, ...duplicates].sort((a, b) => a.line - b.line)
	}

	async #recheck(yaml: string, onSave: boolean): Promise<void> {
		const problems = await this.#problems(yaml)
		// Mientras tanto se siguió escribiendo: vale la revisión del texto nuevo.
		if (yaml !== this.draft) return
		this.check = problems.length > 0 ? { kind: 'problems', problems, onSave } : { kind: 'idle' }
	}
}
