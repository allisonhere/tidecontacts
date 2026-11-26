<script lang="ts">
  import RefLinks from './RefLinks.svelte';
  import RelationshipMap from './RelationshipMap.svelte';
  import { activityDay, href, humanize, localDay, refHref, routeFor, taskGroup, titleOf, webLink } from './api';
  import type { ContactDetail, Ref, Route } from './api';

  let { data, space, capture }: { data: ContactDetail; space: 'people' | 'organizations'; capture: (space: 'tasks' | 'activities', entity: Ref) => void } = $props();
  let contact = $derived(data.record);
  let dossier = $derived(data.dossier);
  let root = $derived<Ref>({ id: contact.ID, kind: space === 'people' ? 'person' : 'organization', name: contact.DisplayName });
  let current = $derived(contact.Memberships?.find((membership) => membership.Current));
  let website = $derived(webLink(contact.Website));
  let today = $derived(localDay());
  let relatedPeople = $derived((dossier?.Organizations ?? []).flatMap((link) => link.People ?? [])
    .filter((ref, index, refs) => ref.id !== contact.ID && refs.findIndex((candidate) => candidate.id === ref.id) === index));
  const dossierURL = (target: 'activities' | 'tasks', entity: string) => href({ ...routeFor(target), entity, view: 'all' });
  const taskURL = (id: string) => href({ ...routeFor('tasks'), id });
  const activityURL = (id: string) => href({ ...routeFor('activities'), id });
  function methodLink(kind: string, value: string): string | undefined {
    if (kind === 'email' && value.includes('@')) return `mailto:${encodeURIComponent(value).replace('%40', '@')}`;
    if ((kind === 'telephone' || kind === 'phone') && /^[+\d\s().-]+$/.test(value)) return `tel:${value.replace(/[^+\d]/g, '')}`;
    return kind === 'url' ? webLink(value) : undefined;
  }
</script>

{#if current || contact.Industry || contact.Archived}
  <p class="detail-subtitle">
    {#if current}
      {current.Title ? `${current.Title} · ` : ''}
      {#if current.OrganizationID}<a href={href({ ...routeFor('organizations'), id: current.OrganizationID })}>{current.Organization}</a>{:else}{current.Organization}{/if}
    {/if}
    {#if contact.Industry}{current ? ' · ' : ''}{contact.Industry}{/if}
    {#if contact.Archived}{current || contact.Industry ? ' · ' : ''}Archived{/if}
  </p>
{/if}

{#if dossier}
  <div class="facts" aria-label="Recorded activity and task counts">
    <div><span>Last interaction</span><strong>{dossier.FollowUp.LastInteraction || 'Not recorded'}</strong>
      {#if dossier.FollowUp.LastType}<small>{humanize(dossier.FollowUp.LastType)}</small>{/if}</div>
    <div><span>Open tasks</span><strong><a href={href({ ...routeFor('tasks'), entity: contact.ID })}>{dossier.OpenTaskCount}</a></strong></div>
    <div><span>Activities</span><strong><a href={dossierURL('activities', contact.ID)}>{dossier.Activities}</a></strong></div>
  </div>

  {#if dossier.FollowUp.Due || dossier.Flagged}
    <section class="notice">
      <p class="eyebrow">{dossier.FollowUp.Due ? 'Follow-up' : 'Flagged'}</p>
      {#if dossier.FollowUp.Due}<strong>{dossier.FollowUp.Due < today ? 'Overdue · ' : ''}<time datetime={dossier.FollowUp.Due}>{dossier.FollowUp.Due}</time></strong>
        {#if dossier.FollowUp.Reason}<p>{dossier.FollowUp.Reason}</p>{/if}
      {/if}
      {#if dossier.FlagNote}<p>{dossier.FlagNote}</p>{:else if dossier.Flagged && dossier.FollowUp.Due}<p>Flagged</p>{/if}
    </section>
  {/if}

  <section class="detail-section">
    <div class="section-heading"><h3>Open tasks &amp; commitments</h3><a href={dossierURL('tasks', contact.ID)}>All tasks</a></div>
    {#if tasks.length}
      <ul class="evidence-list">
        {#each tasks as task (task.ID)}
          <li><a href={taskURL(task.ID)}>{titleOf(task)}</a><small class:attention={taskGroup(task, today) === 'Overdue'}>
            {taskGroup(task, today)}{task.Due ? ` · ${task.Due}` : ''}
          </small></li>
        {/each}
      </ul>
    {:else}<p class="muted">No open tasks or recent commitments.</p>{/if}
    <button type="button" class="quiet capture-inline" onclick={() => capture('tasks', root)}>New related task</button>
  </section>

  <section class="detail-section">
    <div class="section-heading"><h3>Recent activity</h3><a href={dossierURL('activities', contact.ID)}>All activities</a></div>
    {#if dossier.Recent?.length}
      <ol class="timeline compact-timeline">
        {#each dossier.Recent as activity (activity.ID)}
          <li><time datetime={activity.OccurredAt}>{activityDay(activity)}</time><div><span class="eyebrow">{humanize(activity.Type)}</span>
            <a href={activityURL(activity.ID)}>{titleOf(activity)}</a></div></li>
        {/each}
      </ol>
    {:else}<p class="muted">No activities are recorded.</p>{/if}
    <button type="button" class="quiet capture-inline" onclick={() => capture('activities', root)}>Log related activity</button>
  </section>
{/if}

{#if contact.Notes || contact.Biography}
  <section class="detail-section"><h3>Notes</h3>
    {#if contact.Notes}<p class="record-text">{contact.Notes}</p>{/if}
    {#if contact.Biography}<p class="record-text">{contact.Biography}</p>{/if}
  </section>
{/if}

{#if contact.Methods?.length || contact.Website || contact.Memberships?.length || contact.Tags?.length}
  <section class="detail-section">
    <h3>{space === 'people' ? 'Contact details' : 'Organization details'}</h3>
    <dl class="record-facts">
      {#each contact.Methods ?? [] as method (method.ID)}
        {@const url = methodLink(method.Kind, method.Value)}
        {#if method.Value}<div><dt>{method.Label || humanize(method.Kind)}</dt>
          <dd>{#if url}<a href={url}>{method.Value}</a>{:else}{method.Value}{/if}{method.Preferred ? ' · Preferred' : ''}</dd></div>{/if}
      {/each}
      {#if contact.Website}<div><dt>Website</dt><dd>{#if website}<a href={website} rel="noreferrer" target="_blank">{contact.Website}</a>{:else}{contact.Website}{/if}</dd></div>{/if}
      {#each contact.Memberships ?? [] as membership (membership.ID)}
        <div><dt>{membership.Current ? 'Current organization' : 'Former organization'}</dt><dd>
          {#if membership.OrganizationID}<a href={href({ ...routeFor('organizations'), id: membership.OrganizationID })}>{membership.Organization}</a>{:else}{membership.Organization}{/if}
          {membership.Title ? ` · ${membership.Title}` : ''}{membership.Department ? ` · ${membership.Department}` : ''}
        </dd></div>
      {/each}
      {#if contact.Tags?.length}<div><dt>Tags</dt><dd>{contact.Tags.join(', ')}</dd></div>{/if}
    </dl>
  </section>
{/if}

{#if dossier}
  <RefLinks title={space === 'organizations' ? 'People connected to this organization' : 'People at related organizations'} records={relatedPeople} />
  {#if dossier.Dates?.length}
    <section class="detail-section"><h3>Important dates</h3><ul class="evidence-list">
      {#each dossier.Dates as entry (entry.ID)}<li><strong>{entry.Label}</strong><small>{entry.Next || entry.Date}{entry.Notes ? ` · ${entry.Notes}` : ''}</small></li>{/each}
    </ul></section>
  {/if}
  <RelationshipMap {dossier} name={titleOf(contact)} />
{/if}
