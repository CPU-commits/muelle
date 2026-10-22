<script lang="ts">
	import { resolve } from '$app/paths'
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { EmptyState, ErrorState, LinkButton, LoadingState, PageHeader } from '@/shared/ui'
	import { appRefFrom } from '../../../domain'
	import { ComposeConfig } from '../../features/composeConfig'
	import { ServiceConfig } from '../../features/serviceConfig'
	import { AppConfigViewModel } from './AppConfig.viewModel.svelte.ts'

	// `…/apps/[a]/configuration`: el formulario de una app de servicio
	// (`App-Configuracion.dc.html`) o el editor de una compose (`App-Compose.dc.html`).
	let { project, stage, app }: { project: string; stage: string; app: string } = $props()

	const vm = new AppConfigViewModel(() => appRefFrom(project, stage, app))
</script>

{#if vm.app && vm.summary}
	{#key vm.key}
		{#if vm.app.kind === 'service'}
			<ServiceConfig app={vm.app} summary={vm.summary} />
		{:else}
			<ComposeConfig app={vm.app} summary={vm.summary} />
		{/if}
	{/key}
{:else if vm.isLoading}
	<PageHeader title={m.nav_app_config()} />
	<LoadingState label={m.catalog_app_loading()} />
{:else if vm.isNotFound}
	<PageHeader title={m.nav_app_config()} />
	<EmptyState
		icon="alert"
		title={m.catalog_app_not_found_title()}
		body={m.catalog_app_not_found_body()}
	>
		{#snippet action()}
			<LinkButton href={resolve('/')} variant="secondary" size="sm">
				{m.catalog_back_to_projects()}
			</LinkButton>
		{/snippet}
	</EmptyState>
{:else if vm.loadError}
	<PageHeader title={m.nav_app_config()} />
	<ErrorState
		title={m.catalog_app_load_error()}
		body={vm.loadError}
		retryLabel={m.common_retry()}
		onRetry={vm.retry}
	/>
{/if}
