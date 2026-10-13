<script lang="ts">
	import { type QueryClient, setQueryClientContext } from '@tanstack/svelte-query'
	import { untrack } from 'svelte'
	import type { EnvOwner } from '../../../../domain'
	import { EnvAddCard, EnvEditorViewModel } from '..'

	// La tarjeta "Agregar variable" de verdad, con su viewModel: el test escribe en el
	// markup y mira lo que sale por HTTP.
	const props: { client: QueryClient; owner: EnvOwner } = $props()

	const vm = untrack(() => {
		setQueryClientContext(props.client)
		return new EnvEditorViewModel(() => props.owner)
	})
</script>

<EnvAddCard {vm} />
