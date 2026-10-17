<script lang="ts">
	import '@/shared/ui/themes/global.css'
	import Menu from '../Menu.svelte'
	import type { MenuEntry } from '../types'

	// Los dos menús que mezclan items con y sin ícono: un selector del breadcrumb ("Proyectos",
	// y los proyectos con la marca solo en el actual) y las opciones de un proyecto ("Nuevo
	// stage" con +, "Renombrar" sin ícono, "Eliminar proyecto" con la papelera).
	const selector: readonly MenuEntry[] = [
		{ kind: 'link', id: 'projects', label: 'Proyectos', href: '#proyectos' },
		{ kind: 'separator', id: 'sep' },
		{ kind: 'link', id: 'p-auditfy', label: 'auditfy', href: '#auditfy', current: true },
		{ kind: 'link', id: 'p-extractor', label: 'extractor', href: '#extractor' },
	]

	const options: readonly MenuEntry[] = [
		{ kind: 'action', id: 'new', label: 'Nuevo stage', icon: 'plus', onSelect: () => {} },
		{ kind: 'action', id: 'rename', label: 'Renombrar', onSelect: () => {} },
		{ kind: 'separator', id: 'sep' },
		{
			kind: 'action',
			id: 'delete',
			label: 'Eliminar proyecto',
			icon: 'trash',
			tone: 'danger',
			onSelect: () => {},
		},
	]

	const plain: readonly MenuEntry[] = [
		{ kind: 'action', id: 'a', label: 'Editar', onSelect: () => {} },
		{ kind: 'action', id: 'b', label: 'Eliminar', onSelect: () => {} },
	]
</script>

<Menu label="Proyecto auditfy, cambiar" items={selector} align="start" />
<Menu label="Opciones de auditfy" items={options} />
<Menu label="Opciones de FOO" items={plain} />
