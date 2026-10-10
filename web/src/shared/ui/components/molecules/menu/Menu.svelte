<script lang="ts">
	import type { Snippet } from 'svelte'
	import type { Attachment } from 'svelte/attachments'
	import Icon from '../../atoms/icon/Icon.svelte'
	import {
		aligns,
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

	// Botón que abre un menú (patrón "menu button" de WAI-ARIA): flechas para moverse,
	// Escape o Tab para cerrar, y clic afuera también cierra. Usos: la cuenta del header
	// y las "Opciones" (⋯) de una fila.
	type Props = {
		/** Nombre del botón y del menú ("Cuenta: felipe", "Opciones de auditfy"). */
		label: string
		items: readonly MenuEntry[]
		/** `ghost`: los tres puntos de una fila. `bare`: el trigger trae su forma (avatar). */
		variant?: 'ghost' | 'bare'
		/** Contenido del botón; por defecto, los tres puntos. */
		trigger?: Snippet
		/** De qué lado del botón se alinea el panel. */
		align?: 'start' | 'end'
		/** Algo arriba de los items que no es un item (el usuario de la sesión). */
		header?: Snippet
	}

	let { label, items, variant = 'ghost', trigger, align = 'end', header }: Props = $props()

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

	function select(entry: MenuEntry): void {
		close(true)
		if (entry.kind === 'action') entry.onSelect()
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

	// Al abrir, el foco va al primer item (también con el mouse, como pide el patrón).
	const focusFirst: Attachment<HTMLElement> = (element) => {
		menuItems(element)[0]?.focus()
	}

	// Un clic fuera del menú lo cierra sin robarle el foco a lo que se clickeó.
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
		class={variant === 'ghost' ? triggerGhost : triggerBare}
		aria-label={label}
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
		<div class={[panel, aligns[align]]}>
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
				{@attach focusFirst}
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
