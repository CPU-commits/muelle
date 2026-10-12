<script lang="ts">
	import { type QueryClient, setQueryClientContext } from '@tanstack/svelte-query'
	import { untrack } from 'svelte'
	import { TotpSetupViewModel } from '..'
	import type { Me } from '../../../../domain'

	const props: {
		client: QueryClient
		me: Me
		onReady: (vm: TotpSetupViewModel) => void
	} = $props()

	untrack(() => {
		setQueryClientContext(props.client)
		props.onReady(new TotpSetupViewModel(() => props.me))
	})
</script>
