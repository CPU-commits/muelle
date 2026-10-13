<script lang="ts">
	import { setQueryClientContext } from '@tanstack/svelte-query'
	import { createQueryClient } from '@/shared/data/query'
	import { untrack } from 'svelte'
	import { type AppVariablesParams, AppVariablesViewModel } from '..'

	// Monta el viewModel adentro de un componente real (createQuery lee el contexto). No
	// renderiza nada: el test habla con el viewModel.
	const props: {
		params: AppVariablesParams
		onReady: (vm: AppVariablesViewModel) => void
	} = $props()

	untrack(() => {
		// Las mismas opciones que la app, sin reintentos.
		setQueryClientContext(createQueryClient({ retry: false }))
		props.onReady(new AppVariablesViewModel(() => props.params))
	})
</script>
