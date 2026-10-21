<script lang="ts">
	import { setQueryClientContext } from '@tanstack/svelte-query'
	import { createQueryClient } from '@/shared/data/query'
	import { untrack } from 'svelte'
	import type { EnvOwner } from '../../../../domain'
	import { EnvAddCard, EnvEditorViewModel } from '..'

	// La tarjeta "Agregar variable" de verdad, con su viewModel: el test escribe en el
	// markup y mira lo que sale por HTTP.
	const props: { owner: EnvOwner } = $props()

	const vm = untrack(() => {
		// Las mismas opciones que la app, sin reintentos.
		setQueryClientContext(createQueryClient({ retry: false }))
		return new EnvEditorViewModel(() => props.owner)
	})
</script>

<EnvAddCard {vm} />
