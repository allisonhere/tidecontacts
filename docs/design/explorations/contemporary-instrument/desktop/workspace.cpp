#include "workspace.h"

#include <QFile>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QLocale>
#include <QSet>

#include <algorithm>
#include <limits>
#include <utility>

namespace {
const QLocale english(QLocale::English, QLocale::UnitedStates);
constexpr qint64 basis = 10000;

QDate isoDate(const QJsonObject &object, const char *field)
{
    return QDate::fromString(object.value(QLatin1StringView(field)).toString(), Qt::ISODate);
}

QDate monthDate(const QString &period)
{
    return QDate::fromString(period + "-01", Qt::ISODate);
}

QString dateText(QDate date, const QString &pattern = "dd MMM yyyy")
{
    return english.toString(date, pattern);
}

QString money(const QString &currency, qint64 minor)
{
    const QString mark = currency == "USD" ? QStringLiteral("$")
                         : currency == "EUR" ? QStringLiteral("€") : currency + ' ';
    const auto cents = minor % 100;
    QString result = mark + english.toString(minor / 100);
    if (cents)
        result += QStringLiteral(".%1").arg(cents, 2, 10, QChar('0'));
    return result;
}

QString percent(double value)
{
    auto result = QString::number(value, 'f', 2);
    while (result.endsWith('0'))
        result.chop(1);
    if (result.endsWith('.'))
        result.chop(1);
    return result + '%';
}

qint64 roundedWeighted(qint64 numerator)
{
    return (numerator + basis / 2) / basis;
}

qint64 axisStep(qint64 highest)
{
    const auto minimum = (highest + 3) / 4;
    qint64 decade = 100;
    while (5 * decade < minimum)
        decade *= 10;
    return (minimum <= decade ? 1 : minimum <= 2 * decade ? 2 : 5) * decade;
}
}

QString Workspace::load(const QString &path)
{
    QFile file(path);
    if (!file.open(QIODevice::ReadOnly))
        return file.errorString();
    QJsonParseError error;
    const auto document = QJsonDocument::fromJson(file.readAll(), &error);
    if (error.error != QJsonParseError::NoError)
        return error.errorString();
    if (!document.isObject())
        return QStringLiteral("Expected a fixture object.");

    const auto root = document.object();
    m_asOf = isoDate(root, "as_of");
    if (!m_asOf.isValid() || !root.value("targets").isObject() || !root.value("records").isArray())
        return QStringLiteral("Expected a snapshot date, targets, and records.");
    for (const auto value : root.value("months").toArray()) {
        const auto period = value.toString();
        if (!monthDate(period).isValid() || (!m_months.isEmpty() && period <= m_months.last()))
            return QStringLiteral("Expected distinct ISO months in chronological order.");
        m_months.append(period);
    }
    if (m_months.isEmpty())
        return QStringLiteral("Expected at least one report month.");

    QSet<QString> stages, ids;
    for (const auto value : root.value("stages").toArray())
        stages.insert(value.toString());
    const auto records = root.value("records").toArray();
    const qint64 maxMinor = std::numeric_limits<qint64>::max() / (2 * basis)
                            / std::max<qsizetype>(1, records.size());
    const auto targets = root.value("targets").toObject();
    for (auto month = targets.constBegin(); month != targets.constEnd(); ++month) {
        if (!m_months.contains(month.key()) || !month.value().isObject())
            return QStringLiteral("Invalid target month: %1").arg(month.key());
        const auto amounts = month.value().toObject();
        for (auto amount = amounts.constBegin(); amount != amounts.constEnd(); ++amount) {
            const auto minor = amount.value().toInteger(-1);
            if (amount.key().isEmpty() || minor < 0 || minor > maxMinor)
                return QStringLiteral("Invalid target: %1 / %2").arg(month.key(), amount.key());
            m_targets[month.key()].insert(amount.key(), minor);
            if (!m_totals.contains(amount.key()))
                m_totals.insert(amount.key(), CurrencyTotals());
        }
    }

    m_records.reserve(records.size());
    for (const auto value : records) {
        const auto object = value.toObject();
        Record record;
        record.id = object.value("id").toString();
        record.organization = object.value("organization").toString();
        record.contact = object.value("contact").toString();
        record.role = object.value("role").toString();
        record.title = object.value("title").toString();
        record.stage = object.value("stage").toString();
        record.currency = object.value("currency").toString();
        record.minor = object.value("minor").toInteger(-1);
        record.detail = object.value("detail").toString();
        const auto status = object.value("status").toString();
        record.won = status == "won";
        if (record.id.isEmpty() || ids.contains(record.id) || record.organization.isEmpty()
            || record.contact.isEmpty() || record.title.isEmpty() || record.currency.isEmpty()
            || record.minor < 0 || record.minor > maxMinor || (!record.won && status != "open"))
            return QStringLiteral("Invalid or duplicate source record: %1").arg(record.id);
        ids.insert(record.id);
        auto &totals = m_totals[record.currency];
        if (record.won) {
            record.closed = isoDate(object, "closed");
            if (!record.closed.isValid())
                return QStringLiteral("Missing close date for %1.").arg(record.id);
            totals.booked[record.closed.toString("yyyy-MM")] += record.minor;
        } else {
            record.probability = object.value("probability_bp").toInt(-1);
            record.expected = isoDate(object, "expected");
            record.touch = isoDate(object, "last_touch");
            record.due = isoDate(object, "due");
            record.next = object.value("next").toString();
            record.activity = object.value("activity").toString();
            if (record.probability < 0 || record.probability > basis || !stages.contains(record.stage)
                || !record.expected.isValid() || !record.touch.isValid() || !record.due.isValid()
                || record.next.isEmpty())
                return QStringLiteral("Invalid open details for %1.").arg(record.id);
            totals.open += record.minor;
            record.weightedMinor = roundedWeighted(record.minor * record.probability);
            totals.weightedMinor += record.weightedMinor;
        }
        m_records.append(std::move(record));
    }
    m_currencies = m_totals.keys();
    m_currencies.sort();
    if (m_currencies.isEmpty())
        return QStringLiteral("Expected at least one currency.");
    if (m_currencies.removeOne(QStringLiteral("USD")))
        m_currencies.prepend(QStringLiteral("USD"));
    m_currency = m_currencies.first();
    m_period = m_months.contains(m_asOf.toString("yyyy-MM")) ? m_asOf.toString("yyyy-MM") : m_months.last();
    m_queueRecords.reserve(m_records.size());
    rebuildHistory();
    rebuildMetrics();
    rebuildQueue();
    return {};
}

QString Workspace::asOf() const { return dateText(m_asOf); }

qint64 Workspace::target(const QString &period) const
{
    const auto month = m_targets.constFind(period);
    return month == m_targets.cend() ? -1 : month.value().value(m_currency, -1);
}

void Workspace::rebuildHistory()
{
    const auto totals = m_totals.constFind(m_currency);
    const auto &booked = totals.value().booked;
    qint64 highest = 0;
    for (const auto &period : std::as_const(m_months))
        highest = std::max({highest, booked.value(period), target(period)});
    const qint64 step = axisStep(highest);
    const qint64 intervals = std::max<qint64>(2, (highest + step - 1) / step);
    const double ceiling = intervals * step;
    m_axis.clear();
    m_history.clear();
    m_axis.reserve(intervals + 1);
    m_history.reserve(m_months.size());
    for (qint64 tick = intervals; tick >= 0; --tick)
        m_axis.append(money(m_currency, tick * step));
    for (const auto &period : std::as_const(m_months)) {
        const auto date = monthDate(period);
        const auto amount = booked.value(period);
        const auto goal = target(period);
        m_history.append(QVariantMap {
            {"period", period}, {"month", dateText(date, "MMM")}, {"label", dateText(date, "MMMM yyyy")},
            {"amount", money(m_currency, amount)}, {"portion", amount / ceiling},
            {"target", goal < 0 ? QString() : money(m_currency, goal)}, {"targetPortion", goal / ceiling},
        });
    }
}

void Workspace::rebuildMetrics()
{
    const auto entry = m_totals.constFind(m_currency);
    const auto &totals = entry.value();
    const auto booked = totals.booked.value(m_period);
    const auto goal = target(m_period);
    QString targetNote;
    if (goal < 0)
        targetNote = QStringLiteral("No target set");
    else if (booked == goal)
        targetNote = QStringLiteral("%1 target · on target").arg(money(m_currency, goal));
    else if (booked < goal)
        targetNote = QStringLiteral("%1 target · %2 below").arg(money(m_currency, goal), money(m_currency, goal - booked));
    else
        targetNote = QStringLiteral("%1 target · %2 above").arg(money(m_currency, goal), money(m_currency, booked - goal));
    m_metrics = {
        {"month", dateText(monthDate(m_period), "MMMM yyyy")},
        {"booked", money(m_currency, booked)}, {"targetNote", targetNote},
        {"forecast", money(m_currency, totals.weightedMinor)},
        {"pipeline", money(m_currency, totals.open)},
    };
}

QString Workspace::queueKey() const
{
    return m_bookedView ? m_currency + '/' + m_period : m_currency;
}

QVariantMap Workspace::row(const Record &record) const
{
    const auto urgency = record.won ? QStringLiteral("Closed")
                       : record.due < m_asOf ? QStringLiteral("Overdue")
                       : record.due == m_asOf ? QStringLiteral("Due today") : QStringLiteral("Upcoming");
    return {
        {"id", record.id}, {"organization", record.organization}, {"title", record.title},
        {"stage", record.stage}, {"amount", money(record.currency, record.minor)},
        {"urgency", urgency}, {"date", dateText(record.won ? record.closed : record.due, "dd MMM")},
        {"action", record.won ? record.title : record.next},
    };
}

QVariantMap Workspace::detail(const Record &record) const
{
    auto fields = row(record);
    fields.insert("currency", record.currency);
    fields.insert("contact", record.contact);
    fields.insert("role", record.role);
    fields.insert("note", record.detail);
    if (record.won) {
        fields.insert("closed", dateText(record.closed));
    } else {
        fields.insert("due", dateText(record.due));
        fields.insert("expected", dateText(record.expected));
        fields.insert("touch", dateText(record.touch));
        fields.insert("activity", record.activity);
        fields.insert("probability", percent(record.probability / 100.0));
        fields.insert("weighted", money(record.currency, record.weightedMinor));
    }
    return fields;
}

void Workspace::rebuildQueue()
{
    const auto period = monthDate(m_period);
    m_queueRecords.clear();
    for (const auto &record : std::as_const(m_records)) {
        if (record.currency != m_currency || record.won != m_bookedView)
            continue;
        if (!record.won || (record.closed.year() == period.year() && record.closed.month() == period.month()))
            m_queueRecords.append(&record);
    }
    std::sort(m_queueRecords.begin(), m_queueRecords.end(), [this](const Record *a, const Record *b) {
        if (m_bookedView && a->closed != b->closed)
            return a->closed > b->closed;
        if (!m_bookedView && a->due != b->due)
            return a->due < b->due;
        return a->id < b->id;
    });
    const auto remembered = m_selections.value(queueKey());
    const Record *selected = m_queueRecords.isEmpty() ? nullptr : m_queueRecords.first();
    m_queue.clear();
    m_queue.reserve(m_queueRecords.size());
    for (const Record *record : std::as_const(m_queueRecords)) {
        m_queue.append(row(*record));
        if (record->id == remembered)
            selected = record;
    }
    m_source = selected ? detail(*selected) : QVariantMap();
}

void Workspace::setCurrency(const QString &currency)
{
    if (m_currency == currency || !m_currencies.contains(currency))
        return;
    m_currency = currency;
    rebuildHistory();
    rebuildMetrics();
    rebuildQueue();
    emit historyChanged();
    emit scopeChanged();
    emit queueChanged();
    emit sourceChanged();
}

void Workspace::setPeriod(const QString &period)
{
    if (m_period == period || !m_months.contains(period))
        return;
    m_period = period;
    rebuildMetrics();
    if (m_bookedView)
        rebuildQueue();
    emit scopeChanged();
    if (m_bookedView) {
        emit queueChanged();
        emit sourceChanged();
    }
}

void Workspace::setBookedView(bool booked)
{
    if (m_bookedView == booked)
        return;
    m_bookedView = booked;
    rebuildQueue();
    emit queueChanged();
    emit sourceChanged();
}

void Workspace::selectRecord(const QString &id)
{
    if (selectedId() == id)
        return;
    for (const Record *record : std::as_const(m_queueRecords)) {
        if (record->id == id) {
            m_selections.insert(queueKey(), id);
            m_source = detail(*record);
            emit sourceChanged();
            return;
        }
    }
}
