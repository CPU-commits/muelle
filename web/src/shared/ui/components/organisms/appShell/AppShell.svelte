<script lang="ts">
	import type { Snippet } from 'svelte'
	import Avatar from '../../atoms/avatar/Avatar.svelte'
	import Logo from '../../atoms/logo/Logo.svelte'
	import Menu from '../../molecules/menu/Menu.svelte'
	import type { MenuEntry } from '../../molecules/menu/types'
	import { ShellSlots, setShellSlots } from './shellSlots.svelte.ts'
	import {
		accountHint,
		accountName,
		accountUser,
		brand,
		brandName,
		header,
		headerInner,
		main,
		shell,
		skipLink,
		topRow,
	} from './styles.css'

	// El marco de las pantallas con sesión: header con el logo (o el breadcrumb que pida
	// una ruta con <ShellHeader>), el botón de la cuenta con su menú, la nav subrayada y
	// el contenido en el contenedor de 1248 px.
	type Props = {
		homeHref: string
		appName: string
		username: string
		/** El `aria-label` del botón de la cuenta ("Cuenta: felipe"). */
		accountLabel: string
		/** Texto chico bajo el usuario en el menú ("2FA activa"). */
		accountHint?: string
		accountItems: readonly MenuEntry[]
		skipLabel: string
		/** La nav por defecto (Proyectos · Ajustes). */
		nav: Snippet
		children: Snippet
	}

	let {
		homeHref,
		appName,
		username,
		accountLabel,
		accountHint: hint,
		accountItems,
		skipLabel,
		nav,
		children,
	}: Props = $props()

	const slots = new ShellSlots()
	setShellSlots(slots)
</script>

<!-- eslint-disable svelte/no-navigation-without-resolve -- los href llegan ya resueltos (resolve()) desde quien usa el componente -->
<div class={shell}>
	<a href="#contenido" class={skipLink}>{skipLabel}</a>

	<header class={header}>
		<div class={headerInner}>
			<div class={topRow}>
				{#if slots.breadcrumb}
					{@render slots.breadcrumb()}
				{:else}
					<a href={homeHref} class={brand}>
						<Logo />
						<span class={brandName}>{appName}</span>
					</a>
				{/if}

				<Menu label={accountLabel} items={accountItems} variant="bare" align="end">
					{#snippet trigger()}
						<Avatar name={username} />
					{/snippet}
					{#snippet header()}
						<div class={accountName}>
							<span class={accountUser}>{username}</span>
							{#if hint}
								<span class={accountHint}>{hint}</span>
							{/if}
						</div>
					{/snippet}
				</Menu>
			</div>

			{#if slots.nav}
				{@render slots.nav()}
			{:else}
				{@render nav()}
			{/if}
		</div>
	</header>

	<main id="contenido" class={main} tabindex="-1">
		{@render children()}
	</main>
</div>
