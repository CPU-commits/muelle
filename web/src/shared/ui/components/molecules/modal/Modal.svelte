<script lang="ts">
	import type { Snippet } from 'svelte'
	import type { Attachment } from 'svelte/attachments'
	import {
		body,
		description as descriptionClass,
		dialog,
		footer as footerClass,
		heading,
		sizes,
		title as titleClass,
		type ModalSize,
	} from './styles.css'

	type Props = {
		open: boolean
		title: string
		description?: string
		size?: ModalSize
		/** Se llama con Escape o con un clic en el fondo. El dueño pone `open` en false. */
		onClose: () => void
		children?: Snippet
		/** Los botones del pie (chicos: `size="sm"`). */
		footer?: Snippet
	}

	let { open, title, description, size = 'md', onClose, children, footer }: Props = $props()

	const id = $props.id()

	const showModal: Attachment<HTMLDialogElement> = (element) => {
		element.showModal()
		return () => element.close()
	}

	// El fondo es el propio <dialog>: el contenido lo llena, así que un clic cuyo target
	// es el <dialog> cayó afuera del panel.
	function onBackdropClick(event: MouseEvent & { currentTarget: HTMLDialogElement }): void {
		if (event.target === event.currentTarget) onClose()
	}

	function onCancel(event: Event): void {
		event.preventDefault()
		onClose()
	}
</script>

{#if open}
	<dialog
		class={[dialog, sizes[size]]}
		aria-labelledby={`${id}-title`}
		aria-describedby={description ? `${id}-description` : undefined}
		oncancel={onCancel}
		onclick={onBackdropClick}
		{@attach showModal}
	>
		<div class={body}>
			<div class={heading}>
				<h2 id={`${id}-title`} class={titleClass}>{title}</h2>
				{#if description}
					<p id={`${id}-description`} class={descriptionClass}>{description}</p>
				{/if}
			</div>
			{@render children?.()}
		</div>
		{#if footer}
			<div class={footerClass}>{@render footer()}</div>
		{/if}
	</dialog>
{/if}
