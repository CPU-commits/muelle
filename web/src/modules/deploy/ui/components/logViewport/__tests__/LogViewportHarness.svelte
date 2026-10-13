<script lang="ts">
	import '@/shared/ui/themes/global.css'
	import { untrack } from 'svelte'
	import LogViewport from '../LogViewport.svelte'

	// Un LogViewport con líneas que el test agrega y el `follow` en estado, como lo tiene
	// un viewModel. Expone lo que el test necesita por `onReady`.
	export interface HarnessApi {
		push: (count?: number) => void
		readonly follow: boolean
		readonly scroller: HTMLElement | null
	}

	const props: { initial?: number; onReady: (api: HarnessApi) => void } = $props()

	let lines = $state<string[]>([])
	let follow = $state(true)
	let host: HTMLDivElement | undefined = $state()

	untrack(() => {
		lines = Array.from({ length: props.initial ?? 0 }, (_, index) => `línea ${index + 1}`)
		props.onReady({
			push: (count = 1) => {
				const start = lines.length
				lines = [
					...lines,
					...Array.from({ length: count }, (_, i) => `línea ${start + i + 1}`),
				]
			},
			get follow() {
				return follow
			},
			get scroller() {
				return host?.querySelector<HTMLElement>('[role="region"]') ?? null
			},
		})
	})
</script>

<div bind:this={host} style:width="480px">
	<LogViewport
		label="Log de prueba"
		count={lines.length}
		{follow}
		onFollowChange={(next) => (follow = next)}
	>
		<ol>
			{#each lines as line, index (index)}
				<li>{line}</li>
			{/each}
		</ol>
	</LogViewport>
</div>
