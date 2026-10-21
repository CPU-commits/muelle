<script lang="ts">
	import { type QueryClient, setQueryClientContext } from '@tanstack/svelte-query'
	import { untrack } from 'svelte'
	import { type LoginStep, LoginViewModel } from '..'

	// Monta el viewModel adentro de un componente real, que es el único lugar donde
	// `useQueryClient` y `createMutation` pueden leer el contexto. No renderiza nada: el
	// test habla con el viewModel, no con el markup.
	const props: {
		client: QueryClient
		initialStep?: LoginStep
		onReady: (vm: LoginViewModel) => void
	} = $props()

	// `untrack`: las props se leen UNA vez, al montar, y es a propósito.
	untrack(() => {
		setQueryClientContext(props.client)
		props.onReady(new LoginViewModel(() => props.initialStep ?? 'password'))
	})
</script>
