import QtQuick
import QtQuick.Controls
import QtQuick.Layouts
import Tide 1.0

// The selected record: a dossier for people and organizations, plain detail for
// tasks, activities and projects, and a stage board for general pipelines.
FocusScope {
    id: pane

    property bool compact: false
    readonly property string kind: backend.recordKind
    readonly property var record: backend.record
    readonly property var dossier: backend.dossier
    readonly property bool hasRecord: !backend.detailLoading && backend.recordId.length > 0 && backend.detailError.length === 0
                                      && Object.keys(backend.record).length > 0
    readonly property bool isContact: kind === "people" || kind === "organizations"
    readonly property string heading: kind === "people" || kind === "organizations" ? (record.DisplayName || qsTr("Unnamed contact"))
        : kind === "tasks" || kind === "activities" ? (record.Title || qsTr("Untitled record")) : (record.Name || qsTr("Unnamed record"))
    readonly property bool completable: kind === "tasks" && record.Status !== "completed" && record.Status !== "canceled"
    readonly property bool editable: kind !== "pipelines"

    signal backRequested()
    signal editRequested()
    signal archiveRequested(bool archived)
    signal completeRequested()
    signal moveRequested(var item)

    function focusStart() {
        if (backButton.visible)
            backButton.forceActiveFocus(Qt.TabFocusReason)
        else
            scroller.forceActiveFocus(Qt.TabFocusReason)
    }
    property bool wantFocus: false
    onHasRecordChanged: {
        if (hasRecord && wantFocus) {
            wantFocus = false
            Qt.callLater(focusStart)
        }
    }
    function requestFocus() {
        if (hasRecord)
            focusStart()
        else
            wantFocus = true
    }
    function scrollBy(delta) {
        const flick = scroller.contentItem
        flick.contentY = Math.max(0, Math.min(Math.max(0, flick.contentHeight - flick.height), flick.contentY + delta))
    }
    function scrollEdge(last) {
        const flick = scroller.contentItem
        flick.contentY = last ? Math.max(0, flick.contentHeight - flick.height) : 0
    }
    function list(value) { return value || [] }
    function day(value) { return value ? String(value).slice(0, 10) : "" }
    function words(value) { return value ? value.charAt(0).toUpperCase() + String(value).slice(1).replace(/-/g, " ") : "" }
    function refName(ref) { return ref.name || qsTr("Linked record") }

    component Label2 : Label {
        color: Ink.ink
        font.pixelSize: 14
        wrapMode: Text.Wrap
        textFormat: Text.PlainText
        Layout.fillWidth: true
    }
    component Heading : ColumnLayout {
        property alias text: title.text
        Layout.fillWidth: true
        Layout.topMargin: 18
        spacing: 4
        Label {
            id: title
            color: Ink.accent
            font.family: Ink.fixed
            font.pixelSize: 10
            font.weight: Font.DemiBold
            font.letterSpacing: 0.65
            textFormat: Text.PlainText
            Accessible.role: Accessible.Heading
        }
        Rectangle { Layout.fillWidth: true; height: 1; color: Ink.line }
    }
    component Fact : ColumnLayout {
        property string label
        property string value
        spacing: 1
        Label { text: parent.label; color: Ink.quiet; font.family: Ink.fixed; font.pixelSize: 10; font.letterSpacing: 0.5; textFormat: Text.PlainText }
        Label { text: parent.value; color: Ink.ink; font.pixelSize: 17; font.weight: Font.DemiBold; textFormat: Text.PlainText }
    }
    Rectangle { anchors.fill: parent; color: Ink.paper }

    ColumnLayout {
        anchors.fill: parent
        anchors.margins: pane.compact ? 14 : 26
        spacing: 10

        RowLayout {
            Layout.fillWidth: true
            visible: pane.hasRecord || backend.detailLoading || backend.detailError.length > 0
            spacing: 8
            DeskButton {
                id: backButton
                objectName: "recordBack"
                visible: pane.compact || backend.nested
                text: qsTr("‹ Back")
                bordered: true
                onClicked: pane.backRequested()
            }
            Label {
                Layout.fillWidth: true
                text: pane.hasRecord ? pane.heading : ""
                color: Ink.ink
                font.family: Ink.serif
                font.pixelSize: pane.compact ? 24 : 32
                elide: Text.ElideRight
                textFormat: Text.PlainText
                Accessible.role: Accessible.Heading
                Accessible.name: text
            }
            DeskButton { visible: pane.hasRecord && pane.completable; text: qsTr("Complete"); bordered: true; enabled: !backend.writing; onClicked: pane.completeRequested() }
            DeskButton {
                visible: pane.hasRecord && pane.isContact
                text: pane.record.Archived ? qsTr("Restore") : qsTr("Archive")
                bordered: true
                enabled: !backend.writing
                onClicked: pane.archiveRequested(!pane.record.Archived)
            }
            DeskButton { visible: pane.hasRecord && pane.editable; objectName: "recordEdit"; text: qsTr("Edit"); bordered: true; enabled: !backend.writing; onClicked: pane.editRequested() }
            DeskButton { visible: pane.hasRecord; text: qsTr("Copy ID"); enabled: !backend.writing; Accessible.name: qsTr("Copy the record identifier"); onClicked: backend.copyId() }
        }

        Label {
            Layout.fillWidth: true
            Layout.topMargin: 30
            visible: !pane.hasRecord && !backend.detailLoading && backend.detailError.length === 0
            horizontalAlignment: Text.AlignHCenter
            text: qsTr("Select a record to read it here.")
            color: Ink.quiet
        }
        Label { Layout.fillWidth: true; visible: backend.detailLoading; text: qsTr("Loading record…"); color: Ink.quiet }
        Label { Layout.fillWidth: true; visible: backend.detailError.length > 0; text: backend.detailError; color: Ink.error; wrapMode: Text.Wrap; textFormat: Text.PlainText }

        ScrollView {
            id: scroller
            Layout.fillWidth: true
            Layout.fillHeight: true
            visible: pane.hasRecord
            clip: true
            contentWidth: availableWidth
            focus: true
            Accessible.name: qsTr("Record detail")

            ColumnLayout {
                width: scroller.availableWidth
                spacing: 6

                // Subtitle and key facts
                Label2 {
                    visible: text.length > 0
                    color: Ink.quiet
                    text: {
                        const r = pane.record
                        if (pane.kind === "tasks") return pane.words(r.Status) + (r.Archived ? " · " + qsTr("Archived") : "")
                        if (pane.kind === "activities") return pane.words(r.Type) + " · " + pane.day(r.OccurredAt)
                        if (pane.kind === "projects") return pane.words(r.Status)
                        if (pane.isContact) {
                            const m = pane.list(r.Memberships).find(entry => entry.Current)
                            return [m ? [m.Title, m.Organization].filter(Boolean).join(" · ") : "", r.Industry, r.Archived ? qsTr("Archived") : ""].filter(Boolean).join(" · ")
                        }
                        return r.Archived ? qsTr("Archived pipeline") : ""
                    }
                }
                Flow {
                    Layout.fillWidth: true
                    spacing: 30
                    visible: pane.isContact || pane.kind === "tasks" || pane.kind === "projects"
                    Fact { visible: pane.isContact; label: qsTr("LAST INTERACTION"); value: pane.dossier.FollowUp && pane.dossier.FollowUp.LastInteraction ? pane.dossier.FollowUp.LastInteraction : qsTr("Not recorded") }
                    Fact { visible: pane.isContact; label: qsTr("OPEN TASKS"); value: String(pane.dossier.OpenTaskCount || 0) }
                    Fact { visible: pane.isContact; label: qsTr("ACTIVITIES"); value: String(pane.dossier.Activities || 0) }
                    Fact { visible: pane.kind === "tasks"; label: qsTr("DUE"); value: pane.record.Due || qsTr("Not set") }
                    Fact { visible: pane.kind === "tasks"; label: qsTr("PRIORITY"); value: [qsTr("None"), qsTr("Low"), qsTr("Medium"), qsTr("High")][pane.record.Priority || 0] }
                    Fact { visible: pane.kind === "projects"; label: qsTr("TARGET"); value: pane.record.Target || qsTr("Not set") }
                    Fact { visible: pane.kind === "projects"; label: qsTr("OPEN TASKS"); value: String(pane.record.OpenTasks || 0) }
                    Fact { visible: pane.kind === "projects"; label: qsTr("COMPLETED"); value: String(pane.record.DoneTasks || 0) }
                }
                Label2 {
                    visible: pane.isContact && Boolean(pane.dossier.FollowUp && pane.dossier.FollowUp.Due)
                    color: Ink.ochre
                    font.weight: Font.DemiBold
                    text: pane.dossier.FollowUp ? qsTr("Follow-up %1%2").arg(pane.dossier.FollowUp.Due || "").arg(pane.dossier.FollowUp.Reason ? " · " + pane.dossier.FollowUp.Reason : "") : ""
                }

                // Text
                Heading { visible: bodyText.text.length > 0; text: pane.kind === "activities" ? qsTr("ACTIVITY NOTES") : pane.isContact ? qsTr("NOTES") : qsTr("DESCRIPTION") }
                Label2 {
                    id: bodyText
                    visible: text.length > 0
                    text: [pane.record.Notes, pane.record.Biography, pane.record.Body, pane.record.Description].filter(Boolean).join("\n\n")
                }

                // Contact details
                Heading { visible: pane.isContact && (pane.list(pane.record.Methods).length > 0 || !!pane.record.Website); text: qsTr("CONTACT DETAILS") }
                Repeater {
                    model: pane.isContact ? pane.list(pane.record.Methods) : []
                    RowLayout {
                        required property var modelData
                        Layout.fillWidth: true
                        Label2 { Layout.preferredWidth: 130; Layout.fillWidth: false; color: Ink.quiet; text: modelData.Label || pane.words(modelData.Kind) }
                        Label2 { text: modelData.Value + (modelData.Preferred ? " · " + qsTr("Preferred") : ""); font.weight: Font.Medium }
                    }
                }
                RowLayout {
                    visible: pane.isContact && !!pane.record.Website
                    Layout.fillWidth: true
                    Label2 { Layout.preferredWidth: 130; Layout.fillWidth: false; color: Ink.quiet; text: qsTr("Website") }
                    Label2 { text: pane.record.Website || ""; font.weight: Font.Medium }
                }

                // Tasks
                Heading { visible: pane.isContact; text: qsTr("OPEN TASKS & COMMITMENTS") }
                Label2 { visible: pane.isContact && pane.list(pane.dossier.OpenTasks).length === 0; color: Ink.quiet; text: qsTr("No open tasks or recent commitments.") }
                Repeater {
                    model: pane.isContact ? pane.list(pane.dossier.OpenTasks) : []
                    DeskButton {
                        required property var modelData
                        Layout.fillWidth: true
                        text: modelData.Title + (modelData.Due ? " · " + modelData.Due : "")
                        enabled: !backend.writing
                        onClicked: backend.openLink("task", modelData.ID)
                    }
                }

                // Recent activity
                Heading { visible: pane.isContact; text: qsTr("RECENT ACTIVITY") }
                Label2 { visible: pane.isContact && pane.list(pane.dossier.Recent).length === 0; color: Ink.quiet; text: qsTr("No activities are recorded.") }
                Repeater {
                    model: pane.isContact ? pane.list(pane.dossier.Recent) : []
                    DeskButton {
                        required property var modelData
                        Layout.fillWidth: true
                        text: pane.day(modelData.OccurredAt) + "  " + modelData.Title
                        enabled: !backend.writing
                        onClicked: backend.openLink("activity", modelData.ID)
                    }
                }

                // Milestones
                Heading { visible: pane.kind === "projects"; text: qsTr("MILESTONES") }
                Label2 { visible: pane.kind === "projects" && pane.list(pane.record.Milestones).length === 0; color: Ink.quiet; text: qsTr("No milestones are recorded.") }
                Repeater {
                    model: pane.kind === "projects" ? pane.list(pane.record.Milestones) : []
                    Label2 {
                        required property var modelData
                        text: modelData.Title + " · " + (modelData.DoneAt ? qsTr("Completed %1").arg(pane.day(modelData.DoneAt)) : modelData.Due ? qsTr("Due %1").arg(modelData.Due) : qsTr("No due date"))
                    }
                }

                // Stage board
                Heading { visible: pane.kind === "pipelines"; text: qsTr("STAGES") }
                Label2 { visible: pane.kind === "pipelines" && pane.list(pane.record.Stages).length === 0; color: Ink.quiet; text: qsTr("No stages are configured for this pipeline.") }
                Repeater {
                    model: pane.kind === "pipelines" ? pane.list(pane.record.Stages) : []
                    ColumnLayout {
                        id: stageBlock
                        required property var modelData
                        Layout.fillWidth: true
                        Layout.topMargin: 6
                        spacing: 0
                        Label { text: stageBlock.modelData.Name + "  ·  " + pane.list(backend.items).filter(entry => entry.StageID === stageBlock.modelData.ID).length
                                color: Ink.ink; font.weight: Font.DemiBold; font.pixelSize: 14; textFormat: Text.PlainText }
                        Repeater {
                            model: pane.list(backend.items).filter(entry => entry.StageID === stageBlock.modelData.ID)
                            RowLayout {
                                id: itemRow
                                required property var modelData
                                Layout.fillWidth: true
                                Label2 { text: itemRow.modelData.Title + (itemRow.modelData.NextAction ? " — " + itemRow.modelData.NextAction : "") }
                                DeskButton {
                                    text: qsTr("Move")
                                    bordered: true
                                    enabled: !backend.writing && !pane.record.Archived
                                    Accessible.name: qsTr("Move %1 to another stage").arg(itemRow.modelData.Title)
                                    onClicked: pane.moveRequested(itemRow.modelData)
                                }
                            }
                        }
                    }
                }

                // Related records
                Heading { visible: related.model.length > 0; text: qsTr("RELATED RECORDS") }
                Repeater {
                    id: related
                    model: {
                        const r = pane.record
                        const out = []
                        const add = (refs, title) => pane.list(refs).forEach(ref => out.push({ title: title, ref: ref }))
                        if (pane.kind === "tasks") { add(r.Entities, qsTr("People and organizations")); add(r.Projects, qsTr("Project")); add(r.Activities, qsTr("Activity")) }
                        if (pane.kind === "activities") { add(r.Entities, qsTr("People and organizations")); add(r.Projects, qsTr("Project")); add(r.Tasks, qsTr("Task")) }
                        if (pane.kind === "projects") add(r.Members, qsTr("Member"))
                        return out
                    }
                    RowLayout {
                        required property var modelData
                        Layout.fillWidth: true
                        Label2 { Layout.preferredWidth: 150; Layout.fillWidth: false; color: Ink.quiet; text: modelData.title }
                        DeskButton {
                            Layout.fillWidth: true
                            text: pane.refName(modelData.ref)
                            enabled: !backend.writing && Boolean(modelData.ref.id)
                            onClicked: backend.openLink(modelData.ref.kind || "", modelData.ref.id)
                        }
                    }
                }
                Item { Layout.preferredHeight: 24 }
            }
        }
    }
}
