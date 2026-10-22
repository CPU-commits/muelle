<script lang="ts">
	import { UsersSection } from '@/modules/identity'
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { ErrorState, LoadingState } from '@/shared/ui'
	import { AuditLog } from '../../features/auditLog'
	import { ProxySettings } from '../../features/proxySettings'
	import { Registries } from '../../features/registries'
	import { SecuritySettings } from '../../features/securitySettings'
	import { type SettingsProps, SettingsViewModel } from './Settings.viewModel.svelte.ts'
	import { screen } from './styles.css'

	// Una sección de Ajustes, la de la ruta. El marco (h1 y nav) es <SettingsLayout>.
	let props: SettingsProps = $props()

	const vm = new SettingsViewModel(() => props)
</script>

{#snippet settingsCards(render: 'proxy' | 'security')}
	{#if vm.isLoading}
		<LoadingState label={m.common_loading()} />
	{:else if vm.loadError || !vm.settings}
		<ErrorState
			title={vm.loadError ?? m.admin_settings_load_error()}
			retryLabel={m.common_retry()}
			onRetry={vm.retry}
		/>
	{:else if render === 'proxy'}
		<ProxySettings proxy={vm.settings.proxy} />
	{:else}
		<SecuritySettings settings={vm.settings} />
	{/if}
{/snippet}

<div class={screen}>
	{#if vm.section === 'proxy' || vm.section === 'security'}
		{@render settingsCards(vm.section)}
	{:else if vm.section === 'registries'}
		<Registries />
	{:else if vm.section === 'users'}
		<UsersSection />
	{:else}
		<AuditLog />
	{/if}
</div>
