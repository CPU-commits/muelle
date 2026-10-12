<script lang="ts">
	import { resolve } from '$app/paths'
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { Breadcrumb } from '@/shared/ui'

	// "logo / proyecto / stage / app" del header de una app. Va en el `breadcrumb` de
	// <ShellHeader>. Cuando `Breadcrumb` tenga crumbs de menú, acá van los selectores de
	// proyecto, stage y app (`switchProjectTarget` / `switchStageTarget` del dominio).
	let { project, stage, app }: { project: string; stage: string; app: string } = $props()

	// El AppShell dibuja este snippet: al salir de la app, los params de la ruta se vacían
	// un instante antes de que el layout se desmonte, y `resolve` no acepta params vacíos.
	const ready = $derived(Boolean(project && stage && app))
</script>

{#if ready}
	<Breadcrumb
		label={m.nav_location()}
		homeHref={resolve('/')}
		homeLabel={m.nav_home()}
		items={[
			{ label: project, initial: project.charAt(0), href: `${resolve('/')}#p-${project}` },
			{
				label: stage,
				href: resolve('/(app)/projects/[p]/stages/[s]/variables', { p: project, s: stage }),
			},
			{
				label: app,
				href: resolve('/(app)/projects/[p]/stages/[s]/apps/[a]', {
					p: project,
					s: stage,
					a: app,
				}),
			},
		]}
	/>
{/if}
