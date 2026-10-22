<script lang="ts">
	import { focusOnRequest, refocusOnClose } from '../refocus'

	// Lo de la pantalla de variables: el botón que abre el panel de importar (con
	// `refocusOnClose`), el panel mientras está abierto y el título de la lista (con
	// `focusOnRequest`), adentro de un `<main>` con su h1 (el respaldo del Modal).
	let {
		closed = 0,
		panelOpen = false,
		seq = 0,
		done = () => {},
	}: {
		closed?: number
		panelOpen?: boolean
		seq?: number
		done?: (seq: number) => void
	} = $props()
</script>

<main>
	<h1 tabindex="-1">Variables</h1>
	<span {@attach refocusOnClose(() => closed)}>
		<button type="button">Importar .env</button>
	</span>
	{#if panelOpen}
		<div><button type="button">Cancelar</button></div>
	{/if}
	<h2 tabindex="-1" {@attach focusOnRequest(() => seq, done)}>De la app</h2>
	<button type="button">Otra cosa</button>
</main>
