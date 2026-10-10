<script lang="ts">
	import {
		dot,
		pulse as pulseClass,
		sizes,
		statuses,
		type StatusDotSize,
		type StatusTone,
	} from './styles.css'

	type Props = {
		status: StatusTone
		size?: StatusDotSize
		/** El halo "en vivo" del Resumen. */
		pulse?: boolean
		/** Si el punto es la única pista del estado, decilo en texto. Sin label es decorativo. */
		label?: string
	}

	let { status, size = 8, pulse = false, label }: Props = $props()
</script>

{#if label}
	<span
		role="img"
		aria-label={label}
		class={[dot, sizes[size], statuses[status], pulse && pulseClass]}
	></span>
{:else}
	<span aria-hidden="true" class={[dot, sizes[size], statuses[status], pulse && pulseClass]}
	></span>
{/if}
