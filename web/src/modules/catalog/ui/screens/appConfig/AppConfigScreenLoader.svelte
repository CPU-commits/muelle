<script lang="ts">
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { ErrorState, LoadingState, PageHeader } from '@/shared/ui'

	// Lo que exporta el barrel como `AppConfigScreen`. El barrel lo importan Proyectos y el
	// layout de una app: la pantalla de verdad (formularios y editor) viaja en su propio
	// chunk y se baja recién al abrir Configuración.
	let props: { project: string; stage: string; app: string } = $props()
</script>

{#await import('./AppConfigScreen.svelte')}
	<PageHeader title={m.nav_app_config()} />
	<LoadingState label={m.catalog_app_loading()} />
{:then { default: AppConfigScreen }}
	<AppConfigScreen {...props} />
{:catch}
	<PageHeader title={m.nav_app_config()} />
	<ErrorState title={m.catalog_app_load_error()} body={m.common_unavailable()} />
{/await}
