<script lang="ts">
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { deployErrorMessage } from '../../components/labels/deployErrorMessage'
	import {
		Badge,
		Button,
		Card,
		ComingSoon,
		CopyButton,
		EmptyState,
		ErrorState,
		formatRelative,
		LoadingState,
		Mono,
		PageHeader,
		rowMuted,
		StatusDot,
		Table,
		TextLink,
	} from '@/shared/ui'
	import {
		serviceNodes,
		serviceUsage,
		shortDigest,
		shortTaskId,
		taskLabel,
	} from '../../../domain'
	import { MaskedText } from '../../components/maskedText'
	import { DeployDialog } from '../../features/deployDialog'
	import { RollbackDialog } from '../../features/rollbackDialog'
	import {
		actorName,
		healthLabel,
		healthTone,
		megabytesNumber,
		megabytesText,
		percentNumber,
		statusTone,
		taskStateText,
		taskTone,
	} from '../../components/labels/deployLabels'
	import { type SummaryProps, SummaryViewModel } from './Summary.viewModel.svelte.ts'
	import {
		actionRow,
		badgeLink,
		bigValue,
		deployLink,
		detail,
		detailsLine,
		digestRow,
		empty,
		inlineValue,
		live,
		metric,
		metrics,
		muted,
		ofTotal,
		page,
		sampledNote,
		taskError,
		taskId,
		taskState,
		term,
		unit,
	} from './styles.css'

	let props: SummaryProps = $props()

	const vm = new SummaryViewModel(() => props)
</script>

<div class={page}>
	<PageHeader title={props.app} titleSize="lg">
		{#snippet badges()}
			{#if vm.health}
				<Badge shape="pill" size="md" dot={healthTone(vm.health)}
					>{healthLabel(vm.health)}</Badge
				>
			{/if}
			{#if vm.pendingChanges}
				<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- llega ya resuelto (del viewModel o de la ruta) -->
				<a class={badgeLink} href={vm.pendingChangesHref}>
					<Badge tone="warn" shape="pill" size="md" dot
						>{m.deploy_pending_changes()}</Badge
					>
				</a>
			{/if}
		{/snippet}
		{#snippet details()}
			<p class={detailsLine}>
				{#if vm.kind}
					<span
						>{vm.kind === 'compose'
							? m.deploy_kind_compose()
							: m.deploy_kind_service()}</span
					>
				{/if}
				<span class={detail}
					>{m.deploy_stack()} <Mono tone="secondary">{vm.stack}</Mono></span
				>
				<span class={detail}
					>{m.deploy_network()} <Mono tone="secondary">{vm.network}</Mono></span
				>
			</p>
		{/snippet}
		{#snippet actions()}
			<div class={actionRow}>
				<Button variant="secondary" disabled={!vm.actionsEnabled} onclick={vm.openRollback}>
					{m.deploy_action_rollback()}
				</Button>
				<Button disabled={!vm.actionsEnabled} onclick={vm.openDeploy}>
					{m.deploy_action_deploy()}
				</Button>
			</div>
		{/snippet}
	</PageHeader>

	{#if vm.pendingPhase !== null}
		<ComingSoon phase={vm.pendingPhase} feature={m.deploy_summary_coming()} />
	{:else if vm.isLoading}
		<LoadingState label={m.deploy_summary_loading()} />
	{:else if vm.loadError}
		<ErrorState
			title={m.deploy_summary_error_title()}
			body={deployErrorMessage(vm.loadError)}
			retryLabel={m.common_retry()}
			onRetry={vm.refresh}
		/>
	{:else if vm.health === 'not_deployed'}
		<EmptyState
			icon="upload"
			title={m.deploy_summary_not_deployed_title()}
			body={m.deploy_summary_not_deployed_body()}
		>
			{#snippet action()}
				<Button onclick={vm.openDeploy}>{m.deploy_action_deploy()}</Button>
			{/snippet}
		</EmptyState>
	{:else}
		{#each vm.services as service, index (service.name)}
			{@const image = vm.imageOf(service.name)}
			{@const nodes = serviceNodes(service)}
			{@const usage = serviceUsage(service)}
			<Card
				flush
				title={m.deploy_service_title({ name: service.name })}
				description={vm.swarmName(service.name)}
			>
				{#snippet aside()}
					<span class={live}>
						<StatusDot
							status={vm.visible ? 'ok' : 'idle'}
							size={6}
							pulse={vm.visible}
						/>
						{vm.visible ? m.deploy_live() : m.deploy_live_paused()}
					</span>
				{/snippet}
				<dl class={metrics}>
					<div class={metric}>
						<dt class={term}>{m.deploy_replicas()}</dt>
						<dd class={bigValue}>
							{service.runningReplicas}
							<span class={ofTotal}>/ {service.desiredReplicas}</span>
						</dd>
					</div>
					<div class={metric}>
						<dt class={term}>{m.deploy_cpu()}</dt>
						<dd class={bigValue}>
							{#if usage.cpuPercent === null}
								{m.deploy_none()}
							{:else}
								{percentNumber(usage.cpuPercent)}
								<span class={unit}>{m.deploy_cpu_unit()}</span>
							{/if}
						</dd>
						{#if usage.sampled > 0 && usage.sampled < usage.current}
							<dd class={sampledNote}>
								{m.deploy_usage_sampled({
									sampled: usage.sampled,
									total: usage.current,
								})}
							</dd>
						{/if}
					</div>
					<div class={metric}>
						<dt class={term}>{m.deploy_memory()}</dt>
						<dd class={bigValue}>
							{#if usage.memoryPerTaskBytes === null}
								{m.deploy_none()}
							{:else}
								{megabytesNumber(usage.memoryPerTaskBytes)}
								<span class={unit}>
									{service.memoryLimitBytes === null
										? m.deploy_memory_unit_each()
										: m.deploy_memory_with_limit({
												limit: megabytesText(service.memoryLimitBytes),
											})}
								</span>
							{/if}
						</dd>
					</div>
					{#if index === 0}
						<div class={metric}>
							<dt class={term}>{m.deploy_last_deploy()}</dt>
							<dd class={inlineValue}>
								{#if vm.last}
									<StatusDot status={statusTone(vm.last.status)} />
									<TextLink class={deployLink} href={vm.deployHref(vm.last.id)}
										>{m.deploy_id({ id: vm.last.id })}</TextLink
									>
									<span class={muted}>
										{m.deploy_last_deploy_meta({
											when: formatRelative(vm.last.createdAt),
											actor: actorName(vm.last),
										})}
									</span>
								{:else}
									<span class={muted}>{m.deploy_none()}</span>
								{/if}
							</dd>
						</div>
					{/if}
					<div class={metric}>
						<dt class={term}>{m.deploy_image()}</dt>
						<dd class={inlineValue}>
							{#if image}
								<Mono size={13} truncate title={image.ref}>{image.ref}</Mono>
							{:else}
								<span class={muted}>{m.deploy_none()}</span>
							{/if}
						</dd>
					</div>
					<div class={metric}>
						<dt class={term}>{m.deploy_digest()}</dt>
						<dd class={digestRow}>
							{#if image?.digest}
								<Mono size={13} truncate title={image.digest}
									>{shortDigest(image.digest)}</Mono
								>
								<CopyButton
									value={image.digest}
									label={m.deploy_copy_digest()}
									iconOnly
									variant="ghost"
									size="xs"
								/>
							{:else}
								<span class={muted}>{m.deploy_none()}</span>
							{/if}
						</dd>
					</div>
					{#if vm.healthcheck && index === 0}
						<div class={metric}>
							<dt class={term}>{m.deploy_healthcheck()}</dt>
							<dd class={inlineValue}><Mono size={13}>{vm.healthcheck}</Mono></dd>
						</div>
					{/if}
					<div class={metric}>
						<dt class={term}>{m.deploy_node()}</dt>
						<dd class={inlineValue}>
							<Mono size={13}
								>{nodes.length > 0 ? nodes.join(', ') : m.deploy_none()}</Mono
							>
						</dd>
					</div>
				</dl>
			</Card>
		{/each}

		<Card
			flush
			title={m.deploy_tasks_title()}
			description={m.deploy_tasks_meta({ running: vm.runningTasks, history: vm.pastTasks })}
		>
			{#if vm.tasks.length === 0}
				<p class={empty}>{m.deploy_tasks_empty()}</p>
			{:else}
				<Table framed={false} minWidth="640px" caption={m.deploy_tasks_caption()}>
					<thead>
						<tr>
							<th scope="col">{m.deploy_task()}</th>
							<th scope="col">{m.deploy_task_state()}</th>
							<th scope="col">{m.deploy_task_node()}</th>
							<th scope="col">{m.deploy_task_since()}</th>
							<th scope="col">{m.deploy_task_error()}</th>
						</tr>
					</thead>
					<tbody>
						{#each vm.tasks as row (row.key)}
							<tr class={row.current ? undefined : rowMuted}>
								<td>
									<Mono>
										{taskLabel(row.service, row.task)}
										<span class={taskId}>{shortTaskId(row.task.id)}</span>
									</Mono>
								</td>
								<td>
									<span class={taskState}>
										<StatusDot
											status={taskTone(row.task.state, row.task.health)}
										/>
										{taskStateText(row.task.state, row.task.health)}
									</span>
								</td>
								<td
									><Mono tone={row.current ? 'secondary' : 'inherit'}
										>{row.task.node ?? m.deploy_none()}</Mono
									></td
								>
								<td
									>{row.task.since
										? formatRelative(row.task.since)
										: m.deploy_none()}</td
								>
								<td class={row.task.error ? taskError : undefined}>
									<!-- Llega enmascarado como el log (contrato §2.8): puede traer ***. -->
									{#if row.task.error}
										<MaskedText text={row.task.error} />
									{:else}
										{m.deploy_none()}
									{/if}
								</td>
							</tr>
						{/each}
					</tbody>
				</Table>
			{/if}
		</Card>
	{/if}
</div>

<DeployDialog
	open={vm.deployOpen}
	app={vm.ref}
	kind={vm.kind}
	services={vm.deployServices}
	currentImages={vm.currentImages}
	rollbackPin={vm.rollbackPin}
	onDeployed={() => void vm.onQueued()}
	onClose={vm.closeDeploy}
/>

<RollbackDialog
	open={vm.rollbackOpen}
	app={vm.ref}
	target={vm.rollbackTarget}
	historyComplete={vm.historyComplete}
	onDone={() => void vm.onQueued()}
	onClose={vm.closeRollback}
/>
