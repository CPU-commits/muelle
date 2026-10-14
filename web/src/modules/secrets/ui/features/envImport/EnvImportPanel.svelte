<script lang="ts">
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { Banner, Button, Checkbox, ConfirmDialog, RadioGroup, Textarea } from '@/shared/ui'
	import { changeReasonLabel } from '../../components/secretsMessages'
	import type { EnvImportViewModel } from './EnvImport.viewModel.svelte.ts'
	import {
		actions,
		body,
		changeItem,
		footer,
		group,
		groupHint,
		groupKeys,
		groupLabel,
		groups,
		hint,
		issueList,
		issues,
		issuesTitle,
		key,
		keyRemoved,
		live,
		muted,
		notes,
		options,
		panel,
		preview,
		previewTitle,
		reason,
		summary,
		textarea,
		title,
	} from './styles.css'

	// El panel "Importar .env" (App-Variables.dc.html): el texto, el modo, la marca de
	// secretas y, mientras se escribe, los problemas por línea o los cambios que se aplican.
	// `id` es el del panel: el botón que lo abre lo usa en `aria-controls`.
	let { vm, id }: { vm: EnvImportViewModel; id: string } = $props()

	const uid = $props.id()
</script>

{#snippet keyGroup(label: string, keys: readonly string[], note?: string, removed = false)}
	<div class={group}>
		<dt class={groupLabel}>{label}</dt>
		<dd class={groupKeys}>
			{#each keys as name (name)}
				<span class={[key, removed && keyRemoved]}>{name}</span>
			{/each}
			{#if note}
				<span class={groupHint}>{note}</span>
			{/if}
		</dd>
	</div>
{/snippet}

{#if vm.open}
	<section {id} class={panel} aria-labelledby={`${uid}-title`}>
		<div class={body}>
			<h2 id={`${uid}-title`} class={title}>{m.secrets_import_title()}</h2>
			<Textarea
				class={textarea}
				mono
				rows={6}
				aria-label={m.secrets_import_label()}
				aria-describedby={`${uid}-hint`}
				placeholder={m.secrets_import_placeholder()}
				autocomplete="off"
				spellcheck={false}
				value={vm.text}
				disabled={vm.isSubmitting}
				oninput={(event) => vm.setText(event.currentTarget.value)}
			/>
			<p id={`${uid}-hint`} class={hint}>{m.secrets_import_hint()}</p>

			<div class={options}>
				<RadioGroup
					label={m.secrets_import_mode()}
					name={`${uid}-mode`}
					options={vm.modeOptions}
					value={vm.mode}
					disabled={vm.isSubmitting}
					onChange={vm.setMode}
				/>
				<Checkbox
					label={m.secrets_import_secret()}
					checked={vm.secret}
					disabled={vm.isSubmitting}
					onchange={(event) => vm.setSecret(event.currentTarget.checked)}
				/>
			</div>

			<!-- Siempre en el DOM: un lector de pantalla anuncia los problemas cuando aparecen. -->
			<div class={live} aria-live="polite">
				{#if vm.issueMessages.length > 0}
					<div class={issues}>
						<p class={issuesTitle}>
							{m.secrets_import_issues({ count: vm.issueMessages.length })}
						</p>
						<ul class={issueList}>
							{#each vm.issueMessages as message, index (index)}
								<li>{message}</li>
							{/each}
						</ul>
					</div>
				{/if}

				{#if vm.swallowedMessages.length > 0 || vm.duplicateMessages.length > 0}
					<ul class={notes}>
						{#each vm.swallowedMessages as message, index (index)}
							<li>{message}</li>
						{/each}
						{#each vm.duplicateMessages as message, index (index)}
							<li>{message}</li>
						{/each}
					</ul>
				{/if}
			</div>

			{#if vm.showPlan}
				<section class={preview} aria-labelledby={`${uid}-preview`}>
					<h3 id={`${uid}-preview`} class={previewTitle}>{m.secrets_import_preview()}</h3>
					{#if vm.plan.patch === null}
						<p class={muted}>{m.secrets_import_nothing()}</p>
					{:else}
						<dl class={groups}>
							{#if vm.plan.added.length > 0}
								{@render keyGroup(
									m.secrets_import_added({ count: vm.plan.added.length }),
									vm.plan.added,
								)}
							{/if}
							{#if vm.plan.changed.length > 0}
								<div class={group}>
									<dt class={groupLabel}>
										{m.secrets_import_changed({
											count: vm.plan.changed.length,
										})}
									</dt>
									<dd class={groupKeys}>
										{#each vm.plan.changed as change (change.key)}
											<span class={changeItem}>
												<span class={key}>{change.key}</span>
												<span class={reason}
													>{changeReasonLabel(change.reason)}</span
												>
											</span>
										{/each}
									</dd>
								</div>
							{/if}
							{#if vm.plan.overwritten.length > 0}
								{@render keyGroup(
									m.secrets_import_overwritten({
										count: vm.plan.overwritten.length,
									}),
									vm.plan.overwritten,
									m.secrets_import_overwritten_hint(),
								)}
							{/if}
							{#if vm.plan.removed.length > 0}
								{@render keyGroup(
									m.secrets_import_removed({ count: vm.plan.removed.length }),
									vm.plan.removed,
									undefined,
									true,
								)}
							{/if}
						</dl>
					{/if}
					{#if vm.plan.unchanged.length > 0}
						<p class={muted}>
							{m.secrets_import_unchanged({ count: vm.plan.unchanged.length })}
						</p>
					{/if}
				</section>
			{/if}

			{#if vm.limitError}
				<Banner tone="error" title={vm.limitError} />
			{/if}
			{#if vm.serverError && !vm.isConfirming}
				<Banner tone="error" title={vm.serverError} />
			{/if}
		</div>

		<div class={footer}>
			<span class={summary}>
				{vm.hasText
					? m.secrets_import_detected({ count: vm.entryCount })
					: m.secrets_import_empty()}
			</span>
			<div class={actions}>
				<Button variant="secondary" size="sm" disabled={vm.isSubmitting} onclick={vm.close}>
					{m.common_cancel()}
				</Button>
				<Button
					size="sm"
					loading={vm.isSubmitting}
					disabled={!vm.canSubmit}
					onclick={() => void vm.submit()}
				>
					{m.secrets_import_submit()}
				</Button>
			</div>
		</div>
	</section>

	<ConfirmDialog
		open={vm.isConfirming}
		title={vm.confirmTitle}
		body={vm.confirmBody}
		confirmLabel={vm.confirmLabel}
		cancelLabel={m.common_cancel()}
		loading={vm.isSubmitting}
		error={vm.confirmError}
		onConfirm={() => void vm.confirmImport()}
		onCancel={vm.cancelConfirm}
	/>
{/if}
