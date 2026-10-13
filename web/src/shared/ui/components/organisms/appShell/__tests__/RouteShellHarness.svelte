<script lang="ts">
	import '@/shared/ui/themes/global.css'
	import AppShell from '../AppShell.svelte'
	import ShellHeader from '../ShellHeader.svelte'

	// Como las rutas de muelle: el layout de una app (breadcrumb y tabs con `p` y `a`) y
	// la página de variables del stage (breadcrumb con `p`), cuyos snippets leen los
	// `params` que SvelteKit tipa como `string`. Al navegar, SvelteKit cambia los params y
	// monta/desmonta las rutas en el mismo cambio: acá, `params` y `route` en un solo
	// `rerender`. `broken` es un snippet con un bug de verdad (falla con la ruta montada).
	let {
		params,
		route,
		broken = false,
	}: {
		params: { p: string; a: string }
		route: 'app' | 'stage' | 'home'
		broken?: boolean
	} = $props()

	function fail(): string {
		throw new Error('bug en el breadcrumb')
	}
</script>

<AppShell
	homeHref="/"
	appName="muelle"
	username="felipe"
	accountLabel="Cuenta: felipe"
	accountItems={[]}
	skipLabel="Saltar al contenido"
>
	{#snippet nav()}
		<nav aria-label="Secciones">Proyectos · Ajustes</nav>
	{/snippet}

	{#if route === 'app'}
		<ShellHeader>
			{#snippet breadcrumb()}
				<nav aria-label="Ubicación">
					{broken ? fail() : `${params.p.charAt(0)} / ${params.a.toUpperCase()}`}
				</nav>
			{/snippet}
			{#snippet nav()}
				<nav aria-label="Secciones de la app">Resumen de {params.a.toUpperCase()}</nav>
			{/snippet}
		</ShellHeader>
	{:else if route === 'stage'}
		<ShellHeader>
			{#snippet breadcrumb()}
				<nav aria-label="Ubicación">stage de {params.p.toUpperCase()}</nav>
			{/snippet}
		</ShellHeader>
	{/if}

	<p>contenido</p>
</AppShell>
