# Projects and pipelines

## Projects

A project groups people, organizations, tasks, activities, milestones and notes. It needs nothing from TideMail or any other tool.

States: planned, active, on-hold, completed, archived. The list shows current projects (everything but archived); `s` cycles the status filter through all, planned, active, on-hold, completed and archived. `/` filters with `status:`, `is:open|overdue|waiting|archived`, `with:`, `org:`, `tag:`, `pri:` and free words. `is:overdue` means the target date or an open milestone date has passed; `is:waiting` means the project has a task in the waiting state.

`[` / `]` walk the detail views:

| View | Contents |
|---|---|
| Overview | Status, dates, task and milestone progress, description, people, latest activity |
| People | Collaborators and organizations with roles; `L` links, `x` unlinks |
| Tasks | The project's tasks by state; `T` adds one, task keys work on the row under the cursor |
| Timeline | Activities linked to the project; `A` logs one |
| Milestones | `M` adds (`Signed brief due:fri`), `c` toggles done, `S` sets the date, `x` removes |
| Notes | The project's own notes, followed by note activities linked to it |

`n` creates and `e` edits through the form; collaborators may carry a role as `Name (role)`. `>` / `<` change status, `p` priority, `S` the target date, `a` archives or restores, `D` deletes after confirmation. Deleting a project keeps its tasks and activities. A status change is recorded on the project timeline. **Open project…** in the palette jumps to any project by name.

## Pipelines

A pipeline is a list of stages you define and items that move through them: job applications, partnerships, client onboarding, collaborations, personal goals. Items have no amount, probability or owner; they have a title, a next action, notes, tags and links to people, organizations, projects, tasks and activities.

### The board

The left pane lists pipelines (`[` / `]` also switch). The board shows one column per stage:

| Key | Action |
|---|---|
| ← / → (or h / l) | Previous / next stage; past the first column focuses the pipeline list, past the last focuses the item detail |
| ↑ / ↓ (or k / j) | Previous / next item in the stage |
| `>` / `<` | Move the item to the next / previous stage |
| Enter | Open the item's detail; Esc returns |
| `n` | New item in the current stage (on the pipeline list: new pipeline) |
| `e` | Edit item (on the pipeline list: rename pipeline and edit stages) |
| `L` / `t` | Link people or projects / create a task for the item |
| `a` / `D` | Archive / delete (with confirmation) |

No mouse is needed and nothing is dragged. The board shows as many columns as fit (22 cells minimum) and scrolls with the selected stage. When only one column fits, it becomes a stage list with counts above the focused stage's items; the keys are the same.

Each item shows how many days it has been in its stage. Its detail lists the full stage history; history keeps the stage names that applied at the time, so renaming or removing a stage later does not rewrite it.

### Managing pipelines and stages

From the command palette (`:`):

- **New pipeline**, **Rename pipeline / edit stages** — the stage field is an ordered, comma-separated list; existing names keep their items, so reordering is just retyping the order.
- **Add pipeline stage** (after the current column), **Rename pipeline stage**, **Reorder stage left / right**, **Remove empty pipeline stage**. A stage that still holds items cannot be removed; move them first.
- **Move item to stage…** picks any stage directly.
- **Move pipeline up / down**, **Archive or unarchive pipeline**. Archived pipelines stay listed, dimmed. Pipelines are archived rather than deleted.

Filters on the board: `stage:`, `with:`, `org:`, `project:`, `tag:`, `is:archived`, and free words.
