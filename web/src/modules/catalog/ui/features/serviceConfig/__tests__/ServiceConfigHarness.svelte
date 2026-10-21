<script lang="ts">
	import { type QueryClient, setQueryClientContext } from '@tanstack/svelte-query'
	import { untrack } from 'svelte'
	import type { AppSummary, ServiceApp } from '../../../../domain'
	import ServiceConfig from '../ServiceConfig.svelte'

	// Las tarjetas de verdad, con su markup, dentro del contexto de TanStack Query.
	const props: { client: QueryClient; app: ServiceApp; summary: AppSummary } = $props()

	untrack(() => setQueryClientContext(props.client))
</script>

<ServiceConfig app={props.app} summary={props.summary} />
