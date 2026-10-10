<script lang="ts">
	import { resolve } from '$app/paths'
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { Breadcrumb, EmptyState, PageHeader, ShellHeader } from '@/shared/ui'
	import type { PageProps } from './$types'

	let { params }: PageProps = $props()
</script>

<!-- Placeholder de F0: lo reemplaza F1-secrets con las variables del stage. -->
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

<PageHeader title={m.nav_stage_variables()} />
<EmptyState icon="clock" title={m.common_wip_title()} body={m.common_wip_body()} />
