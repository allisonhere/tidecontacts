#include "backend.h"

#include <QColor>
#include <QCommandLineParser>
#include <QDir>
#include <QFile>
#include <QFileInfo>
#include <QFileSystemWatcher>
#include <QFontDatabase>
#include <QGuiApplication>
#include <QImage>
#include <QQmlApplicationEngine>
#include <QQmlContext>
#include <QQuickStyle>
#include <QQuickWindow>
#include <QRegularExpression>
#include <QTextStream>
#include <cmath>

namespace {
class DeskTheme final : public QObject {
    Q_OBJECT
    Q_PROPERTY(QColor accent READ accent NOTIFY changed)

public:
    DeskTheme()
    {
        connect(&m_watcher, &QFileSystemWatcher::fileChanged, this, &DeskTheme::reload);
        connect(&m_watcher, &QFileSystemWatcher::directoryChanged, this, &DeskTheme::reload);
        reload();
    }
    QColor accent() const { return m_accent; }

signals:
    void changed();

private:
    static double lightness(const QColor &color)
    {
        const auto linear = [](double c) { return c <= 0.04045 ? c / 12.92 : std::pow((c + 0.055) / 1.055, 2.4); };
        return 0.2126 * linear(color.redF()) + 0.7152 * linear(color.greenF()) + 0.0722 * linear(color.blueF());
    }

    void reload()
    {
        const QString state = QDir::homePath() + "/.local/state";
        const QString current = state + "/omarchy/current";
        const QString file = current + "/theme/colors.toml";
        const QStringList paths{state, state + "/omarchy", current, current + "/theme", file};
        for (const auto &path : paths) {
            const QFileInfo info(path);
            const auto watched = info.isDir() ? m_watcher.directories() : m_watcher.files();
            if (info.exists() && !watched.contains(path))
                m_watcher.addPath(path);
        }

        QColor next("#31624e");
        QFile config(file);
        if (config.open(QIODevice::ReadOnly)) {
            static const QRegularExpression accentLine("^\\s*accent\\s*=\\s*[\"'](#[0-9a-fA-F]{6})[\"']",
                                                       QRegularExpression::MultilineOption);
            const auto match = accentLine.match(QString::fromUtf8(config.read(65536)));
            if (match.hasMatch()) {
                next = QColor(match.captured(1));
                // Text and focus remain legible on even the darkest paper surface.
                const double target = (lightness(QColor("#e8ede1")) + 0.05) / 4.5 - 0.05;
                while (lightness(next) > target)
                    next = QColor::fromRgbF(next.redF() * 0.85, next.greenF() * 0.85, next.blueF() * 0.85);
            }
        }
        if (next != m_accent) {
            m_accent = next;
            emit changed();
        }
    }

    QColor m_accent;
    QFileSystemWatcher m_watcher;
};
}

int main(int argc, char **argv)
{
    QGuiApplication app(argc, argv);
    app.setApplicationName("tidecontacts-desktop");
    app.setApplicationDisplayName("TideContacts");
    app.setDesktopFileName("tidecontacts-desktop");
    QQuickStyle::setStyle("Material");

    QCommandLineParser args;
    args.setApplicationDescription("Native TideContacts Community workspace.");
    args.addHelpOption();
    const QCommandLineOption profile("profile", "Open the demo or local profile.", "demo|local", "demo");
    const QCommandLineOption server("server", "Absolute path to tidecontacts-web (default: beside this application).", "path");
    const QCommandLineOption capture("capture", "Save the live demo workspace as a PNG and exit.", "png");
    args.addOptions({profile, server, capture});
    args.process(app);

    const auto profileName = args.value(profile);
    const bool demo = profileName == "demo";
    if (!args.positionalArguments().isEmpty() || (!demo && profileName != "local") || (args.isSet(capture) && !demo)) {
        QTextStream(stderr) << "Use --profile demo or --profile local; --capture is available only with demo.\n";
        return 2;
    }
    const auto program = args.isSet(server) ? args.value(server) : QCoreApplication::applicationDirPath() + "/tidecontacts-web";
    const QFileInfo executable(program);
    if (!executable.isAbsolute() || !executable.isFile() || !executable.isExecutable()) {
        QTextStream(stderr) << "No executable sidecar. Pass --server /absolute/path/to/tidecontacts-web.\n";
        return 2;
    }

    Backend backend(demo);
    DeskTheme theme;
    qmlRegisterSingletonType(QUrl("qrc:/Ink.qml"), "Tide", 1, 0, "Ink");
    QQmlApplicationEngine engine;
    engine.rootContext()->setContextProperty("backend", &backend);
    engine.rootContext()->setContextProperty("deskTheme", &theme);
    engine.rootContext()->setContextProperty("fixedFace", QFontDatabase::systemFont(QFontDatabase::FixedFont).family());
    engine.load(QUrl("qrc:/Main.qml"));
    auto *window = qobject_cast<QQuickWindow *>(engine.rootObjects().value(0));
    if (!window)
        return 1;

    if (args.isSet(capture)) {
        auto *timer = new QTimer(window);
        timer->setInterval(40);
        QObject::connect(timer, &QTimer::timeout, window, [window, timer, &backend, &app, path = args.value(capture)] {
            if (!backend.sessionError().isEmpty() || !backend.indexError().isEmpty() || !backend.detailError().isEmpty()) {
                QTextStream(stderr) << "Capture failed: the live demo records could not be loaded.\n";
                timer->stop();
                app.exit(1);
            } else if (backend.captureReady() && window->isExposed() && window->property("sceneHasRecord").toBool()) {
                const auto image = window->grabWindow();
                timer->stop();
                if (image.isNull() || !image.save(path, "PNG")) {
                    QTextStream(stderr) << "Could not save the workspace PNG.\n";
                    app.exit(1);
                } else {
                    app.exit(0);
                }
            }
        });
        QTimer::singleShot(15000, window, [&app] {
            QTextStream(stderr) << "Timed out waiting for a rendered live demo record.\n";
            app.exit(1);
        });
        timer->start();
    }
    backend.start(program);
    return app.exec();
}

#include "main.moc"
