<script lang="ts">
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { SubNavLayout } from '@/shared/ui'
	import type { AppSummary, ServiceApp } from '../../../domain'
	import { ConfigHeader } from '../../components/configHeader'
	import { DeleteAppCard } from '../deleteApp'
	import HealthSection from './HealthSection.svelte'
	import ImageSection from './ImageSection.svelte'
	import ResourcesSection from './ResourcesSection.svelte'
	import RunSection from './RunSection.svelte'
	import { SECTION_IDS, ServiceConfigViewModel } from './ServiceConfig.viewModel.svelte.ts'
	import { page, sections } from './styles.css'
	import VolumesSection from './VolumesSection.svelte'

	// Las tarjetas de una app de formulario. Quien la monta la pone en un `{#key}` por
	// app: los formularios arrancan con los valores de la app que reciben.
	let { app, summary }: { app: ServiceApp; summary: AppSummary } = $props()

	const vm = new ServiceConfigViewModel(() => app)
</script>

<div class={page}>
	<ConfigHeader {summary}>
		{#snippet subtitle()}{m.catalog_config_subtitle()}{/snippet}
	</ConfigHeader>

	<SubNavLayout label={m.catalog_config_sections()} items={vm.navItems} spy={vm.sections}>
		<div class={sections}>
			<ImageSection section={vm.image} id={SECTION_IDS.image} />
			<RunSection section={vm.run} id={SECTION_IDS.run} />
			<HealthSection {vm} id={SECTION_IDS.health} />
			<VolumesSection {vm} id={SECTION_IDS.volumes} />
			<ResourcesSection section={vm.resources} id={SECTION_IDS.resources} />
			<DeleteAppCard appRef={vm.ref} id={SECTION_IDS.delete} />
		</div>
	</SubNavLayout>
</div>
