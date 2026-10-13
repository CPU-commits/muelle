<script lang="ts">
	import '@/shared/ui/themes/global.css'
	import ConfirmDialog from '../../confirmDialog/ConfirmDialog.svelte'
	import Menu from '../Menu.svelte'
	import type { MenuEntry } from '../types'

	// Las "Opciones" (⋯) de una fila con un "Eliminar" que pide confirmación, como en
	// Proyectos o Variables.
	let confirming = $state(false)

	const items: readonly MenuEntry[] = [
		{ kind: 'action', id: 'renombrar', label: 'Renombrar', onSelect: () => {} },
		{
			kind: 'action',
			id: 'eliminar',
			label: 'Eliminar',
			tone: 'danger',
			onSelect: () => (confirming = true),
		},
	]
</script>

<main>
	<h1>Proyectos</h1>
	<Menu label="Opciones de auditfy" {items} />
</main>

<ConfirmDialog
	open={confirming}
	title="Eliminar auditfy"
	body="No se puede deshacer."
	confirmLabel="Eliminar"
	cancelLabel="Cancelar"
	onConfirm={() => (confirming = false)}
	onCancel={() => (confirming = false)}
/>
