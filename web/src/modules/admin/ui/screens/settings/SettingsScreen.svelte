<script lang="ts">
	import { UsersSection } from '@/modules/identity'
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { ComingSoon, ErrorState, LoadingState } from '@/shared/ui'
	import { AuditLog } from '../../features/auditLog'
	import { ProxySettings } from '../../features/proxySettings'
	import { Registries } from '../../features/registries'
	import { SecuritySettings } from '../../features/securitySettings'
	import { SETTINGS_SECTIONS } from '../../features/settingsLayout'
	import { SettingsViewModel } from './Settings.viewModel.svelte.ts'
	import { screen } from './styles.css'

	// Las secciones de Ajustes, en el orden del diseño. El marco (h1 y nav) es
	// <SettingsLayout>, que pone la ruta.
	const vm = new SettingsViewModel()
</script>

<div class={screen}>
	{#if vm.isLoading}
		<LoadingState label={m.common_loading()} />
	{:else if vm.loadError || !vm.settings}
		<ErrorState
			title={vm.loadError ?? m.admin_settings_load_error()}
			retryLabel={m.common_retry()}
			onRetry={vm.retry}
		/>
	{:else}
		{#if vm.settings.proxy}
			<ProxySettings id={SETTINGS_SECTIONS.proxy} proxy={vm.settings.proxy} />
		{:else}
			<div id={SETTINGS_SECTIONS.proxy}>
				<ComingSoon phase={vm.proxyPhase} feature={m.admin_proxy_feature()} />
			</div>
		{/if}
		<SecuritySettings id={SETTINGS_SECTIONS.security} settings={vm.settings} />
	{/if}

	<Registries id={SETTINGS_SECTIONS.registries} />
	<UsersSection id={SETTINGS_SECTIONS.users} />
	<AuditLog id={SETTINGS_SECTIONS.audit} />
</div>
