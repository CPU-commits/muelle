<script lang="ts">
	import { type QueryClient, setQueryClientContext } from '@tanstack/svelte-query'
	import { untrack } from 'svelte'
	import { UsersSectionViewModel } from '..'

	// Monta el viewModel adentro de un componente real, el único lugar donde
	// `createQuery` y `createMutation` pueden leer el contexto. No renderiza nada.
	const props: { client: QueryClient; onReady: (vm: UsersSectionViewModel) => void } = $props()

	untrack(() => {
		setQueryClientContext(props.client)
		props.onReady(new UsersSectionViewModel())
	})
</script>
