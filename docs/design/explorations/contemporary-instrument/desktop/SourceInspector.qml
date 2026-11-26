import QtQuick
import QtQuick.Layouts

Rectangle {
    id: panel
    required property var ink
    signal returnRequested()
    readonly property var record: workspace.source
    readonly property bool hasSource: !!record.id
    readonly property bool won: workspace.bookedView
    readonly property bool urgent: hasSource && !won && record.urgency !== qsTr("Upcoming")

    objectName: "source-inspector"
    color: ink.panel
    border.color: activeFocus ? ink.accent : ink.rule
    implicitWidth: 320
    implicitHeight: content.implicitHeight + provenance.implicitHeight + 59
    activeFocusOnTab: true
    Accessible.role: Accessible.Pane
    Accessible.name: qsTr("Source inspector")
    Keys.onLeftPressed: returnRequested()

    component Prose : ColumnLayout {
        required property string label
        required property string body
        spacing: 6
        InkText { text: parent.label; instrument: true; caps: true; quiet: true; font.pixelSize: 10 }
        InkText { Layout.fillWidth: true; text: parent.body; wrapMode: Text.Wrap; font.pixelSize: 13; lineHeight: 1.18 }
    }

    ColumnLayout {
        id: content
        anchors { top: parent.top; left: parent.left; right: parent.right; margins: 21 }
        spacing: 16

        RowLayout {
            Layout.fillWidth: true
            spacing: 8
            Rectangle { Layout.preferredWidth: 3; Layout.preferredHeight: 13; color: ink.accent }
            InkText { text: qsTr("Source inspector"); instrument: true; caps: true; font.pixelSize: 11 }
            Item { Layout.fillWidth: true }
            InkText {
                objectName: "source-identity"
                text: panel.hasSource ? panel.record.id : "—"
                instrument: true
                quiet: true
                font.pixelSize: 10
            }
        }

        Rectangle { Layout.fillWidth: true; Layout.preferredHeight: 1; color: ink.rule }

        InkText {
            visible: !panel.hasSource
            Layout.fillWidth: true
            text: qsTr("No source records in this scope.")
            wrapMode: Text.Wrap
            quiet: true
        }

        ColumnLayout {
            visible: panel.hasSource
            Layout.fillWidth: true
            spacing: 5
            InkText {
                text: panel.hasSource ? panel.record.organization : ""
                color: ink.accent
                font.pixelSize: 13
                font.weight: Font.DemiBold
            }
            InkText {
                objectName: "source-title"
                Layout.fillWidth: true
                text: panel.hasSource ? panel.record.title : ""
                font.pixelSize: 25
                font.weight: Font.DemiBold
                wrapMode: Text.Wrap
                Accessible.role: Accessible.Heading
            }
            InkText {
                Layout.fillWidth: true
                text: !panel.hasSource ? "" : panel.record.role ? qsTr("%1 · %2").arg(panel.record.contact).arg(panel.record.role) : panel.record.contact
                quiet: true
                font.pixelSize: 12
                wrapMode: Text.Wrap
            }
        }

        ColumnLayout {
            visible: panel.hasSource
            Layout.fillWidth: true
            spacing: 9
            RowLayout {
                Layout.fillWidth: true
                InkText {
                    text: panel.hasSource ? panel.record.stage : ""
                    instrument: true
                    caps: true
                    color: ink.accent
                    font.pixelSize: 10
                }
                Item { Layout.fillWidth: true }
                InkText {
                    text: panel.hasSource ? qsTr("%1  %2").arg(panel.record.amount).arg(panel.record.currency) : ""
                    instrument: true
                    font.pixelSize: 12
                }
            }
            InkText {
                Layout.fillWidth: true
                text: !panel.hasSource ? "" : panel.won ? qsTr("Closed on %1").arg(panel.record.closed)
                      : qsTr("%1 weighted at %2").arg(panel.record.weighted).arg(panel.record.probability)
                quiet: true
                font.pixelSize: 12
                wrapMode: Text.Wrap
            }
            InkText {
                visible: !panel.won
                text: panel.hasSource && !panel.won ? qsTr("Expected close: %1").arg(panel.record.expected) : ""
                quiet: true
                font.pixelSize: 12
            }
        }

        Rectangle { visible: panel.hasSource; Layout.fillWidth: true; Layout.preferredHeight: 1; color: ink.rule }

        ColumnLayout {
            visible: panel.hasSource && !panel.won
            Layout.fillWidth: true
            spacing: 6
            InkText {
                text: qsTr("Next action")
                instrument: true
                caps: true
                quiet: true
                font.pixelSize: 10
            }
            InkText {
                Layout.fillWidth: true
                text: panel.hasSource ? panel.record.action : ""
                font.pixelSize: 14
                font.weight: Font.DemiBold
                wrapMode: Text.Wrap
            }
            InkText {
                text: panel.hasSource && !panel.won ? qsTr("%1 · %2").arg(panel.record.urgency).arg(panel.record.due) : ""
                color: panel.urgent ? ink.ochre : "#a5b7bb"
                font.pixelSize: 12
            }
        }

        Prose {
            visible: panel.hasSource && !panel.won
            Layout.fillWidth: true
            label: panel.hasSource && !panel.won ? qsTr("Last contact / %1").arg(panel.record.touch) : ""
            body: panel.hasSource && !panel.won ? panel.record.activity : ""
        }

        Prose {
            visible: panel.hasSource
            Layout.fillWidth: true
            label: qsTr("Source note")
            body: panel.hasSource ? panel.record.note : ""
        }

        QuietButton {
            visible: panel.hasSource
            Layout.topMargin: 1
            Layout.leftMargin: -9
            text: qsTr("←  Return to source row")
            onClicked: panel.returnRequested()
        }
    }

    ColumnLayout {
        id: provenance
        anchors { bottom: parent.bottom; left: parent.left; right: parent.right; margins: 21 }
        spacing: 9
        Rectangle { Layout.fillWidth: true; Layout.preferredHeight: 1; color: ink.rule }
        InkText { text: qsTr("Fictional source · read-only"); instrument: true; caps: true; quiet: true; font.pixelSize: 10 }
    }
}
