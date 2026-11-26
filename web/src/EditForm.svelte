<script lang="ts">
  import { tick, untrack } from 'svelte';
  import Modal from './Modal.svelte';
  import RecordPicker from './RecordPicker.svelte';
  import { activityDay, activityTypes, errorMessage, humanize, isContact, localDay, priorities, projectStatuses, save, singular, taskStatuses } from './api';
  import type { Activity, Contact, Editable, EditRecord, Mutation, Project, Ref, Task } from './api';

  let { space, record, context, close, saved }: {
    space: Editable; record: EditRecord | null; context: { entity?: Ref; project?: Ref };
    close: () => void; saved: (space: Editable, record: EditRecord, created: boolean) => void;
  } = $props();

  type Draft = {
    title: string; description: string; email: string; website: string; industry: string; tags: string;
    status: string; date: string; type: string; priority: number; archived: boolean;
    entity: Ref | null; project: Ref | null; organization: Ref | null;
  };
  function makeDraft(space: Editable, record: EditRecord | null, context: { entity?: Ref; project?: Ref }): Draft {
    const base: Draft = { title: '', description: '', email: '', website: '', industry: '', tags: '', status: '', date: '', type: '', priority: 0,
      archived: false, entity: null, project: null, organization: null };
    if (isContact(space)) {
      const contact = record as Contact | null;
      const mails = (contact?.Methods ?? []).filter((method) => method.Kind === 'email' && (!method.Source || method.Source === 'manual'));
      const membership = contact?.Memberships?.find((entry) => entry.Current);
      return { ...base, title: contact?.DisplayName ?? '', description: contact?.Notes ?? '', email: (mails.find((method) => method.Preferred) ?? mails[0])?.Value ?? '',
        website: contact?.Website ?? '', industry: contact?.Industry ?? '', tags: contact?.Tags?.join(', ') ?? '', archived: contact?.Archived ?? false,
        organization: membership?.OrganizationID ? { kind: 'organization', id: membership.OrganizationID, name: membership.Organization } : null };
    }
    if (space === 'tasks') {
      const task = record as Task | null;
      return { ...base, title: task?.Title ?? '', description: task?.Description ?? '', date: task?.Due ?? '', status: task?.Status ?? 'inbox',
        priority: task?.Priority ?? 0, entity: context.entity ?? null, project: context.project ?? null };
    }
    if (space === 'activities') {
      const activity = record as Activity | null;
      return { ...base, title: activity?.Title ?? '', description: activity?.Body ?? '', date: activity ? activityDay(activity) : localDay(),
        type: activity?.Type ?? 'note', entity: context.entity ?? null, project: context.project ?? null };
    }
    const project = record as Project | null;
    return { ...base, title: project?.Name ?? '', description: project?.Description ?? '', date: project?.Target ?? '',
      status: project?.Status ?? 'planned', priority: project?.Priority ?? 0, entity: context.entity ?? null };
  }

  let original = $derived(makeDraft(space, record, context));
  let draft = $state<Draft>(untrack(() => makeDraft(space, record, context)));
  let dirty = $derived(JSON.stringify(draft) !== JSON.stringify(original));
  let busy = $state(false);
  let discard = $state(false);
  let failure = $state('');
  let errors = $state<{ title?: string; email?: string; date?: string; type?: string }>({});
  let titleInput: HTMLInputElement;
  let emailInput = $state<HTMLInputElement>();
  let dateInput = $state<HTMLInputElement>();
  let typeInput = $state<HTMLInputElement>();

  function dismiss() {
    if (busy) return;
    if (discard) { discard = false; return; }
    if (dirty) discard = true;
    else close();
  }
  function warn(event: BeforeUnloadEvent) {
    if (dirty || busy) { event.preventDefault(); event.returnValue = ''; }
  }
  function mutation(): Mutation {
    const changed = <K extends keyof Draft>(key: K) => !record || draft[key] !== original[key];
    const payload: Mutation = record ? { revision: record.Revision } : {};
    if (isContact(space)) {
      if (changed('title')) payload.name = draft.title.trim();
      if (changed('description')) payload.notes = draft.description;
      if (changed('email')) payload.email = draft.email.trim();
      if (changed('website')) payload.website = draft.website.trim();
      if (changed('industry')) payload.industry = draft.industry.trim();
      if (changed('tags')) payload.tags = draft.tags.split(',').map((tag) => tag.trim()).filter(Boolean);
      if (changed('archived') && record) payload.archived = draft.archived;
      if (space === 'people' && draft.organization?.id !== original.organization?.id) payload.organization_id = draft.organization?.id ?? null;
      return payload;
    }
    if (changed('title')) payload.title = draft.title.trim();
    if (changed('description')) payload.description = draft.description;
    if (changed('date')) payload.date = draft.date;
    if (space === 'activities') {
      if (changed('type')) payload.type = draft.type.trim();
    } else {
      if (changed('status')) payload.status = draft.status;
      if (changed('priority')) payload.priority = draft.priority;
    }
    if (draft.entity?.id && (!record || draft.entity.id !== original.entity?.id)) payload.entity_id = draft.entity.id;
    if (space !== 'projects' && draft.project?.id && (!record || draft.project.id !== original.project?.id)) payload.project_id = draft.project.id;
    return payload;
  }
  async function submit(event: SubmitEvent) {
    event.preventDefault();
    if (busy || discard) return;
    failure = '';
    errors = {};
    if (!draft.title.trim() && (space !== 'activities' || !draft.description.trim())) errors.title = space === 'activities' ? 'Enter a title or write an activity note.' : 'Enter a name or title.';
    if (emailInput?.validity.typeMismatch) errors.email = 'Enter a valid email address.';
    if (dateInput?.validity.badInput || (space === 'activities' && !draft.date)) errors.date = 'Enter a valid date.';
    if (space === 'activities' && !draft.type.trim()) errors.type = 'Enter an activity type.';
    if (errors.title || errors.email || errors.date || errors.type) {
      await tick();
      (errors.title ? titleInput : errors.email ? emailInput : errors.date ? dateInput : typeInput)?.focus();
      return;
    }
    busy = true;
    try {
      const result = await save(space, record?.ID ?? '', mutation());
      saved(space, result, !record);
    } catch (error) {
      failure = errorMessage(error);
    } finally { busy = false; }
  }
</script>

<svelte:window onbeforeunload={warn} />
<Modal title={`${record ? 'Edit' : 'New'} ${singular[space]}`} {dismiss}>
  <form class="editor" onsubmit={submit} novalidate>
    <label>{isContact(space) || space === 'projects' ? 'Name' : 'Title'}
      <input bind:this={titleInput} bind:value={draft.title} data-initial aria-invalid={Boolean(errors.title)} aria-describedby={errors.title ? 'title-error' : undefined} />
      {#if errors.title}<small class="error" id="title-error">{errors.title}</small>{/if}
    </label>
    <label>{isContact(space) ? 'Notes' : space === 'activities' ? 'Activity notes' : 'Description'}
      <textarea rows="4" bind:value={draft.description}></textarea>
    </label>
    {#if isContact(space)}
      <div class="form-fields">
        <label>Manual email <input type="email" bind:this={emailInput} bind:value={draft.email} aria-invalid={Boolean(errors.email)} aria-describedby={errors.email ? 'email-error' : undefined} />
          {#if errors.email}<small class="error" id="email-error">{errors.email}</small>{/if}
        </label>
        <label>Website <input bind:value={draft.website} inputmode="url" /></label>
      </div>
      <div class="form-fields">
        {#if space === 'organizations'}<label>Industry <input bind:value={draft.industry} /></label>{/if}
        <label>Tags <input bind:value={draft.tags} placeholder="Separate tags with commas" /></label>
      </div>
      {#if space === 'people'}
        <RecordPicker label="Current organization" kinds={['organizations']} value={draft.organization} change={(value) => { draft.organization = value; }} />
      {/if}
      {#if record}<label class="checkbox"><input type="checkbox" bind:checked={draft.archived} /> Archived</label>{/if}
    {:else}
      <div class="form-fields">
        {#if space === 'activities'}
          <label>Type <input bind:this={typeInput} bind:value={draft.type} list="activity-types" aria-invalid={Boolean(errors.type)} aria-describedby={errors.type ? 'type-error' : undefined} />
            {#if errors.type}<small class="error" id="type-error">{errors.type}</small>{/if}
          </label>
          <datalist id="activity-types">{#each activityTypes as type}<option value={type}></option>{/each}</datalist>
        {:else}
          <label>Status <select bind:value={draft.status}>
            {#each space === 'tasks' ? taskStatuses : projectStatuses as status}
              <option value={status}>{humanize(status)}</option>
            {/each}
          </select></label>
        {/if}
        <label>{space === 'tasks' ? 'Due date' : space === 'projects' ? 'Target date' : 'Occurred on'}
          <input type="date" bind:this={dateInput} bind:value={draft.date} aria-invalid={Boolean(errors.date)} aria-describedby={errors.date ? 'date-error' : undefined} />
          {#if errors.date}<small class="error" id="date-error">{errors.date}</small>{/if}
        </label>
      </div>
      {#if space !== 'activities'}
        <label>Priority <select bind:value={draft.priority}>
          {#each priorities as priority, index}<option value={index}>{priority}</option>{/each}
        </select></label>
      {/if}
      <RecordPicker label={space === 'projects' ? (record ? 'Add a project member' : 'Project member') : (record ? 'Add a related person or organization' : 'Related person or organization')} kinds={['people', 'organizations']}
        value={draft.entity} change={(value) => { draft.entity = value; }} />
      {#if space !== 'projects'}
        <RecordPicker label={record ? 'Add a related project' : 'Related project'} kinds={['projects']} value={draft.project} change={(value) => { draft.project = value; }} />
      {/if}
    {/if}
    {#if failure}<p class="error" role="alert">{failure}</p>{/if}
    {#if discard}
      <div class="notice" role="alert">
        <strong>Discard the unsaved changes?</strong>
        <div class="actions"><button type="button" class="quiet" onclick={() => { discard = false; }}>Keep editing</button><button type="button" class="quiet" onclick={close}>Discard changes</button></div>
      </div>
    {/if}
    <div class="form-actions">
      <button type="button" class="quiet" onclick={dismiss} disabled={busy}>Cancel</button>
      <button type="submit" class="primary" disabled={busy || discard || Boolean(record && !dirty)}>{busy ? 'Saving…' : record ? 'Save changes' : `Create ${singular[space]}`}</button>
    </div>
  </form>
</Modal>
