<script lang="ts">
	import { type QueryClient, setQueryClientContext } from '@tanstack/svelte-query'
	import { untrack } from 'svelte'
	import { SecuritySettingsViewModel } from '..'
	import type { Settings } from '../../../../domain'

	// Monta el viewModel adentro de un componente real. `settings` es un getter: el test
	// lo cambia para simular lo que deja el cache después de guardar.
	const props: {
		client: QueryClient
		settings: () => Settings
		onReady: (vm: SecuritySettingsViewModel) => void
	} = $props()

	untrack(() => {
		setQueryClientContext(props.client)
		props.onReady(new SecuritySettingsViewModel(props.settings))
	})
</script>
