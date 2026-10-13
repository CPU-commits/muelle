<script lang="ts">
	import type { Snippet } from 'svelte'
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import {
		Button,
		ConfirmDialog,
		EmptyState,
		ErrorState,
		Icon,
		LoadingState,
		Menu,
		Modal,
		Mono,
		PageHeader,
		SearchInput,
	} from '@/shared/ui'
	import { type App, projectInitial, type Stage, stageNetwork } from '../../../domain'
	import { AppRow } from '../../components/appRow'
	import {
		NewAppDialog,
		NewProjectDialog,
		NewStageDialog,
		RenameProjectDialog,
	} from '../../features/projectDialogs'
	import { ProjectsViewModel } from './Projects.viewModel.svelte.ts'
	import {
		apps,
		card,
		emptyRow,
		initial,
		meta,
		project as projectClass,
		projectHead,
		projectName,
		projects,
		projectTitle,
		search,
		stageActions,
		stageName,
		stageRow,
		stageRowNext,
		stageTitle,
		toolbar,
		variablesLink,
	} from './styles.css'

	// Lo de la Fase 2 entra por snippets, para que F2 los llene con el módulo `deploy` sin
	// tocar esta pantalla: el punto y el texto del estado ("Corriendo · 2/2"), el último
	// deploy ("#143 · hace 12 min") y la cantidad de variables del stage (de `secrets`).
	type Props = {
		appStatusDot?: Snippet<[App]>
		appStatus?: Snippet<[App]>
		appLastDeploy?: Snippet<[App]>
		stageVariablesCount?: Snippet<[Stage]>
	}

	let { appStatusDot, appStatus, appLastDeploy, stageVariablesCount }: Props = $props()

	const vm = new ProjectsViewModel()
</script>

<PageHeader title={m.nav_projects()} {...vm.hasProjects ? { meta: vm.appsMeta } : {}}>
	{#snippet actions()}
		<div class={toolbar}>
			<SearchInput
				class={search}
				label={m.catalog_search_label()}
				placeholder={m.catalog_search_placeholder()}
				value={vm.search}
				oninput={(event) => vm.setSearch(event.currentTarget.value)}
			/>
			<Button onclick={vm.openNewProject}>
				<Icon name="plus" />
				{m.catalog_new_project()}
			</Button>
		</div>
	{/snippet}
</PageHeader>

{#if vm.isLoading}
	<LoadingState label={m.catalog_loading()} />
{:else if vm.loadError}
	<ErrorState
		title={m.catalog_load_error()}
		body={vm.loadError}
		retryLabel={m.common_retry()}
		onRetry={vm.retry}
	/>
{:else if !vm.hasProjects}
	<EmptyState icon="plus" title={m.catalog_empty_title()} body={m.catalog_empty_body()}>
		{#snippet action()}
			<Button size="sm" onclick={vm.openNewProject}>{m.catalog_new_project()}</Button>
		{/snippet}
	</EmptyState>
{:else if vm.noMatches}
	<EmptyState
		icon="search"
		title={m.catalog_search_empty_title({ query: vm.search.trim() })}
		body={m.catalog_search_empty_body()}
	>
		{#snippet action()}
			<Button variant="secondary" size="sm" onclick={vm.clearSearch}>
				{m.catalog_search_clear()}
			</Button>
		{/snippet}
	</EmptyState>
{:else}
	<div class={projects}>
		{#each vm.projects as node (node.project.slug)}
			{@const slug = node.project.slug}
			<section id={`p-${slug}`} class={projectClass} aria-labelledby={`p-${slug}-title`}>
				<div class={projectHead}>
					<div class={projectTitle}>
						<span class={initial} aria-hidden="true"
							>{projectInitial(node.project)}</span
						>
						<h2 id={`p-${slug}-title`} class={projectName}>{node.project.name}</h2>
						{#if node.project.name !== slug}
							<Mono tone="tertiary" size={13}>{slug}</Mono>
						{/if}
						<span class={meta}>{vm.projectMeta(node)}</span>
					</div>
					<Menu
						label={m.catalog_project_options({ project: slug })}
						items={vm.projectMenu(node)}
					/>
				</div>

				<div class={card}>
					{#each node.stages as stageNode, index (stageNode.stage.slug)}
						{@const stage = stageNode.stage}
						<div class={[stageRow, index > 0 && stageRowNext]}>
							<div class={stageTitle}>
								<h3 class={stageName}>{stage.slug}</h3>
								<Mono tone="tertiary" size={12} title={m.catalog_stage_network()}>
									{stageNetwork(stage)}
								</Mono>
							</div>
							<div class={stageActions}>
								<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- el href llega resuelto desde el viewModel -->
								<a class={variablesLink} href={vm.stageVariablesHref(stageNode)}>
									{m.catalog_stage_variables()}
									{#if stageVariablesCount}
										<span aria-hidden="true">·</span>
										{@render stageVariablesCount(stage)}
									{/if}
								</a>
								<Button
									variant="ghost"
									size="xs"
									iconOnly
									aria-label={m.catalog_stage_new_app({
										project: slug,
										stage: stage.slug,
									})}
									onclick={() => vm.openNewApp(slug, stage.slug)}
								>
									<Icon name="plus" size={15} />
								</Button>
								<Menu
									label={m.catalog_stage_options({
										project: slug,
										stage: stage.slug,
									})}
									items={vm.stageMenu(slug, stageNode)}
								/>
							</div>
						</div>
						{#if stageNode.apps.length > 0}
							<ul
								class={apps}
								aria-label={m.catalog_stage_apps({
									project: slug,
									stage: stage.slug,
								})}
							>
								{#each stageNode.apps as app (app.slug)}
									<AppRow
										{app}
										href={vm.appHref(app)}
										statusDot={appStatusDot}
										status={appStatus}
										lastDeploy={appLastDeploy}
									/>
								{/each}
							</ul>
						{:else}
							<p class={emptyRow}>
								{m.catalog_stage_no_apps()}
								<Button
									variant="secondary"
									size="xs"
									onclick={() => vm.openNewApp(slug, stage.slug)}
								>
									<Icon name="plus" size={14} />
									{m.catalog_stage_add_app()}
								</Button>
							</p>
						{/if}
					{:else}
						<p class={emptyRow}>
							{m.catalog_project_no_stages()}
							<Button
								variant="secondary"
								size="xs"
								onclick={() => vm.openNewStage(slug)}
							>
								<Icon name="plus" size={14} />
								{m.catalog_project_new_stage()}
							</Button>
						</p>
					{/each}
				</div>
			</section>
		{/each}
	</div>
{/if}

{#if vm.dialog?.kind === 'newProject'}
	<NewProjectDialog onClose={vm.closeDialog} />
{:else if vm.dialog?.kind === 'renameProject'}
	<RenameProjectDialog project={vm.dialog.project} onClose={vm.closeDialog} />
{:else if vm.dialog?.kind === 'newStage'}
	<NewStageDialog project={vm.dialog.project} onClose={vm.closeDialog} />
{:else if vm.dialog?.kind === 'newApp'}
	<NewAppDialog project={vm.dialog.project} stage={vm.dialog.stage} onClose={vm.closeDialog} />
{/if}

<!-- Todavía no está vacío: no hay nada que confirmar, solo qué hacer antes. -->
<Modal
	open={vm.confirmation?.blocked === true}
	title={vm.confirmation?.title ?? ''}
	description={vm.confirmation?.body ?? ''}
	size="sm"
	onClose={vm.closeDialog}
>
	{#snippet footer()}
		<Button variant="secondary" size="sm" onclick={vm.closeDialog}>{m.common_close()}</Button>
	{/snippet}
</Modal>

<ConfirmDialog
	open={vm.confirmation?.blocked === false}
	title={vm.confirmation?.title ?? ''}
	body={vm.confirmation?.body ?? ''}
	confirmLabel={m.common_delete()}
	cancelLabel={m.common_cancel()}
	loading={vm.isDeleting}
	error={vm.deleteError}
	onConfirm={vm.confirmDelete}
	onCancel={vm.closeDialog}
/>
