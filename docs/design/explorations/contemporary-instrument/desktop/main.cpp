#include "workspace.h"

#include <QCommandLineParser>
#include <QFontDatabase>
#include <QGuiApplication>
#include <QImage>
#include <QQmlApplicationEngine>
#include <QQmlContext>
#include <QQuickItem>
#include <QQuickWindow>
#include <QTest>
#include <QTextStream>
#include <QTimer>

namespace {
QQuickItem *namedItem(QQuickItem *root, const QString &name)
{
    if (root->objectName() == name)
        return root;
    for (auto *child : root->childItems()) {
        if (auto *match = namedItem(child, name))
            return match;
    }
    return nullptr;
}

bool click(QQuickWindow *window, QQuickItem *item)
{
    if (!item || !item->isVisible() || item->width() <= 0 || item->height() <= 0)
        return false;
    const auto center = item->mapToScene(QPointF(item->width() / 2, item->height() / 2)).toPoint();
    if (!QRect(QPoint(0, 0), window->size()).contains(center))
        return false;
    QTest::mouseClick(window, Qt::LeftButton, Qt::NoModifier, center);
    QTest::qWait(24);
    return true;
}

QString verify(QQuickWindow *window, Workspace &workspace)
{
    window->requestActivate();
    if (!QTest::qWaitForWindowActive(window, 1000))
        return QStringLiteral("The native Qt window did not receive keyboard focus.");

    const auto lookup = [window](const QString &name) { return namedItem(window->contentItem(), name); };
    const auto sourceMatches = [&] {
        const auto *identity = lookup("source-identity");
        const auto *title = lookup("source-title");
        return identity && title && identity->property("text").toString() == workspace.selectedId()
               && title->property("text").toString() == workspace.source().value("title").toString();
    };
    const auto metricMatches = [&](const QString &key, const QString &expected) {
        const auto *metric = lookup(key == "targetNote" ? QStringLiteral("context-booked") : "metric-" + key);
        return metric && metric->property("text").toString() == expected
               && workspace.metrics().value(key).toString() == expected;
    };
    const QString first = QStringLiteral("meridian");
    const QString second = QStringLiteral("harbor");
    if (workspace.currency() != "USD" || workspace.period() != "2026-11" || workspace.queue().size() != 6
        || workspace.selectedId() != first || !sourceMatches() || !metricMatches("booked", "$28,400")
        || !metricMatches("forecast", "$42,855") || !metricMatches("pipeline", "$87,300")
        || !metricMatches("targetNote", QStringLiteral("$32,000 target · $3,600 below")))
        return QStringLiteral("The initial shared fixture did not produce the expected November USD report.");
    const auto *zero = lookup("axis-zero");
    const auto *bookedBar = lookup("booked-bar-2026-11");
    if (!zero || !bookedBar
        || qAbs(zero->mapToScene({0, 0}).y() - bookedBar->mapToScene({0, bookedBar->height()}).y()) > 1)
        return QStringLiteral("Booked bars do not align to the zero axis.");

    if (!click(window, lookup("row-" + second)) || workspace.selectedId() != second || !sourceMatches()
        || workspace.source().value("weighted").toString() != "$13,800")
        return QStringLiteral("Clicking the Harbor Studio ledger row did not update the inspector.");
    auto *inspector = lookup("source-inspector");
    QTest::keyClick(window, Qt::Key_3, Qt::AltModifier);
    if (!inspector || !inspector->hasActiveFocus())
        return QStringLiteral("Alt+3 did not focus the source inspector.");
    QTest::keyClick(window, Qt::Key_Escape);
    auto *row = lookup("row-" + second);
    if (!row || !row->hasActiveFocus())
        return QStringLiteral("Escape did not return focus to the selected ledger row.");
    QTest::keyClick(window, Qt::Key_Up);
    if (workspace.selectedId() != first || !sourceMatches())
        return QStringLiteral("Up did not select and inspect the preceding ledger row.");

    if (!click(window, lookup("currency-EUR")) || workspace.currency() != "EUR"
        || workspace.period() != "2026-11" || workspace.queue().size() != 2 || workspace.selectedId() != "alder"
        || !sourceMatches() || !metricMatches("booked", QStringLiteral("€9,200"))
        || !metricMatches("forecast", QStringLiteral("€11,320")) || !metricMatches("pipeline", QStringLiteral("€19,600"))
        || !metricMatches("targetNote", QStringLiteral("€12,000 target · €2,800 below")))
        return QStringLiteral("The EUR selector did not produce the expected November EUR report.");
    if (!click(window, lookup("month-2026-07")) || workspace.period() != "2026-07"
        || !metricMatches("booked", QStringLiteral("€4,200")) || !metricMatches("targetNote", "No target set")
        || !metricMatches("forecast", QStringLiteral("€11,320")) || !metricMatches("pipeline", QStringLiteral("€19,600"))
        || workspace.selectedId() != "alder")
        return QStringLiteral("Selecting July did not preserve the EUR all-open scope.");
    if (!click(window, lookup("booked-queue")) || !workspace.bookedView() || workspace.queue().size() != 1
        || workspace.selectedId() != "e-jul" || workspace.source().value("currency").toString() != "EUR" || !sourceMatches())
        return QStringLiteral("Booked sources did not show the July EUR source record.");

    window->resize(1200, 780);
    QTest::qWait(40);
    inspector = lookup("source-inspector");
    if (window->width() != 1200 || window->height() != 780 || !inspector || inspector->width() < 270
        || inspector->mapToScene(QPointF(inspector->width(), 0)).x() > window->width())
        return QStringLiteral("The resized desktop did not keep the inspector within the window.");

    QTextStream(stdout) << "Verified exact November USD and EUR totals, zero-aligned bars, July EUR, "
                           "pointer selection, inspector keyboard return, booked sources and 1200×780 layout.\n";
    return {};
}
}

int main(int argc, char **argv)
{
    qputenv("QT_QUICK_CONTROLS_STYLE", "Material");
    QGuiApplication app(argc, argv);
    app.setApplicationName("tide-instrument");
    app.setDesktopFileName("tide-instrument");

    QCommandLineParser parser;
    parser.setApplicationDescription("Native TideContacts design exploration, using a local fictional fixture.");
    parser.addHelpOption();
    const QCommandLineOption fixture("fixture", "Read a local JSON fixture.", "path");
    const QCommandLineOption capture("capture", "Save the initial native scene as a PNG and exit.", "png");
    const QCommandLineOption check("verify", "Exercise native controls against the shared fictional fixture and exit.");
    parser.addOptions({fixture, capture, check});
    parser.process(app);

    if (!parser.isSet(fixture) || !parser.positionalArguments().isEmpty())
        parser.showHelp(2);
    Workspace workspace;
    const QString fixtureError = workspace.load(parser.value(fixture));
    if (!fixtureError.isEmpty()) {
        QTextStream(stderr) << parser.value(fixture) << ": " << fixtureError << '\n';
        return 1;
    }

    QQmlApplicationEngine engine;
    engine.rootContext()->setContextProperty("workspace", &workspace);
    engine.rootContext()->setContextProperty("uiFace", QFontDatabase::systemFont(QFontDatabase::GeneralFont).family());
    engine.rootContext()->setContextProperty("fixedFace", QFontDatabase::systemFont(QFontDatabase::FixedFont).family());
    engine.load(QUrl("qrc:/Main.qml"));
    auto *window = qobject_cast<QQuickWindow *>(engine.rootObjects().value(0));
    if (!window)
        return 1;

    if (parser.isSet(check) || parser.isSet(capture)) {
        QTimer::singleShot(0, &app, [&] {
            QString error;
            if (!QTest::qWaitForWindowExposed(window, 1500)) {
                error = QStringLiteral("The native Qt window was not exposed.");
            } else {
                if (parser.isSet(capture)) {
                    const auto image = window->grabWindow();
                    if (image.isNull() || !image.save(parser.value(capture), "PNG"))
                        error = QStringLiteral("Could not capture the native Qt scene to %1.").arg(parser.value(capture));
                }
                if (parser.isSet(check) && error.isEmpty())
                    error = verify(window, workspace);
            }
            if (!error.isEmpty())
                QTextStream(stderr) << error << '\n';
            app.exit(error.isEmpty() ? 0 : 1);
        });
    }
    return app.exec();
}
