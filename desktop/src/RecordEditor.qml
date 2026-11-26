import QtQuick
import QtQuick.Controls
import QtQuick.Layouts
import Tide 1.0

// Edit or create a record in a modal. Only changed fields are sent, and the
// backend adds the revision it last read, so a stale draft is refused.
Dialog {
    id: editor
    objectName: "recordEditor"
    anchors.centerIn: parent
    width: Math.min(parent.width - 30, 560)
    height: Math.min(parent.height - 30, implicitHeight)
    modal: true
    focus: true
    closePolicy: Popup.NoAutoClose

    property bool creating: false
    property string kind: "people"
    property var original: ({})
    property bool discarding: false
    property string fieldError: ""
    readonly property bool contact: kind === "people" || kind === "organizations"
    readonly property var statuses: kind === "tasks" ? ["inbox", "next", "scheduled", "waiting", "completed", "canceled"]
                                                     : ["planned", "active", "on-hold", "completed", "archived"]
    readonly property var activityTypes: ["email", "call", "meeting", "conversation", "message", "note", "event", "project-update"]
    readonly property var dateLabel: kind === "tasks" ? qsTr("Due date") : kind === "projects" ? qsTr("Target date") : qsTr("Occurred on")
    title: (creating ? qsTr("New ") : qsTr("Edit ")) + ({ people: qsTr("person"), organizations: qsTr("organization"), tasks: qsTr("task"),
                                                     activities: qsTr("activity"), projects: qsTr("project") }[kind] || qsTr("record"))

    function textOf(key) { return original[key] ? String(original[key]) : "" }
    function begin(create, recordKind, record) {
        creating = create
        kind = recordKind
        original = record || ({})
        discarding = false
        fieldError = ""
        backend.clearWriteError()
        const day = value => value ? String(value).slice(0, 10) : ""
        if (contact) {
            primary.text = textOf("DisplayName")
            body.text = textOf("Notes")
            website.text = textOf("Website")
            industry.text = textOf("Industry")
            tags.text = (original.Tags || []).join(", ")
            email.text = ""
        } else if (kind === "tasks") {
            primary.text = textOf("Title"); body.text = textOf("Description"); date.text = day(original.Due)
            choice.currentIndex = Math.max(0, statuses.indexOf(original.Status || "inbox")); priority.currentIndex = original.Priority || 0
        } else if (kind === "activities") {
            primary.text = textOf("Title"); body.text = textOf("Body"); date.text = day(original.OccurredAt)
            choice.currentIndex = Math.max(0, activityTypes.indexOf(original.Type || "note"))
        } else {
            primary.text = textOf("Name"); body.text = textOf("Description"); date.text = day(original.Target)
            choice.currentIndex = Math.max(0, statuses.indexOf(original.Status || "planned")); priority.currentIndex = original.Priority || 0
        }
        open()
    }
    function fields() {
        const out = {}
        const set = (key, value, before) => { if (creating ? value !== "" : value !== before) out[key] = value }
        if (contact) {
            set("name", primary.text.trim(), textOf("DisplayName"))
            set("notes", body.text, textOf("Notes"))
            set("website", website.text.trim(), textOf("Website"))
            if (kind === "organizations") set("industry", industry.text.trim(), textOf("Industry"))
            const list = tags.text.split(",").map(tag => tag.trim()).filter(Boolean)
            if (list.join(",") !== (original.Tags || []).join(",") && (creating ? list.length > 0 : true)) out.tags = list
            if (creating && email.text.trim() !== "") out.email = email.text.trim()
        } else if (kind === "tasks" || kind === "projects") {
            set(kind === "tasks" ? "title" : "name", primary.text.trim(), textOf(kind === "tasks" ? "Title" : "Name"))
            set("description", body.text, textOf("Description"))
            set("status", statuses[choice.currentIndex], textOf("Status"))
            set("date", date.text.trim(), kind === "tasks" ? textOf("Due").slice(0, 10) : textOf("Target").slice(0, 10))
            if (priority.currentIndex !== (original.Priority || 0) || (creating && priority.currentIndex > 0)) out.priority = priority.currentIndex
        } else {
            set("title", primary.text.trim(), textOf("Title"))
            set("description", body.text, textOf("Body"))
            set("type", activityTypes[choice.currentIndex], textOf("Type"))
            set("date", date.text.trim(), textOf("OccurredAt").slice(0, 10))
        }
        return out
    }
    function dirty() {
        const changed = fields()
        if (creating)
            return primary.text.trim() !== "" || body.text !== "" || Object.keys(changed).some(key => key !== "status" && key !== "type")
        return Object.keys(changed).length > 0
    }
    function validate() {
        if (primary.text.trim() === "")
            return contact || kind === "projects" ? qsTr("Enter a name.") : qsTr("Enter a title.")
        if (!contact && date.text.trim() !== "" && !/^\d{4}-\d{2}-\d{2}$/.test(date.text.trim()))
            return qsTr("Enter the date as YYYY-MM-DD.")
        if (creating && contact && email.text.trim() !== "" && !/^[^\s@]+@[^\s@]+$/.test(email.text.trim()))
            return qsTr("Enter a valid email address.")
        return ""
    }
    function save() {
        fieldError = validate()
        if (fieldError !== "")
            return
        backend.saveRecord(creating, fields())
    }
    function requestClose() {
        if (backend.writing)
            return
        if (dirty() && !discarding) {
            discarding = true
            keepEditing.forceActiveFocus(Qt.TabFocusReason)
        } else {
            close()
        }
    }

    Connections { target: backend; function onWriteSucceeded() { if (editor.visible) editor.close() } }
    onOpened: primary.forceActiveFocus(Qt.PopupFocusReason)

    component Caption : Label {
        color: Ink.quiet
        font.pixelSize: 12
        font.weight: Font.Medium
        textFormat: Text.PlainText
    }

    contentItem: ScrollView {
        id: scroll
        clip: true
        contentWidth: availableWidth
        Keys.onEscapePressed: editor.requestClose()
        ColumnLayout {
            width: scroll.availableWidth
            spacing: 8

            Caption { text: editor.contact || editor.kind === "projects" ? qsTr("Name") : qsTr("Title") }
            TextField { id: primary; objectName: "editorName"; Layout.fillWidth: true; enabled: !backend.writing; Accessible.name: qsTr("Name or title"); onAccepted: editor.save() }

            Caption { text: editor.contact ? qsTr("Notes") : editor.kind === "activities" ? qsTr("Activity notes") : qsTr("Description") }
            TextArea {
                id: body
                Layout.fillWidth: true
                Layout.preferredHeight: 90
                enabled: !backend.writing
                wrapMode: TextArea.Wrap
                Accessible.name: qsTr("Notes or description")
                KeyNavigation.priority: KeyNavigation.BeforeItem
                Keys.onTabPressed: event => { event.accepted = true; nextItemInFocusChain(true).forceActiveFocus(Qt.TabFocusReason) }
                Keys.onBacktabPressed: event => { event.accepted = true; nextItemInFocusChain(false).forceActiveFocus(Qt.BacktabFocusReason) }
            }

            // People and organizations
            Caption { visible: editor.creating && editor.contact; text: qsTr("Manual email") }
            TextField { id: email; visible: editor.creating && editor.contact; Layout.fillWidth: true; enabled: !backend.writing; inputMethodHints: Qt.ImhEmailCharactersOnly; Accessible.name: qsTr("Manual email") }
            Caption { visible: editor.contact; text: qsTr("Website") }
            TextField { id: website; visible: editor.contact; Layout.fillWidth: true; enabled: !backend.writing; Accessible.name: qsTr("Website") }
            Caption { visible: editor.kind === "organizations"; text: qsTr("Industry") }
            TextField { id: industry; visible: editor.kind === "organizations"; Layout.fillWidth: true; enabled: !backend.writing; Accessible.name: qsTr("Industry") }
            Caption { visible: editor.contact; text: qsTr("Tags") }
            TextField { id: tags; visible: editor.contact; Layout.fillWidth: true; enabled: !backend.writing; placeholderText: qsTr("Separate tags with commas"); Accessible.name: qsTr("Tags") }

            // Tasks, activities and projects
            Caption { visible: !editor.contact; text: editor.kind === "activities" ? qsTr("Type") : qsTr("Status") }
            ComboBox {
                id: choice
                visible: !editor.contact
                Layout.fillWidth: true
                enabled: !backend.writing
                model: editor.kind === "activities" ? editor.activityTypes : editor.statuses
                displayText: currentText.charAt(0).toUpperCase() + currentText.slice(1).replace(/-/g, " ")
                Accessible.name: editor.kind === "activities" ? qsTr("Activity type") : qsTr("Status")
            }
            Caption { visible: !editor.contact; text: editor.dateLabel + qsTr(" (YYYY-MM-DD)") }
            TextField { id: date; visible: !editor.contact; Layout.fillWidth: true; enabled: !backend.writing; inputMethodHints: Qt.ImhDate; Accessible.name: editor.dateLabel }
            Caption { visible: editor.kind === "tasks" || editor.kind === "projects"; text: qsTr("Priority") }
            ComboBox {
                id: priority
                visible: editor.kind === "tasks" || editor.kind === "projects"
                Layout.fillWidth: true
                enabled: !backend.writing
                model: [qsTr("None"), qsTr("Low"), qsTr("Medium"), qsTr("High")]
                Accessible.name: qsTr("Priority")
            }

            Label {
                Layout.fillWidth: true
                visible: text.length > 0
                color: Ink.error
                wrapMode: Text.Wrap
                textFormat: Text.PlainText
                text: editor.fieldError || backend.writeError || backend.sessionError
                Accessible.role: Accessible.AlertMessage
            }
            Label {
                Layout.fillWidth: true
                visible: editor.discarding
                color: Ink.ochre
                font.weight: Font.DemiBold
                wrapMode: Text.Wrap
                text: qsTr("Discard the unsaved changes?")
            }
            RowLayout {
                Layout.alignment: Qt.AlignRight
                DeskButton { id: keepEditing; visible: editor.discarding; text: qsTr("Keep editing"); onClicked: { editor.discarding = false; primary.forceActiveFocus() } }
                DeskButton { visible: editor.discarding; text: qsTr("Discard changes"); bordered: true; onClicked: editor.close() }
                DeskButton { visible: !editor.discarding; objectName: "editorCancel"; text: qsTr("Cancel"); enabled: !backend.writing; onClicked: editor.requestClose() }
                DeskButton {
                    visible: !editor.discarding
                    objectName: "editorSave"
                    bordered: true
                    text: backend.writing ? qsTr("Saving…") : editor.creating ? qsTr("Create") : qsTr("Save changes")
                    enabled: backend.ready && !backend.writing && (editor.creating || editor.dirty())
                    onClicked: editor.save()
                }
            }
        }
    }
}
