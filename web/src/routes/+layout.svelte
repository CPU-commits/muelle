<script lang="ts">
	import '@/shared/ui/themes/fonts'
	import '@/shared/ui/themes/global.css'
	import { QueryClientProvider } from '@tanstack/svelte-query'
	import type { Snippet } from 'svelte'
	import { queryClient } from '@/shared/data/query'
	import favicon from '@/shared/ui/assets/favicon.svg'
	import { Toaster } from '@/shared/ui'

	let { children }: { children: Snippet } = $props()
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
	<title>muelle</title>
</svelte:head>

<QueryClientProvider client={queryClient}>
	{@render children()}
</QueryClientProvider>

<Toaster />
