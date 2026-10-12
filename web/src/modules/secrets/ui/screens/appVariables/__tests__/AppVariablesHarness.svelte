<script lang="ts">
	import { type QueryClient, setQueryClientContext } from '@tanstack/svelte-query'
	import { untrack } from 'svelte'
	import { type AppVariablesParams, AppVariablesViewModel } from '..'

	// Monta el viewModel adentro de un componente real (createQuery lee el contexto). No
	// renderiza nada: el test habla con el viewModel.
	const props: {
		client: QueryClient
		params: AppVariablesParams
		onReady: (vm: AppVariablesViewModel) => void
	} = $props()

	untrack(() => {
		setQueryClientContext(props.client)
		props.onReady(new AppVariablesViewModel(() => props.params))
	})
</script>
