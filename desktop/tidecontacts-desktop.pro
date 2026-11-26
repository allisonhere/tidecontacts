QT += gui qml quick quickcontrols2 network
CONFIG += c++17
TEMPLATE = app
TARGET = tidecontacts-desktop

HEADERS += src/backend.h
SOURCES += src/main.cpp src/backend.cpp
RESOURCES += src/desktop.qrc
