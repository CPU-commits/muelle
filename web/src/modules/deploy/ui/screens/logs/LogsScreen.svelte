<script lang="ts">
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { deployErrorMessage } from '../../components/labels/deployErrorMessage'
	import {
		Banner,
		Button,
		ComingSoon,
		EmptyState,
		ErrorState,
		LoadingState,
		PageHeader,
		SearchInput,
		Select,
		srOnly,
		StatusDot,
	} from '@/shared/ui'
	import { clockTimeMs } from '../../components/labels/deployLabels'
	import { LogViewport } from '../../components/logViewport'
	import { type LogsProps, LogsViewModel } from './Logs.viewModel.svelte.ts'
	import {
		box,
		levels,
		line,
		lineError,
		lines,
		message,
		note,
		page,
		placeholder,
		search,
		select,
		status,
		task,
		time,
		toolbar,
	} from './styles.css'

	let props: LogsProps = $props()

	const vm = new LogsViewModel(() => props)
</script>

<div class={page}>
	<PageHeader title={m.nav_app_logs()} subtitle={m.deploy_logs_subtitle()} />

	{#if vm.pendingPhase !== null}
		<ComingSoon phase={vm.pendingPhase} feature={m.deploy_logs_coming()} />
	{:else if vm.isLoading}
		<LoadingState label={m.deploy_logs_loading()} />
	{:else if vm.loadError}
		<ErrorState
			title={m.deploy_logs_error_title()}
			body={deployErrorMessage(vm.loadError)}
			retryLabel={m.common_retry()}
			onRetry={vm.retry}
		/>
	{:else if vm.noServices}
		<EmptyState
			icon="clock"
			title={m.deploy_logs_no_services_title()}
			body={m.deploy_logs_no_services_body()}
		/>
	{:else}
		<div class={toolbar}>
			<SearchInput
				class={search}
				label={m.deploy_logs_filter()}
				placeholder={m.deploy_logs_filter_placeholder()}
				bind:value={vm.query}
			/>
			<Select
				class={select}
				aria-label={m.deploy_logs_service()}
				value={vm.service ?? ''}
				onchange={(event) => vm.selectService(event.currentTarget.value)}
			>
				{#each vm.services as service (service)}
					<option value={service}>{service}</option>
				{/each}
			</Select>
			<Select
				class={select}
				aria-label={m.deploy_logs_task()}
				value={vm.task}
				onchange={(event) => vm.setTask(event.currentTarget.value)}
			>
				<option value="">{m.deploy_logs_all_tasks()}</option>
				{#each vm.tasks as taskName (taskName)}
					<option value={taskName}>{taskName}</option>
				{/each}
			</Select>
			<Select
				class={select}
				aria-label={m.deploy_logs_tail()}
				value={String(vm.tail)}
				onchange={(event) => vm.setTail(event.currentTarget.value)}
			>
				{#each vm.tailOptions as option (option)}
					<option value={String(option)}
						>{m.deploy_logs_tail_option({ count: option })}</option
					>
				{/each}
			</Select>
			<Button variant="secondary" onclick={vm.togglePause}>
				<StatusDot status={vm.paused ? 'idle' : 'ok'} pulse={!vm.paused} />
				{vm.paused ? m.deploy_logs_resume() : m.deploy_logs_pause()}
			</Button>
			<Button variant="ghost" onclick={vm.clear}>{m.deploy_logs_clear()}</Button>
		</div>

		<!-- El banner sigue montado entre intentos: se anuncia una vez, no en cada uno. -->
		{#if vm.problem}
			<Banner
				tone="warn"
				title={vm.problem === 'dropped'
					? m.deploy_logs_lost_title()
					: m.deploy_logs_unreachable_title()}
				body={m.deploy_logs_retrying_body()}
			>
				{#snippet action()}
					<Button
						variant="secondary"
						size="sm"
						loading={vm.connection === 'connecting'}
						onclick={vm.reconnect}
					>
						{m.deploy_logs_reconnect()}
					</Button>
				{/snippet}
			</Banner>
		{/if}

		{#if vm.connection === 'ended'}
			<Banner
				tone="info"
				title={m.deploy_logs_ended_title()}
				body={m.deploy_logs_ended_body()}
			>
				{#snippet action()}
					<Button variant="secondary" size="sm" onclick={vm.restart}>
						{m.deploy_logs_restart()}
					</Button>
				{/snippet}
			</Banner>
		{/if}

		<!-- Se anuncia una vez; la cuenta, que cambia con cada línea, queda fuera. -->
		<p class={srOnly} role="status">{vm.announcement}</p>
		<p class={status}>
			{#if vm.connection === 'connecting' && !vm.problem}
				<span>{m.deploy_logs_connecting()}</span>
			{/if}
			{#if vm.paused && vm.pending.length > 0}
				<span>{m.deploy_logs_new_lines({ count: vm.pending.length })}</span>
			{/if}
		</p>

		<div class={box}>
			<LogViewport
				label={m.deploy_logs_lines()}
				count={vm.visibleLines.length}
				follow={vm.follow}
				onFollowChange={vm.setFollow}
				size="lg"
			>
				{#if vm.visibleLines.length === 0}
					<p class={placeholder}>{vm.emptyText}</p>
				{:else}
					<ol class={lines}>
						{#each vm.visibleLines as keyed (keyed.key)}
							<li class={[line, keyed.line.level === 'error' && lineError]}>
								<span class={time}>
									{keyed.line.time ? clockTimeMs(keyed.line.time) : ''}
								</span>
								<span class={task} title={keyed.line.task ?? undefined}>
									{keyed.line.task ?? ''}
								</span>
								<span class={[message, levels[keyed.line.level]]}
									>{keyed.line.text}</span
								>
							</li>
						{/each}
					</ol>
				{/if}
			</LogViewport>
		</div>

		{#if vm.isCapped}
			<p class={note}>{m.deploy_logs_capped({ cap: vm.cap })}</p>
		{/if}
	{/if}
</div>
