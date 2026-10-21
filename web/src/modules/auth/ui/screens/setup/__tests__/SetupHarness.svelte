<script lang="ts">
	import { type QueryClient, setQueryClientContext } from '@tanstack/svelte-query'
	import { untrack } from 'svelte'
	import { SetupViewModel } from '..'

	// Monta el viewModel en un componente real (createMutation lee el contexto). No
	// renderiza nada: el test habla con el viewModel.
	const props: {
		client: QueryClient
		token: string | null
		onReady: (vm: SetupViewModel) => void
	} = $props()

	untrack(() => {
		setQueryClientContext(props.client)
		props.onReady(new SetupViewModel(() => props.token))
	})
</script>
