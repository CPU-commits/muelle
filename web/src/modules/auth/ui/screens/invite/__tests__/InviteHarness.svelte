<script lang="ts">
	import { type QueryClient, setQueryClientContext } from '@tanstack/svelte-query'
	import { untrack } from 'svelte'
	import { InviteViewModel } from '..'

	const props: {
		client: QueryClient
		token: string | null
		onReady: (vm: InviteViewModel) => void
	} = $props()

	untrack(() => {
		setQueryClientContext(props.client)
		props.onReady(new InviteViewModel(() => props.token))
	})
</script>
