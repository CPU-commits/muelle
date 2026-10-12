<script lang="ts">
	import { type QueryClient, setQueryClientContext } from '@tanstack/svelte-query'
	import { untrack } from 'svelte'
	import type { EnvOwner } from '../../../../domain'
	import { EnvEditorViewModel } from '..'

	// Monta el viewModel adentro de un componente real, que es el único lugar donde
	// `useQueryClient`, `createQuery` y `createMutation` pueden leer el contexto. No
	// renderiza nada: el test habla con el viewModel, no con el markup.
	const props: {
		client: QueryClient
		owner: EnvOwner
		overridden?: ReadonlySet<string>
		onReady: (vm: EnvEditorViewModel) => void
	} = $props()

	// `untrack`: las props se leen UNA vez, al montar, y es a propósito.
	untrack(() => {
		setQueryClientContext(props.client)
		const overridden = props.overridden
		props.onReady(
			new EnvEditorViewModel(() => props.owner, overridden ? () => overridden : undefined),
		)
	})
</script>
