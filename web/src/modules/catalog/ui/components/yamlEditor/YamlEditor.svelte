<script lang="ts">
	import { EditorState } from '@codemirror/state'
	import { EditorView } from '@codemirror/view'
	import { untrack } from 'svelte'
	import type { Attachment } from 'svelte/attachments'
	import { editorExtensions, setErrorLines } from './editorExtensions'
	import { root } from './styles.css'

	// El editor de YAML de App·Compose (CodeMirror 6). Es controlado: `value` entra,
	// `onChange` sale con cada cambio; si `value` cambia desde afuera (descartar, otra
	// app), el documento se reemplaza.
	type Props = {
		value: string
		/** El nombre accesible del área de edición. */
		label: string
		/** El id del texto de ayuda (cómo salir del editor con el teclado). */
		describedBy?: string
		/** Las líneas (desde 1) que se marcan con error. */
		errorLines?: readonly number[]
		/** Llevar el cursor a una línea: cada pedido trae un `nonce` nuevo. */
		focusRequest?: { line: number; nonce: number } | null
		onChange: (value: string) => void
	}

	let {
		value,
		label,
		describedBy,
		errorLines = [],
		focusRequest = null,
		onChange,
	}: Props = $props()

	let view = $state.raw<EditorView>()

	// Se crea una sola vez: lo que se lee acá va sin seguimiento, o cada tecla
	// recrearía el editor.
	const mount: Attachment<HTMLDivElement> = (element) => {
		const created = untrack(
			() =>
				new EditorView({
					parent: element,
					state: EditorState.create({
						doc: value,
						extensions: editorExtensions({
							label,
							...(describedBy ? { describedBy } : {}),
							onChange: (next) => onChange(next),
						}),
					}),
				}),
		)
		view = created
		return () => {
			created.destroy()
			view = undefined
		}
	}

	$effect(() => {
		const current = view
		if (!current || current.state.doc.toString() === value) return
		current.dispatch({ changes: { from: 0, to: current.state.doc.length, insert: value } })
	})

	$effect(() => {
		view?.dispatch({ effects: setErrorLines.of(errorLines) })
	})

	$effect(() => {
		const current = view
		if (!current || !focusRequest) return
		const doc = current.state.doc
		const line = doc.line(Math.min(Math.max(focusRequest.line, 1), doc.lines))
		current.dispatch({ selection: { anchor: line.from }, scrollIntoView: true })
		current.focus()
	})
</script>

<div class={root} {@attach mount}></div>
