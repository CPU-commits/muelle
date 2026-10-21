<script lang="ts" generics="T">
	import { type QueryClient, setQueryClientContext } from '@tanstack/svelte-query'
	import { untrack } from 'svelte'

	// Monta un viewModel del catálogo adentro de un componente real, el único lugar donde
	// `useQueryClient`, `createQuery`, `createMutation` y `createForm` pueden leer el
	// contexto. No renderiza nada: el test habla con el viewModel.
	const props: { client: QueryClient; create: () => T; onReady: (vm: T) => void } = $props()

	// `untrack`: las props se leen UNA vez, al montar, y es a propósito.
	untrack(() => {
		setQueryClientContext(props.client)
		props.onReady(props.create())
	})
</script>
