<script lang="ts">
	import { ShadowHost } from '@/shared/ui'
	import { root } from './styles.css'
	import { type FocusRequest, YamlEditorViewModel } from './YamlEditor.viewModel.svelte.ts'

	// El editor de YAML de App·Compose (CodeMirror 6), adentro de un ShadowHost: con
	// `style-src 'self'` la CSP bloquea el `<style>` que CodeMirror mete en el documento; en
	// un shadow root usa hojas construidas, que sí se permiten (web/CLAUDE.md, CSP).
	type Props = {
		value: string
		/** El nombre accesible del área de edición. */
		label: string
		/** El texto de ayuda (cómo salir del editor con el teclado). */
		description?: string
		/** Las líneas (desde 1) que se marcan con error. */
		errorLines?: readonly number[]
		/** Llevar el cursor a una línea: cada pedido trae un `nonce` nuevo. */
		focusRequest?: FocusRequest | null
		onChange: (value: string) => void
	}

	let {
		value,
		label,
		description,
		errorLines = [],
		focusRequest = null,
		onChange,
	}: Props = $props()

	const vm = new YamlEditorViewModel({
		value: () => value,
		label: () => label,
		description: () => description,
		errorLines: () => errorLines,
		focusRequest: () => focusRequest,
		onChange: () => onChange,
	})
</script>

<ShadowHost class={root} mount={vm.mount} />
