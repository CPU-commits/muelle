<script lang="ts">
	import type { Snippet } from 'svelte'
	import {
		mono,
		sizes,
		tones,
		truncate as truncateClass,
		type MonoSize,
		type MonoTone,
	} from './styles.css'

	// Texto técnico en Geist Mono: slugs, imágenes, digests, redes, claves de variables.
	type Props = {
		tone?: MonoTone
		/** En px; `inherit` (por defecto) toma el tamaño del texto que lo rodea. */
		size?: MonoSize
		/** Recorta con "…" y muestra el texto entero en el `title`. */
		truncate?: boolean
		title?: string
		children: Snippet
	}

	let { tone = 'inherit', size = 'inherit', truncate = false, title, children }: Props = $props()
</script>

<span class={[mono, tones[tone], sizes[size], truncate && truncateClass]} {title}>
	{@render children()}
</span>
