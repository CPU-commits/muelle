<script lang="ts" generics="T">
	import { type QueryClient, setQueryClientContext } from '@tanstack/svelte-query'
	import { untrack } from 'svelte'

	// Monta cualquier viewModel del módulo adentro de un componente real: el único lugar
	// donde `useQueryClient`, `createQuery`, `createMutation` y `$effect` funcionan. No
	// renderiza nada: el test habla con el viewModel, no con el markup.
	const props: {
		client: QueryClient
		create: () => T
		onReady: (vm: T) => void
	} = $props()

	// `untrack`: las props se leen UNA vez, al montar, y es a propósito.
	untrack(() => {
		setQueryClientContext(props.client)
		props.onReady(props.create())
	})
</script>
