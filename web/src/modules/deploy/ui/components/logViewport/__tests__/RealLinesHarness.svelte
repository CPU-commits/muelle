<script lang="ts">
	import '@/shared/ui/themes/global.css'
	import { untrack } from 'svelte'
	import * as deploysStyles from '../../../screens/deploys/styles.css'
	import * as logsStyles from '../../../screens/logs/styles.css'
	import LogViewport from '../LogViewport.svelte'

	// Un LogViewport con las líneas de verdad de Logs o de Deploys (sus clases, con el
	// `content-visibility` y el corte de línea), para medir que el final queda a la vista.
	export interface RealLinesApi {
		push: (texts: readonly string[]) => void
		readonly follow: boolean
		readonly scroller: HTMLElement | null
	}

	const props: {
		variant: 'logs' | 'deploys'
		width: number
		onReady: (api: RealLinesApi) => void
	} = $props()

	let lines = $state.raw<string[]>([])
	let follow = $state(true)
	let host: HTMLDivElement | undefined = $state()

	untrack(() =>
		props.onReady({
			push: (texts) => {
				lines = [...lines, ...texts]
			},
			get follow() {
				return follow
			},
			get scroller() {
				return host?.querySelector<HTMLElement>('[role="region"]') ?? null
			},
		}),
	)
</script>

<div bind:this={host} style:width={`${props.width}px`}>
	{#if props.variant === 'logs'}
		<div class={logsStyles.box}>
			<LogViewport
				label="Líneas de log"
				count={lines.length}
				{follow}
				onFollowChange={(next) => (follow = next)}
				size="lg"
			>
				<ol class={logsStyles.lines}>
					{#each lines as text, index (index)}
						<li class={logsStyles.line}>
							<span class={logsStyles.time}>14:31:40,118</span>
							<span class={logsStyles.task}>api.1</span>
							<span class={[logsStyles.message, logsStyles.levels.info]}>{text}</span>
						</li>
					{/each}
				</ol>
			</LogViewport>
		</div>
	{:else}
		<LogViewport
			label="Log del deploy"
			count={lines.length}
			{follow}
			onFollowChange={(next) => (follow = next)}
		>
			<ol class={deploysStyles.lines}>
				{#each lines as text, index (index)}
					<li class={deploysStyles.line}>
						<span class={deploysStyles.time}>14:31:40</span>
						<span class={[deploysStyles.text, deploysStyles.tones.normal]}>{text}</span>
					</li>
				{/each}
			</ol>
		</LogViewport>
	{/if}
</div>
