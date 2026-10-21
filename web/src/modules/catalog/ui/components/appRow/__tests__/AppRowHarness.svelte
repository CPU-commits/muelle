<script lang="ts">
	import '@/shared/ui/themes/global.css'
	import type { App } from '../../../../domain'
	import type { AppRowDeployFactory } from '../appRowDeploy'
	import AppRow from '../AppRow.svelte'

	// Filas de Proyectos dentro de su `<ul>`. `spacer` deja las filas abajo de la pantalla,
	// para ver que el estado de la Fase 2 se pide recién cuando la fila se ve.
	let {
		apps,
		deploy,
		spacer = 0,
	}: { apps: readonly App[]; deploy?: AppRowDeployFactory; spacer?: number } = $props()
</script>

<div style:height={`${spacer}px`}></div>
<ul>
	{#each apps as app (app.slug)}
		<AppRow {app} href={`#${app.slug}`} {deploy} />
	{/each}
</ul>
