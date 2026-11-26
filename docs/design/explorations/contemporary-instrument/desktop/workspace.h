#pragma once

#include <QDate>
#include <QHash>
#include <QList>
#include <QMap>
#include <QObject>
#include <QStringList>
#include <QVariant>

class Workspace final : public QObject
{
    Q_OBJECT
    Q_PROPERTY(QString asOf READ asOf CONSTANT)
    Q_PROPERTY(QStringList currencies READ currencies CONSTANT)
    Q_PROPERTY(QString currency READ currency NOTIFY scopeChanged)
    Q_PROPERTY(QString period READ period NOTIFY scopeChanged)
    Q_PROPERTY(QVariantMap metrics READ metrics NOTIFY scopeChanged)
    Q_PROPERTY(QVariantList history READ history NOTIFY historyChanged)
    Q_PROPERTY(QStringList axis READ axis NOTIFY historyChanged)
    Q_PROPERTY(bool bookedView READ bookedView NOTIFY queueChanged)
    Q_PROPERTY(QVariantList queue READ queue NOTIFY queueChanged)
    Q_PROPERTY(QVariantMap source READ source NOTIFY sourceChanged)
    Q_PROPERTY(QString selectedId READ selectedId NOTIFY sourceChanged)

public:
    QString load(const QString &path);

    QString asOf() const;
    QStringList currencies() const { return m_currencies; }
    QString currency() const { return m_currency; }
    QString period() const { return m_period; }
    QVariantMap metrics() const { return m_metrics; }
    QVariantList history() const { return m_history; }
    QStringList axis() const { return m_axis; }
    bool bookedView() const { return m_bookedView; }
    QVariantList queue() const { return m_queue; }
    QVariantMap source() const { return m_source; }
    QString selectedId() const { return m_source.value("id").toString(); }

    Q_INVOKABLE void setCurrency(const QString &currency);
    Q_INVOKABLE void setPeriod(const QString &period);
    Q_INVOKABLE void setBookedView(bool booked);
    Q_INVOKABLE void selectRecord(const QString &id);

signals:
    void scopeChanged();
    void historyChanged();
    void queueChanged();
    void sourceChanged();

private:
    struct Record {
        QString id, organization, contact, role, title, stage, currency, next, detail, activity;
        QDate expected, touch, due, closed;
        qint64 minor = 0;
        qint64 weightedMinor = 0;
        int probability = 0;
        bool won = false;
    };

    struct CurrencyTotals {
        QHash<QString, qint64> booked;
        qint64 open = 0;
        qint64 weightedMinor = 0;
    };

    qint64 target(const QString &period) const;

    void rebuildHistory();
    void rebuildMetrics();
    void rebuildQueue();
    QString queueKey() const;
    QVariantMap row(const Record &record) const;
    QVariantMap detail(const Record &record) const;

    QDate m_asOf;
    QStringList m_months, m_currencies;
    QHash<QString, QHash<QString, qint64>> m_targets;
    QHash<QString, CurrencyTotals> m_totals;
    QList<Record> m_records;
    QString m_currency, m_period;
    bool m_bookedView = false;
    QList<const Record *> m_queueRecords;
    QHash<QString, QString> m_selections;
    QVariantMap m_source, m_metrics;
    QVariantList m_queue, m_history;
    QStringList m_axis;
};
