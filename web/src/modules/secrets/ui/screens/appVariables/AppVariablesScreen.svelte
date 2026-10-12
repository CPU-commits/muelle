<script lang="ts">
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import {
		Banner,
		Button,
		EmptyState,
		ErrorState,
		Icon,
		LinkButton,
		LoadingState,
		PageHeader,
	} from '@/shared/ui'
	import { refocusOnClose } from '../../components/refocus'
	import { EnvAddCard, EnvDialogs, EnvVarList } from '../../features/envEditor'
	import { EnvImportPanel } from '../../features/envImport'
	import { InheritedEnvList } from '../../features/inheritedEnv'
	import { AppVariablesViewModel } from './AppVariables.viewModel.svelte.ts'
	import { page } from './styles.css'

	// App-Variables.dc.html.
	let { project, stage, app }: { project: string; stage: string; app: string } = $props()

	const vm = new AppVariablesViewModel(() => ({ project, stage, app }))
	const importId = $props.id()
</script>

<div class={page}>
	<PageHeader title={m.secrets_title()} subtitle={m.secrets_app_subtitle()}>
		{#snippet actions()}
			<span {@attach refocusOnClose(() => vm.importer.closedCount)}>
				<Button
					variant="secondary"
					aria-expanded={vm.importer.open}
					aria-controls={vm.importer.open ? importId : undefined}
					disabled={!vm.editor.hasLoaded}
					onclick={vm.importer.toggle}
				>
					<Icon name="download" />
					{m.secrets_import_toggle()}
				</Button>
			</span>
		{/snippet}
	</PageHeader>

	{#if vm.editor.isLoading}
		<LoadingState label={m.secrets_loading()} />
	{:else if vm.editor.isNotFound}
		<EmptyState
			icon="alert"
			title={m.secrets_not_found_app()}
			body={m.secrets_not_found_body()}
		>
			{#snippet action()}
				<LinkButton href={vm.homeHref} variant="secondary" size="sm">
					{m.secrets_go_home()}
				</LinkButton>
			{/snippet}
		</EmptyState>
	{:else if vm.editor.loadError}
		<ErrorState
			title={m.secrets_load_error()}
			body={vm.editor.loadErrorMessage}
			retryLabel={m.common_retry()}
			onRetry={vm.editor.retry}
		/>
	{:else}
		{#if vm.hasPendingChanges}
			<Banner
				dot="warn"
				title={m.secrets_pending_app_title()}
				body={m.secrets_pending_app_body()}
			>
				{#snippet action()}
					<LinkButton href={vm.summaryHref} size="sm"
						>{m.secrets_pending_deploy()}</LinkButton
					>
				{/snippet}
			</Banner>
		{:else}
			<Banner
				dot="idle"
				title={m.secrets_pending_app_idle_title()}
				body={m.secrets_pending_app_idle_body()}
			/>
		{/if}

		<EnvImportPanel vm={vm.importer} id={importId} />
		<EnvAddCard vm={vm.editor} />
		<EnvVarList
			vm={vm.editor}
			title={m.secrets_list_app_title()}
			emptyTitle={m.secrets_empty_app_title()}
			emptyBody={m.secrets_empty_app_body()}
		/>
		<InheritedEnvList
			stage={vm.stage}
			vars={vm.stageVars}
			overridden={vm.overriddenKeys}
			href={vm.stageHref}
			loading={vm.stageLoading}
			error={vm.stageError}
			onRetry={vm.retryStage}
		/>
	{/if}
</div>

<EnvDialogs vm={vm.editor} />
