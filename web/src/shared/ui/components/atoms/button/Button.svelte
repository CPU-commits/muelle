<script lang="ts">
	import type { Snippet } from 'svelte'
	import type { HTMLButtonAttributes } from 'svelte/elements'
	import Spinner from '../spinner/Spinner.svelte'
	import {
		base,
		content,
		contentHidden,
		fullWidth as fullWidthClass,
		iconOnly as iconOnlyClass,
		sizes,
		spinnerSlot,
		variants,
		type ButtonSize,
		type ButtonVariant,
	} from './styles.css'

	type Props = HTMLButtonAttributes & {
		variant?: ButtonVariant
		size?: ButtonSize
		/**
		 * Mientras carga no responde al clic, pero no se deshabilita: un botón `disabled`
		 * pierde el foco y el lector de pantalla se queda sin dónde estar.
		 */
		loading?: boolean
		/** Cuadrado, para un ícono solo. Pasale `aria-label`. */
		iconOnly?: boolean
		fullWidth?: boolean
		children: Snippet
	}

	let {
		variant = 'primary',
		size = 'md',
		loading = false,
		iconOnly = false,
		fullWidth = false,
		disabled = false,
		type = 'button',
		class: className,
		onclick,
		children,
		...rest
	}: Props = $props()

	// Cargando, un clic (o el Enter que manda un formulario) no hace nada: preventDefault
	// también frena el submit de un type="submit".
	function handleClick(event: MouseEvent & { currentTarget: EventTarget & HTMLButtonElement }) {
		if (loading) {
			event.preventDefault()
			return
		}
		onclick?.(event)
	}
</script>

<button
	{...rest}
	{type}
	{disabled}
	aria-disabled={loading || undefined}
	aria-busy={loading}
	class={[
		base,
		variants[variant],
		sizes[size],
		iconOnly && iconOnlyClass,
		fullWidth && fullWidthClass,
		className,
	]}
	onclick={handleClick}
>
	{#if loading}
		<span class={spinnerSlot}><Spinner size="sm" decorative /></span>
	{/if}
	<span class={[content, loading && contentHidden]}>{@render children()}</span>
</button>
