<script lang="ts">
	import { resolve } from '$app/paths'
	import { page } from '$app/state'
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { SessionShell } from '@/modules/auth'
	import { Tabs } from '@/shared/ui'
	import type { LayoutProps } from './$types'

	let { data, children }: LayoutProps = $props()
</script>

<SessionShell me={data.me}>
	{#snippet nav()}
		<Tabs
			label={m.nav_sections()}
			items={[
				{
					href: resolve('/'),
					label: m.nav_projects(),
					active: !page.url.pathname.startsWith(resolve('/ajustes')),
				},
				{
					href: resolve('/ajustes'),
					label: m.nav_settings(),
					active: page.url.pathname.startsWith(resolve('/ajustes')),
				},
			]}
		/>
	{/snippet}

	{@render children()}
</SessionShell>
