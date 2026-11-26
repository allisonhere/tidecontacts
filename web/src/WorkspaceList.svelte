<script lang="ts">
  import { activityDay, href, humanize, localDay, priorities, taskGroup, titleOf } from './api';
  import type { Activity, Entry, Pipeline, Project, Route, Space, Summary, Task } from './api';
  let { route, items }: { route: Route<Space>; items: Entry[] } = $props();

  function groups<T>(rows: T[], key: (record: T) => string) {
    const groups = new Map<string, T[]>();
    for (const row of rows) {
      const label = key(row);
      const group = groups.get(label);
      if (group) group.push(row);
      else groups.set(label, [row]);
    }
    return [...groups];
  }
  const taskOrder = ['Overdue', 'Due today', 'Upcoming', 'Waiting', 'Inbox', 'No due date', 'Completed', 'Canceled', 'Archived'];
  const today = localDay();
  let taskGroups = $derived(route.space === 'tasks'
    ? groups(items as Task[], (task) => taskGroup(task, today)).sort(([a], [b]) => taskOrder.indexOf(a) - taskOrder.indexOf(b)) : []);
  const recordURL = (item: Entry) => href({ ...route, id: item.ID, item: '' });
  const selected = (item: Entry) => route.id === item.ID ? 'true' : undefined;
</script>

{#if route.space === 'people'}
  <ul class="directory-list plain-list">
    {#each items as entry (entry.ID)}
      {@const person = entry as Summary}
      <li><a class="record-row person-row" href={recordURL(person)} aria-current={selected(person)}>
        <span class="portrait" aria-hidden="true">{titleOf(person).split(/\s+/).slice(0, 2).map((part) => part.charAt(0)).join('').toUpperCase()}</span>
        <span><strong>{titleOf(person)}</strong>
          {#if person.Organization || person.PrimaryEmail}<small>{person.Organization || person.PrimaryEmail}</small>{/if}
          {#if person.Archived}<small>Archived</small>{/if}
        </span>
        <span class="row-arrow" aria-hidden="true">›</span>
      </a></li>
    {/each}
  </ul>
{:else if route.space === 'organizations'}
  <table class="organization-table">
    <thead><tr><th scope="col">Organization</th><th scope="col">Contact &amp; tags</th></tr></thead>
    <tbody>
      {#each items as entry (entry.ID)}
        {@const organization = entry as Summary}
        <tr>
          <th scope="row"><a href={recordURL(organization)} aria-current={selected(organization)}>{titleOf(organization)}</a>
            {#if organization.Archived}<small>Archived</small>{/if}
          </th>
          <td>{organization.PrimaryEmail || '—'}{#if organization.Tags}<small>{organization.Tags}</small>{/if}</td>
        </tr>
      {/each}
    </tbody>
  </table>
{:else if route.space === 'activities'}
  <div class="timeline-list">
    {#each groups(items as Activity[], (activity) => activityDay(activity)) as [day, activities] (day)}
      <section class="list-group">
        <h2>{#if day}<time datetime={day}>{day}</time>{:else}Undated{/if}</h2>
        <ol class="timeline">
          {#each activities as activity (activity.ID)}
            <li><span class="eyebrow timeline-marker">{humanize(activity.Type)}</span><div>
              <a href={recordURL(activity)} aria-current={selected(activity)}>{titleOf(activity)}</a>
              {#if activity.Entities?.length}<small>{activity.Entities.map((entity) => entity.name).filter(Boolean).join(' · ')}</small>{/if}
            </div></li>
          {/each}
        </ol>
      </section>
    {/each}
  </div>
{:else if route.space === 'tasks'}
  <div class="task-register">
    {#each taskGroups as [group, tasks] (group)}
      <section class="list-group">
        <h2 class:attention={group === 'Overdue'}>{group}<span>{tasks.length}</span></h2>
        <ul class="plain-list">
          {#each tasks as task (task.ID)}
            <li><a class="record-row task-row" href={recordURL(task)} aria-current={selected(task)}>
              <span class="task-mark" aria-hidden="true">{task.Status === 'completed' ? '✓' : '◦'}</span>
              <span><strong>{titleOf(task)}</strong><small>{humanize(task.Status)}{task.Priority ? ` · ${priorities[task.Priority]} priority` : ''}</small></span>
              {#if task.Due}<time datetime={task.Due}>{task.Due}</time>{/if}
            </a></li>
          {/each}
        </ul>
      </section>
    {/each}
  </div>
{:else if route.space === 'projects'}
  <ul class="project-index plain-list">
    {#each items as entry (entry.ID)}
      {@const project = entry as Project}
      <li><a class="project-entry" href={recordURL(project)} aria-current={selected(project)}>
        <span class="eyebrow">{humanize(project.Status)}</span>
        <strong>{titleOf(project)}</strong>
        {#if project.Description}<small class="excerpt">{project.Description}</small>{/if}
        <small>{project.Target ? `Target ${project.Target} · ` : ''}{project.OpenTasks} open · {project.DoneTasks} complete</small>
      </a></li>
    {/each}
  </ul>
{:else}
  <ul class="pipeline-index plain-list">
    {#each items as entry (entry.ID)}
      {@const pipeline = entry as Pipeline}
      <li><a class="pipeline-entry" href={recordURL(pipeline)} aria-current={selected(pipeline)}>
        <strong>{titleOf(pipeline)}</strong>
        {#if pipeline.Description}<small class="excerpt">{pipeline.Description}</small>{/if}
        {#if pipeline.Stages?.length}<span class="stages-preview">{#each pipeline.Stages as stage (stage.ID)}<small>{stage.Name} <b>{stage.Count}</b></small>{/each}</span>
        {:else}<small>No stages configured</small>{/if}
      </a></li>
    {/each}
  </ul>
{/if}
