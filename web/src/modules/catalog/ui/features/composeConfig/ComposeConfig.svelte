<script lang="ts">
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { Badge, Button, Card, Mono, StatusDot } from '@/shared/ui'
	import type { AppSummary, ComposeApp } from '../../../domain'
	import { composeServiceDetails, composeServiceWarning } from '../../components/catalogMessages'
	import { ConfigHeader } from '../../components/configHeader'
	import { YamlEditor } from '../../components/yamlEditor'
	import { DeleteAppCard } from '../deleteApp'
	import { ComposeConfigViewModel } from './ComposeConfig.viewModel.svelte.ts'
	import {
		actions as actionsClass,
		aside,
		barEnd,
		editor,
		editorBar,
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
			{#if vm.check.kind === 'syntax'}
				{@const count = vm.check.lines.length}
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
						{#each vm.check.lines as line (line)}
							<li>
								<button
									type="button"
									class={lineLink}
									aria-label={m.catalog_compose_go_to_line({ line })}
									onclick={() => vm.goToLine(line)}
								>
									{m.catalog_compose_line({ line })}
								</button>
								· {m.catalog_compose_syntax_error()}
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
				<YamlEditor
					value={vm.draft}
					label={m.catalog_compose_editor_label()}
					describedBy={`${ids}-hint`}
					errorLines={vm.errorLines}
					focusRequest={vm.focusRequest}
					onChange={vm.setDraft}
				/>
				<p id={`${ids}-hint`} class={hint}>{m.catalog_compose_editor_hint()}</p>
			</section>

			<DeleteAppCard appRef={vm.ref} id="c-del" />
		</div>

		<aside class={aside}>
			<Card flush title={m.catalog_compose_services()}>
				{#if vm.services.length === 0}
					<p class={servicesEmpty}>{m.catalog_compose_services_empty()}</p>
				{:else}
					<ul class={serviceList}>
						{#each vm.services as item (item.name)}
							{@const warning = composeServiceWarning(item)}
							<li class={service}>
								<span class={serviceHead}>
									<span class={serviceName}>{item.name}</span>
									{#if warning}
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
</div>
