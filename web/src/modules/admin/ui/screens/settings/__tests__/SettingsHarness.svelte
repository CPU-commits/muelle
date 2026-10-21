<script lang="ts">
	import { type QueryClient, setQueryClientContext } from '@tanstack/svelte-query'
	import { untrack } from 'svelte'
	import { SettingsViewModel } from '..'

	// Monta el viewModel adentro de un componente real. No renderiza nada.
	const props: {
		client: QueryClient
		panelMoved?: boolean
		onReady: (vm: SettingsViewModel) => void
	} = $props()

	untrack(() => {
		setQueryClientContext(props.client)
		props.onReady(new SettingsViewModel(() => ({ panelMoved: props.panelMoved ?? false })))
	})
</script>
