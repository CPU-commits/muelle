<script lang="ts">
	import { type QueryClient, setQueryClientContext } from '@tanstack/svelte-query'
	import { untrack } from 'svelte'
	import { SessionShellViewModel } from '..'
	import type { Me } from '../../../../domain'

	const props: {
		client: QueryClient
		me: Me
		onReady: (vm: SessionShellViewModel) => void
	} = $props()

	untrack(() => {
		setQueryClientContext(props.client)
		props.onReady(new SessionShellViewModel(() => props.me))
	})
</script>
