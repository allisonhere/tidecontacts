import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Material
import QtQuick.Layouts
import Tide 1.0

ApplicationWindow {
    id: root
    width: 1180
    height: 800
    minimumWidth: 440
    minimumHeight: 500
    visible: true
    title: backend.demo ? qsTr("TideContacts — Demo profile") : qsTr("TideContacts — Local profile")
    color: Ink.paper
    Material.theme: Material.Light
    Material.accent: Ink.accent
    Material.foreground: Ink.ink
    Material.background: Ink.paper
    palette.window: Ink.paper
    palette.base: Ink.paper
    palette.text: Ink.ink
    palette.windowText: Ink.ink
    palette.buttonText: Ink.ink
    palette.highlight: Ink.accent
    palette.highlightedText: Ink.paper

    readonly property bool compact: width < 810
    readonly property bool hasRail: width >= 1010
    readonly property bool sceneHasRecord: recordPane.visible && recordPane.hasRecord
    readonly property bool dialogOpen: editor.visible || confirm.visible || keys.visible
    readonly property var workspaces: [
        { key: "people", label: qsTr("People"), help: qsTr("Find a person and open their relationship dossier.") },
        { key: "organizations", label: qsTr("Organizations"), help: qsTr("Find an organization and its people and work.") },
        { key: "tasks", label: qsTr("Tasks"), help: qsTr("Review follow-ups and work by status or date.") },
        { key: "activities", label: qsTr("Activities"), help: qsTr("Read the record of conversations and work.") },
        { key: "projects", label: qsTr("Projects"), help: qsTr("Review shared work, milestones and members.") },
        { key: "pipelines", label: qsTr("Pipelines"), help: qsTr("Review stages and move existing items.") }
    ]
    readonly property var taskViews: [
        { key: "all", label: qsTr("All tasks") }, { key: "open", label: qsTr("Open") },
        { key: "inbox", label: qsTr("Inbox") }, { key: "today", label: qsTr("Today") },
        { key: "upcoming", label: qsTr("Upcoming") }, { key: "waiting", label: qsTr("Waiting") },
        { key: "overdue", label: qsTr("Overdue") }, { key: "completed", label: qsTr("Completed") }
    ]
    readonly property var sorts: [
        { key: "name", label: qsTr("By name") }, { key: "updated", label: qsTr("Recently updated") }
    ]
    readonly property var scope: workspaces.find(entry => entry.key === backend.workspace) || workspaces[0]

    function chooseWorkspace(kind) {
        if (kind !== backend.workspace) {
            searchWait.stop()
            backend.browse(kind)
            if (!hasRail && !root.compact)
                return
            if (root.compact)
                recordList.forceActiveFocus(Qt.OtherFocusReason)
        }
    }

    function chooseRecord(row) {
        const entry = backend.rows[row]
        if (!entry)
            return
        recordList.currentIndex = row
        backend.select(entry.ID)
        if (root.compact)
            Qt.callLater(recordPane.focusStart)
        else
            recordList.forceActiveFocus(Qt.OtherFocusReason)
    }

    function focusRecords() {
        if (root.compact && backend.inspecting && !backend.nested)
            backend.back()
        if (indexPane.visible) {
            recordList.forceActiveFocus(Qt.ShortcutFocusReason)
            if (recordList.currentIndex >= 0)
                recordList.positionViewAtIndex(recordList.currentIndex, ListView.Contain)
        }
    }

    function returnToIndex() {
        const nested = backend.nested
        backend.back()
        if (nested)
            Qt.callLater(recordPane.focusStart)
        else
            focusRecords()
    }

    function findRecord() {
        if (root.compact) {
            while (backend.nested)
                backend.back()
            if (backend.inspecting)
                backend.back()
        }
        searchField.forceActiveFocus(Qt.ShortcutFocusReason)
        searchField.selectAll()
    }

    function focusDossier() {
        if (root.compact && !backend.inspecting) {
            chooseRecord(recordList.currentIndex)
            return
        }
        recordPane.focusStart()
    }

    function startEdit(create) {
        editor.begin(create, create ? backend.workspace : backend.recordKind, create ? ({}) : backend.record)
    }

    onClosing: event => {
        if (editor.visible || backend.writing) {
            event.accepted = false
            if (editor.visible && !backend.writing)
                editor.requestClose()
        }
    }

    Shortcut { sequence: "Ctrl+K"; context: Qt.WindowShortcut; enabled: !root.dialogOpen && !backend.writing; onActivated: root.findRecord() }
    Shortcut { sequence: "Alt+1"; context: Qt.WindowShortcut; enabled: !root.dialogOpen; onActivated: {
        const group = root.hasRail ? railLinks : compactLinks
        const control = group.itemAt(root.workspaces.findIndex(entry => entry.key === backend.workspace))
        if (control)
            control.forceActiveFocus(Qt.ShortcutFocusReason)
    } }
    Shortcut { sequence: "Alt+2"; context: Qt.WindowShortcut; enabled: !root.dialogOpen && !backend.writing; onActivated: {
        while (root.compact && backend.nested)
            backend.back()
        root.focusRecords()
    } }
    Shortcut { sequence: "Alt+3"; context: Qt.WindowShortcut; enabled: !root.dialogOpen && !backend.writing; onActivated: root.focusDossier() }
    Shortcut { sequence: "Ctrl+N"; context: Qt.WindowShortcut; enabled: !root.dialogOpen && addRecord.visible && addRecord.enabled; onActivated: root.startEdit(true) }
    Shortcut { sequences: [StandardKey.Refresh]; context: Qt.WindowShortcut; enabled: !root.dialogOpen && !backend.writing; onActivated: backend.refresh() }
    Shortcut { sequences: [StandardKey.Quit]; context: Qt.WindowShortcut; onActivated: root.close() }
    Shortcut { sequences: ["F1", "?"]; context: Qt.WindowShortcut; enabled: !root.dialogOpen && !searchField.activeFocus; onActivated: keys.open() }
    Shortcut {
        sequence: "Escape"
        context: Qt.WindowShortcut
        enabled: !root.dialogOpen && !taskFilter.popup.visible && !contactSort.popup.visible
                 && !searchField.activeFocus && !backend.writing && (backend.inspecting || recordPane.activeFocus)
        onActivated: root.returnToIndex()
    }

    component Kicker : Label {
        color: Ink.accent
        font.family: Ink.fixed
        font.pixelSize: 10
        font.weight: Font.DemiBold
        font.letterSpacing: 0.65
        textFormat: Text.PlainText
    }

    component Explanation : Label {
        color: Ink.quiet
        font.pixelSize: 13
        wrapMode: Text.WordWrap
        textFormat: Text.PlainText
    }

    header: Rectangle {
        implicitHeight: 58
        color: Ink.paper
        Rectangle { anchors.bottom: parent.bottom; width: parent.width; height: 1; color: Ink.line }
        RowLayout {
            anchors.fill: parent
            anchors.leftMargin: 20
            anchors.rightMargin: 18
            spacing: 11
            Item {
                Layout.preferredWidth: 24
                Layout.preferredHeight: 24
                Accessible.ignored: true
                Rectangle { x: 2; y: 4; width: 18; height: 2; color: Ink.accent }
                Rectangle { x: 10; y: 5; width: 2; height: 14; color: Ink.accent }
                Rectangle { x: 5; y: 20; width: 13; height: 1; color: Ink.rule }
                Rectangle { x: 20; y: 1; width: 1; height: 6; color: Ink.rule }
            }
            Label { text: "TideContacts"; font.pixelSize: 17; font.weight: Font.DemiBold; color: Ink.ink }
            Rectangle { Layout.preferredHeight: 17; Layout.preferredWidth: 1; color: Ink.line; visible: !root.compact }
            Kicker { text: qsTr("RELATIONSHIP DESK"); visible: !root.compact }
            Item { Layout.fillWidth: true }
            Rectangle { Layout.preferredWidth: 6; Layout.preferredHeight: 6; color: backend.ready ? Ink.accent : Ink.rule }
            Label { text: backend.demo ? qsTr("Demo profile") : qsTr("Local profile"); color: Ink.quiet; font.pixelSize: 12 }
            DeskButton { text: qsTr("Keys"); Accessible.name: qsTr("Keyboard shortcuts"); onClicked: keys.open() }
        }
    }

    ColumnLayout {
        anchors.fill: parent
        spacing: 0
        Rectangle {
            Layout.fillWidth: true
            Layout.preferredHeight: compactNav.implicitHeight + 12
            color: Ink.rail
            visible: !root.hasRail
            Flow {
                id: compactNav
                x: 12
                y: 6
                width: parent.width - 24
                spacing: 3
                Repeater {
                    id: compactLinks
                    model: root.hasRail ? [] : root.workspaces
                    delegate: DeskButton {
                        required property var modelData
                        objectName: "workspace-" + modelData.key
                        text: modelData.label
                        current: backend.workspace === modelData.key
                        enabled: !backend.writing
                        onClicked: root.chooseWorkspace(modelData.key)
                    }
                }
            }
            Rectangle { anchors.bottom: parent.bottom; width: parent.width; height: 1; color: Ink.line }
        }
        Explanation {
            Layout.fillWidth: true
            Layout.margins: 12
            visible: text.length > 0
            text: backend.sessionError
            color: Ink.error
        }
        RowLayout {
            Layout.fillWidth: true
            Layout.fillHeight: true
            spacing: 0

            Rectangle {
                Layout.preferredWidth: 179
                Layout.fillHeight: true
                color: Ink.rail
                visible: root.hasRail
                Rectangle { anchors.right: parent.right; width: 1; height: parent.height; color: Ink.line }
                ColumnLayout {
                    anchors.fill: parent
                    anchors.margins: 19
                    anchors.topMargin: 26
                    spacing: 6
                    Kicker { text: qsTr("WORKSPACE / COMMUNITY") }
                    Label {
                        text: qsTr("Relationship\ndesk")
                        color: Ink.ink
                        font.family: Ink.serif
                        font.pixelSize: 27
                        Layout.bottomMargin: 19
                    }
                    Repeater {
                        id: railLinks
                        model: root.hasRail ? root.workspaces : []
                        delegate: DeskButton {
                            required property var modelData
                            Layout.fillWidth: true
                            objectName: "workspace-" + modelData.key
                            text: modelData.label
                            current: backend.workspace === modelData.key
                            enabled: !backend.writing
                            onClicked: root.chooseWorkspace(modelData.key)
                        }
                    }
                    Item { Layout.fillHeight: true }
                    Rectangle { Layout.fillWidth: true; Layout.preferredHeight: 1; color: Ink.line }
                    Explanation {
                        Layout.fillWidth: true
                        text: backend.demo ? qsTr("Fictional records. Changes stay in the demo profile.")
                                           : qsTr("Changes are saved to your local profile.")
                    }
                }
            }

            Rectangle {
                id: indexPane
                visible: !root.compact || !backend.inspecting
                Layout.fillWidth: root.compact
                Layout.preferredWidth: root.compact ? root.width : Math.min(340, Math.round(root.width * 0.31))
                Layout.fillHeight: true
                color: Ink.index
                Rectangle { anchors.right: parent.right; width: 1; height: parent.height; color: Ink.line }
                ColumnLayout {
                    anchors.fill: parent
                    anchors.topMargin: 24
                    anchors.leftMargin: root.compact ? 18 : 15
                    anchors.rightMargin: root.compact ? 18 : 16
                    anchors.bottomMargin: 10
                    spacing: 7
                    Kicker { text: qsTr("RECORD INDEX") }
                    Label {
                        Layout.fillWidth: true
                        text: root.scope.label
                        color: Ink.ink
                        font.family: Ink.serif
                        font.pixelSize: 33
                        elide: Text.ElideRight
                        Accessible.role: Accessible.Heading
                    }
                    Explanation { Layout.fillWidth: true; text: root.scope.help }
                    TextField {
                        id: searchField
                        objectName: "indexSearch"
                        Layout.fillWidth: true
                        Layout.topMargin: 3
                        placeholderText: qsTr("Search %1").arg(root.scope.label)
                        Accessible.name: placeholderText
                        text: backend.query
                        enabled: !backend.writing
                        onTextEdited: searchWait.restart()
                        onAccepted: { searchWait.stop(); backend.search(text) }
                        Keys.onEscapePressed: root.focusRecords()
                    }
                    Timer { id: searchWait; interval: 230; onTriggered: backend.search(searchField.text) }
                    RowLayout {
                        Layout.fillWidth: true
                        spacing: 5
                        ComboBox {
                            id: taskFilter
                            objectName: "taskView"
                            Layout.fillWidth: true
                            visible: backend.workspace === "tasks"
                            enabled: !backend.writing
                            model: root.taskViews
                            textRole: "label"
                            valueRole: "key"
                            Accessible.name: qsTr("Task view")
                            currentIndex: root.taskViews.findIndex(entry => entry.key === backend.taskView)
                            onActivated: backend.setTaskView(currentValue)
                        }
                        ComboBox {
                            id: contactSort
                            objectName: "contactSort"
                            Layout.fillWidth: true
                            visible: backend.workspace === "people" || backend.workspace === "organizations"
                            enabled: !backend.writing
                            model: root.sorts
                            textRole: "label"
                            valueRole: "key"
                            Accessible.name: qsTr("Contact sort order")
                            currentIndex: root.sorts.findIndex(entry => entry.key === backend.sort)
                            onActivated: backend.setSort(currentValue)
                        }
                        Item { Layout.fillWidth: true; visible: !taskFilter.visible && !contactSort.visible }
                        DeskButton {
                            id: addRecord
                            objectName: "newRecord"
                            visible: backend.workspace !== "pipelines"
                            enabled: backend.ready && !backend.writing
                            text: qsTr("New…")
                            bordered: true
                            Accessible.name: qsTr("Create a record in %1").arg(root.scope.label)
                            onClicked: root.startEdit(true)
                        }
                        DeskButton {
                            objectName: "refresh"
                            text: qsTr("Refresh")
                            enabled: backend.ready && !backend.loading && !backend.writing
                            onClicked: backend.refresh()
                        }
                    }
                    Rectangle { Layout.fillWidth: true; Layout.preferredHeight: 1; color: Ink.rule }
                    Explanation {
                        Layout.fillWidth: true
                        visible: backend.loading || backend.indexError.length > 0 || backend.rows.length === 0
                        text: backend.indexError.length > 0 ? backend.indexError
                            : !backend.ready ? (backend.sessionError ? qsTr("Service unavailable.") : qsTr("Opening the profile…"))
                            : backend.loading ? qsTr("Loading records…") : qsTr("No matching records.")
                        color: backend.indexError.length > 0 ? Ink.error : Ink.quiet
                    }
                    ListView {
                        id: recordList
                        objectName: "indexList"
                        Layout.fillWidth: true
                        Layout.fillHeight: true
                        clip: true
                        activeFocusOnTab: true
                        Accessible.role: Accessible.List
                        Accessible.name: qsTr("%1 result list").arg(root.scope.label)
                        keyNavigationEnabled: false
                        boundsBehavior: Flickable.StopAtBounds
                        model: backend.rows
                        ScrollBar.vertical: ScrollBar { policy: ScrollBar.AsNeeded }
                        Keys.onDownPressed: { incrementCurrentIndex(); positionViewAtIndex(currentIndex, ListView.Contain) }
                        Keys.onUpPressed: { decrementCurrentIndex(); positionViewAtIndex(currentIndex, ListView.Contain) }
                        Keys.onReturnPressed: root.chooseRecord(currentIndex)
                        Keys.onEnterPressed: root.chooseRecord(currentIndex)
                        Keys.onSpacePressed: root.chooseRecord(currentIndex)
                        delegate: DeskButton {
                            id: rowButton
                            required property int index
                            required property var modelData
                            readonly property string subtitle: backend.workspace === "people" || backend.workspace === "organizations"
                                ? [modelData.Organization, modelData.PrimaryEmail, modelData.Archived ? qsTr("Archived") : ""].filter(Boolean).join(" · ")
                                : [modelData.Status || modelData.Type, (modelData.Due || modelData.Target || modelData.OccurredAt || "").substring(0, 10)].filter(Boolean).join(" · ")
                            objectName: "record-" + index
                            width: ListView.view.width - (ListView.view.contentHeight > ListView.view.height ? 11 : 0)
                            text: modelData.Name || modelData.Title || qsTr("Untitled record")
                            enabled: !backend.writing
                            current: backend.recordKind === backend.workspace && backend.recordId === modelData.ID
                            focusPolicy: Qt.NoFocus
                            Accessible.name: subtitle ? qsTr("%1, %2").arg(text).arg(subtitle) : text
                            contentItem: Column {
                                spacing: 2
                                Label {
                                    width: parent.width
                                    text: rowButton.text
                                    color: rowButton.current ? Ink.accent : Ink.ink
                                    font.pixelSize: 14
                                    font.weight: Font.DemiBold
                                    elide: Text.ElideRight
                                    textFormat: Text.PlainText
                                }
                                Explanation {
                                    width: parent.width
                                    text: rowButton.subtitle
                                    visible: text.length > 0
                                    font.pixelSize: 11
                                    elide: Text.ElideRight
                                    wrapMode: Text.NoWrap
                                }
                            }
                            Rectangle { anchors.bottom: parent.bottom; width: parent.width; height: 1; color: Ink.line }
                            Rectangle {
                                anchors.fill: parent
                                visible: recordList.activeFocus && rowButton.ListView.isCurrentItem
                                color: "transparent"
                                border.width: 2
                                border.color: Ink.accent
                            }
                            onClicked: root.chooseRecord(index)
                        }
                    }
                    RowLayout {
                        Layout.fillWidth: true
                        visible: backend.offset > 0 || backend.hasNext
                        DeskButton { text: qsTr("Previous"); enabled: backend.offset > 0 && !backend.loading && !backend.writing; onClicked: backend.previousPage() }
                        Item { Layout.fillWidth: true }
                        Kicker { text: backend.rows.length ? qsTr("%1–%2").arg(backend.offset + 1).arg(backend.offset + backend.rows.length) : "" }
                        Item { Layout.fillWidth: true }
                        DeskButton { text: qsTr("Next"); enabled: backend.hasNext && !backend.loading && !backend.writing; onClicked: backend.nextPage() }
                    }
                }
            }

            RecordPane {
                id: recordPane
                Layout.fillWidth: true
                Layout.fillHeight: true
                visible: !root.compact || backend.inspecting
                compact: root.compact
                onBackRequested: root.returnToIndex()
                onEditRequested: root.startEdit(false)
                onArchiveRequested: archived => confirm.begin(archived ? "archive" : "restore", {})
                onCompleteRequested: confirm.begin("complete", {})
                onMoveRequested: item => confirm.begin("move", item)
            }
        }
    }

    RecordEditor { id: editor }

    Dialog {
        id: confirm
        objectName: "recordConfirmation"
        anchors.centerIn: parent
        width: Math.min(root.width - 30, 470)
        modal: true
        focus: true
        closePolicy: backend.writing ? Popup.NoAutoClose : Popup.CloseOnEscape
        property string action: ""
        property var item: ({})
        readonly property var stages: action === "move" ? (backend.record.Stages || []).filter(stage => stage.ID !== item.StageID) : []
        title: action === "move" ? qsTr("Move pipeline item") : action === "complete" ? qsTr("Complete task")
               : action === "archive" ? qsTr("Archive contact") : qsTr("Restore contact")

        function begin(kind, target) {
            backend.clearWriteError()
            action = kind
            item = target
            stageChoice.currentIndex = -1
            open()
        }

        onOpened: (action === "move" ? stageChoice : cancelAction).forceActiveFocus(Qt.PopupFocusReason)
        Connections { target: backend; function onWriteSucceeded() { if (confirm.visible) confirm.close() } }
        contentItem: ColumnLayout {
            spacing: 10
            Explanation {
                Layout.fillWidth: true
                text: confirm.action === "move" ? qsTr("Move “%1” from “%2” to the selected stage.").arg(confirm.item.Title || "").arg(confirm.item.Stage || "")
                    : confirm.action === "complete" ? qsTr("Mark “%1” as completed.").arg(backend.record.Title || "")
                    : confirm.action === "archive" ? qsTr("Archive “%1” in this profile.").arg(backend.record.DisplayName || "")
                    : qsTr("Restore “%1” in this profile.").arg(backend.record.DisplayName || "")
            }
            ComboBox {
                id: stageChoice
                objectName: "stageChoice"
                Layout.fillWidth: true
                visible: confirm.action === "move"
                model: confirm.stages
                textRole: "Name"
                valueRole: "ID"
                displayText: currentIndex < 0 ? qsTr("Choose a destination stage") : currentText
                Accessible.name: qsTr("Destination stage")
                enabled: !backend.writing
            }
            Explanation { Layout.fillWidth: true; visible: text.length > 0; text: backend.writeError || backend.sessionError; color: Ink.error }
            RowLayout {
                Layout.alignment: Qt.AlignRight
                DeskButton { id: cancelAction; text: qsTr("Cancel"); enabled: !backend.writing; onClicked: confirm.close() }
                DeskButton {
                    objectName: "confirmAction"
                    text: backend.writing ? qsTr("Saving…") : confirm.action === "move" ? qsTr("Move") : confirm.action === "complete" ? qsTr("Complete")
                          : confirm.action === "archive" ? qsTr("Archive") : qsTr("Restore")
                    bordered: true
                    enabled: backend.ready && !backend.writing && (confirm.action !== "move" || stageChoice.currentIndex >= 0)
                    onClicked: {
                        if (confirm.action === "move")
                            backend.moveItem(confirm.item.ID, stageChoice.currentValue)
                        else if (confirm.action === "complete")
                            backend.completeTask()
                        else
                            backend.archiveContact(confirm.action === "archive")
                    }
                }
            }
        }
    }

    Dialog {
        id: keys
        anchors.centerIn: parent
        width: Math.min(root.width - 30, 410)
        modal: true
        focus: true
        title: qsTr("Keyboard shortcuts")
        standardButtons: Dialog.Close
        contentItem: Explanation {
            text: qsTr("Ctrl+K   Find a record in this workspace\nAlt+1   Focus workspace navigation\nAlt+2   Focus the record index\nAlt+3   Focus the dossier\n↑ / ↓   Move inside the index\nEnter   Open the focused record\nEsc   Return to the previous record or index\nCtrl+N   Create a record\nF5   Refresh the workspace\nCtrl+Q   Close the window")
            lineHeight: 1.6
        }
    }
}
