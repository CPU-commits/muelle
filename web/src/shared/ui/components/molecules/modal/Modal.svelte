<script lang="ts">
	import { type Snippet, tick } from 'svelte'
	import type { Attachment } from 'svelte/attachments'
	import { restoreFocus } from './focus'
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
		/**
		 * Escape, un clic en el fondo o el navegador cerrándolo por su cuenta. El dueño pone
		 * `open` en false; si no lo hace (está guardando), el diálogo sigue abierto.
		 */
		onClose: () => void
		children?: Snippet
		/** Los botones del pie (chicos: `size="sm"`). */
		footer?: Snippet
	}

	let { open, title, description, size = 'md', onClose, children, footer }: Props = $props()

	const id = $props.id()
	let element: HTMLDialogElement | undefined
	let closingFromProp = false
	let pressedOnBackdrop = false

	// <dialog> nativo con showModal(): el fondo queda inerte y el foco atrapado. Antes de
	// abrir se recuerda quién tenía el foco, para devolvérselo al cerrar (el {#if} saca el
	// diálogo del DOM antes de que el navegador pueda hacerlo).
	const showModal: Attachment<HTMLDialogElement> = (node) => {
		const opener = document.activeElement instanceof HTMLElement ? document.activeElement : null
		element = node
		closingFromProp = false
		node.showModal()
		return () => {
			closingFromProp = true
			if (node.open) node.close()
			element = undefined
			restoreFocus(opener)
		}
	}

	function onCancel(event: Event): void {
		event.preventDefault()
		onClose()
	}

	// El navegador puede cerrarlo sin pasar por `cancel` (un segundo Escape seguido, un
	// form method="dialog"). Se le avisa al dueño y, si quiere seguir abierto, se reabre:
	// el <dialog> y el `open` del dueño nunca quedan distintos.
	async function onNativeClose(): Promise<void> {
		if (closingFromProp) return
		onClose()
		await tick()
		if (open && element?.isConnected && !element.open) element.showModal()
	}

	// El fondo es el propio <dialog>: el contenido lo llena, así que un clic cuyo target es
	// el <dialog> cayó afuera del panel. Cuenta solo si también se apretó ahí: arrastrar una
	// selección de texto y soltar sobre el fondo no cierra.
	function onPointerDown(event: PointerEvent & { currentTarget: HTMLDialogElement }): void {
		pressedOnBackdrop = event.target === event.currentTarget
	}

	function onBackdropClick(event: MouseEvent & { currentTarget: HTMLDialogElement }): void {
		const onBackdrop = pressedOnBackdrop && event.target === event.currentTarget
		pressedOnBackdrop = false
		if (onBackdrop) onClose()
	}
</script>

{#if open}
	<dialog
		class={[dialog, sizes[size]]}
		aria-labelledby={`${id}-title`}
		aria-describedby={description ? `${id}-description` : undefined}
		oncancel={onCancel}
		onclose={() => void onNativeClose()}
		onpointerdown={onPointerDown}
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
