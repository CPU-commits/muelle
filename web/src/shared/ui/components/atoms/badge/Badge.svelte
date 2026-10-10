<script lang="ts">
	import type { Snippet } from 'svelte'
	import StatusDot from '../statusDot/StatusDot.svelte'
	import type { StatusTone } from '../statusDot/styles.css'
	import {
		badge,
		shapes,
		sizes,
		tones,
		type BadgeShape,
		type BadgeSize,
		type BadgeTone,
	} from './styles.css'

	type Props = {
		tone?: BadgeTone
		/** `tag` (radio 4, por defecto) o `pill` (999). */
		shape?: BadgeShape
		/** `sm` 20 px (filas) o `md` 24 px (encabezado de la app). */
		size?: BadgeSize
		/** Un punto de estado antes del texto; por defecto el del tono. */
		dot?: boolean | StatusTone
		children: Snippet
	}

	let { tone = 'neutral', shape = 'tag', size = 'sm', dot = false, children }: Props = $props()

	const DOT_BY_TONE: Partial<Record<BadgeTone, StatusTone>> = {
		ok: 'ok',
		warn: 'warn',
		error: 'bad',
	}

	const dotStatus = $derived(
		dot === false ? null : dot === true ? (DOT_BY_TONE[tone] ?? 'idle') : dot,
	)
</script>

<span class={[badge, tones[tone], shapes[shape], sizes[size]]}>
	{#if dotStatus}
		<StatusDot status={dotStatus} size={size === 'md' ? 8 : 6} />
	{/if}
	{@render children()}
</span>
