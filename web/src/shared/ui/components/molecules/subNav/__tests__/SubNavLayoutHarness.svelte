<script lang="ts">
	// Los estilos globales de la app (box-sizing, márgenes, el anillo de foco): sin ellos las
	// medidas no son las del panel.
	import '@/shared/ui/themes/global.css'
	import type { SectionSpy } from '../sectionSpy.svelte.ts'
	import SubNavLayout from '../SubNavLayout.svelte'

	// Un SubNavLayout del ancho que pida el test, con las secciones de Configuración, debajo
	// de 120 px de "header" (como en el AppShell, la nav no arranca pegada arriba), con 80 px
	// abajo y con un contenido más alto que la pantalla. Con `sections`, el contenido son seis
	// secciones con id (`seccion-0`…) de esos altos, a 20 px una de otra, como las tarjetas de
	// Configuración; sin, un bloque de 3000 px. Con `spy`, la activa es la de `spy.current`
	// (sin ninguna, la primera).
	let {
		width,
		sections,
		spy,
		onselect,
	}: {
		width: number
		sections?: readonly number[]
		spy?: SectionSpy
		onselect?: (id: string) => void
	} = $props()

	const LABELS = ['Imagen', 'Ejecución', 'Healthcheck', 'Volúmenes', 'Recursos', 'Eliminar app']
	const items = $derived(
		LABELS.map((label, index) => {
			const id = `seccion-${index}`
			return {
				href: `#${id}`,
				label,
				active: (spy?.current ?? 'seccion-0') === id,
				...(onselect ? { onselect: () => onselect(id) } : {}),
			}
		}),
	)
</script>

<div style:height="120px"></div>
<div style:width={`${width}px`}>
	<SubNavLayout label="Secciones" {items} {spy}>
		{#if sections}
			<div
				data-testid="contenido"
				style:display="flex"
				style:flex-direction="column"
				style:gap="20px"
			>
				{#each sections as height, index (index)}
					<section id={`seccion-${index}`} style:height={`${height}px`}></section>
				{/each}
			</div>
		{:else}
			<div data-testid="contenido" style:height="3000px"></div>
		{/if}
	</SubNavLayout>
</div>
<!-- El aire de abajo del AppShell: al fondo de la página, la última sección no llega al borde. -->
<div style:height="80px"></div>
