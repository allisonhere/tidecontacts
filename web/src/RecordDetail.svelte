<script lang="ts">
  import RefLinks from './RefLinks.svelte';
  import { activityDay, href, humanize, localDay, priorities, routeFor, taskGroup, webLink } from './api';
  import type { Activity, Project, Ref, Task } from './api';

  let { record, capture }: { record: Activity | Project | Task; capture: (space: 'tasks' | 'activities', project: Ref) => void } = $props();
  let today = $derived(localDay());
</script>

{#if 'OccurredAt' in record}
  {@const activity = record}
  {@const sourceURL = webLink(activity.ExternalURL)}
  <p class="detail-subtitle"><span class="label">{humanize(activity.Type)}</span> <time datetime={activity.OccurredAt}>{activityDay(activity)}</time>{activity.Timezone ? ` · ${activity.Timezone}` : ''}</p>
  {#if activity.Body}<section class="detail-section"><h3>Activity notes</h3><p class="record-text">{activity.Body}</p></section>{/if}
  {#if activity.ExternalURL || activity.ExternalRef}
    <section class="detail-section"><h3>Source</h3>
      {#if sourceURL}<p><a href={sourceURL} rel="noreferrer" target="_blank">Open recorded source ↗</a></p>
      {:else if activity.ExternalURL}<p>{activity.ExternalURL}</p>{/if}
      {#if activity.ExternalRef}<p class="muted">{activity.ExternalRef}</p>{/if}
    </section>
  {/if}
  <RefLinks title="People and organizations" records={activity.Entities} />
  <RefLinks title="Projects" records={activity.Projects} />
  <RefLinks title="Tasks" records={activity.Tasks} />
{:else if 'Due' in record}
  {@const task = record}
  <p class="detail-subtitle"><span class="label">{humanize(task.Status)}</span>{task.Archived ? ' · Archived' : ''}</p>
  <div class="facts" aria-label="Task details">
    <div><span>Due</span><strong class:attention={taskGroup(task, today) === 'Overdue'}>{task.Due || 'Not set'}</strong>
      {#if taskGroup(task, today) === 'Overdue'}<small class="attention">Overdue</small>{:else if task.DueTime}<small>{task.DueTime}</small>{/if}</div>
    <div><span>Priority</span><strong>{priorities[task.Priority] ?? 'None'}</strong></div>
    {#if task.CompletedAt}<div><span>Completed</span><strong><time datetime={task.CompletedAt}>{task.CompletedAt.slice(0, 10)}</time></strong></div>{/if}
  </div>
  {#if task.Description}<section class="detail-section"><h3>Description</h3><p class="record-text">{task.Description}</p></section>{/if}
  <RefLinks title="People and organizations" records={task.Entities} />
  <RefLinks title="Projects" records={task.Projects} />
  <RefLinks title="Activities" records={task.Activities} />
{:else}
  {@const project = record}
  {@const root = { kind: 'project', id: project.ID, name: project.Name }}
  <p class="detail-subtitle"><span class="label">{humanize(project.Status)}</span> {priorities[project.Priority] !== 'None' ? `${priorities[project.Priority]} priority` : ''}</p>
  <div class="facts" aria-label="Project dates and task counts">
    <div><span>Target</span><strong>{project.Target || 'Not set'}</strong>{#if project.Start}<small>Start · {project.Start}</small>{/if}</div>
    <div><span>Open tasks</span><strong><a href={href({ ...routeFor('tasks'), project: project.ID })}>{project.OpenTasks}</a></strong></div>
    <div><span>Completed tasks</span><strong><a href={href({ ...routeFor('tasks'), project: project.ID, view: 'completed' })}>{project.DoneTasks}</a></strong></div>
  </div>
  <div class="actions capture-inline">
    <button type="button" class="quiet" onclick={() => capture('tasks', root)}>New project task</button>
    <button type="button" class="quiet" onclick={() => capture('activities', root)}>Log project activity</button>
    <a href={href({ ...routeFor('activities'), project: project.ID })}>Project activities</a>
  </div>
  {#if project.Description || project.Notes}
    <section class="detail-section"><h3>Description &amp; notes</h3>
      {#if project.Description}<p class="record-text">{project.Description}</p>{/if}
      {#if project.Notes}<p class="record-text">{project.Notes}</p>{/if}
    </section>
  {/if}
  <section class="detail-section"><h3>Milestones</h3>
    {#if project.Milestones?.length}
      <ul class="evidence-list">{#each project.Milestones as milestone (milestone.ID)}
        <li><strong>{milestone.Title}</strong><small class:attention={Boolean(milestone.Due && milestone.Due < today && !milestone.DoneAt)}>
          {milestone.DoneAt ? `Completed · ${milestone.DoneAt.slice(0, 10)}` : milestone.Due ? `${milestone.Due < today ? 'Overdue · ' : 'Due · '}${milestone.Due}` : 'No due date'}
        </small></li>
      {/each}</ul>
    {:else}<p class="muted">No milestones are recorded.</p>{/if}
  </section>
  <RefLinks title="Project members" records={project.Members} />
{/if}
