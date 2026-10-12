<script lang="ts">
	import type { Snippet } from 'svelte'
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { Badge, formatRelative, Icon, StatusDot } from '@/shared/ui'
	import { type App, appContents } from '../../../domain'
	import { kindLabel } from '../catalogMessages'
	import {
		chevron,
		deploy,
		dot as dotClass,
		image,
		name,
		row,
		slug,
		status as statusClass,
	} from './styles.css'

	// Una app en Proyectos. El estado ("Corriendo · 2/2") y el último deploy ("#143 · hace
	// 12 min") son de la Fase 2: llegan por los snippets. Sin ellos, el punto marca las
	// apps sin desplegar, la columna de estado dice el tipo y la del deploy, cuándo fue.
	type Props = {
		app: App
		href: string
		statusDot?: Snippet<[App]> | undefined
		status?: Snippet<[App]> | undefined
		lastDeploy?: Snippet<[App]> | undefined
	}

	let { app, href, statusDot, status, lastDeploy }: Props = $props()

	const contents = $derived(appContents(app))
	const what = $derived(
		contents.kind === 'image'
			? contents.image
			: contents.services.join(' · ') || m.catalog_app_no_services(),
	)
</script>

<!-- eslint-disable svelte/no-navigation-without-resolve -- el href llega ya resuelto (resolve()) desde el viewModel -->
<li>
	<a {href} class={row}>
		<span class={name}>
			<span class={dotClass}>
				{#if statusDot}
					{@render statusDot(app)}
				{:else if app.lastDeployedAt === null}
					<StatusDot status="idle" label={m.catalog_badge_never()} />
				{/if}
			</span>
			<span class={slug}>{app.slug}</span>
			{#if app.kind === 'compose'}
				<Badge tone="mono">{m.catalog_app_compose_badge()}</Badge>
			{/if}
			{#if app.pendingChanges}
				<Badge tone="warn" shape="pill" dot>{m.catalog_app_pending()}</Badge>
			{/if}
		</span>
		<span class={statusClass}>
			{#if status}
				{@render status(app)}
			{:else}
				{kindLabel(app.kind)}
			{/if}
		</span>
		<span class={image} title={what}>{what}</span>
		<span class={deploy}>
			{#if lastDeploy}
				{@render lastDeploy(app)}
			{:else if app.lastDeployedAt}
				{formatRelative(app.lastDeployedAt)}
			{:else}
				{m.catalog_app_no_deploys()}
			{/if}
		</span>
		<span class={chevron}><Icon name="chevronRight" /></span>
	</a>
</li>
