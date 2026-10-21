<script lang="ts">
	import { resolve } from '$app/paths'
	import type { Snippet } from 'svelte'
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { AppShell } from '@/shared/ui'
	import type { Me } from '../../../domain'
	import { SessionShellViewModel } from './SessionShell.viewModel.svelte.ts'

	// El AppShell con la cuenta de la sesión ya conectada (avatar, menú, cerrar sesión).
	// La nav la pone la ruta: es de la app, no de auth.
	type Props = { me: Me; nav: Snippet; children: Snippet }

	let { me, nav, children }: Props = $props()

	const vm = new SessionShellViewModel(() => me)
</script>

<AppShell
	homeHref={resolve('/')}
	appName="muelle"
	username={vm.username}
	accountLabel={m.nav_account({ username: vm.username })}
	accountHint={vm.totpEnabled ? m.nav_account_totp_on() : m.nav_account_totp_off()}
	accountItems={vm.accountItems}
	skipLabel={m.nav_skip()}
	{nav}
>
	{@render children()}
</AppShell>
