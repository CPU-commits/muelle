<script lang="ts">
	import type { Snippet } from 'svelte'
	import type { HTMLAnchorAttributes } from 'svelte/elements'
	import {
		base,
		content,
		fullWidth as fullWidthClass,
		iconOnly as iconOnlyClass,
		sizes,
		variants,
		type ButtonSize,
		type ButtonVariant,
	} from './styles.css'

	// Un link con la forma de un botón ("Desplegar" que lleva a Deploys). Navega: si la
	// acción no cambia de URL, es un <Button>.
	type Props = HTMLAnchorAttributes & {
		href: string
		variant?: ButtonVariant
		size?: ButtonSize
		iconOnly?: boolean
		fullWidth?: boolean
		children: Snippet
	}

	let {
		href,
		variant = 'primary',
		size = 'md',
		iconOnly = false,
		fullWidth = false,
		class: className,
		children,
		...rest
	}: Props = $props()
</script>

<!-- eslint-disable svelte/no-navigation-without-resolve -- los href llegan ya resueltos (resolve()) desde quien usa el componente -->
<a
	{...rest}
	{href}
	class={[
		base,
		variants[variant],
		sizes[size],
		iconOnly && iconOnlyClass,
		fullWidth && fullWidthClass,
		className,
	]}
>
	<span class={content}>{@render children()}</span>
</a>
