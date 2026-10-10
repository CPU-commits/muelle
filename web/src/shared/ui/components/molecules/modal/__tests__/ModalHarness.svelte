<script lang="ts">
	import '@/shared/ui/themes/global.css'
	import Modal from '../Modal.svelte'

	// Una página con un h1 en <main>, un botón que abre el modal y, adentro, uno que lo
	// cierra y otro que además borra el botón que lo abrió (una fila eliminada).
	let { blocked = false }: { blocked?: boolean } = $props()

	let open = $state(false)
	let showOpener = $state(true)
	let closes = $state(0)

	function close(): void {
		closes += 1
		if (!blocked) open = false
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

<Modal {open} title="Eliminar FOO" description="No se puede deshacer." onClose={close}>
	<input aria-label="adentro" />
	{#snippet footer()}
		<button type="button" onclick={() => (open = false)}>cerrar</button>
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
