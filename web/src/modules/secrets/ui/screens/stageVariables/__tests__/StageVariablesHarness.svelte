<script lang="ts">
	import { type QueryClient, setQueryClientContext } from '@tanstack/svelte-query'
	import { untrack } from 'svelte'
	import { type StageVariablesParams, StageVariablesViewModel } from '..'

	// Monta el viewModel adentro de un componente real (createQuery lee el contexto). No
	// renderiza nada: el test habla con el viewModel.
	const props: {
		client: QueryClient
		params: StageVariablesParams
		onReady: (vm: StageVariablesViewModel) => void
	} = $props()

	untrack(() => {
		setQueryClientContext(props.client)
		props.onReady(new StageVariablesViewModel(() => props.params))
	})
</script>
