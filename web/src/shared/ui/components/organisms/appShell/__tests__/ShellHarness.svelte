<script lang="ts">
	// Los estilos globales de la app (box-sizing, márgenes): sin ellos el layout no es el real.
	import '@/shared/ui/themes/global.css'
	import type { MenuEntry } from '../../../molecules/menu/types'
	import AppShell from '../AppShell.svelte'
	import ShellHeader from '../ShellHeader.svelte'

	// Un AppShell con una "ruta anidada" que se monta y desmonta con `override`.
	let {
		override,
		onLogout,
		longCrumbs = false,
	}: { override: boolean; onLogout: () => void; longCrumbs?: boolean } = $props()

	const items: readonly MenuEntry[] = [
		{ kind: 'link', id: 'totp', label: 'Verificación en dos pasos', href: '/2fa' },
		{ kind: 'separator', id: 'sep' },
		{ kind: 'action', id: 'logout', label: 'Cerrar sesión', onSelect: () => onLogout() },
	]
</script>

<AppShell
	homeHref="/"
	appName="muelle"
	username="felipe"
	accountLabel="Cuenta: felipe"
	accountHint="2FA activa"
	accountItems={items}
	skipLabel="Saltar al contenido"
>
	{#snippet nav()}
		<nav aria-label="Secciones">Proyectos · Ajustes</nav>
	{/snippet}

	{#if override}
		<ShellHeader>
			{#snippet breadcrumb()}
				<nav aria-label="Ubicación">
					{longCrumbs
						? 'un-proyecto-con-nombre-largo / production-europa / api-gateway-principal'
						: 'auditfy / staging / api'}
				</nav>
			{/snippet}
			{#snippet nav()}
				<nav aria-label="Secciones de la app">Resumen · Logs</nav>
			{/snippet}
		</ShellHeader>
	{/if}

	<p>contenido</p>
</AppShell>
