<script lang="ts">
	import { resolve } from '$app/paths'
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { Breadcrumb } from '@/shared/ui'
	import { LocationBreadcrumbViewModel } from './LocationBreadcrumb.viewModel.svelte.ts'

	// "logo / proyecto ⇅ / stage ⇅ / app ⇅" del header de una app, con los selectores del
	// diseño. Va en el `breadcrumb` de <ShellHeader> del layout `apps/[a]`.
	let { project, stage, app }: { project: string; stage: string; app: string } = $props()

	const vm = new LocationBreadcrumbViewModel(() => ({ project, stage, app }))
</script>

{#if vm.ready}
	<Breadcrumb
		label={m.nav_location()}
		homeHref={resolve('/')}
		homeLabel={m.nav_home()}
		items={vm.items}
	/>
{/if}
