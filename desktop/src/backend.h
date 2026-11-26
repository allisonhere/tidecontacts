#pragma once

#include <QJsonObject>
#include <QNetworkAccessManager>
#include <QProcess>
#include <QTimer>
#include <QUrl>
#include <QVariant>
#include <QVector>
#include <functional>

class Backend final : public QObject {
    Q_OBJECT
    Q_PROPERTY(QString workspace READ workspace NOTIFY scopeChanged)
    Q_PROPERTY(QString query READ query NOTIFY scopeChanged)
    Q_PROPERTY(QString taskView READ taskView NOTIFY scopeChanged)
    Q_PROPERTY(QString sort READ sort NOTIFY scopeChanged)
    Q_PROPERTY(int offset READ offset NOTIFY scopeChanged)
    Q_PROPERTY(bool hasNext READ hasNext NOTIFY indexChanged)
    Q_PROPERTY(QVariantList rows READ rows NOTIFY indexChanged)
    Q_PROPERTY(QString recordKind READ recordKind NOTIFY detailChanged)
    Q_PROPERTY(QString recordId READ recordId NOTIFY detailChanged)
    Q_PROPERTY(QVariantMap record READ record NOTIFY detailChanged)
    Q_PROPERTY(QVariantMap dossier READ dossier NOTIFY detailChanged)
    Q_PROPERTY(QVariantList items READ items NOTIFY detailChanged)
    Q_PROPERTY(bool inspecting READ inspecting NOTIFY detailChanged)
    Q_PROPERTY(bool nested READ nested NOTIFY detailChanged)
    Q_PROPERTY(bool demo READ demo CONSTANT)
    Q_PROPERTY(bool ready READ ready NOTIFY sessionChanged)
    Q_PROPERTY(bool loading READ loading NOTIFY indexChanged)
    Q_PROPERTY(bool detailLoading READ detailLoading NOTIFY detailChanged)
    Q_PROPERTY(bool writing READ writing NOTIFY writeChanged)
    Q_PROPERTY(QString indexError READ indexError NOTIFY indexChanged)
    Q_PROPERTY(QString detailError READ detailError NOTIFY detailChanged)
    Q_PROPERTY(QString sessionError READ sessionError NOTIFY sessionChanged)
    Q_PROPERTY(QString writeError READ writeError NOTIFY writeChanged)

public:
    explicit Backend(bool demo, QObject *parent = nullptr);
    ~Backend() override;
    void start(const QString &server);

    QString workspace() const { return m_workspace; }
    QString query() const { return m_query; }
    QString taskView() const { return m_taskView; }
    QString sort() const { return m_sort; }
    int offset() const { return m_offset; }
    bool hasNext() const { return m_nextOffset > m_offset; }
    QVariantList rows() const { return m_rows; }
    QString recordKind() const { return m_address.kind; }
    QString recordId() const { return m_address.id; }
    QVariantMap record() const { return m_record; }
    QVariantMap dossier() const { return m_dossier; }
    QVariantList items() const { return m_items; }
    bool inspecting() const { return m_inspecting; }
    bool nested() const { return !m_history.isEmpty(); }
    bool demo() const { return m_demo; }
    bool ready() const { return m_ready; }
    bool loading() const { return m_loading; }
    bool detailLoading() const { return m_detailLoading; }
    bool writing() const { return m_writing; }
    QString indexError() const { return m_indexError; }
    QString detailError() const { return m_detailError; }
    QString sessionError() const { return m_sessionError; }
    QString writeError() const { return m_writeError; }
    bool captureReady() const;

    Q_INVOKABLE void browse(const QString &kind);
    Q_INVOKABLE void search(const QString &query);
    Q_INVOKABLE void setTaskView(const QString &view);
    Q_INVOKABLE void setSort(const QString &sort);
    Q_INVOKABLE void nextPage();
    Q_INVOKABLE void previousPage();
    Q_INVOKABLE void select(const QString &id);
    Q_INVOKABLE void openLink(const QString &kind, const QString &id);
    Q_INVOKABLE void back();
    Q_INVOKABLE void refresh();
    Q_INVOKABLE void saveRecord(bool create, const QVariantMap &fields);
    Q_INVOKABLE void createLinked(const QString &kind, const QVariantMap &fields);
    Q_INVOKABLE void archiveContact(bool archived);
    Q_INVOKABLE void completeTask();
    Q_INVOKABLE void moveItem(const QString &id, const QString &stage);
    Q_INVOKABLE void clearWriteError();
    Q_INVOKABLE void copyId();

signals:
    void sessionChanged();
    void scopeChanged();
    void indexChanged();
    void detailChanged();
    void writeChanged();
    void writeSucceeded();

private:
    struct Address {
        QString kind;
        QString id;
    };
    using Result = std::function<void(const QJsonValue &, const QString &)>;

    void readLaunch();
    void failSession(const QString &message);
    void request(const QByteArray &method, const QUrl &url, const QJsonObject &body, Result receive);
    QUrl endpoint(const QString &kind, const QString &id = {}, const QString &action = {}) const;
    void resetScope();
    void loadIndex(bool replace);
    void loadDetail(const Address &address);
    void mutate(const QByteArray &method, const QUrl &url, const QJsonObject &body,
                const QString &createdKind = {});

    const bool m_demo;
    QProcess m_process;
    QNetworkAccessManager m_network;
    QTimer m_launchTimeout;
    QUrl m_origin;
    QByteArray m_token, m_launch;
    bool m_ready = false, m_loading = false, m_detailLoading = false;
    bool m_writing = false, m_inspecting = false, m_stopping = false;
    QString m_sessionError, m_indexError, m_detailError, m_writeError;
    QString m_workspace = QStringLiteral("people");
    QString m_query, m_taskView = QStringLiteral("all"), m_sort = QStringLiteral("name");
    int m_offset = 0, m_nextOffset = -1;
    QVector<int> m_pages;
    quint64 m_indexSerial = 0, m_detailSerial = 0;
    Address m_address;
    QVector<Address> m_history;
    QVariantList m_rows, m_items;
    QVariantMap m_record, m_dossier;
};
