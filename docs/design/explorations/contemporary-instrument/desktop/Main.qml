import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Material
import QtQuick.Layouts

ApplicationWindow {
    id: window
    width: 1440
    height: 960
    minimumWidth: 1080
    minimumHeight: 640
    color: colors.canvas
    visible: true
    title: qsTr("TideContacts — Revenue workspace")
    Material.theme: Material.Dark
    Material.accent: colors.accent

    QtObject {
        id: colors
        readonly property color canvas: "#111b20"
        readonly property color chrome: "#142127"
        readonly property color panel: "#162329"
        readonly property color rule: "#2c4148"
        readonly property color baseline: "#829b9c"
        readonly property color accent: "#8ed1c4"
        readonly property color bar: "#577f7c"
        readonly property color ochre: "#e6b87c"
    }

    function reveal(item) {
        const top = item.mapToItem(page.contentItem, 0, 0).y
        const bottom = top + item.height
        if (top < page.contentY + 12)
            page.contentY = Math.max(0, top - 16)
        else if (bottom > page.contentY + page.height - 12)
            page.contentY = Math.min(page.contentHeight - page.height, bottom - page.height + 16)
    }

    function focusPerformance() {
        for (let i = 0; i < currencies.count; ++i) {
            const button = currencies.itemAt(i)
            if (button.text === workspace.currency) {
                button.forceActiveFocus(Qt.ShortcutFocusReason)
                reveal(button)
                return
            }
        }
    }

    function focusQueue() {
        for (let i = 0; i < rows.count; ++i) {
            const row = rows.itemAt(i)
            if (row.modelData.id === workspace.selectedId) {
                focusRow(i)
                return
            }
        }
        bookedMode.forceActiveFocus(Qt.ShortcutFocusReason)
        reveal(bookedMode)
    }

    function focusRow(index) {
        const row = rows.itemAt(index)
        if (row) {
            workspace.selectRecord(row.modelData.id)
            row.forceActiveFocus(Qt.ShortcutFocusReason)
            reveal(row)
        }
    }

    function focusInspector() {
        inspector.forceActiveFocus(Qt.ShortcutFocusReason)
        reveal(inspectorStart)
    }

    Shortcut { sequence: "Alt+1"; context: Qt.ApplicationShortcut; onActivated: window.focusPerformance() }
    Shortcut { sequence: "Alt+2"; context: Qt.ApplicationShortcut; onActivated: window.focusQueue() }
    Shortcut { sequence: "Alt+3"; context: Qt.ApplicationShortcut; onActivated: window.focusInspector() }
    Shortcut { sequence: "Escape"; context: Qt.ApplicationShortcut; enabled: !keys.opened; onActivated: window.focusQueue() }
    Shortcut { sequence: "?"; context: Qt.ApplicationShortcut; onActivated: keys.open() }
    Shortcut { sequence: "Q"; context: Qt.ApplicationShortcut; onActivated: window.close() }

    component Metric : ColumnLayout {
        id: metric
        required property string label
        required property string amount
        required property string metricKey
        required property string contextLine
        property color numberColor: "#e5eeeb"
        spacing: 5
        InkText { text: metric.label; instrument: true; caps: true; quiet: true; font.pixelSize: 10 }
        InkText {
            objectName: "metric-" + metric.metricKey
            text: metric.amount
            instrument: true
            font.pixelSize: 28
            color: metric.numberColor
        }
        InkText {
            objectName: "context-" + metric.metricKey
            Layout.fillWidth: true
            text: metric.contextLine
            quiet: true
            font.pixelSize: 12
            elide: Text.ElideRight
        }
    }

    component LedgerCells : RowLayout {
        id: cells
        required property var entry
        property bool headings: false
        property bool selected: false
        property bool booked: false
        spacing: 12

        ColumnLayout {
            Layout.preferredWidth: 83
            Layout.maximumWidth: 83
            spacing: 0
            InkText {
                text: cells.headings ? (cells.booked ? qsTr("Closed") : qsTr("Follow-up")) : cells.entry.date
                instrument: true
                caps: cells.headings
                quiet: cells.headings
                font.pixelSize: cells.headings ? 10 : 12
            }
            InkText {
                visible: !cells.headings && !cells.booked
                text: cells.headings ? "" : cells.entry.urgency
                quiet: true
                color: text === qsTr("Overdue") || text === qsTr("Due today") ? "#e6b87c" : "#a5b7bb"
                font.pixelSize: 10
            }
        }
        InkText {
            Layout.fillWidth: true
            Layout.preferredWidth: 128
            Layout.minimumWidth: 85
            text: cells.headings ? qsTr("Organization") : cells.entry.organization
            instrument: cells.headings
            caps: cells.headings
            quiet: cells.headings
            color: cells.headings ? "#a5b7bb" : cells.selected ? "#8ed1c4" : "#e5eeeb"
            font.pixelSize: cells.headings ? 10 : 13
            font.weight: cells.headings ? Font.Normal : Font.DemiBold
            elide: Text.ElideRight
        }
        InkText {
            Layout.fillWidth: true
            Layout.preferredWidth: 194
            Layout.minimumWidth: 130
            text: cells.headings ? (cells.booked ? qsTr("Source record") : qsTr("Next action")) : cells.entry.action
            instrument: cells.headings
            caps: cells.headings
            quiet: cells.headings
            font.pixelSize: cells.headings ? 10 : 13
            elide: Text.ElideRight
        }
        InkText {
            Layout.preferredWidth: 92
            Layout.maximumWidth: 92
            text: cells.headings ? qsTr("Stage") : cells.entry.stage
            instrument: true
            caps: cells.headings
            quiet: true
            font.pixelSize: cells.headings ? 10 : 11
        }
        InkText {
            Layout.preferredWidth: 85
            Layout.maximumWidth: 85
            text: cells.headings ? qsTr("Amount") : cells.entry.amount
            instrument: true
            caps: cells.headings
            quiet: cells.headings
            font.pixelSize: cells.headings ? 10 : 12
            horizontalAlignment: Text.AlignRight
        }
    }

    Rectangle {
        id: chrome
        anchors { top: parent.top; left: parent.left; right: parent.right }
        height: 54
        color: colors.chrome

        RowLayout {
            anchors { fill: parent; leftMargin: 20; rightMargin: 26 }
            spacing: 14
            Item {
                Layout.preferredWidth: 23
                Layout.preferredHeight: 23
                Accessible.ignored: true
                Rectangle { x: 2; y: 4; width: 18; height: 2; color: colors.accent }
                Rectangle { x: 10; y: 5; width: 2; height: 14; color: colors.accent }
                Rectangle { x: 5; y: 20; width: 13; height: 1; color: colors.baseline }
                Rectangle { x: 20; y: 1; width: 1; height: 6; color: colors.baseline }
            }
            InkText { text: "TideContacts"; font.pixelSize: 16; font.weight: Font.DemiBold }
            Rectangle { Layout.preferredHeight: 16; Layout.preferredWidth: 1; color: colors.rule }
            InkText { text: qsTr("Revenue desk"); instrument: true; caps: true; quiet: true; font.pixelSize: 11 }
            Item { Layout.fillWidth: true }
            Rectangle { Layout.preferredWidth: 7; Layout.preferredHeight: 7; color: colors.accent }
            InkText { text: qsTr("Fictional snapshot"); instrument: true; caps: true; quiet: true; font.pixelSize: 10 }
            InkText { text: workspace.asOf; instrument: true; font.pixelSize: 11 }
        }
        Rectangle {
            anchors { left: parent.left; right: parent.right; bottom: parent.bottom }
            height: 1
            color: colors.rule
        }
    }

    Rectangle {
        id: rail
        anchors { top: chrome.bottom; left: parent.left; bottom: parent.bottom }
        width: 67
        color: colors.chrome
        Rectangle {
            anchors { top: parent.top; right: parent.right; bottom: parent.bottom }
            width: 1
            color: colors.rule
        }
        Column {
            anchors.horizontalCenter: parent.horizontalCenter
            y: 26
            spacing: 8
            QuietButton { width: 59; text: qsTr("Plot"); Accessible.name: qsTr("Performance (Alt+1)"); onClicked: window.focusPerformance() }
            QuietButton { width: 59; text: qsTr("Queue"); Accessible.name: qsTr("Review queue (Alt+2)"); onClicked: window.focusQueue() }
            QuietButton { width: 59; text: qsTr("Source"); Accessible.name: qsTr("Source inspector (Alt+3)"); onClicked: window.focusInspector() }
        }
        QuietButton {
            anchors { bottom: parent.bottom; horizontalCenter: parent.horizontalCenter; bottomMargin: 18 }
            width: 33
            text: "?"
            Accessible.name: qsTr("Keyboard shortcuts")
            onClicked: keys.open()
        }
    }

    Flickable {
        id: page
        anchors { top: chrome.bottom; bottom: parent.bottom; left: rail.right; right: parent.right }
        contentWidth: width
        contentHeight: board.implicitHeight + 43
        boundsBehavior: Flickable.StopAtBounds
        clip: true
        ScrollBar.vertical: ScrollBar { policy: ScrollBar.AsNeeded }

        ColumnLayout {
            id: board
            x: 28
            y: 22
            width: page.width - 56
            spacing: 16

            RowLayout {
                Layout.fillWidth: true
                spacing: 22
                ColumnLayout {
                    Layout.fillWidth: true
                    spacing: 4
                    InkText {
                        text: qsTr("Contemporary instrument")
                        instrument: true
                        caps: true
                        color: colors.accent
                        font.pixelSize: 11
                    }
                    InkText {
                        text: qsTr("Revenue workspace")
                        font.pixelSize: 32
                        font.weight: Font.DemiBold
                        Accessible.role: Accessible.Heading
                    }
                    InkText {
                        text: qsTr("Booked results and the next actions that need review.")
                        quiet: true
                        font.pixelSize: 13
                    }
                }
                ColumnLayout {
                    Layout.alignment: Qt.AlignBottom | Qt.AlignRight
                    Layout.bottomMargin: 2
                    spacing: 5
                    InkText { text: qsTr("Display currency"); instrument: true; caps: true; quiet: true; font.pixelSize: 10 }
                    RowLayout {
                        spacing: 4
                        Repeater {
                            id: currencies
                            model: workspace.currencies
                            delegate: QuietButton {
                                required property string modelData
                                objectName: "currency-" + modelData
                                text: modelData
                                current: modelData === workspace.currency
                                onClicked: workspace.setCurrency(modelData)
                            }
                        }
                    }
                }
            }

            Rectangle { Layout.fillWidth: true; Layout.preferredHeight: 1; color: colors.rule }

            RowLayout {
                Layout.fillWidth: true
                spacing: 25
                ColumnLayout {
                    Layout.fillWidth: true
                    Layout.alignment: Qt.AlignTop
                    spacing: 12

                    RowLayout {
                        Layout.fillWidth: true
                        InkText { text: qsTr("01  /  Performance"); instrument: true; caps: true; font.pixelSize: 11; quiet: true }
                        Item { Layout.fillWidth: true }
                        InkText {
                            text: qsTr("Close month / %1").arg(workspace.metrics.month)
                            instrument: true
                            caps: true
                            font.pixelSize: 10
                            color: colors.accent
                        }
                    }

                    RowLayout {
                        Layout.fillWidth: true
                        Layout.bottomMargin: 2
                        spacing: 16
                        Metric {
                            Layout.fillWidth: true
                            Layout.preferredWidth: 180
                            label: qsTr("Booked value")
                            metricKey: "booked"
                            amount: workspace.metrics.booked
                            contextLine: workspace.metrics.targetNote
                            numberColor: colors.accent
                        }
                        Rectangle { Layout.fillHeight: true; Layout.preferredWidth: 1; color: colors.rule }
                        Metric {
                            Layout.fillWidth: true
                            Layout.preferredWidth: 180
                            label: qsTr("Weighted forecast")
                            metricKey: "forecast"
                            amount: workspace.metrics.forecast
                            contextLine: qsTr("All open · weighted per record")
                        }
                        Rectangle { Layout.fillHeight: true; Layout.preferredWidth: 1; color: colors.rule }
                        Metric {
                            Layout.fillWidth: true
                            Layout.preferredWidth: 180
                            label: qsTr("Open pipeline")
                            metricKey: "pipeline"
                            amount: workspace.metrics.pipeline
                            contextLine: qsTr("All open · before weighting")
                        }
                    }

                    Rectangle { Layout.fillWidth: true; Layout.preferredHeight: 1; color: colors.rule }
                    InstrumentPlot { Layout.fillWidth: true; ink: colors }
                    Rectangle { Layout.fillWidth: true; Layout.preferredHeight: 1; Layout.topMargin: 1; color: colors.rule }

                    RowLayout {
                        Layout.fillWidth: true
                        spacing: 10
                        ColumnLayout {
                            Layout.fillWidth: true
                            spacing: 4
                            InkText {
                                text: qsTr("02  /  Record review")
                                instrument: true
                                caps: true
                                font.pixelSize: 11
                                quiet: true
                            }
                            InkText {
                                readonly property int count: workspace.queue.length
                                text: workspace.bookedView
                                      ? (count === 1 ? qsTr("%1 closed source · %2 · %3") : qsTr("%1 closed sources · %2 · %3"))
                                              .arg(count).arg(workspace.metrics.month).arg(workspace.currency)
                                      : (count === 1 ? qsTr("%1 open record · %2 · all expected dates") : qsTr("%1 open records · %2 · all expected dates"))
                                              .arg(count).arg(workspace.currency)
                                font.pixelSize: 12
                            }
                        }
                        QuietButton {
                            objectName: "open-queue"
                            text: qsTr("Open queue")
                            current: !workspace.bookedView
                            onClicked: workspace.setBookedView(false)
                        }
                        QuietButton {
                            id: bookedMode
                            objectName: "booked-queue"
                            text: qsTr("Booked sources")
                            current: workspace.bookedView
                            onClicked: workspace.setBookedView(true)
                        }
                    }

                    ColumnLayout {
                        Layout.fillWidth: true
                        spacing: 0
                        Rectangle {
                            Layout.fillWidth: true
                            Layout.preferredHeight: 28
                            color: colors.chrome
                            LedgerCells {
                                anchors { fill: parent; leftMargin: 12; rightMargin: 11 }
                                entry: ({})
                                headings: true
                                booked: workspace.bookedView
                            }
                        }
                        Repeater {
                            id: rows
                            model: workspace.queue
                            delegate: Button {
                                id: entryButton
                                required property int index
                                required property var modelData
                                readonly property bool selected: modelData.id === workspace.selectedId
                                objectName: "row-" + modelData.id
                                Layout.fillWidth: true
                                Layout.preferredHeight: 40
                                topPadding: 0
                                bottomPadding: 0
                                leftPadding: 12
                                rightPadding: 11
                                hoverEnabled: true
                                focusPolicy: Qt.StrongFocus
                                Accessible.selected: selected
                                Accessible.name: qsTr("%1, %2, %3 %4, %5, %6").arg(modelData.organization).arg(modelData.action)
                                                 .arg(modelData.urgency).arg(modelData.date).arg(modelData.stage).arg(modelData.amount)
                                onClicked: workspace.selectRecord(modelData.id)
                                Keys.onUpPressed: window.focusRow(index - 1)
                                Keys.onDownPressed: window.focusRow(index + 1)
                                Keys.onRightPressed: { workspace.selectRecord(modelData.id); window.focusInspector() }
                                Keys.onReturnPressed: workspace.selectRecord(modelData.id)
                                onActiveFocusChanged: if (activeFocus) window.reveal(entryButton)
                                contentItem: LedgerCells {
                                    entry: entryButton.modelData
                                    selected: entryButton.selected
                                    booked: workspace.bookedView
                                }
                                background: Rectangle {
                                    color: entryButton.selected ? "#21352f" : entryButton.hovered ? "#1b2b30" : colors.canvas
                                    border.color: entryButton.activeFocus ? colors.accent : "transparent"
                                    Rectangle {
                                        anchors { left: parent.left; right: parent.right; bottom: parent.bottom }
                                        height: 1
                                        color: colors.rule
                                        visible: !entryButton.activeFocus
                                    }
                                    Rectangle {
                                        anchors { left: parent.left; top: parent.top; bottom: parent.bottom; topMargin: 1; bottomMargin: 1 }
                                        width: 2
                                        color: colors.accent
                                        visible: entryButton.selected
                                    }
                                }
                            }
                        }
                        InkText {
                            visible: rows.count === 0
                            Layout.fillWidth: true
                            topPadding: 12
                            bottomPadding: 12
                            text: qsTr("No matching records in this scope.")
                            quiet: true
                        }
                    }
                }

                SourceInspector {
                    id: inspector
                    Layout.preferredWidth: Math.round(Math.min(350, Math.max(275, board.width * 0.26)))
                    Layout.fillHeight: true
                    ink: colors
                    onReturnRequested: window.focusQueue()
                    Item { id: inspectorStart; width: 1; height: 28 }
                }
            }

            RowLayout {
                Layout.fillWidth: true
                spacing: 12
                InkText {
                    Layout.fillWidth: true
                    text: qsTr("Currencies are never combined. Booked value is not recognized revenue.")
                    quiet: true
                    font.pixelSize: 11
                }
                InkText { text: qsTr("↑/↓ Queue   ·   Alt+3 Inspect   ·   Esc Return"); instrument: true; quiet: true; font.pixelSize: 10 }
            }
        }
    }

    Popup {
        id: keys
        anchors.centerIn: Overlay.overlay
        width: 360
        padding: 22
        modal: true
        focus: true
        closePolicy: Popup.CloseOnEscape | Popup.CloseOnPressOutside
        background: Rectangle { color: colors.panel; border.color: colors.baseline }
        contentItem: ColumnLayout {
            spacing: 9
            InkText { text: qsTr("Keyboard shortcuts"); font.pixelSize: 19; font.weight: Font.DemiBold }
            InkText { text: qsTr("Alt+1  Currency scope"); instrument: true; quiet: true }
            InkText { text: qsTr("Alt+2  Selected source row"); instrument: true; quiet: true }
            InkText { text: qsTr("Alt+3  Source inspector"); instrument: true; quiet: true }
            InkText { text: qsTr("↑ / ↓  Select adjacent row"); instrument: true; quiet: true }
            InkText { text: qsTr("Tab / Space  Focus and activate controls"); instrument: true; quiet: true; font.pixelSize: 12 }
            InkText { text: qsTr("Esc  Return to source row"); instrument: true; quiet: true }
            InkText { text: qsTr("Q    Close workspace"); instrument: true; quiet: true }
        }
    }
}
