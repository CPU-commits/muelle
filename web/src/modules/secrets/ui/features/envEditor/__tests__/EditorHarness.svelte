<script lang="ts">
	import { type QueryClient, setQueryClientContext } from '@tanstack/svelte-query'
	import { createQueryClient } from '@/shared/data/query'
	import { untrack } from 'svelte'
	import type { EnvOwner } from '../../../../domain'
	import { EnvEditorViewModel } from '..'

	// Monta el viewModel adentro de un componente real, que es el único lugar donde
	// `useQueryClient`, `createQuery` y `createMutation` pueden leer el contexto. No
	// renderiza nada: el test habla con el viewModel, no con el markup.
	const props: {
		owner: EnvOwner
		overridden?: ReadonlySet<string>
		/** El cache, si el test lo quiere mirar; si no, uno nuevo. */
		client?: QueryClient
		onReady: (vm: EnvEditorViewModel) => void
	} = $props()

	// `untrack`: las props se leen UNA vez, al montar, y es a propósito.
	untrack(() => {
		// Las mismas opciones que la app, sin reintentos.
		setQueryClientContext(props.client ?? createQueryClient({ retry: false }))
		const overridden = props.overridden
		props.onReady(
			new EnvEditorViewModel(() => props.owner, overridden ? () => overridden : undefined),
		)
	})
</script>
