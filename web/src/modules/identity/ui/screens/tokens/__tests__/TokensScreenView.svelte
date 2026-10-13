<script lang="ts">
	import { type QueryClient, setQueryClientContext } from '@tanstack/svelte-query'
	import { untrack } from 'svelte'
	import { TokensScreen } from '..'

	// Renderiza la pantalla de verdad (markup incluido) con el QueryClient del test.
	const props: { client: QueryClient } = $props()

	untrack(() => setQueryClientContext(props.client))
</script>

<TokensScreen />
