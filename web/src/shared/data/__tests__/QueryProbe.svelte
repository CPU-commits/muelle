<script lang="ts">
	import { createQuery, setQueryClientContext } from '@tanstack/svelte-query'
	import { createQueryClient } from '../query'

	// Como un viewModel que mira `data` primero: mientras no hay datos muestra "cargando", y
	// recién después (acá, con el botón) lee `error`. Con el modo "tracked" de TanStack, el
	// observer solo avisa cambios de lo que ya se leyó: el error no cambia `data`, nadie se
	// entera, y `error` queda con el valor viejo.
	let { load }: { load: () => Promise<string> } = $props()

	setQueryClientContext(createQueryClient({ retry: false }))

	const query = createQuery(() => ({ queryKey: ['probe'], queryFn: load }))
	let showError = $state(false)
</script>

<p data-testid="data">{query.data ?? 'sin datos'}</p>
<button type="button" onclick={() => (showError = true)}>ver error</button>
{#if showError}
	<p data-testid="error">{query.error?.message ?? 'sin error'}</p>
{/if}
