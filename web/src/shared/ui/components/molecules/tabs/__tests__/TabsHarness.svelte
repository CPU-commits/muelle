<script lang="ts">
	// Los estilos globales de la app (box-sizing, márgenes): sin ellos las medidas no son las
	// del panel.
	import '@/shared/ui/themes/global.css'
	import type { NavItem } from '../types'
	import Tabs from '../Tabs.svelte'

	// Las Tabs del header de una app en una caja del ancho que pida el test.
	let { width, items }: { width: number; items: readonly NavItem[] } = $props()
</script>

<div style:width={`${width}px`}>
	<Tabs label="Secciones de la app" {items} />
</div>
