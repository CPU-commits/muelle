<script lang="ts">
	import { resolve } from '$app/paths'
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { Breadcrumb } from '@/shared/ui'
	import { LocationBreadcrumbViewModel } from './LocationBreadcrumb.viewModel.svelte.ts'

	// "logo / proyecto ⇅ / stage ⇅" de las variables de un stage (la página es de
	// `secrets`): cambiar de stage o de proyecto lleva a las variables del otro.
	let { project, stage }: { project: string; stage: string } = $props()

	const vm = new LocationBreadcrumbViewModel(() => ({ project, stage, app: null }))
</script>

{#if vm.ready}
	<Breadcrumb
		label={m.nav_location()}
		homeHref={resolve('/')}
		homeLabel={m.nav_home()}
		items={vm.items}
	/>
{/if}
