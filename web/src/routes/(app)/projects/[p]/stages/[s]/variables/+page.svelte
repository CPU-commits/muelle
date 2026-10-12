<script lang="ts">
	import { resolve } from '$app/paths'
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { Breadcrumb, ShellHeader } from '@/shared/ui'
	import { StageVariablesScreen } from '@/modules/secrets'
	import type { PageProps } from './$types'

	let { params }: PageProps = $props()
</script>

<ShellHeader>
	{#snippet breadcrumb()}
		<Breadcrumb
			label={m.nav_location()}
			homeHref={resolve('/')}
			homeLabel={m.nav_home()}
			items={[
				{ label: params.p, initial: params.p.charAt(0), href: resolve('/') },
				{ label: params.s },
			]}
		/>
	{/snippet}
</ShellHeader>

<!-- El `key`: al cambiar de stage la pantalla arranca de cero (sin un .env a medio importar). -->
{#key `${params.p}/${params.s}`}
	<StageVariablesScreen project={params.p} stage={params.s} />
{/key}
