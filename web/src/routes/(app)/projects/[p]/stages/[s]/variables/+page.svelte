<script lang="ts">
	import { resolve } from '$app/paths'
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { Breadcrumb, ShellHeader } from '@/shared/ui'
	import { StageVariablesScreen } from '@/modules/secrets'
	import type { PageProps } from './$types'

	let { params }: PageProps = $props()
</script>

<!-- El snippet se pinta en el header del AppShell y, al salir de la ruta, puede verse una
     vez con los params de la ruta nueva (sin `p` ni `s`): no los da por hechos, y no hace
     queries (ver "Los snippets de <ShellHeader>" en web/CLAUDE.md). -->
<ShellHeader>
	{#snippet breadcrumb()}
		{@const project = params.p ?? ''}
		{@const stage = params.s ?? ''}
		<Breadcrumb
			label={m.nav_location()}
			homeHref={resolve('/')}
			homeLabel={m.nav_home()}
			items={[
				{ label: project, initial: project.charAt(0), href: resolve('/') },
				{ label: stage },
			]}
		/>
	{/snippet}
</ShellHeader>

<!-- El `key`: al cambiar de stage la pantalla arranca de cero (sin un .env a medio importar). -->
{#key `${params.p}/${params.s}`}
	<StageVariablesScreen project={params.p} stage={params.s} />
{/key}
