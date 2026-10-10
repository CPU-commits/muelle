<script lang="ts">
	// Los estilos globales de la app (box-sizing, márgenes): sin ellos el layout no es el real.
	import '@/shared/ui/themes/global.css'
	import List from '../../list/List.svelte'
	import ListRow from '../../list/ListRow.svelte'
	import Table from '../../table/Table.svelte'
	import Menu from '../Menu.svelte'
	import type { MenuEntry } from '../types'

	// Menús dentro de contenedores con `overflow` (List, Table, Card): el panel no se tiene
	// que recortar. `atBottom` pone un menú pegado al borde de abajo para probar el flip.
	let { onSelect, atBottom = false }: { onSelect: (id: string) => void; atBottom?: boolean } =
		$props()

	const items: readonly MenuEntry[] = [
		{ kind: 'action', id: 'editar', label: 'Editar', onSelect: () => onSelect('editar') },
		{ kind: 'link', id: 'ver', label: 'Ver', href: '#ver' },
		{ kind: 'separator', id: 'sep' },
		{
			kind: 'action',
			id: 'eliminar',
			label: 'Eliminar',
			tone: 'danger',
			onSelect: () => onSelect('eliminar'),
		},
	]
</script>

<div>
	<List label="Variables">
		<ListRow>FOO <Menu label="Opciones de FOO" {items} /></ListRow>
	</List>
	<Table caption="Tareas">
		<tbody>
			<tr>
				<td>api.1</td>
				<td><Menu label="Opciones de api.1" {items} /></td>
			</tr>
		</tbody>
	</Table>
	{#if atBottom}
		<div style:position="fixed" style:bottom="4px" style:left="4px">
			<Menu label="Opciones abajo" {items} align="end" />
		</div>
	{/if}
</div>
