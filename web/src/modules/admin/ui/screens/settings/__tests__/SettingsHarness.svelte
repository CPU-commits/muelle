<script lang="ts">
	import { type QueryClient, setQueryClientContext } from '@tanstack/svelte-query'
	import { untrack } from 'svelte'
	import { type SettingsSection, SettingsViewModel } from '..'

	// Monta el viewModel adentro de un componente real. No renderiza nada.
	const props: {
		client: QueryClient
		section?: SettingsSection
		panelMoved?: boolean
		onReady: (vm: SettingsViewModel) => void
	} = $props()

	untrack(() => {
		setQueryClientContext(props.client)
		props.onReady(
			new SettingsViewModel(() => ({
				section: props.section ?? 'proxy',
				panelMoved: props.panelMoved ?? false,
			})),
		)
	})
</script>
