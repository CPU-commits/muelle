<script lang="ts">
	import { type QueryClient, setQueryClientContext } from '@tanstack/svelte-query'
	import { untrack } from 'svelte'
	import { type SettingsSection, SettingsScreen } from '..'

	// Renderiza la pantalla de verdad (markup incluido) con el QueryClient del test.
	const props: { client: QueryClient; section?: SettingsSection } = $props()

	untrack(() => setQueryClientContext(props.client))
</script>

<SettingsScreen section={props.section ?? 'proxy'} />
