<script lang="ts">
	import { type QueryClient, setQueryClientContext } from '@tanstack/svelte-query'
	import { untrack } from 'svelte'
	import { TokensViewModel } from '..'

	// Monta el viewModel adentro de un componente real. No renderiza nada. `origin` es el
	// del panel (en la app, el de `page.url`).
	const props: {
		client: QueryClient
		origin?: string
		onReady: (vm: TokensViewModel) => void
	} = $props()

	untrack(() => {
		setQueryClientContext(props.client)
		const origin = props.origin ?? 'https://panel.example'
		props.onReady(new TokensViewModel(() => origin))
	})
</script>
