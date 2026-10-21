<script lang="ts">
	import { tick } from 'svelte'
	import '@/shared/ui/themes/global.css'
	import Modal from '../Modal.svelte'

	// Una página con un h1 en <main>, un botón que abre el modal y, adentro, uno que lo
	// cierra y otro que además borra el botón que lo abrió (una fila eliminada).
	// `dismissible={false}`: como el modal que muestra una sola vez la URL de una invitación.
	// `tall`: un contenido más alto que la pantalla de un teléfono.
	let {
		blocked = false,
		dismissible = true,
		tall = false,
	}: { blocked?: boolean; dismissible?: boolean; tall?: boolean } = $props()

	let open = $state(false)
	let showOpener = $state(true)
	let closes = $state(0)

	function close(): void {
		closes += 1
		if (!blocked) open = false
	}

	// Cierra y vuelve a abrir enseguida (otro <dialog>): el `close` tardío del viejo no
	// tiene que cerrar el nuevo.
	async function reopen(): Promise<void> {
		open = false
		await tick()
		open = true
	}
</script>

<main>
	<h1>Variables</h1>
	{#if showOpener}
		<button type="button" onclick={() => (open = true)}>abrir</button>
	{/if}
	<span data-testid="state">{open ? 'abierto' : 'cerrado'}</span>
	<span data-testid="closes">{closes}</span>
</main>

<Modal
	{open}
	{dismissible}
	title="Eliminar FOO"
	description="No se puede deshacer."
	onClose={close}
>
	<input aria-label="adentro" />
	{#if tall}
		{#each { length: 40 }, line (line)}
			<p>Línea {line + 1} de un formulario largo.</p>
		{/each}
	{/if}
	{#snippet footer()}
		<button type="button" onclick={() => (open = false)}>cerrar</button>
		<button type="button" onclick={reopen}>reabrir</button>
		<button
			type="button"
			onclick={() => {
				showOpener = false
				open = false
			}}
		>
			borrar
		</button>
	{/snippet}
</Modal>
