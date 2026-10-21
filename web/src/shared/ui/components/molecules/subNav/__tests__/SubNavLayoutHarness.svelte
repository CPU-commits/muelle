<script lang="ts">
	// Los estilos globales de la app (box-sizing, márgenes, el anillo de foco): sin ellos las
	// medidas no son las del panel.
	import '@/shared/ui/themes/global.css'
	import SubNavLayout from '../SubNavLayout.svelte'

	// Un SubNavLayout del ancho que pida el test, con las secciones de Configuración, debajo
	// de 120 px de "header" (como en el AppShell, la nav no arranca pegada arriba) y con un
	// contenido más alto que la pantalla.
	let { width }: { width: number } = $props()

	const items = [
		'Imagen',
		'Ejecución',
		'Healthcheck',
		'Volúmenes',
		'Recursos',
		'Eliminar app',
	].map((label, index) => ({ href: `#seccion-${index}`, label, active: index === 0 }))
</script>

<div style:height="120px"></div>
<div style:width={`${width}px`}>
	<SubNavLayout label="Secciones" {items}>
		<div data-testid="contenido" style:height="3000px"></div>
	</SubNavLayout>
</div>
