import { EditorState } from '@codemirror/state'
import { EditorView } from '@codemirror/view'
import { untrack } from 'svelte'
import type { ShadowMount } from '@/shared/ui'
import { editorExtensions, setErrorLines } from './editorExtensions'

/** Un pedido de llevar el cursor a una línea: cada uno trae un `nonce` nuevo. */
export interface FocusRequest {
	line: number
	nonce: number
}

/** Las props del editor, como getters (web/CLAUDE.md, "Props reactivas"). */
export interface YamlEditorInput {
	value: () => string
	label: () => string
	description: () => string | undefined
	errorLines: () => readonly number[]
	focusRequest: () => FocusRequest | null
	onChange: () => (value: string) => void
}

/**
 * El estado del editor de YAML (CodeMirror 6). Es controlado: `value` entra y `onChange`
 * sale con cada cambio; si `value` cambia desde afuera (descartar, otra app), el documento
 * se reemplaza. Se carga con el chunk del editor (`loadYamlEditor`), nunca antes.
 */
export class YamlEditorViewModel {
	readonly #input: YamlEditorInput
	#view = $state.raw<EditorView>()

	constructor(input: YamlEditorInput) {
		this.#input = input

		$effect(() => {
			const view = this.#view
			const value = this.#input.value()
			if (!view || view.state.doc.toString() === value) return
			view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: value } })
		})

		$effect(() => {
			const lines = this.#input.errorLines()
			this.#view?.dispatch({ effects: setErrorLines.of(lines) })
		})

		$effect(() => {
			const view = this.#view
			const request = this.#input.focusRequest()
			if (!view || !request) return
			const doc = view.state.doc
			const line = doc.line(Math.min(Math.max(request.line, 1), doc.lines))
			view.dispatch({ selection: { anchor: line.from }, scrollIntoView: true })
			view.focus()
		})
	}

	/**
	 * Para `ShadowHost`: crea el editor adentro del shadow root. Es estable (un campo del
	 * viewModel) y lee las props sin seguimiento: si no, cada tecla lo volvería a montar.
	 * `root` es obligatorio: sin él, CodeMirror pone su `<style>` en el documento y la CSP
	 * lo bloquea.
	 */
	readonly mount: ShadowMount = (container, root) => {
		const view = untrack(
			() =>
				new EditorView({
					parent: container,
					root,
					state: EditorState.create({
						doc: this.#input.value(),
						extensions: editorExtensions({
							label: this.#input.label(),
							...this.#description(),
							onChange: (next) => this.#input.onChange()(next),
						}),
					}),
				}),
		)
		this.#view = view
		return () => {
			view.destroy()
			if (this.#view === view) this.#view = undefined
		}
	}

	#description(): { description?: string } {
		const description = this.#input.description()
		return description ? { description } : {}
	}
}
