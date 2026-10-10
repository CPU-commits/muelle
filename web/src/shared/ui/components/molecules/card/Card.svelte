<script lang="ts">
	import type { Snippet } from 'svelte'
	import {
		bandAside,
		bandHeader,
		bandMeta,
		bandTitle,
		body,
		card,
		description as descriptionClass,
		footer as footerClass,
		footerActions,
		footerDanger,
		footerNote,
		heading,
		title as titleClass,
		tones,
		type CardTone,
	} from './styles.css'

	// La sección con borde del diseño. Dos formas:
	// - normal (Ajustes, Configuración): título 16 + descripción y el contenido con
	//   padding;
	// - `flush` (Resumen: "Servicio api", "Tareas"): una franja de encabezado (título 14 +
	//   meta + `aside` a la derecha) y el contenido pegado al borde, para una tabla o un
	//   `<dl>`.
	// Las dos pueden tener un pie más oscuro con una nota a la izquierda y botones a la
	// derecha.
	type Props = {
		title?: string
		/** Normal: el párrafo bajo el título. `flush`: el texto gris al lado del título. */
		description?: string
		/** `h2` por defecto; `h3` si la tarjeta va bajo otro título de sección. */
		headingLevel?: 2 | 3
		tone?: CardTone
		flush?: boolean
		id?: string
		/** Solo en `flush`: lo que va a la derecha del encabezado ("En vivo · cada 5 s"). */
		aside?: Snippet
		/** Texto chico a la izquierda del pie ("Cualquier otra operación responde 403."). */
		note?: string
		/** Botones a la derecha del pie. */
		footer?: Snippet
		children?: Snippet
	}

	let {
		title,
		description,
		headingLevel = 2,
		tone = 'default',
		flush = false,
		id,
		aside,
		note,
		footer,
		children,
	}: Props = $props()

	const titleId = $props.id()
	const tag = $derived(`h${headingLevel}`)
</script>

<section {id} class={[card, tones[tone]]} aria-labelledby={title ? titleId : undefined}>
	{#if flush}
		{#if title}
			<div class={bandHeader}>
				<div class={bandMeta}>
					<svelte:element this={tag} id={titleId} class={bandTitle}
						>{title}</svelte:element
					>
					{#if description}
						<span>{description}</span>
					{/if}
				</div>
				{#if aside}
					<div class={bandAside}>{@render aside()}</div>
				{/if}
			</div>
		{/if}
		{@render children?.()}
	{:else if title || children}
		<div class={body}>
			{#if title}
				<div class={heading}>
					<svelte:element this={tag} id={titleId} class={titleClass}
						>{title}</svelte:element
					>
					{#if description}
						<p class={descriptionClass}>{description}</p>
					{/if}
				</div>
			{/if}
			{@render children?.()}
		</div>
	{/if}
	{#if footer || note}
		<div class={[footerClass, tone === 'danger' && footerDanger]}>
			{#if note}
				<span class={footerNote}>{note}</span>
			{/if}
			{#if footer}
				<div class={footerActions}>{@render footer()}</div>
			{/if}
		</div>
	{/if}
</section>
