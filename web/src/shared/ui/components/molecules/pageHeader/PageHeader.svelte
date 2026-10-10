<script lang="ts">
	import type { Snippet } from 'svelte'
	import {
		actions as actionsClass,
		header,
		meta as metaClass,
		subtitle as subtitleClass,
		text,
		title as titleClass,
		titleRow,
		titleSizes,
		type PageTitleSize,
	} from './styles.css'

	// El h1 de cada pantalla, con lo que va al lado ("10 apps", badges) y los botones
	// de la derecha.
	type Props = {
		title: string
		subtitle?: string
		/** Texto gris en la línea del título ("10 apps"). */
		meta?: string
		titleSize?: PageTitleSize
		/** Lo que va en la línea del título después de `meta` (badges de estado). */
		badges?: Snippet
		/** Lo que va bajo el título en vez de `subtitle` (la línea "Servicio · stack · red"). */
		details?: Snippet
		actions?: Snippet
	}

	let { title, subtitle, meta, titleSize = 'md', badges, details, actions }: Props = $props()
</script>

<header class={header}>
	<div class={text}>
		<div class={titleRow}>
			<h1 class={[titleClass, titleSizes[titleSize]]}>{title}</h1>
			{#if meta}
				<span class={metaClass}>{meta}</span>
			{/if}
			{@render badges?.()}
		</div>
		{#if details}
			{@render details()}
		{:else if subtitle}
			<p class={subtitleClass}>{subtitle}</p>
		{/if}
	</div>
	{#if actions}
		<div class={actionsClass}>{@render actions()}</div>
	{/if}
</header>
