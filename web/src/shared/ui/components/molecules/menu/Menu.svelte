<script lang="ts">
	import type { Snippet } from 'svelte'
	import type { Attachment } from 'svelte/attachments'
	import Icon from '../../atoms/icon/Icon.svelte'
	import {
		header as headerClass,
		item as itemClass,
		itemDanger,
		itemIcon,
		list,
		panel,
		root,
		separator,
		triggerBare,
		triggerGhost,
	} from './styles.css'
	import type { MenuEntry } from './types'
	import { placeBelow } from './position'

	// Botón que abre un menú (patrón "menu button" de WAI-ARIA): flechas para moverse,
	// Escape o Tab para cerrar, y clic afuera también cierra. Usos: la cuenta del header,
	// las "Opciones" (⋯) de una fila y los selectores del breadcrumb.
	//
	// El panel va en la capa superior (`popover="manual"`), así que ningún `overflow` de
	// una List, Table o Card lo recorta. Se ubica con el rect del botón: abajo, o arriba si
	// no entra, y siempre adentro de la pantalla.
	type Props = {
		/** Nombre del botón y del menú ("Cuenta: felipe", "Opciones de auditfy"). */
		label: string
		items: readonly MenuEntry[]
		/** `ghost`: los tres puntos de una fila. `bare`: el trigger trae su forma (avatar). */
		variant?: 'ghost' | 'bare'
		/** Contenido del botón; por defecto, los tres puntos. */
		trigger?: Snippet
		/** Clase para el botón, en vez de la del `variant` (el selector del breadcrumb). */
		triggerClass?: string
		/** De qué lado del botón se alinea el panel. */
		align?: 'start' | 'end'
		/** El `aria-current` del botón: el selector del breadcrumb que es la página actual. */
		current?: 'page' | 'location' | undefined
		/** Algo arriba de los items que no es un item (el usuario de la sesión). */
		header?: Snippet
	}

	let {
		label,
		items,
		variant = 'ghost',
		trigger,
		triggerClass,
		align = 'end',
		current,
		header,
	}: Props = $props()

	const menuId = $props.id()
	let open = $state(false)
	let triggerElement: HTMLButtonElement | undefined

	function menuItems(container: HTMLElement): HTMLElement[] {
		return Array.from(
			container.querySelectorAll<HTMLElement>('[role="menuitem"]:not(:disabled)'),
		)
	}

	function close(returnFocus: boolean): void {
		open = false
		if (returnFocus) triggerElement?.focus()
	}

	function toggle(): void {
		open = !open
	}

	// El foco vuelve al botón ANTES de `onSelect`: el item desaparece con el menú, y un
	// diálogo que abra `onSelect` (un ConfirmDialog de "Eliminar") recuerda quién tenía el
	// foco para devolvérselo al cerrarse. Con el item, terminaría en <body>.
	function select(entry: MenuEntry): void {
		if (entry.kind !== 'action') return
		close(true)
		entry.onSelect()
	}

	function onTriggerKeydown(event: KeyboardEvent): void {
		if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
			event.preventDefault()
			open = true
		}
	}

	function onMenuKeydown(event: KeyboardEvent & { currentTarget: HTMLElement }): void {
		const elements = menuItems(event.currentTarget)
		const current = elements.indexOf(document.activeElement as HTMLElement)
		const focusAt = (index: number) => {
			const length = elements.length
			if (length === 0) return
			elements[(index + length) % length]?.focus()
		}
		switch (event.key) {
			case 'ArrowDown':
				event.preventDefault()
				focusAt(current + 1)
				break
			case 'ArrowUp':
				event.preventDefault()
				focusAt(current - 1)
				break
			case 'Home':
				event.preventDefault()
				focusAt(0)
				break
			case 'End':
				event.preventDefault()
				focusAt(elements.length - 1)
				break
			case 'Escape':
				event.preventDefault()
				close(true)
				break
			case 'Tab':
				close(false)
				break
		}
	}

	// Para devolverle el foco al cerrar con Escape o al elegir un item.
	const rememberTrigger: Attachment<HTMLButtonElement> = (element) => {
		triggerElement = element
		return () => {
			triggerElement = undefined
		}
	}

	// Muestra el panel en la capa superior, lo ubica junto al botón (y lo reubica si la
	// página scrollea o cambia de tamaño) y le da el foco al primer item, como pide el
	// patrón, también con el mouse.
	const showPanel: Attachment<HTMLElement> = (element) => {
		element.showPopover()
		const place = () => {
			if (!triggerElement) return
			// Reubicar mide el alto natural del panel: se guarda el scroll de adentro.
			const scrollTop = element.scrollTop
			placeBelow(element, triggerElement, align)
			element.scrollTop = scrollTop
		}
		// El scroll del propio panel (un menú largo) no lo mueve: solo el de la página.
		const onScroll = (event: Event) => {
			if (event.target instanceof Node && element.contains(event.target)) return
			place()
		}
		place()
		menuItems(element)[0]?.focus()
		window.addEventListener('resize', place)
		window.addEventListener('scroll', onScroll, true)
		return () => {
			window.removeEventListener('resize', place)
			window.removeEventListener('scroll', onScroll, true)
			if (element.matches(':popover-open')) element.hidePopover()
		}
	}

	// Un clic fuera del menú lo cierra sin robarle el foco a lo que se clickeó. El panel
	// sigue siendo hijo de este div en el DOM aunque se vea en la capa superior.
	const closeOnOutsideClick: Attachment<HTMLElement> = (element) => {
		const onPointerDown = (event: PointerEvent) => {
			if (open && !element.contains(event.target as Node)) close(false)
		}
		document.addEventListener('pointerdown', onPointerDown)
		return () => document.removeEventListener('pointerdown', onPointerDown)
	}
</script>

<!-- eslint-disable svelte/no-navigation-without-resolve -- los href llegan ya resueltos (resolve()) desde quien usa el componente -->
<div class={root} {@attach closeOnOutsideClick}>
	<button
		{@attach rememberTrigger}
		type="button"
		class={triggerClass ?? (variant === 'ghost' ? triggerGhost : triggerBare)}
		aria-label={label}
		aria-current={current}
		aria-haspopup="menu"
		aria-expanded={open}
		aria-controls={open ? menuId : undefined}
		onclick={toggle}
		onkeydown={onTriggerKeydown}
	>
		{#if trigger}
			{@render trigger()}
		{:else}
			<Icon name="more" />
		{/if}
	</button>

	{#if open}
		<div popover="manual" class={panel} {@attach showPanel}>
			{#if header}
				<div class={headerClass}>{@render header()}</div>
			{/if}
			<ul
				id={menuId}
				role="menu"
				aria-label={label}
				tabindex="-1"
				class={list}
				onkeydown={onMenuKeydown}
			>
				{#each items as entry (entry.id)}
					{#if entry.kind === 'separator'}
						<li role="separator" class={separator}></li>
					{:else if entry.kind === 'link'}
						<li role="none">
							<a
								role="menuitem"
								tabindex="-1"
								href={entry.href}
								class={itemClass}
								onclick={() => close(false)}
							>
								{#if entry.icon}
									<span class={itemIcon}><Icon name={entry.icon} /></span>
								{/if}
								{entry.label}
							</a>
						</li>
					{:else}
						<li role="none">
							<button
								role="menuitem"
								tabindex="-1"
								type="button"
								disabled={entry.disabled}
								class={[itemClass, entry.tone === 'danger' && itemDanger]}
								onclick={() => select(entry)}
							>
								{#if entry.icon}
									<span class={itemIcon}><Icon name={entry.icon} /></span>
								{/if}
								{entry.label}
							</button>
						</li>
					{/if}
				{/each}
			</ul>
		</div>
	{/if}
</div>
