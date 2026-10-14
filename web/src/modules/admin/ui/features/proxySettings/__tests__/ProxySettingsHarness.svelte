<script lang="ts">
	import { type QueryClient, setQueryClientContext } from '@tanstack/svelte-query'
	import { untrack } from 'svelte'
	import { ProxySettingsViewModel } from '..'
	import type { ProxySettings } from '../../../../domain'

	// Monta el viewModel adentro de un componente real. No renderiza nada.
	const props: {
		client: QueryClient
		proxy: () => ProxySettings
		onReady: (vm: ProxySettingsViewModel) => void
	} = $props()

	untrack(() => {
		setQueryClientContext(props.client)
		props.onReady(new ProxySettingsViewModel(props.proxy))
	})
</script>
