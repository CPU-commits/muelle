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
		 * Escape, un clic en el fondo o el navegador cerrándolo por su cuenta (nunca con
		 * `dismissible={false}`). El dueño pone `open` en false; si no lo hace (está
		 * guardando), el diálogo sigue abierto.
		 */
		onClose: () => void
		/**
		 * `false` para los modales de un solo uso (la URL de una invitación, un token nuevo):
		 * ni Escape ni el fondo lo cierran, y si el navegador lo cierra igual se vuelve a
		 * abrir. Solo lo cierra el dueño, con un botón que pone `open` en false.
		 */
		dismissible?: boolean
		children?: Snippet
		/** Los botones del pie (chicos: `size="sm"`). */
		footer?: Snippet
	}

	let {
		open,
		title,
		description,
		size = 'md',
		onClose,
		dismissible = true,
		children,
		footer,
	}: Props = $props()

	const id = $props.id()
	let element: HTMLDialogElement | undefined
	let closingFromProp = false
	let pressedOnBackdrop = false
	// Lo último que tuvo el foco adentro: al reabrirlo, el foco vuelve ahí y no al primer
	// control (el navegador ya se lo devolvió a la página al cerrarlo).
	let lastFocused: HTMLElement | null = null

	// <dialog> nativo con showModal(): el fondo queda inerte y el foco atrapado. Antes de
	// abrir se recuerda quién tenía el foco, para devolvérselo al cerrar (el {#if} saca el
	// diálogo del DOM antes de que el navegador pueda hacerlo).
	const showModal: Attachment<HTMLDialogElement> = (node) => {
		const opener = document.activeElement instanceof HTMLElement ? document.activeElement : null
		element = node
		closingFromProp = false
		lastFocused = null
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
		if (dismissible) onClose()
	}

	// Se guarda a quién volver ANTES de showModal(): al abrir, el navegador enfoca el primer
	// control y ese focusin pisaría `lastFocused`. `data-reopened` apaga la animación de
	// entrada: no es un diálogo nuevo.
	function reopen(): void {
		if (!element?.isConnected || element.open) return
		const target = lastFocused
		element.dataset['reopened'] = ''
		element.showModal()
		if (target?.isConnected && element.contains(target)) target.focus()
	}

	// El navegador puede cerrarlo sin pasar por un `cancel` que se pueda frenar: Chrome
	// deja cancelar un pedido de cierre una sola vez sin activación del usuario (Escape no
	// cuenta), así que al segundo Escape seguido lo cierra igual; también un form
	// method="dialog". El <dialog> y el `open` del dueño nunca quedan distintos:
	// - `dismissible={false}`: se reabre en el acto, sin avisar;
	// - si no, se le avisa al dueño y, si quiere seguir abierto (está guardando), se reabre.
	async function onNativeClose(event: Event): Promise<void> {
		// El `close` llega en una tarea aparte: si el dueño cerró y volvió a abrir enseguida,
		// el de un <dialog> anterior no tiene que tocar al de ahora.
		if (event.currentTarget !== element) return
		if (closingFromProp) return
		if (!dismissible) {
			reopen()
			return
		}
		onClose()
		await tick()
		if (open) reopen()
	}

	function onFocusIn(event: FocusEvent): void {
		if (event.target instanceof HTMLElement) lastFocused = event.target
	}

	// El fondo es el propio <dialog>: el contenido lo llena, así que un clic cuyo target es
	// el <dialog> cayó afuera del panel. Cuenta solo si también se APRETÓ ahí: apretar en el
	// panel y soltar sobre el fondo (una selección de texto, o un error de validación que
	// desaparece y mueve el layout) le manda el click al <dialog>, el ancestro común, y no
	// tiene que cerrar ni perder el formulario.
	function onPointerDown(event: PointerEvent & { currentTarget: HTMLDialogElement }): void {
		pressedOnBackdrop = event.target === event.currentTarget
	}

	function onBackdropClick(event: MouseEvent & { currentTarget: HTMLDialogElement }): void {
		const onBackdrop = pressedOnBackdrop && event.target === event.currentTarget
		pressedOnBackdrop = false
		if (onBackdrop && dismissible) onClose()
	}
</script>

{#if open}
	<dialog
		class={[dialog, sizes[size]]}
		aria-labelledby={`${id}-title`}
		aria-describedby={description ? `${id}-description` : undefined}
		oncancel={onCancel}
		onclose={(event) => void onNativeClose(event)}
		onfocusin={onFocusIn}
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
