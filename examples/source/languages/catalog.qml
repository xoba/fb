// A list of the example catalog's files with their sizes.
import QtQuick
import QtQuick.Controls

ApplicationWindow {
    width: 320
    height: 160
    visible: true
    title: qsTr("Example folder")

    ListModel {
        id: files
        ListElement { name: "notes.md"; bytes: 120 }
        ListElement { name: "letter.eml"; bytes: 240 }
    }

    ListView {
        anchors.fill: parent
        model: files
        delegate: Label {
            required property string name
            required property int bytes
            text: name + ": " + bytes + " bytes"
            padding: 8
        }
    }
}
