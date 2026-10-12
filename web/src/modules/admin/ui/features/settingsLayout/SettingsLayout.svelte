<script lang="ts">
	import type { Snippet } from 'svelte'
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { PageHeader, SubNavLayout } from '@/shared/ui'
	import { SettingsLayoutViewModel } from './SettingsLayout.viewModel.svelte.ts'

	// El marco de Ajustes y de Ajustes · Tokens: el h1 y la nav de secciones a la izquierda.
	let { children }: { children: Snippet } = $props()

	const vm = new SettingsLayoutViewModel()
</script>

<PageHeader title={m.nav_settings()} />
<SubNavLayout label={m.admin_nav_label()} items={vm.items}>
	{@render children()}
</SubNavLayout>
