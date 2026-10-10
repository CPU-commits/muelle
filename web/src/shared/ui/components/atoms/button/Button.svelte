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
		children,
		...rest
	}: Props = $props()
</script>

<button
	{...rest}
	{type}
	disabled={disabled || loading}
	aria-busy={loading}
	class={[
		base,
		variants[variant],
		sizes[size],
		iconOnly && iconOnlyClass,
		fullWidth && fullWidthClass,
		className,
	]}
>
	{#if loading}
		<span class={spinnerSlot}><Spinner size="sm" /></span>
	{/if}
	<span class={[content, loading && contentHidden]}>{@render children()}</span>
</button>
