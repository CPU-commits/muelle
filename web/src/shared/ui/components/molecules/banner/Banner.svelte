<script lang="ts">
	import type { Snippet } from 'svelte'
	import StatusDot from '../../atoms/statusDot/StatusDot.svelte'
	import type { StatusTone } from '../../atoms/statusDot/styles.css'
	import {
		action as actionClass,
		banner,
		body as bodyClass,
		text,
		title as titleClass,
		tones,
		type BannerTone,
	} from './styles.css'

	// Un aviso de ancho completo: "Proxy desincronizado", "Docker no responde", "2
	// variables cambiaron". El título va en peso 500 y el cuerpo, gris, en la misma línea.
	type Props = {
		tone?: BannerTone
		title: string
		body?: string
		/** El color del punto; por defecto el del tono (`info` lo usa en ámbar). */
		dot?: StatusTone
		/** Botones a la derecha ("Reintentar", "Desplegar"). */
		action?: Snippet
	}

	let { tone = 'info', title, body, dot, action }: Props = $props()

	const DOT_BY_TONE: Record<BannerTone, StatusTone> = { info: 'warn', warn: 'warn', error: 'bad' }
</script>

<div class={[banner, tones[tone]]} role={tone === 'error' ? 'alert' : 'status'}>
	<StatusDot status={dot ?? DOT_BY_TONE[tone]} />
	<p class={text}>
		<span class={titleClass}>{title}</span>
		{#if body}
			<span class={bodyClass}>{body}</span>
		{/if}
	</p>
	{#if action}
		<div class={actionClass}>{@render action()}</div>
	{/if}
</div>
