<script lang="ts">
	import { type QueryClient, setQueryClientContext } from '@tanstack/svelte-query'
	import { untrack } from 'svelte'
	import { CreateTokenModalViewModel } from '..'
	import type { CreatedApiToken } from '../../../../domain'

	// Monta el viewModel adentro de un componente real. No renderiza nada.
	const props: {
		client: QueryClient
		onCreated: (created: CreatedApiToken) => void
		onReady: (vm: CreateTokenModalViewModel) => void
	} = $props()

	untrack(() => {
		setQueryClientContext(props.client)
		props.onReady(new CreateTokenModalViewModel({ onCreated: props.onCreated }))
	})
</script>
