import QtQuick
import QtQuick.Controls

Button {
    id: control
    property bool current: false
    property color accent: "#8ed1c4"

    hoverEnabled: true
    focusPolicy: Qt.StrongFocus
    leftPadding: 10
    rightPadding: 10
    topPadding: 6
    bottomPadding: 6
    implicitHeight: 32

    Accessible.selected: current
    contentItem: InkText {
        text: control.text
        instrument: true
        color: control.current || control.activeFocus ? control.accent : "#a5b7bb"
        font.pixelSize: 12
        horizontalAlignment: Text.AlignHCenter
        verticalAlignment: Text.AlignVCenter
    }
    background: Rectangle {
        color: control.down ? "#304842" : control.current || control.hovered ? "#233933" : "transparent"
        border.color: control.activeFocus ? control.accent : control.current ? "#507c73" : "transparent"
    }
}
