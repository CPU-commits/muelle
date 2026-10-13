<script lang="ts">
	import type { Snippet } from 'svelte'
	import type { Attachment } from 'svelte/attachments'
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { Button, Icon } from '@/shared/ui'
	import { jumpSlot, root, scroller, sizes } from './styles.css'

	// La caja con scroll de un log. Mientras `follow` es true, cada línea nueva (`count`)
	// baja hasta el final. Si quien lee sube, avisa `onFollowChange(false)` y deja de
	// bajar; volver al final (o "Ir al final") lo reactiva. El estado de `follow` es del
	// viewModel: esto solo mira el scroll.
	type Props = {
		label: string
		/** Cuántas líneas hay: cambia cuando llegan nuevas. */
		count: number
		follow: boolean
		onFollowChange: (follow: boolean) => void
		size?: 'md' | 'lg'
		children: Snippet
	}

	let { label, count, follow, onFollowChange, size = 'md', children }: Props = $props()

	/** A menos de esto del final, se considera "en el final". */
	const NEAR_BOTTOM_PX = 24
	/** Teclas que suben: quien lee quiere ver lo de antes. */
	const UP_KEYS = new Set(['ArrowUp', 'PageUp', 'Home'])

	// El `scrollTop` que dejó el último scroll automático. Con líneas llegando rápido, entre
	// ese scroll y su evento crece el contenido: el evento ve "lejos del final" aunque nadie
	// tocó nada. Un scroll que termina justo donde lo dejamos es nuestro, no de quien lee.
	let autoScrollTop = -1
	let scrollerElement: HTMLElement | null = null

	// Corre de nuevo cada vez que cambian `count` o `follow` (un attachment es un effect).
	// Baja en el próximo frame, cuando las líneas nuevas ya están pintadas.
	const stickToBottom: Attachment<HTMLDivElement> = (element) => {
		scrollerElement = element
		void count
		if (!follow) return
		const frame = requestAnimationFrame(() => toBottom(element))
		return () => cancelAnimationFrame(frame)
	}

	function toBottom(element: HTMLElement): void {
		element.scrollTop = element.scrollHeight
		autoScrollTop = element.scrollTop
	}

	// "Ir al final" baja en el acto: un evento de scroll viejo que llegue antes del próximo
	// frame ya ve el final y no vuelve a apagar el seguimiento.
	function jumpToEnd(): void {
		if (scrollerElement) toBottom(scrollerElement)
		onFollowChange(true)
	}

	function onScroll(event: Event & { currentTarget: HTMLDivElement }): void {
		const element = event.currentTarget
		const distance = element.scrollHeight - element.scrollTop - element.clientHeight
		if (distance <= NEAR_BOTTOM_PX) {
			if (!follow) onFollowChange(true)
			return
		}
		// Nuestro propio scroll (y el contenido creció después): no es quien lee.
		if (Math.abs(element.scrollTop - autoScrollTop) <= 1) return
		if (follow) onFollowChange(false)
	}

	// Lo que hace quien lee para subir deja de seguir enseguida, sin esperar al scroll.
	function onWheel(event: WheelEvent): void {
		if (event.deltaY < 0 && follow) onFollowChange(false)
	}

	function onKeydown(event: KeyboardEvent): void {
		if (UP_KEYS.has(event.key) && follow) onFollowChange(false)
	}

	// Solo se escucha (el scroll con rueda o teclado es el nativo de la región): por eso
	// va con addEventListener y no como handler de un elemento no interactivo.
	const watchInput: Attachment<HTMLDivElement> = (element) => {
		element.addEventListener('wheel', onWheel, { passive: true })
		element.addEventListener('keydown', onKeydown)
		return () => {
			element.removeEventListener('wheel', onWheel)
			element.removeEventListener('keydown', onKeydown)
		}
	}
</script>

<div class={root}>
	<!-- Una región con scroll tiene que poder recibir el foco para leerse con teclado. -->
	<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
	<div
		class={[scroller, sizes[size]]}
		role="region"
		aria-label={label}
		tabindex="0"
		onscroll={onScroll}
		{@attach stickToBottom}
		{@attach watchInput}
	>
		{@render children()}
	</div>
	{#if !follow}
		<div class={jumpSlot}>
			<Button variant="secondary" size="sm" onclick={jumpToEnd}>
				<Icon name="chevronDown" size={14} />
				{m.deploy_log_jump()}
			</Button>
		</div>
	{/if}
</div>
