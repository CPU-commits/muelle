<script lang="ts">
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { Badge, Button, Card, Mono, Spinner, StatusDot, Textarea } from '@/shared/ui'
	import type { AppSummary, ComposeApp } from '../../../domain'
	import { composeServiceDetails, composeServiceWarning } from '../../components/catalogMessages'
	import { ConfigHeader } from '../../components/configHeader'
	import { DeleteAppCard } from '../deleteApp'
	import { ComposeConfigViewModel } from './ComposeConfig.viewModel.svelte.ts'
	import {
		actions as actionsClass,
		aside,
		barEnd,
		editor,
		editorBar,
		editorFallback,
		editorLoading,
		errors,
		errorsList,
		errorsTitle,
		fileName,
		hint,
		injectionNote,
		injections,
		injectionTerm,
		injectionValue,
		layout,
		lineLink,
		main,
		page,
		service,
		serviceDetails,
		serviceError,
		serviceHead,
		serviceList,
		serviceName,
		servicesEmpty,
		serviceWarning,
	} from './styles.css'

	// Quien la monta la pone en un `{#key}` por app: el borrador arranca con su YAML.
	let { app, summary }: { app: ComposeApp; summary: AppSummary } = $props()

	const vm = new ComposeConfigViewModel(() => app)
	const ids = $props.id()
</script>

<div class={page}>
	<ConfigHeader {summary}>
		{#snippet subtitle()}
			{m.catalog_compose_subtitle({ variables: '${VARIABLES}' })}
		{/snippet}
		{#snippet actions()}
			<div class={actionsClass}>
				<Button variant="secondary" onclick={vm.validate}
					>{m.catalog_compose_validate()}</Button
				>
				<Button loading={vm.isSaving} onclick={vm.save}>{m.common_save()}</Button>
			</div>
		{/snippet}
	</ConfigHeader>

	<div class={layout}>
		<div class={main}>
			{#if vm.check.kind === 'problems'}
				{@const count = vm.check.problems.length}
				<div class={errors} role="alert">
					<p class={errorsTitle}>
						<StatusDot status="bad" />
						{#if vm.check.onSave}
							{count === 1
								? m.catalog_compose_not_saved_one()
								: m.catalog_compose_not_saved_other({ count })}
						{:else}
							{count === 1
								? m.catalog_compose_check_one()
								: m.catalog_compose_check_other({ count })}
						{/if}
					</p>
					<ul class={errorsList}>
						{#each vm.check.problems as problem, index (index)}
							<li>
								<button
									type="button"
									class={lineLink}
									aria-label={m.catalog_compose_go_to_line({
										line: problem.line,
									})}
									onclick={() => vm.goToLine(problem.line)}
								>
									{m.catalog_compose_line({ line: problem.line })}
								</button>
								·
								{problem.kind === 'syntax'
									? m.catalog_compose_syntax_error()
									: m.catalog_compose_duplicate_service({
											service: problem.service,
										})}
							</li>
						{/each}
					</ul>
				</div>
			{:else if vm.check.kind === 'rejected'}
				<div class={errors} role="alert">
					<p class={errorsTitle}>
						<StatusDot status="bad" />
						{m.catalog_config_save_error()}
					</p>
					<p class={errorsList}>{vm.check.message}</p>
				</div>
			{/if}

			<section class={editor} aria-labelledby={`${ids}-file`}>
				<div class={editorBar}>
					<h2 id={`${ids}-file`} class={fileName}>{m.catalog_compose_file()}</h2>
					<div class={barEnd}>
						{#if vm.isDirty}
							<Badge tone="warn" shape="pill" dot>{m.catalog_compose_unsaved()}</Badge
							>
							<Button variant="ghost" size="xs" onclick={vm.discard}>
								{m.catalog_compose_discard()}
							</Button>
						{/if}
						<span>
							{vm.lines === 1
								? m.catalog_compose_lines_one()
								: m.catalog_compose_lines_other({ count: vm.lines })}
						</span>
					</div>
				</div>
				{#await vm.editor}
					<div class={editorLoading}>
						<Spinner size="sm" label={m.catalog_compose_editor_loading()} />
					</div>
				{:then { default: YamlEditor }}
					<YamlEditor
						value={vm.draft}
						label={m.catalog_compose_editor_label()}
						description={m.catalog_compose_editor_hint()}
						errorLines={vm.errorLines}
						focusRequest={vm.focusRequest}
						onChange={vm.setDraft}
					/>
				{:catch}
					<!-- Sin el editor (no se pudo bajar el chunk), el YAML se edita igual. -->
					<div class={editorFallback}>
						<p>{m.catalog_compose_editor_failed()}</p>
						<Textarea
							mono
							rows={20}
							aria-label={m.catalog_compose_editor_label()}
							aria-describedby={`${ids}-hint`}
							spellcheck={false}
							value={vm.draft}
							oninput={(event) => vm.setDraft(event.currentTarget.value)}
						/>
					</div>
				{/await}
				<p id={`${ids}-hint`} class={hint}>{m.catalog_compose_editor_hint()}</p>
			</section>
		</div>

		<aside class={aside}>
			<Card flush title={m.catalog_compose_services()}>
				{#if vm.services.length === 0}
					<p class={servicesEmpty}>{m.catalog_compose_services_empty()}</p>
				{:else}
					<ul class={serviceList}>
						<!-- Por índice: un nombre repetido (un error del YAML) no puede romper la lista. -->
						{#each vm.services as item, index (index)}
							{@const warning = composeServiceWarning(item)}
							<li class={service}>
								<span class={serviceHead}>
									<span class={serviceName}>{item.name}</span>
									{#if item.duplicate}
										<span class={serviceError}
											>{m.catalog_compose_duplicate_short()}</span
										>
									{:else if warning}
										<span class={serviceWarning}>{warning}</span>
									{/if}
								</span>
								<span class={serviceDetails}>{composeServiceDetails(item)}</span>
							</li>
						{/each}
					</ul>
				{/if}
			</Card>

			<Card flush title={m.catalog_compose_injections()}>
				<dl class={injections}>
					<dt class={injectionTerm}>{m.catalog_compose_inj_network()}</dt>
					<dd class={injectionValue}><Mono>{summary.stageNetwork}</Mono></dd>
					<dt class={injectionTerm}>{m.catalog_compose_inj_images()}</dt>
					<dd class={injectionValue}>{m.catalog_compose_inj_images_value()}</dd>
					<dt class={injectionTerm}>{m.catalog_compose_inj_update()}</dt>
					<dd class={injectionValue}>{m.catalog_compose_inj_update_value()}</dd>
					<dt class={injectionTerm}>{m.catalog_compose_inj_logs()}</dt>
					<dd class={injectionValue}>{m.catalog_compose_inj_logs_value()}</dd>
					<dt class={injectionTerm}>{m.catalog_compose_inj_labels()}</dt>
					<dd class={injectionValue}><Mono>muelle.*</Mono></dd>
				</dl>
				<p class={injectionNote}>{m.catalog_compose_inj_note()}</p>
			</Card>
		</aside>
	</div>

	<DeleteAppCard appRef={vm.ref} id="c-del" />
</div>
