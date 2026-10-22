<script lang="ts">
	import { resolve } from '$app/paths'
	import { page } from '$app/state'
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { AppBreadcrumb } from '@/modules/catalog'
	import { ShellHeader, Tabs } from '@/shared/ui'
	import type { LayoutProps } from './$types'

	// El detalle de una app: el header del AppShell cambia el logo por "proyecto / stage
	// / app" y la nav por los tabs de la app. Lo arma F0 con los parámetros de la ruta;
	// F1-catalog cambia los items por selectores (`kind: 'menu'`). El layout solo llena
	// el ShellHeader: el encabezado de `main` (h1, kind, imagen, acciones) es de cada
	// página (ver "Quién dibuja el header de una app" en web/CLAUDE.md).
	let { params, children }: LayoutProps = $props()
</script>

<ShellHeader>
	{#snippet breadcrumb()}
		<AppBreadcrumb project={params.p} stage={params.s} app={params.a} />
	{/snippet}

	{#snippet nav()}
		<!-- Al salir de la app el AppShell vuelve a dibujar esto con los params ya vacíos. -->
		{#if params.p && params.s && params.a}
			{@const path = page.url.pathname}
			{@const base = resolve('/(app)/projects/[p]/stages/[s]/apps/[a]', params)}
			{@const config = resolve(
				'/(app)/projects/[p]/stages/[s]/apps/[a]/configuration',
				params,
			)}
			{@const variables = resolve(
				'/(app)/projects/[p]/stages/[s]/apps/[a]/variables',
				params,
			)}
			{@const domains = resolve('/(app)/projects/[p]/stages/[s]/apps/[a]/domains', params)}
			{@const deploys = resolve('/(app)/projects/[p]/stages/[s]/apps/[a]/deploys', params)}
			{@const logs = resolve('/(app)/projects/[p]/stages/[s]/apps/[a]/logs', params)}
			<Tabs
				label={m.nav_app_sections()}
				items={[
					{ href: base, label: m.nav_app_summary(), active: path === base },
					{ href: config, label: m.nav_app_config(), active: path.startsWith(config) },
					{
						href: variables,
						label: m.nav_app_variables(),
						active: path.startsWith(variables),
					},
					{ href: domains, label: m.nav_app_domains(), active: path.startsWith(domains) },
					{ href: deploys, label: m.nav_app_deploys(), active: path.startsWith(deploys) },
					{ href: logs, label: m.nav_app_logs(), active: path.startsWith(logs) },
				]}
			/>
		{/if}
	{/snippet}
</ShellHeader>

{@render children()}
