import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

ColumnLayout {
    id: instrument
    required property var ink
    spacing: 10

    RowLayout {
        Layout.fillWidth: true
        spacing: 14
        InkText {
            text: qsTr("Booked by close month")
            font.pixelSize: 17
            font.weight: Font.DemiBold
            Accessible.role: Accessible.Heading
        }
        Item { Layout.fillWidth: true }
        Rectangle { Layout.preferredWidth: 10; Layout.preferredHeight: 10; color: ink.bar }
        InkText { text: qsTr("Booked"); quiet: true; font.pixelSize: 12 }
        Rectangle { Layout.preferredWidth: 14; Layout.preferredHeight: 2; color: ink.ochre }
        InkText { text: qsTr("Target, where set"); quiet: true; font.pixelSize: 12 }
    }

    Item {
        id: plot
        Layout.fillWidth: true
        Layout.preferredHeight: 202
        readonly property int topGutter: 13
        readonly property int labelGutter: 71
        readonly property real graphHeight: height - topGutter - 35
        readonly property real zeroY: topGutter + graphHeight

        Repeater {
            model: workspace.axis
            delegate: Item {
                required property int index
                required property string modelData
                readonly property bool zero: index === workspace.axis.length - 1
                x: 0
                y: Math.round(plot.topGutter + plot.graphHeight * index / (workspace.axis.length - 1))
                width: plot.width
                InkText {
                    width: plot.labelGutter - 12
                    y: -height / 2
                    text: parent.modelData
                    instrument: true
                    quiet: !parent.zero
                    font.pixelSize: 11
                    horizontalAlignment: Text.AlignRight
                }
                Rectangle {
                    objectName: parent.zero ? "axis-zero" : ""
                    x: plot.labelGutter
                    width: plot.width - x
                    height: 1
                    color: parent.zero ? ink.baseline : ink.rule
                }
            }
        }
        RowLayout {
            x: plot.labelGutter
            width: plot.width - x
            height: plot.height
            spacing: 0
            Repeater {
                model: workspace.history
                delegate: Button {
                    id: monthButton
                    required property var modelData
                    readonly property bool current: modelData.period === workspace.period
                    readonly property real amountHeight: Math.round(plot.graphHeight * modelData.portion)
                    readonly property bool hasTarget: modelData.target !== ""
                    objectName: "month-" + modelData.period
                    Layout.fillWidth: true
                    Layout.fillHeight: true
                    Layout.preferredWidth: 110
                    leftPadding: 0
                    rightPadding: 0
                    topPadding: 0
                    bottomPadding: 0
                    hoverEnabled: true
                    focusPolicy: Qt.StrongFocus
                    Accessible.selected: current
                    Accessible.name: hasTarget ? qsTr("%1: %2 booked, %3 target").arg(modelData.label).arg(modelData.amount).arg(modelData.target)
                                               : qsTr("%1: %2 booked, no target set").arg(modelData.label).arg(modelData.amount)
                    onClicked: workspace.setPeriod(modelData.period)
                    background: Rectangle {
                        color: monthButton.activeFocus || monthButton.hovered || monthButton.current ? "#187cb9ac" : "transparent"
                        border.color: monthButton.activeFocus ? ink.accent : "transparent"
                    }
                    contentItem: Item {
                        Rectangle {
                            objectName: "booked-bar-" + monthButton.modelData.period
                            anchors.horizontalCenter: parent.horizontalCenter
                            width: Math.min(58, parent.width * 0.49)
                            height: monthButton.amountHeight
                            y: plot.zeroY - height
                            color: monthButton.current ? ink.accent : ink.bar
                        }
                        Rectangle {
                            visible: monthButton.hasTarget
                            anchors.horizontalCenter: parent.horizontalCenter
                            anchors.horizontalCenterOffset: Math.min(parent.width * 0.35, 41)
                            width: 18
                            height: 2
                            y: plot.zeroY - Math.round(plot.graphHeight * monthButton.modelData.targetPortion)
                            color: ink.ochre
                        }
                        InkText {
                            anchors.horizontalCenter: parent.horizontalCenter
                            y: Math.max(0, plot.zeroY - monthButton.amountHeight - height - 5)
                            text: monthButton.modelData.amount
                            instrument: true
                            font.pixelSize: 11
                            color: monthButton.current ? ink.accent : "#c0d2d2"
                        }
                        InkText {
                            anchors.horizontalCenter: parent.horizontalCenter
                            y: plot.zeroY + 13
                            text: monthButton.modelData.month
                            instrument: true
                            caps: true
                            quiet: !monthButton.current
                            font.pixelSize: 11
                            color: monthButton.current ? ink.accent : "#a5b7bb"
                        }
                        Rectangle {
                            visible: monthButton.current
                            anchors.horizontalCenter: parent.horizontalCenter
                            y: plot.height - 2
                            width: 21
                            height: 2
                            color: ink.accent
                        }
                    }
                }
            }
        }
    }
}
