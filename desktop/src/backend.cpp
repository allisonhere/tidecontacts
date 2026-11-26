#include "backend.h"

#include <QClipboard>
#include <QGuiApplication>
#include <QJsonArray>
#include <QJsonDocument>
#include <QNetworkProxy>
#include <QNetworkReply>
#include <QNetworkRequest>
#include <QRegularExpression>
#include <QUrlQuery>
#include <utility>

#ifdef Q_OS_LINUX
#include <csignal>
#include <sys/prctl.h>
#include <unistd.h>
#endif

namespace {
bool isContact(const QString &kind)
{
    return kind == "people" || kind == "organizations";
}

bool isWorkspace(const QString &kind)
{
    return isContact(kind) || kind == "tasks" || kind == "activities" || kind == "projects" || kind == "pipelines";
}

QString collectionFor(const QString &kind)
{
    if (kind == "person") return QStringLiteral("people");
    if (kind == "organization") return QStringLiteral("organizations");
    if (kind == "task") return QStringLiteral("tasks");
    if (kind == "activity") return QStringLiteral("activities");
    if (kind == "project") return QStringLiteral("projects");
    if (kind == "pipeline") return QStringLiteral("pipelines");
    return {};
}

bool usableId(const QString &id)
{
    return !id.isEmpty() && id != "." && id != ".." && !id.contains(QChar::Null);
}
}

Backend::Backend(bool demo, QObject *parent) : QObject(parent), m_demo(demo)
{
    m_network.setProxy(QNetworkProxy::NoProxy);
    m_launchTimeout.setSingleShot(true);
    connect(&m_launchTimeout, &QTimer::timeout, this, [this] {
        failSession(tr("The service did not start in time."));
        m_process.kill();
    });
    connect(&m_process, &QProcess::readyReadStandardOutput, this, &Backend::readLaunch);
    connect(&m_process, &QProcess::readyReadStandardError, this, [this] {
        // Child diagnostics never enter the GUI or the session protocol.
        m_process.readAllStandardError();
    });
    connect(&m_process, &QProcess::errorOccurred, this, [this](QProcess::ProcessError error) {
        if (error == QProcess::FailedToStart)
            failSession(tr("Could not start the local service. Check the executable selected with --server."));
    });
    connect(&m_process, &QProcess::finished, this, [this](int code, QProcess::ExitStatus status) {
        if (!m_stopping)
            failSession(status == QProcess::CrashExit ? tr("The local service stopped unexpectedly.")
                                                    : tr("The local service exited (code %1).").arg(code));
    });
#ifdef Q_OS_LINUX
    // Also stop the private child if the desktop is killed before it can close normally.
    m_process.setChildProcessModifier([parentPid = getpid()] {
        if (prctl(PR_SET_PDEATHSIG, SIGTERM, 0, 0, 0) == -1 || getppid() != parentPid)
            _exit(127);
    });
#endif
}

Backend::~Backend()
{
    m_stopping = true;
    if (m_process.state() != QProcess::NotRunning) {
        m_process.terminate();
        if (!m_process.waitForFinished(1000)) {
            m_process.kill();
            m_process.waitForFinished(1000);
        }
    }
}

void Backend::start(const QString &server)
{
    m_launchTimeout.start(12000);
    m_process.start(server, {QStringLiteral("--profile"), m_demo ? QStringLiteral("demo") : QStringLiteral("local"),
                             QStringLiteral("--api-only")});
}

void Backend::readLaunch()
{
    if (m_stopping)
        return;
    m_launch += m_process.readAllStandardOutput();
    if (m_launch.isEmpty())
        return;
    if (m_launch.size() <= 512 && !m_launch.contains('\n') && m_origin.isEmpty())
        return;

    // Parse the complete wire format rather than accepting URLs that only appear local.
    static const QRegularExpression line("\\Ahttp://127\\.0\\.0\\.1:([1-9][0-9]{0,4})/#session=([A-Za-z0-9_-]{1,256})\\n\\z");
    const auto match = line.match(QString::fromLatin1(m_launch));
    m_launch.clear();
    const int port = match.captured(1).toInt();
    if (!m_origin.isEmpty() || !match.hasMatch() || port < 1 || port > 65535) {
        failSession(tr("The service did not announce a valid loopback session."));
        m_process.kill();
        return;
    }
    m_origin.setScheme(QStringLiteral("http"));
    m_origin.setHost(QStringLiteral("127.0.0.1"));
    m_origin.setPort(port);
    m_token = match.captured(2).toLatin1();

    request("GET", endpoint(QStringLiteral("profile")), {}, [this](const QJsonValue &data, const QString &error) {
        const auto demo = data.toObject().value("demo");
        if (!error.isEmpty() || !demo.isBool() || demo.toBool() != m_demo) {
            failSession(error.isEmpty() ? tr("The service opened a different profile than requested.") : error);
            m_process.kill();
            return;
        }
        m_launchTimeout.stop();
        m_ready = true;
        emit sessionChanged();
        loadIndex(true);
    });
}

void Backend::failSession(const QString &message)
{
    if (!m_sessionError.isEmpty())
        return;
    m_launchTimeout.stop();
    m_token.clear();
    m_origin.clear();
    m_launch.clear();
    m_ready = m_loading = m_detailLoading = m_writing = false;
    m_sessionError = message;
    emit sessionChanged();
    emit indexChanged();
    emit detailChanged();
    emit writeChanged();
}

QUrl Backend::endpoint(const QString &kind, const QString &id, const QString &action) const
{
    QUrl url = m_origin;
    QString path = QStringLiteral("/api/v1/") + kind;
    if (!id.isEmpty())
        path += QChar('/') + QString::fromLatin1(QUrl::toPercentEncoding(id));
    if (!action.isEmpty())
        path += QChar('/') + action;
    url.setPath(path, QUrl::StrictMode);
    return url;
}

void Backend::request(const QByteArray &method, const QUrl &url, const QJsonObject &body, Result receive)
{
    if (m_token.isEmpty() || !url.isValid() || url.scheme() != m_origin.scheme() || url.host() != m_origin.host()
        || url.port() != m_origin.port() || !url.userInfo().isEmpty() || url.hasFragment()) {
        receive({}, tr("The request was not sent to the private service."));
        return;
    }
    QNetworkRequest req(url);
    req.setAttribute(QNetworkRequest::RedirectPolicyAttribute, QNetworkRequest::ManualRedirectPolicy);
    req.setAttribute(QNetworkRequest::CookieLoadControlAttribute, QNetworkRequest::Manual);
    req.setAttribute(QNetworkRequest::CookieSaveControlAttribute, QNetworkRequest::Manual);
    req.setAttribute(QNetworkRequest::AuthenticationReuseAttribute, QNetworkRequest::Manual);
    req.setTransferTimeout(10000);
    req.setRawHeader("Authorization", "Bearer " + m_token);
    req.setRawHeader("Accept", "application/json");
    QNetworkReply *reply;
    if (method == "GET") {
        reply = m_network.get(req);
    } else {
        req.setHeader(QNetworkRequest::ContentTypeHeader, "application/json");
        reply = m_network.sendCustomRequest(req, method, QJsonDocument(body).toJson(QJsonDocument::Compact));
    }

    connect(reply, &QNetworkReply::finished, this, [this, reply, receive = std::move(receive)] {
        reply->deleteLater();
        if (m_token.isEmpty() || m_stopping)
            return;
        const int status = reply->attribute(QNetworkRequest::HttpStatusCodeAttribute).toInt();
        if (status >= 300 && status < 400) {
            receive({}, tr("The private service attempted to redirect a request."));
            return;
        }
        const auto envelope = QJsonDocument::fromJson(reply->readAll()).object();
        if (status >= 200 && status < 300 && reply->error() == QNetworkReply::NoError) {
            if (envelope.value("schema_version").toInt() != 1 || !envelope.value("data").isObject()) {
                receive({}, tr("The service returned an unexpected response."));
                return;
            }
            receive(envelope.value("data"), {});
            return;
        }
        const auto error = envelope.value("error").toObject().value("message").toString();
        receive({}, !error.isEmpty() ? error : status == 0 ? tr("Could not reach the local service.")
                                                          : tr("The service rejected this request (HTTP %1).").arg(status));
    });
}

void Backend::browse(const QString &kind)
{
    if (m_writing || !isWorkspace(kind) || m_workspace == kind)
        return;
    m_workspace = kind;
    m_query.clear();
    resetScope();
}

void Backend::search(const QString &query)
{
    if (m_writing || m_query == query)
        return;
    m_query = query;
    resetScope();
}

void Backend::setTaskView(const QString &view)
{
    static const QStringList views{"all", "open", "inbox", "today", "upcoming", "waiting", "overdue", "completed"};
    if (m_writing || m_workspace != "tasks" || m_taskView == view || !views.contains(view))
        return;
    m_taskView = view;
    resetScope();
}

void Backend::setSort(const QString &sort)
{
    if (m_writing || !isContact(m_workspace) || m_sort == sort || (sort != "name" && sort != "updated"))
        return;
    m_sort = sort;
    resetScope();
}

void Backend::resetScope()
{
    m_pages.clear();
    m_offset = 0;
    m_history.clear();
    m_inspecting = false;
    emit scopeChanged();
    loadDetail({});
    loadIndex(true);
}

void Backend::nextPage()
{
    if (!m_ready || m_loading || m_writing || !hasNext())
        return;
    m_pages.append(m_offset);
    m_offset = m_nextOffset;
    m_history.clear();
    m_inspecting = false;
    emit scopeChanged();
    loadDetail({});
    loadIndex(true);
}

void Backend::previousPage()
{
    if (!m_ready || m_loading || m_writing || m_pages.isEmpty())
        return;
    m_offset = m_pages.takeLast();
    m_history.clear();
    m_inspecting = false;
    emit scopeChanged();
    loadDetail({});
    loadIndex(true);
}

void Backend::loadIndex(bool replace)
{
    const auto serial = ++m_indexSerial;
    m_indexError.clear();
    m_loading = m_ready;
    if (replace) {
        m_rows.clear();
        m_nextOffset = -1;
    }
    emit indexChanged();
    if (!m_ready)
        return;

    auto url = endpoint(m_workspace);
    QUrlQuery scope;
    scope.addQueryItem(QStringLiteral("offset"), QString::number(m_offset));
    scope.addQueryItem(QStringLiteral("limit"), QStringLiteral("50"));
    if (!m_query.trimmed().isEmpty())
        scope.addQueryItem(QStringLiteral("q"), QString::fromLatin1(QUrl::toPercentEncoding(m_query.trimmed())));
    if (m_workspace == "tasks")
        scope.addQueryItem(QStringLiteral("view"), m_taskView);
    if (isContact(m_workspace))
        scope.addQueryItem(QStringLiteral("sort"), m_sort);
    url.setQuery(scope);
    request("GET", url, {}, [this, serial](const QJsonValue &data, const QString &error) {
        if (serial != m_indexSerial)
            return;
        m_loading = false;
        m_indexError = error;
        if (error.isEmpty()) {
            const auto result = data.toObject();
            const auto items = result.value("items");
            if (!items.isArray() && !items.isNull()) {
                m_indexError = tr("The service returned an invalid record list.");
            } else {
                m_rows = items.toArray().toVariantList();
                m_nextOffset = result.value("next_offset").toInt(-1);
            }
        }
        emit indexChanged();
        if (m_indexError.isEmpty() && m_address.id.isEmpty() && !m_rows.isEmpty()) {
            const auto id = m_rows.first().toMap().value(QStringLiteral("ID")).toString();
            if (usableId(id))
                loadDetail({m_workspace, id});
        }
    });
}

void Backend::select(const QString &id)
{
    if (!m_ready || m_writing || !usableId(id))
        return;
    m_history.clear();
    m_inspecting = true;
    if (m_address.kind == m_workspace && m_address.id == id && (!m_record.isEmpty() || m_detailLoading)) {
        emit detailChanged();
        return;
    }
    loadDetail({m_workspace, id});
}

void Backend::openLink(const QString &kind, const QString &id)
{
    const auto collection = collectionFor(kind);
    if (!m_ready || m_writing || collection.isEmpty() || !usableId(id)
        || (m_address.kind == collection && m_address.id == id))
        return;
    if (!m_address.id.isEmpty())
        m_history.append(m_address);
    m_inspecting = true;
    loadDetail({collection, id});
}

void Backend::back()
{
    if (m_writing)
        return;
    if (!m_history.isEmpty()) {
        loadDetail(m_history.takeLast());
    } else if (m_inspecting) {
        m_inspecting = false;
        emit detailChanged();
    }
}

void Backend::refresh()
{
    if (!m_ready || m_writing)
        return;
    loadIndex(false);
    if (!m_address.id.isEmpty())
        loadDetail(m_address);
}

void Backend::loadDetail(const Address &address)
{
    const auto serial = ++m_detailSerial;
    m_address = address;
    m_record.clear();
    m_dossier.clear();
    m_items.clear();
    m_detailError.clear();
    m_detailLoading = m_ready && !address.id.isEmpty();
    emit detailChanged();
    if (!m_detailLoading)
        return;

    request("GET", endpoint(address.kind, address.id), {}, [this, serial](const QJsonValue &data, const QString &error) {
        if (serial != m_detailSerial)
            return;
        m_detailLoading = false;
        m_detailError = error;
        if (error.isEmpty()) {
            const auto result = data.toObject();
            const bool contact = isContact(m_address.kind);
            const bool pipeline = m_address.kind == "pipelines";
            const auto record = (contact || pipeline) ? result.value("record").toObject() : result;
            if (record.value("ID").toString() != m_address.id) {
                m_detailError = tr("The service returned a different record than requested.");
            } else {
                m_record = record.toVariantMap();
                if (contact)
                    m_dossier = result.value("dossier").toObject().toVariantMap();
                if (pipeline)
                    m_items = result.value("items").toArray().toVariantList();
            }
        }
        emit detailChanged();
    });
}

void Backend::saveRecord(bool create, const QVariantMap &fields)
{
    const auto kind = create ? m_workspace : m_address.kind;
    if (!m_ready || m_writing || !isWorkspace(kind) || kind == "pipelines" || (!create && m_record.isEmpty()))
        return;
    auto body = QJsonObject::fromVariantMap(fields);
    if (!create)
        body.insert("revision", m_record.value(QStringLiteral("Revision")).toInt());
    mutate(create ? "POST" : "PATCH", endpoint(kind, create ? QString() : m_address.id), body,
           create ? kind : QString());
}

void Backend::createLinked(const QString &kind, const QVariantMap &fields)
{
    // A task or activity created from the open record; the record then reloads in place.
    if (!m_ready || m_writing || (kind != "tasks" && kind != "activities") || m_address.id.isEmpty())
        return;
    mutate("POST", endpoint(kind), QJsonObject::fromVariantMap(fields));
}

void Backend::archiveContact(bool archived)
{
    if (!isContact(m_address.kind) || m_record.isEmpty() || m_record.value(QStringLiteral("Archived")).toBool() == archived)
        return;
    mutate("PATCH", endpoint(m_address.kind, m_address.id),
           {{"revision", m_record.value(QStringLiteral("Revision")).toInt()}, {"archived", archived}});
}

void Backend::completeTask()
{
    if (m_address.kind != "tasks" || m_record.isEmpty())
        return;
    const auto status = m_record.value(QStringLiteral("Status")).toString();
    if (status != "completed" && status != "canceled")
        mutate("POST", endpoint(m_address.kind, m_address.id, QStringLiteral("complete")),
               {{"revision", m_record.value(QStringLiteral("Revision")).toInt()}});
}

void Backend::moveItem(const QString &id, const QString &stage)
{
    if (m_address.kind != "pipelines" || !usableId(id) || stage.isEmpty())
        return;
    bool knownStage = false;
    for (const auto &entry : m_record.value(QStringLiteral("Stages")).toList()) {
        if (entry.toMap().value(QStringLiteral("ID")).toString() == stage) {
            knownStage = true;
            break;
        }
    }
    if (!knownStage)
        return;
    for (const auto &entry : std::as_const(m_items)) {
        const auto item = entry.toMap();
        if (item.value(QStringLiteral("ID")).toString() != id || item.value(QStringLiteral("StageID")).toString() == stage)
            continue;
        mutate("POST", endpoint(QStringLiteral("items"), id, QStringLiteral("move")),
               {{"revision", item.value(QStringLiteral("Revision")).toInt()}, {"stage_id", stage}});
        return;
    }
}

void Backend::mutate(const QByteArray &method, const QUrl &url, const QJsonObject &body, const QString &createdKind)
{
    if (!m_ready || m_writing)
        return;
    m_writing = true;
    m_writeError.clear();
    emit writeChanged();
    request(method, url, body, [this, createdKind](const QJsonValue &data, const QString &error) {
        m_writing = false;
        m_writeError = error;
        const auto id = data.toObject().value("ID").toString();
        if (error.isEmpty() && !usableId(id))
            m_writeError = tr("The service did not return the saved record. Refresh before trying again.");
        emit writeChanged();
        if (!m_writeError.isEmpty())
            return;
        emit writeSucceeded();
        loadIndex(false);
        if (!createdKind.isEmpty()) {
            m_history.clear();
            m_inspecting = true;
            loadDetail({createdKind, id});
        } else {
            loadDetail(m_address);
        }
    });
}

void Backend::clearWriteError()
{
    if (!m_writeError.isEmpty() && !m_writing) {
        m_writeError.clear();
        emit writeChanged();
    }
}

void Backend::copyId()
{
    if (!m_record.isEmpty())
        QGuiApplication::clipboard()->setText(m_address.id);
}

bool Backend::captureReady() const
{
    return m_ready && !m_loading && !m_detailLoading && !m_rows.isEmpty() && !m_record.isEmpty()
        && m_record.value(QStringLiteral("ID")).toString() == m_rows.first().toMap().value(QStringLiteral("ID")).toString();
}
