package ext

import "regexp"

// Declarative UI documents. An extension describes *what* to show; the host
// decides layout, focus, theme, keys and width. Everything here is plain data:
// the host sanitizes every string (see Clean), bounds every collection, and
// never lets an extension emit styling or terminal sequences.
//
// All of these are additive to API version 1.

// Methods the host may call on an extension that holds ui.register.
const (
	MethodViewRender = "view.render"
	MethodActionRun  = "action.run"
	MethodFormGet    = "form.get"
	MethodFormSubmit = "form.submit"
	MethodSignals    = "signals.list"
	MethodCLIRun     = "cli.run"
	MethodCLISchema  = "cli.schema"
)

var idToken = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,63}$`)

// Tones map to theme roles in the host, never to literal colors.
var tones = map[string]bool{"": true, "accent": true, "ok": true, "warn": true, "danger": true, "muted": true, "info": true}

// ViewRequest asks an extension to render one view of one of its workspaces.
type ViewRequest struct {
	Workspace string            `json:"workspace"`
	Tab       string            `json:"tab,omitempty"`
	Item      string            `json:"item,omitempty"` // dossier target
	Query     string            `json:"query,omitempty"`
	State     map[string]string `json:"state,omitempty"` // opaque, host-stored per view
	Width     int               `json:"width,omitempty"` // hint only
	Narrow    bool              `json:"narrow,omitempty"`
}

// View is the document returned by view.render.
type View struct {
	Title      string    `json:"title,omitempty"`
	Badge      string    `json:"badge,omitempty"`   // right of the title, e.g. PROFESSIONAL
	Tabs       []Tab     `json:"tabs,omitempty"`    // section strip
	Tab        string    `json:"tab,omitempty"`     // active tab id
	Heading    string    `json:"heading,omitempty"` // line above the body
	Subheading string    `json:"subheading,omitempty"`
	Tiles      []Tile    `json:"tiles,omitempty"`
	Board      *Board    `json:"board,omitempty"`
	List       *List     `json:"list,omitempty"`
	Sections   []Section `json:"sections,omitempty"`
	Panels     []Panel   `json:"panels,omitempty"`
	Notices    []Notice  `json:"notices,omitempty"`
	Actions    []Action  `json:"actions,omitempty"`
	Open       string    `json:"open,omitempty"` // action id run by Enter on the selected item
	Searchable bool      `json:"searchable,omitempty"`
	Empty      string    `json:"empty,omitempty"`
	Dossier    bool      `json:"dossier,omitempty"` // pushes onto the back stack when opened
}

// Tab is one section.
type Tab struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// Tile is a headline figure.
type Tile struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Note  string `json:"note,omitempty"`
	Tone  string `json:"tone,omitempty"`
}

// Board is a set of columns of cards.
type Board struct {
	Columns []Column `json:"columns"`
}

// Column is one board stage.
type Column struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Total string `json:"total,omitempty"`
	Note  string `json:"note,omitempty"`
	Tone  string `json:"tone,omitempty"`
	Cards []Card `json:"cards"`
}

// Card is one board item. Progress is 0-100, or -1 for none.
type Card struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Subtitle  string `json:"subtitle,omitempty"`
	Amount    string `json:"amount,omitempty"`
	Progress  int    `json:"progress"`
	Left      string `json:"left,omitempty"`  // under the bar, left
	Right     string `json:"right,omitempty"` // under the bar, right
	Tone      string `json:"tone,omitempty"`
	Badge     string `json:"badge,omitempty"`
	BadgeTone string `json:"badge_tone,omitempty"`
}

// List is a table of rows.
type List struct {
	Columns []string `json:"columns,omitempty"`
	Rows    []Row    `json:"rows"`
}

// Row is one list item.
type Row struct {
	ID    string   `json:"id"`
	Cells []string `json:"cells"`
	Tone  string   `json:"tone,omitempty"`
	Badge string   `json:"badge,omitempty"`
}

// Section is a titled group of lines.
type Section struct {
	Title string  `json:"title,omitempty"`
	Lines []SLine `json:"lines"`
}

// SLine is a section line. Target makes it selectable: Enter follows it.
type SLine struct {
	ID     string  `json:"id,omitempty"` // passed to item actions (default: the target id)
	Label  string  `json:"label,omitempty"`
	Text   string  `json:"text"`
	Right  string  `json:"right,omitempty"` // right-aligned figure (panels)
	Mark   string  `json:"mark,omitempty"`  // one-glyph status marker (panels)
	Tone   string  `json:"tone,omitempty"`
	Target *Target `json:"target,omitempty"`
}

// Target is where Enter on a line or notice leads.
type Target struct {
	// Kind is "ext" (another view of this extension) or a core record kind the
	// host can open: contact, organization, task, activity, project.
	Kind string `json:"kind"`
	// Workspace opens an "ext" target in another workspace of the extension.
	Workspace string `json:"workspace,omitempty"`
	Tab       string `json:"tab,omitempty"`
	ID        string `json:"id"`
	// Action, when set with Kind "ext", is run instead of navigating.
	Action string `json:"action,omitempty"`
}

// Notice is a status line below the body.
type Notice struct {
	Tone   string  `json:"tone,omitempty"`
	Text   string  `json:"text"`
	Target *Target `json:"target,omitempty"`
}

// Action binds a key to an extension action in the current view.
type Action struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	ID      string `json:"id"`
	Scope   string `json:"scope,omitempty"` // "item" (needs a selected card/row/line) or "view"
	Confirm string `json:"confirm,omitempty"`
}

// reservedKeys can never be bound by an extension.
var reservedKeys = map[string]bool{
	"esc": true, "enter": true, "tab": true, "shift+tab": true, "up": true, "down": true, "left": true, "right": true,
	"h": true, "j": true, "k": true, "l": true, "q": true, "?": true, ":": true, "/": true, "[": true, "]": true,
	" ": true, "ctrl+c": true, "ctrl+z": true, "ctrl+k": true, "ctrl+g": true, "pgup": true, "pgdown": true, "home": true, "end": true,
}

// ValidActionKey allows one printable ASCII character that is not reserved.
func ValidActionKey(k string) bool {
	return len(k) == 1 && k[0] > 0x20 && k[0] < 0x7f && !reservedKeys[k]
}

// ActionRequest runs one action.
type ActionRequest struct {
	Workspace string            `json:"workspace"`
	Action    string            `json:"action"`
	Tab       string            `json:"tab,omitempty"`
	Item      string            `json:"item,omitempty"`   // dossier being viewed
	Target    string            `json:"target,omitempty"` // selected card/row/line id
	State     map[string]string `json:"state,omitempty"`
	Args      map[string]string `json:"args,omitempty"`
}

// ActionResult is the answer to an action, a command, or a form submission.
// Exactly the fields that are set are acted on, in this order: Errors (the
// form stays open), Form, Confirm, Open, then Notice/Refresh/State/Close.
type ActionResult struct {
	Errors  map[string]string `json:"errors,omitempty"` // form field key -> message
	Notice  string            `json:"notice,omitempty"`
	Tone    string            `json:"tone,omitempty"`
	Refresh bool              `json:"refresh,omitempty"`
	State   map[string]string `json:"state,omitempty"` // merged into the view state
	Form    *Form             `json:"form,omitempty"`
	Confirm *Confirm          `json:"confirm,omitempty"`
	Open    *Target           `json:"open,omitempty"`
	Close   bool              `json:"close,omitempty"`
	// Title and Lines carry plain Content (command output) for compatibility.
	Title string `json:"title,omitempty"`
	Lines []Line `json:"lines,omitempty"`
}

// Confirm asks the user before the host runs Action with Args.
type Confirm struct {
	Text   string            `json:"text"`
	Action string            `json:"action"`
	Args   map[string]string `json:"args,omitempty"`
	Target string            `json:"target,omitempty"`
}

// FormRequest fetches a form definition.
type FormRequest struct {
	Workspace string            `json:"workspace,omitempty"`
	Form      string            `json:"form"`
	Target    string            `json:"target,omitempty"`
	Args      map[string]string `json:"args,omitempty"`
}

// Form is a modal form rendered with the host's form components.
type Form struct {
	ID     string            `json:"id"`
	Title  string            `json:"title"`
	Target string            `json:"target,omitempty"`
	Args   map[string]string `json:"args,omitempty"` // echoed back on submit
	Fields []FormField       `json:"fields"`
}

// Field types: text, multiline, select, contact (one person or organization),
// contacts (several). Money, percentage and date entry are text fields the
// extension validates; it answers with field errors.
type FormField struct {
	Key      string    `json:"key"`
	Label    string    `json:"label"`
	Type     string    `json:"type"`
	Value    string    `json:"value,omitempty"`
	Options  []string  `json:"options,omitempty"`
	Help     string    `json:"help,omitempty"`
	Required bool      `json:"required,omitempty"`
	Refs     []RefItem `json:"refs,omitempty"`
}

// RefItem is a selected core record.
type RefItem struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
	Kind string `json:"kind,omitempty"`
}

// FormSubmit carries the user's answers back.
type FormSubmit struct {
	Workspace string               `json:"workspace,omitempty"`
	Form      string               `json:"form"`
	Target    string               `json:"target,omitempty"`
	Args      map[string]string    `json:"args,omitempty"`
	Values    map[string]string    `json:"values"`
	Refs      map[string][]RefItem `json:"refs,omitempty"`
}

// Signal is an attention item contributed to the People Inbox. It states its
// facts so the user can see why it appeared; the host never scores or ranks.
type Signal struct {
	Key      string   `json:"key"`
	Type     string   `json:"type"`
	Title    string   `json:"title"`
	Severity string   `json:"severity"` // info, warn, danger
	Reason   string   `json:"reason"`
	Facts    []string `json:"facts,omitempty"`
	Date     string   `json:"date,omitempty"`
	Action   string   `json:"action,omitempty"` // recommended next step
	Target   *Target  `json:"target,omitempty"`
}

const (
	maxTabs, maxTiles, maxColumns, maxCards, maxRows, maxSections, maxSectionLines, maxNotices, maxActions, maxFormFields, maxSignals = 16, 8, 12, 200, 500, 16, 120, 8, 24, 30, 200
)

func tone(t string) string {
	if tones[t] {
		return t
	}
	return ""
}
func tokenOr(s string) string {
	if idToken.MatchString(s) {
		return s
	}
	return ""
}

func sanitizeTarget(t *Target) *Target {
	if t == nil {
		return nil
	}
	switch t.Kind {
	case "ext", "contact", "organization", "task", "activity", "project":
	default:
		return nil
	}
	id := tokenOr(t.ID)
	if id == "" {
		return nil
	}
	return &Target{Kind: t.Kind, Workspace: tokenOr(t.Workspace), Tab: tokenOr(t.Tab), ID: id, Action: tokenOr(t.Action)}
}

func sanitizeMap(m map[string]string, n int) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := map[string]string{}
	for k, v := range m {
		if len(out) >= n {
			break
		}
		if k = tokenOr(k); k != "" {
			out[k] = Clean(v, 200, false)
		}
	}
	return out
}

// SanitizeView bounds and cleans an untrusted view.
func SanitizeView(v View) View {
	o := View{Title: Clean(v.Title, 60, false), Badge: Clean(v.Badge, 24, false), Tab: tokenOr(v.Tab), Heading: Clean(v.Heading, 80, false), Subheading: Clean(v.Subheading, 60, false),
		Open: tokenOr(v.Open), Searchable: v.Searchable, Empty: Clean(v.Empty, 160, false), Dossier: v.Dossier}
	for i, t := range v.Tabs {
		if i < maxTabs && tokenOr(t.ID) != "" {
			o.Tabs = append(o.Tabs, Tab{t.ID, Clean(t.Title, 20, false)})
		}
	}
	for i, t := range v.Tiles {
		if i < maxTiles {
			o.Tiles = append(o.Tiles, Tile{Clean(t.Label, 24, false), Clean(t.Value, 30, false), Clean(t.Note, 40, false), tone(t.Tone)})
		}
	}
	if v.Board != nil {
		b := &Board{}
		for i, c := range v.Board.Columns {
			if i >= maxColumns || tokenOr(c.ID) == "" {
				continue
			}
			col := Column{ID: c.ID, Title: Clean(c.Title, 30, false), Total: Clean(c.Total, 30, false), Note: Clean(c.Note, 40, false), Tone: tone(c.Tone)}
			for j, cd := range c.Cards {
				if j >= maxCards || tokenOr(cd.ID) == "" {
					continue
				}
				p := cd.Progress
				if p < 0 || p > 100 {
					p = -1
				}
				col.Cards = append(col.Cards, Card{ID: cd.ID, Title: Clean(cd.Title, 60, false), Subtitle: Clean(cd.Subtitle, 60, false), Amount: Clean(cd.Amount, 30, false), Progress: p,
					Left: Clean(cd.Left, 20, false), Right: Clean(cd.Right, 20, false), Tone: tone(cd.Tone), Badge: Clean(cd.Badge, 40, false), BadgeTone: tone(cd.BadgeTone)})
			}
			b.Columns = append(b.Columns, col)
		}
		o.Board = b
	}
	if v.List != nil {
		l := &List{}
		for _, c := range v.List.Columns {
			l.Columns = append(l.Columns, Clean(c, 24, false))
		}
		for i, r := range v.List.Rows {
			if i >= maxRows || tokenOr(r.ID) == "" {
				continue
			}
			row := Row{ID: r.ID, Tone: tone(r.Tone), Badge: Clean(r.Badge, 30, false)}
			for j, c := range r.Cells {
				if j < 8 {
					row.Cells = append(row.Cells, Clean(c, 80, false))
				}
			}
			l.Rows = append(l.Rows, row)
		}
		o.List = l
	}
	for i, s := range v.Sections {
		if i >= maxSections {
			break
		}
		sec := Section{Title: Clean(s.Title, 40, false)}
		for j, ln := range s.Lines {
			if j >= maxSectionLines {
				break
			}
			sec.Lines = append(sec.Lines, sanitizeLine(ln))
		}
		o.Sections = append(o.Sections, sec)
	}
	o.Panels = sanitizePanels(v.Panels)
	for i, n := range v.Notices {
		if i < maxNotices {
			o.Notices = append(o.Notices, Notice{Tone: tone(n.Tone), Text: Clean(n.Text, 160, false), Target: sanitizeTarget(n.Target)})
		}
	}
	used := map[string]bool{}
	for _, a := range v.Actions {
		if len(o.Actions) >= maxActions || !ValidActionKey(a.Key) || used[a.Key] || tokenOr(a.ID) == "" {
			continue
		}
		used[a.Key] = true
		scope := a.Scope
		if scope != "item" {
			scope = "view"
		}
		o.Actions = append(o.Actions, Action{Key: a.Key, Label: Clean(a.Label, 24, false), ID: a.ID, Scope: scope, Confirm: Clean(a.Confirm, 160, false)})
	}
	return o
}

// SanitizeResult bounds and cleans an action result.
func SanitizeResult(r ActionResult) ActionResult {
	o := ActionResult{Notice: Clean(r.Notice, 200, false), Tone: tone(r.Tone), Refresh: r.Refresh, State: sanitizeMap(r.State, 16), Close: r.Close, Open: sanitizeTarget(r.Open), Title: Clean(r.Title, 60, false)}
	if len(r.Errors) > 0 {
		o.Errors = map[string]string{}
		for k, v := range r.Errors {
			if len(o.Errors) < maxFormFields && tokenOr(k) != "" {
				o.Errors[k] = Clean(v, 160, false)
			}
		}
	}
	if r.Form != nil {
		f := SanitizeForm(*r.Form)
		o.Form = &f
	}
	if r.Confirm != nil && tokenOr(r.Confirm.Action) != "" {
		o.Confirm = &Confirm{Text: Clean(r.Confirm.Text, 240, false), Action: r.Confirm.Action, Args: sanitizeMap(r.Confirm.Args, 8), Target: tokenOr(r.Confirm.Target)}
	}
	c := SanitizeContent(Content{Lines: r.Lines})
	o.Lines = c.Lines
	return o
}

// SanitizeForm bounds and cleans an untrusted form.
func SanitizeForm(f Form) Form {
	o := Form{ID: tokenOr(f.ID), Title: Clean(f.Title, 60, false), Target: tokenOr(f.Target), Args: sanitizeMap(f.Args, 8)}
	used := map[string]bool{}
	for _, fd := range f.Fields {
		if len(o.Fields) >= maxFormFields || tokenOr(fd.Key) == "" || used[fd.Key] {
			continue
		}
		used[fd.Key] = true
		t := fd.Type
		switch t {
		case "text", "multiline", "select", "contact", "contacts":
		default:
			t = "text"
		}
		n := FormField{Key: fd.Key, Label: Clean(fd.Label, 28, false), Type: t, Value: Clean(fd.Value, 4000, t == "multiline"), Help: Clean(fd.Help, 160, false), Required: fd.Required}
		for i, opt := range fd.Options {
			if i < 40 {
				n.Options = append(n.Options, Clean(opt, 40, false))
			}
		}
		for i, r := range fd.Refs {
			if i < 50 && tokenOr(r.ID) != "" {
				n.Refs = append(n.Refs, RefItem{ID: r.ID, Name: Clean(r.Name, 80, false), Kind: Clean(r.Kind, 20, false)})
			}
		}
		o.Fields = append(o.Fields, n)
	}
	return o
}

// SanitizeSignals bounds and cleans contributed signals.
func SanitizeSignals(in []Signal) []Signal {
	var out []Signal
	for i, s := range in {
		if i >= maxSignals || tokenOr(s.Key) == "" {
			continue
		}
		sev := s.Severity
		if sev != "danger" && sev != "warn" {
			sev = "info"
		}
		n := Signal{Key: s.Key, Type: Clean(s.Type, 40, false), Title: Clean(s.Title, 80, false), Severity: sev, Reason: Clean(s.Reason, 200, false), Date: Clean(s.Date, 20, false), Action: Clean(s.Action, 120, false), Target: sanitizeTarget(s.Target)}
		for j, f := range s.Facts {
			if j < 8 {
				n.Facts = append(n.Facts, Clean(f, 160, false))
			}
		}
		out = append(out, n)
	}
	return out
}
