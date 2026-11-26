import QtQuick
import QtQuick.Controls
import Tide 1.0

Button {
    id: control
    property bool current: false
    property bool bordered: false
    property int alignment: Text.AlignLeft

    focusPolicy: Qt.StrongFocus
    hoverEnabled: true
    leftPadding: 10
    rightPadding: 10
    topPadding: 6
    bottomPadding: 6
    implicitHeight: Math.max(34, implicitContentHeight + topPadding + bottomPadding)
    Accessible.name: text
    Accessible.selected: current

    contentItem: Text {
        text: control.text
        textFormat: Text.PlainText
        color: !control.enabled ? Ink.quiet : control.current || control.hovered ? Ink.accent : Ink.ink
        font.pixelSize: 13
        font.weight: control.current ? Font.DemiBold : Font.Medium
        elide: Text.ElideRight
        horizontalAlignment: control.alignment
        verticalAlignment: Text.AlignVCenter
    }
    background: Rectangle {
        color: control.current || control.down || control.hovered ? Ink.selected : "transparent"
        border.color: control.activeFocus ? Ink.accent : control.bordered ? Ink.rule : "transparent"
        border.width: control.activeFocus ? 2 : 1
        Rectangle {
            visible: control.current
            width: 2
            height: parent.height
            color: Ink.accent
        }
    }
}
