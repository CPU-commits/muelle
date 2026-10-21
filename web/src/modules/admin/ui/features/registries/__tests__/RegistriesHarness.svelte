<script lang="ts">
	import { type QueryClient, setQueryClientContext } from '@tanstack/svelte-query'
	import { untrack } from 'svelte'
	import { RegistriesViewModel } from '..'

	// Monta el viewModel adentro de un componente real. No renderiza nada.
	const props: { client: QueryClient; onReady: (vm: RegistriesViewModel) => void } = $props()

	untrack(() => {
		setQueryClientContext(props.client)
		props.onReady(new RegistriesViewModel())
	})
</script>
