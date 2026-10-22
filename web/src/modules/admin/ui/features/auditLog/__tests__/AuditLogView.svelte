<script lang="ts">
	import { type QueryClient, setQueryClientContext } from '@tanstack/svelte-query'
	import { untrack } from 'svelte'
	import { AuditLog } from '..'

	// Renderiza la sección de verdad (markup incluido) con el QueryClient del test.
	const props: { client: QueryClient } = $props()

	untrack(() => setQueryClientContext(props.client))
</script>

<AuditLog />
