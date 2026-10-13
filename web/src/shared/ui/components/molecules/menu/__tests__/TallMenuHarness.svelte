<script lang="ts">
	import '@/shared/ui/themes/global.css'
	import Menu from '../Menu.svelte'
	import type { MenuEntry } from '../types'

	// Un menú largo (20 items: los proyectos de un selector del breadcrumb) con el botón a
	// la altura que pida el test, para ver que el panel no se va de la pantalla.
	let { top, onSelect }: { top: number; onSelect: (id: string) => void } = $props()

	const items: readonly MenuEntry[] = Array.from({ length: 20 }, (_, index) => ({
		kind: 'action',
		id: `proyecto-${index + 1}`,
		label: `proyecto-${index + 1}`,
		onSelect: () => onSelect(`proyecto-${index + 1}`),
	}))
</script>

<div style:height={`${top}px`}></div>
<Menu label="Proyectos" {items} align="start" />
