<script lang="ts">
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import {
		Banner,
		Button,
		ComingSoon,
		CopyButton,
		EmptyState,
		ErrorState,
		formatRelative,
		LinkButton,
		LoadingState,
		PageHeader,
		srOnly,
		StatusDot,
	} from '@/shared/ui'
	import { imageTag } from '../../../domain'
	import {
		DeployStatusDot,
		DeployStatusLabel,
		deployStatusTextTones,
	} from '../../components/deployStatusLabel'
	import {
		clockTime,
		durationText,
		sentence,
		statusLabel,
		triggerText,
	} from '../../components/labels/deployLabels'
	import { deployErrorMessage } from '../../components/labels/deployErrorMessage'
	import { LogViewport } from '../../components/logViewport'
	import { MaskedText } from '../../components/maskedText'
	import { DeployDialog } from '../../features/deployDialog'
	import { type DeploysProps, DeploysViewModel } from './Deploys.viewModel.svelte.ts'
	import {
		cursor,
		failure,
		failureDot,
		failureText,
		history,
		historyList,
		layout,
		line,
		lines,
		logErrorSlot,
		logHeader,
		logMeta,
		logPanel,
		logSub,
		logTitle,
		monoTag,
		notice,
		page,
		placeholder,
		row,
		rowBottom,
		rowId,
		rowLabel,
		rowTop,
		rowWhen,
		text,
		time,
		tones,
	} from './styles.css'

	let props: DeploysProps = $props()

	const vm = new DeploysViewModel(() => props)
	const logTitleId = $props.id()
</script>

<div class={page}>
	<p class={srOnly} role="status">{vm.announcement}</p>
	<PageHeader title={m.nav_app_deploys()} subtitle={m.deploy_deploys_subtitle()}>
		{#snippet actions()}
			<Button disabled={!vm.deploys.isSuccess} onclick={vm.openDeploy}>
				{m.deploy_action_deploy()}
			</Button>
		{/snippet}
	</PageHeader>

	{#if vm.pendingPhase !== null}
		<ComingSoon phase={vm.pendingPhase} feature={m.deploy_deploys_coming()} />
	{:else if vm.isLoading}
		<LoadingState label={m.deploy_deploys_loading()} />
	{:else if vm.loadError}
		<ErrorState
			title={m.deploy_deploys_error_title()}
			body={deployErrorMessage(vm.loadError)}
			retryLabel={m.common_retry()}
			onRetry={vm.retry}
		/>
	{:else if vm.isEmpty}
		<EmptyState
			icon="upload"
			title={m.deploy_deploys_empty_title()}
			body={m.deploy_deploys_empty_body()}
		>
			{#snippet action()}
				<Button onclick={vm.openDeploy}>{m.deploy_action_deploy()}</Button>
			{/snippet}
		</EmptyState>
	{:else}
		{#if vm.linkedMissing !== null}
			<Banner tone="warn" title={m.deploy_linked_missing({ id: vm.linkedMissing })} />
		{/if}
		<div class={layout}>
			<section class={history} aria-label={m.deploy_history()}>
				<ul class={historyList}>
					{#each vm.history as deploy (deploy.id)}
						<li>
							<button
								type="button"
								class={row}
								aria-pressed={vm.isSelected(deploy)}
								onclick={() => vm.select(deploy.id)}
							>
								<span class={rowTop}>
									<DeployStatusDot status={deploy.status} />
									<span class={rowId}>{m.deploy_id({ id: deploy.id })}</span>
									<span class={[rowLabel, deployStatusTextTones[deploy.status]]}>
										{deploy.supersededBy
											? m.deploy_superseded_by({ id: deploy.supersededBy })
											: statusLabel(deploy.status)}
									</span>
									<span class={rowWhen}>{formatRelative(deploy.createdAt)}</span>
								</span>
								<span class={rowBottom}>
									{#if deploy.images.length > 0}
										<span class={monoTag}
											>{deploy.images
												.map((image) => imageTag(image.ref))
												.join(', ')}</span
										>
										·
									{/if}
									{triggerText(deploy)}
								</span>
							</button>
						</li>
					{/each}
				</ul>
			</section>

			{#if vm.selected && vm.selectedStatus}
				<section class={logPanel} aria-labelledby={logTitleId} {@attach vm.revealOnSelect}>
					<div class={logHeader}>
						<div class={logMeta}>
							<h2 id={logTitleId} class={logTitle}>
								{m.deploy_id({ id: vm.selected.id })}
							</h2>
							<DeployStatusLabel status={vm.selectedStatus} variant="pill" />
							<span class={logSub}>
								{triggerText(vm.selected)} · {durationText(vm.durationMs)}
							</span>
						</div>
						<CopyButton value={vm.logText} variant="ghost" size="sm" />
					</div>

					{#if vm.selectedStatus === 'failed' && vm.selected.error}
						<div class={failure} role="alert">
							<span class={failureDot}><StatusDot status="bad" /></span>
							<p class={failureText}>{sentence(vm.selected.error)}</p>
						</div>
					{/if}
					{#if vm.proxyWarning?.kind === 'failing'}
						<!-- El paso 7 no trae el motivo (lo lee un token del proyecto): sale de
						     GET /proxy, que solo ve la sesión (contrato §2.11). -->
						<Banner
							tone="warn"
							title={m.deploy_proxy_warning_title()}
							body={vm.proxyWarning.error
								? m.deploy_proxy_warning_body({ error: vm.proxyWarning.error })
								: m.deploy_proxy_warning_generic()}
						>
							{#snippet action()}
								<LinkButton href={vm.domainsHref} variant="secondary" size="sm">
									{m.deploy_proxy_warning_domains()}
								</LinkButton>
							{/snippet}
						</Banner>
					{:else if vm.proxyWarning?.kind === 'synced'}
						<Banner tone="info" dot="ok" title={m.deploy_proxy_warning_synced()} />
					{/if}
					{#if vm.polling && vm.isActive}
						<p class={notice} role="status">{m.deploy_log_polling()}</p>
					{/if}

					<LogViewport
						label={m.deploy_log({ id: vm.selected.id })}
						count={vm.log.lines.length}
						follow={vm.follow}
						onFollowChange={vm.setFollow}
					>
						{#if vm.logError}
							<div class={logErrorSlot}>
								<ErrorState
									title={m.deploy_log_error_title()}
									body={deployErrorMessage(vm.logError)}
									retryLabel={m.common_retry()}
									onRetry={vm.retryLog}
								/>
							</div>
						{:else if vm.log.lines.length === 0}
							<p class={placeholder}>
								{vm.logLoading ? m.deploy_log_loading() : m.deploy_log_empty()}
							</p>
						{:else}
							<ol class={lines}>
								{#each vm.log.lines as logLine (logLine.n)}
									<li class={line}>
										<span class={time}>
											{logLine.time ? clockTime(logLine.time) : ''}
										</span>
										<span class={[text, tones[logLine.tone]]}>
											<MaskedText text={logLine.text} />
										</span>
									</li>
								{/each}
								{#if vm.isActive}
									<li class={line}>
										<span class={time}></span>
										<span class={cursor} aria-hidden="true">▍</span>
										<span class={srOnly}>{m.deploy_log_writing()}</span>
									</li>
								{/if}
							</ol>
						{/if}
					</LogViewport>
				</section>
			{/if}
		</div>
	{/if}
</div>

<DeployDialog
	open={vm.deployOpen}
	app={vm.ref}
	kind={vm.kind}
	services={vm.deployServices}
	currentImages={vm.currentImages}
	rollbackPin={vm.rollbackPin}
	onDeployed={vm.onDeployed}
	onClose={vm.closeDeploy}
/>
