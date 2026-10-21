<script lang="ts">
	import { type Snippet, tick } from 'svelte'
	import Avatar from '../../atoms/avatar/Avatar.svelte'
	import Logo from '../../atoms/logo/Logo.svelte'
	import Menu from '../../molecules/menu/Menu.svelte'
	import type { MenuEntry } from '../../molecules/menu/types'
	import { ShellSlots, setShellSlots } from './shellSlots.svelte.ts'
	import {
		account,
		accountHint,
		accountName,
		accountUser,
		brand,
		brandName,
		header,
		headerInner,
		leading,
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

	// Los snippets de <ShellHeader> se pintan acá, fuera de la ruta que los puso. Al salir
	// de esa ruta, SvelteKit cambia los `params` y desmonta la ruta en el MISMO cambio, y
	// el header (que va antes que `main` en el árbol) llega a re-pintarse una vez con los
	// params de la ruta nueva: `params.p` undefined, un `resolve()` sin `a`. El
	// <svelte:boundary> ataja ese error; en el mismo cambio la ruta suelta el slot y vuelve
	// lo de antes, sin que se llegue a ver. Si después el snippet sigue puesto, no era la
	// navegación: es un bug del snippet, y va a la consola.
	function reportSlotError(snippet: Snippet): (error: unknown) => void {
		return (error) => {
			void tick().then(() => {
				if (slots.holds(snippet)) console.error(error)
			})
		}
	}
</script>

<!-- eslint-disable svelte/no-navigation-without-resolve -- los href llegan ya resueltos (resolve()) desde quien usa el componente -->
{#snippet brandLink()}
	<a href={homeHref} class={brand}>
		<Logo />
		<span class={brandName}>{appName}</span>
	</a>
{/snippet}

<div class={shell}>
	<a href="#contenido" class={skipLink}>{skipLabel}</a>

	<header class={header}>
		<div class={headerInner}>
			<div class={topRow}>
				<div class={leading}>
					{#if slots.breadcrumb}
						{@const crumb = slots.breadcrumb}
						{#key crumb}
							<svelte:boundary onerror={reportSlotError(crumb)}>
								{@render crumb()}
								{#snippet failed()}
									{@render brandLink()}
								{/snippet}
							</svelte:boundary>
						{/key}
					{:else}
						{@render brandLink()}
					{/if}
				</div>

				<div class={account}>
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
			</div>

			{#if slots.nav}
				{@const routeNav = slots.nav}
				{#key routeNav}
					<svelte:boundary onerror={reportSlotError(routeNav)}>
						{@render routeNav()}
						{#snippet failed()}
							{@render nav()}
						{/snippet}
					</svelte:boundary>
				{/key}
			{:else}
				{@render nav()}
			{/if}
		</div>
	</header>

	<main id="contenido" class={main} tabindex="-1">
		{@render children()}
	</main>
</div>
