<script lang="ts">
	import type { Snippet } from 'svelte'
	import Logo from '../../atoms/logo/Logo.svelte'
	import Mono from '../../atoms/mono/Mono.svelte'
	import {
		brand,
		brandName,
		content,
		header,
		main,
		page,
		widths,
		type AuthWidth,
	} from './styles.css'

	type Props = {
		appName: string
		/** El host del panel, en mono a la derecha ("panel.tudominio.cl"). */
		host: string
		/** `sm` 360 px (login), `md` 440 px (2FA con el QR). */
		width?: AuthWidth
		children: Snippet
	}

	let { appName, host, width = 'sm', children }: Props = $props()
</script>

<div class={page}>
	<header class={header}>
		<span class={brand}>
			<Logo />
			<span class={brandName}>{appName}</span>
		</span>
		<Mono size={12} tone="tertiary">{host}</Mono>
	</header>
	<main class={main}>
		<div class={[content, widths[width]]}>{@render children()}</div>
	</main>
</div>
