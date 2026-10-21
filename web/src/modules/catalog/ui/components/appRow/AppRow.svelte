<script lang="ts">
	import { untrack } from 'svelte'
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { Badge, formatRelative, Icon, StatusDot } from '@/shared/ui'
	import type { App } from '../../../domain'
	import { kindLabel } from '../catalogMessages'
	import type { AppRowDeployFactory } from './appRowDeploy'
	import { AppRowViewModel } from './AppRow.viewModel.svelte.ts'
	import {
		chevron,
		deploy as deployClass,
		dot as dotClass,
		failed,
		image,
		name,
		row,
		slug,
		status as statusClass,
		statusTones,
	} from './styles.css'
	import { whenVisible } from './whenVisible'

	// Una app en Proyectos. El estado ("Corriendo · 2/2"), la imagen que corre y el último
	// deploy ("#143 · hace 12 min") son de la Fase 2: llegan con `deploy` (lo crea la fila al
	// montarse y pide recién cuando la fila se ve). Sin él, el punto marca las apps sin
	// desplegar, la columna de estado dice el tipo y la del deploy, cuándo fue.
	type Props = {
		app: App
		href: string
		deploy?: AppRowDeployFactory | undefined
	}

	let { app, href, deploy }: Props = $props()

	// `untrack`: `deploy` se lee una vez, a propósito. La fila crea su estado al montarse,
	// como un viewModel, y no lo vuelve a crear.
	const vm = new AppRowViewModel(
		() => app,
		untrack(() => deploy),
	)
</script>

<!-- eslint-disable svelte/no-navigation-without-resolve -- el href llega ya resuelto (resolve()) desde el viewModel -->
<li>
	<a {href} class={row} {@attach vm.deploy && whenVisible(vm.markSeen)}>
		<span class={name}>
			<span class={dotClass}>
				{#if vm.deploy}
					<StatusDot status={vm.deploy.tone} />
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
		{#if vm.deploy}
			<span class={[statusClass, statusTones[vm.deploy.tone]]}>{vm.deploy.statusText}</span>
		{:else}
			<span class={statusClass}>{kindLabel(app.kind)}</span>
		{/if}
		<span class={image} title={vm.image}>{vm.image}</span>
		<span class={deployClass}>
			{#if vm.deploy}
				{vm.deploy.lastDeployText}
				{#if vm.deploy.lastDeployFailed}
					<span class={failed}>· {vm.deploy.lastDeployFailedText}</span>
				{/if}
			{:else if app.lastDeployedAt}
				{formatRelative(app.lastDeployedAt)}
			{:else}
				{m.catalog_app_no_deploys()}
			{/if}
		</span>
		<span class={chevron}><Icon name="chevronRight" /></span>
	</a>
</li>
