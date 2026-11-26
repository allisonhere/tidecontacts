import QtQuick

Text {
    property bool instrument: false
    property bool quiet: false
    property bool caps: false

    color: quiet ? "#a5b7bb" : "#e5eeeb"
    font.family: instrument ? fixedFace : uiFace
    textFormat: Text.PlainText
    font.pixelSize: 14
    font.capitalization: caps ? Font.AllUppercase : Font.MixedCase
    font.letterSpacing: caps ? 1.05 : 0
}
