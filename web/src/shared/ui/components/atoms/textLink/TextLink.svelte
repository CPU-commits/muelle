<script lang="ts">
	import type { Snippet } from 'svelte'
	import type { HTMLAnchorAttributes } from 'svelte/elements'
	import { textLink } from '../../../themes/utilities.css'

	// Un link dentro de un texto ("configura tu 2FA primero", "ver el deploy #7"), con la
	// clase `textLink`. Existe para que un módulo arme el `href` con `withQuery(resolve(…),
	// {…})` sin un `<a>` propio, que eslint marca si el `href` no es un `resolve()` pelado.
	type Props = HTMLAnchorAttributes & {
		/** Ya resuelto con `resolve()` (y, si hace falta, `withQuery`). */
		href: string
		children: Snippet
	}

	let { href, class: className, children, ...rest }: Props = $props()
</script>

<!-- eslint-disable svelte/no-navigation-without-resolve -- los href llegan ya resueltos (resolve()) desde quien usa el componente -->
<a {...rest} {href} class={[textLink, className]}>{@render children()}</a>
