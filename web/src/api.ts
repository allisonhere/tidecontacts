export const spaces = ['people', 'organizations', 'activities', 'tasks', 'projects', 'pipelines'] as const;
export type Space = (typeof spaces)[number];
export type Editable = Exclude<Space, 'pipelines'>;
export const labels: Record<Space, string> = {
  people: 'People', organizations: 'Organizations', activities: 'Activities', tasks: 'Tasks', projects: 'Projects', pipelines: 'General pipelines'
};
export const singular: Record<Space, string> = {
  people: 'person', organizations: 'organization', activities: 'activity', tasks: 'task', projects: 'project', pipelines: 'pipeline'
};
export const descriptions: Record<Space, string> = {
  people: 'A directory and a dossier for each relationship.',
  organizations: 'Organizations and the people connected to them.',
  activities: 'A dated journal of recorded interactions and events.',
  tasks: 'Review what is due, waiting, or complete.',
  projects: 'Review active work, milestones, and members.',
  pipelines: 'Track items through recorded stages.'
};

export type Ref = { kind?: string; id?: string; name?: string; role?: string };
export type Identified = { ID: string };
export type Versioned = Identified & { Revision: number };
export type Summary = Versioned & { Name: string; Organization: string; PrimaryEmail?: string; Tags: string; Archived: boolean };
export type Method = { ID: string; Kind: string; Value: string; Label: string; Preferred: boolean; Source: string };
export type Membership = { ID: string; OrganizationID: string; Organization: string; Title: string; Department: string; Current: boolean };
export type Contact = Versioned & {
  DisplayName: string; Notes: string; Biography: string; Website: string; Industry: string; Source: string; Archived: boolean;
  Methods: Method[] | null; Memberships: Membership[] | null; Tags: string[] | null;
};
export type Task = Versioned & {
  Title: string; Description: string; Status: string; Priority: number; Due: string; DueTime: string;
  CompletedAt: string; Entities: Ref[] | null; Projects: Ref[] | null; Activities: Ref[] | null; Archived: boolean;
};
export type Activity = Versioned & {
  Title: string; Body: string; Type: string; OccurredAt: string; Timezone: string; ExternalURL: string; ExternalRef: string; Provenance: string;
  Entities: Ref[] | null; Projects: Ref[] | null; Tasks: Ref[] | null;
};
export type Milestone = Identified & { Title: string; Due: string; DoneAt: string };
export type Project = Versioned & {
  Name: string; Description: string; Status: string; Notes: string; Target: string; Start: string;
  Priority: number; Members: Ref[] | null; Milestones: Milestone[] | null; OpenTasks: number; DoneTasks: number;
};
export type Edge = { Person: Ref; Type: string; Notes: string; Count: number };
export type OrgLink = { Organization: Ref; Title: string; Current: boolean; People: Ref[] | null };
export type ProjectLink = { Project: Ref; Status: string; People: Ref[] | null };
export type Dossier = {
  Root: Ref; Outgoing: Edge[] | null; Incoming: Edge[] | null; Organizations: OrgLink[] | null; Projects: ProjectLink[] | null;
  Shared: Edge[] | null; FollowUp: { Due: string; Reason: string; LastInteraction: string; LastType: string };
  Dates: { ID: string; Label: string; Date: string; Next: string; Notes: string }[] | null;
  Recent: Activity[] | null; OpenTasks: Task[] | null; Commitments: Task[] | null;
  Activities: number; OpenTaskCount: number; Flagged: boolean; FlagNote: string;
};
export type ContactDetail = { record: Contact; dossier: Dossier | null };
export type Stage = Identified & { Name: string; Count: number };
export type Pipeline = Identified & { Name: string; Description: string; Stages: Stage[] | null; Archived: boolean };
export type PipelineItem = Versioned & {
  Title: string; StageID: string; Stage: string; NextAction: string; Notes: string; StageEnteredAt: string;
  Entities: Ref[] | null; Projects: Ref[] | null; Tasks: Ref[] | null; Activities: Ref[] | null;
};
export type PipelineDetail = { record: Pipeline; items: PipelineItem[] | null };
export type Entry = Summary | Activity | Task | Project | Pipeline;
export type EditRecord = Contact | Activity | Task | Project;
export type Detail = ContactDetail | Activity | Task | Project | PipelineDetail;
export type Page<T = Entry> = { items: T[] | null; next_offset: number | null };
export type Profile = { demo: boolean; professional: string[] | null };
export type Mutation = {
  revision?: number; name?: string; notes?: string; website?: string; industry?: string; tags?: string[];
  email?: string; organization_id?: string | null; archived?: boolean; title?: string; description?: string;
  status?: string; date?: string; type?: string; priority?: number; entity_id?: string; project_id?: string;
};
export const proSpaces = ['sales', 'business', 'automation'] as const;
export type ProSpace = (typeof proSpaces)[number];
export type Location = Space | ProSpace | 'settings';
export type Route<T extends Location = Location> = { space: T; id: string; item: string; q: string; offset: number; view: string; sort: string; entity: string; project: string };
export const taskViews = ['open', 'all', 'inbox', 'today', 'upcoming', 'waiting', 'overdue', 'unscheduled', 'scheduled', 'completed'];
export const taskStatuses = ['inbox', 'next', 'scheduled', 'waiting', 'completed', 'canceled'];
export const projectStatuses = ['planned', 'active', 'on-hold', 'completed', 'archived'];
export const activityTypes = ['email', 'call', 'meeting', 'conversation', 'message', 'note', 'event', 'task-completion', 'project-update'];
export const priorities = ['None', 'Low', 'Medium', 'High'];
export const pageSize = 25;

export function routeFor<T extends Location>(space: T): Route<T> {
  return { space, id: '', item: '', q: '', offset: 0, view: 'open', sort: 'name', entity: '', project: '' };
}
export function isSpace(space: Location): space is Space {
  return (spaces as readonly string[]).includes(space);
}
export function parseRoute(url: URL): Route {
  const params = url.searchParams;
  const [, key, rawID] = url.pathname.split('/');
  const space: Location = key === 'settings' || spaces.includes(key as Space) || proSpaces.includes(key as ProSpace) ? key as Location : 'people';
  let id = '';
  if (rawID && isSpace(space)) {
    try { id = decodeURIComponent(rawID); } catch { id = rawID; }
  }
  const route = routeFor(space);
  const offset = Number(params.get('offset'));
  const view = params.get('view') ?? '';
  return { ...route, id, item: space === 'pipelines' ? params.get('item') ?? '' : '',
    q: params.get('q') ?? '', offset: Number.isSafeInteger(offset) && offset >= 0 ? offset : 0,
    view: taskViews.includes(view) ? view : 'open', sort: params.get('sort') === 'updated' ? 'updated' : 'name',
    entity: params.get('entity') ?? '', project: params.get('project') ?? '' };
}
export const readRoute = (): Route => parseRoute(new URL(location.href));
export function href(route: Route): string {
  const params = new URLSearchParams();
  if (route.q) params.set('q', route.q);
  if (route.offset) params.set('offset', String(route.offset));
  if (route.space === 'tasks' && route.view !== 'open') params.set('view', route.view);
  if (isContact(route.space) && route.sort !== 'name') params.set('sort', route.sort);
  if (route.item) params.set('item', route.item);
  if (route.entity) params.set('entity', route.entity);
  if (route.project) params.set('project', route.project);
  const path = `/${route.space}${route.id ? `/${encodeURIComponent(route.id)}` : ''}`;
  return params.size ? `${path}?${params}` : path;
}
export function refHref(ref: Ref): string | undefined {
  if (!ref.id) return;
  const space = spaces.find((key) => singular[key] === ref.kind);
  if (space) return href({ ...routeFor(space), id: ref.id });
}
export function isContact(space: Location): space is 'people' | 'organizations' {
  return space === 'people' || space === 'organizations';
}
export function titleOf(record: Entry | EditRecord | PipelineItem): string {
  if ('DisplayName' in record) return record.DisplayName || 'Unnamed contact';
  if ('Name' in record) return record.Name || 'Unnamed record';
  return record.Title || ('Type' in record ? humanize(record.Type) : 'Untitled record');
}
export function humanize(text: string): string {
  return text ? text.charAt(0).toUpperCase() + text.slice(1).replaceAll('-', ' ') : '';
}
export function webLink(value: string): string | undefined {
  if (!value) return;
  try {
    const url = new URL(value);
    if (url.protocol === 'https:' || url.protocol === 'http:') return url.href;
  } catch { /* Recorded data may be a label or a local path, not a web link. */ }
}
export function localDay(date = new Date()): string {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`;
}
export function activityDay(activity: Activity): string {
  return activity.OccurredAt?.slice(0, 10) || '';
}
export function taskGroup(task: Task, today: string): string {
  if (task.Archived) return 'Archived';
  if (task.Status === 'completed' || task.Status === 'canceled') return humanize(task.Status);
  if (task.Due && task.Due < today) return 'Overdue';
  if (task.Due === today) return 'Due today';
  if (task.Status === 'waiting') return 'Waiting';
  if (task.Due) return 'Upcoming';
  return task.Status === 'inbox' ? 'Inbox' : 'No due date';
}

const token = (() => {
  const fragment = new URLSearchParams(location.hash.slice(1));
  const incoming = fragment.get('session');
  if (fragment.has('session')) history.replaceState(history.state, '', location.pathname + location.search);
  try {
    if (incoming) sessionStorage.setItem('tidecontacts-session', incoming);
    return incoming || sessionStorage.getItem('tidecontacts-session');
  } catch {
    return incoming;
  }
})();
export const hasSession = Boolean(token);
export const sessionToken = (): string => token ?? '';
export class APIError extends Error {
  constructor(public readonly code: string, message: string) { super(message); }
}
export function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : 'The request could not be completed.';
}
async function request<T>(path: string, signal?: AbortSignal, body?: object, method = 'GET'): Promise<T> {
  if (!token) throw new APIError('session', 'Open TideContacts from its launcher to start a session.');
  const headers: Record<string, string> = { Authorization: `Bearer ${token}` };
  if (body) headers['Content-Type'] = 'application/json';
  const response = await fetch(`/api/v1/${path}`, {
    method, headers, signal, credentials: 'omit', cache: 'no-store', redirect: 'error',
    body: body ? JSON.stringify(body) : undefined
  });
  const envelope = await response.json() as { schema_version: number; data: T; error?: { code: string; message: string } };
  if (envelope.schema_version !== 1) throw new APIError('schema', 'Unsupported TideContacts response version.');
  if (envelope.error) throw new APIError(envelope.error.code, envelope.error.message);
  if (!response.ok) throw new APIError('request', `The request failed (${response.status}).`);
  return envelope.data;
}
export function list<T = Entry>(route: Route<Space>, signal?: AbortSignal, limit = pageSize): Promise<Page<T>> {
  const query = new URLSearchParams({ limit: String(limit), offset: String(route.offset) });
  if (route.q) query.set('q', route.q);
  if (route.space === 'tasks') query.set('view', route.view);
  if (isContact(route.space)) query.set('sort', route.sort);
  if (route.entity) query.set('entity', route.entity);
  if (route.project) query.set('project', route.project);
  return request(`${route.space}?${query}`, signal);
}
const recordPath = (space: Space, id: string) => `${space}/${encodeURIComponent(id)}`;
export const profile = (signal: AbortSignal) => request<Profile>('profile', signal);
export const detail = (route: Route<Space>, signal?: AbortSignal) => request<Detail>(recordPath(route.space, route.id), signal);
export const save = (space: Editable, id: string, body: Mutation) =>
  request<EditRecord>(id ? recordPath(space, id) : space, undefined, body, id ? 'PATCH' : 'POST');
export const complete = (task: Task) => request<Task>(`${recordPath('tasks', task.ID)}/complete`, undefined, { revision: task.Revision }, 'POST');
export const move = (item: PipelineItem, stage: string) => request<PipelineItem>(`items/${encodeURIComponent(item.ID)}/move`, undefined, { revision: item.Revision, stage_id: stage }, 'POST');
