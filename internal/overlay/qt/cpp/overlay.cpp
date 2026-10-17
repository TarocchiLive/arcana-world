#include "bridge.h"
#include "platform.h"

#include <QApplication>
#include <QEventLoop>
#include <QDir>
#include <QElapsedTimer>
#include <QFile>
#include <QFontDatabase>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QMouseEvent>
#include <QPainter>
#include <QPainterPath>
#include <QPointer>
#include <QScreen>
#include <QTextLayout>
#include <QTextCharFormat>
#include <QTextBoundaryFinder>
#include <QFontMetricsF>
#include <QtMath>
#include <QThread>
#include <QTimer>
#include <QVariantAnimation>
#include <QWindow>
#include <algorithm>
#include <array>
#include <cstdlib>
#include <cstring>
#include <memory>
#include <map>
#include <mutex>
#include <vector>

namespace {
std::mutex lifecycleMutex;
std::mutex errorMutex;
QByteArray lastError;
QApplication *application = nullptr;
QString policyFailure;
bool running = false;
bool finished = false;
int argcValue = 1;
char appName[] = "arcana-overlay-qt";
char *argvValue[] = {appName, nullptr};

void fail(const QString &message) {
    std::lock_guard<std::mutex> lock(errorMutex);
    lastError = message.toUtf8();
}

void failPolicy(const QString &message) {
    // 原生窗口策略失败必须结束辅助进程，让托管生命周期收到具体错误。
    if (policyFailure.isEmpty()) policyFailure = message;
    fail(policyFailure);
    QApplication::exit(1);
}

bool parse(const char *json, QJsonObject &object) {
    if (!json) {
        fail(QStringLiteral("Elegant overlay: null configuration"));
        return false;
    }
    QJsonParseError error;
    const auto document = QJsonDocument::fromJson(QByteArray(json), &error);
    if (error.error != QJsonParseError::NoError || !document.isObject()) {
        fail(QStringLiteral("Elegant overlay: invalid configuration JSON: ") + error.errorString());
        return false;
    }
    object = document.object();
    return true;
}

// Go Normalize 负责配置校验；这里只处理显示设备相关的几何约束。
QRect frame(const QJsonObject &config, QScreen *screen) {
    const QRect bounds = screen->geometry();
    const int width = qRound(std::clamp(config.value("width").toDouble(420), 1.0, double(bounds.width())));
    const int height = qRound(std::clamp(config.value("height").toDouble(180), 1.0, double(bounds.height())));
    const auto position = config.value("position").toObject();
    const QString anchor = position.value("anchor").toString("left");
    const auto axis = [](double offset, int available, bool first, bool last) {
        if (last) offset = available - offset;
        else if (!first) offset += available / 2.0;
        return qRound(std::clamp(offset, 0.0, double(available)));
    };
    return {bounds.x() + axis(position.value("x").toDouble(), bounds.width() - width,
                             anchor.contains("left"), anchor.contains("right")),
            bounds.y() + axis(position.value("y").toDouble(), bounds.height() - height,
                             anchor.contains("top"), anchor.contains("bottom")), width, height};
}

#if defined(Q_OS_WIN)
QString resolveDisplaySelection(const QJsonObject &config, QString &resolvedDisplayId) {
    if (!config.value("displays").toString().isEmpty() || !config.value("output").toString().isEmpty()) return {};
    const quint32 index = quint32(config.value("display_id").toDouble());
    if (!index) return {};
    const QString id = awqt::screenIdForDisplay(index);
    if (id.isEmpty()) return QStringLiteral("Elegant overlay: Windows display ID %1 not found").arg(index);
    // 只在选择改变时解析枚举序号，之后直接匹配稳定设备 ID。
    resolvedDisplayId = id;
    return {};
}
#endif

bool selected(const QJsonObject &config, QScreen *screen, const QString &resolvedDisplayId = QString()) {
    const QString serialized = config.value("displays").toString();
    if (!serialized.isEmpty()) {
        const auto ids = QJsonDocument::fromJson(serialized.toUtf8()).array();
        return ids.contains(awqt::screenId(screen));
    }
    const QString output = config.value("output").toString();
    if (!output.isEmpty()) return output == screen->name();
    if (!resolvedDisplayId.isEmpty()) return resolvedDisplayId == awqt::screenId(screen);
#if !defined(Q_OS_WIN)
    const quint32 id = quint32(config.value("display_id").toDouble());
    if (id) return id == awqt::nativeDisplayId(screen);
#endif
    return screen == QGuiApplication::primaryScreen();
}

QColor color(const QJsonObject &colors, const char *key, const char *fallback) {
    return QColor(colors.value(QLatin1String(key)).toString(QLatin1String(fallback)));
}

QString elideGraphemes(QString text, const QFont &font, qreal width) {
    while (!text.isEmpty() && text.back().isSpace()) text.chop(1);
    const QFontMetricsF metrics(font);
    const QString ellipsis(QChar(0x2026));
    const qreal available = width - metrics.horizontalAdvance(ellipsis);
    if (available < 0) return {};
    std::vector<qsizetype> boundaries{0};
    QTextBoundaryFinder finder(QTextBoundaryFinder::Grapheme, text);
    while (finder.toNextBoundary() >= 0) boundaries.push_back(finder.position());
    size_t low = 0, high = boundaries.size();
    while (low + 1 < high) {
        const size_t middle = (low + high) / 2;
        if (metrics.horizontalAdvance(text, int(boundaries[middle])) <= available) low = middle;
        else high = middle;
    }
    return text.left(boundaries[low]) + ellipsis;
}

struct MessageLayout {
    std::unique_ptr<QTextLayout> layout;
    qreal height = 0;
};

// 使用实际字体布局限制完整行数；省略仅在字素边界进行，不拆开 emoji 或组合字符。
MessageLayout layoutMessage(QString text, const QFont &font, const QTextCharFormat &format,
                            qreal width, qreal height, int limit, qreal gap, Qt::Alignment alignment) {
    text.replace(u'\n', QChar(0x2028));
    auto make = [&](const QString &value) {
        auto layout = std::make_unique<QTextLayout>(value, font);
        layout->setCacheEnabled(true);
        QTextOption option;
        option.setWrapMode(QTextOption::WrapAtWordBoundaryOrAnywhere);
        option.setAlignment(alignment);
        layout->setTextOption(option);
        layout->setFormats({QTextLayout::FormatRange{0, int(value.size()), format}});
        return layout;
    };
    auto layout = make(text);
    std::array<QTextLine, 12> visible;
    int count = 0;
    qreal used = 0;
    bool clipped = false;
    layout->beginLayout();
    while (true) {
        QTextLine line = layout->createLine();
        if (!line.isValid()) break;
        line.setLineWidth(width);
        const qreal top = used + (count ? gap : 0);
        if (count == std::min(limit, 12) || top + line.height() > height + .01) {
            clipped = true;
            break;
        }
        line.setPosition(QPointF(0, top));
        visible[size_t(count++)] = line;
        used = top + line.height();
        if (line.naturalTextWidth() > width + .01) { clipped = true; break; }
    }
    layout->endLayout();
    if (!clipped) return {std::move(layout), used};
    if (!count) return {};
    QString shortened;
    for (int i = 0; i < count; ++i) {
        const auto &line = visible[size_t(i)];
        QString segment = text.mid(line.textStart(), line.textLength());
        if (segment.endsWith(QChar(0x2028))) segment.chop(1);
        if (i) shortened += QChar(0x2028);
        shortened += i == count - 1 ? elideGraphemes(segment, font, width) : segment;
    }
    layout = make(shortened);
    layout->beginLayout();
    used = 0;
    int index = 0;
    while (true) {
        auto line = layout->createLine();
        if (!line.isValid()) break;
        line.setLineWidth(width);
        const qreal top = used + (index++ ? gap : 0);
        line.setPosition(QPointF(0, top));
        used = top + line.height();
    }
    layout->endLayout();
    if (used > height + .01 || index > limit) {
        if (count == 1) return {};
        return layoutMessage(text, font, format, width, height, count - 1, gap, alignment);
    }
    return {std::move(layout), used};
}

// 配置只在快照变化时解析，绘制和动画帧复用已解析的颜色与排版参数。
struct Appearance {
    Qt::Alignment alignment = Qt::AlignLeft;
    qreal historyScale = 1, latestScale = 1, lineSpacing = 3, messageSpacing = 8;
    qreal historyAlpha = .95, cornerRadius = 16, borderAlpha = .18, shadowStrength = .3;
    qreal animationDistance = 10;
    int maxMessages = 6, maxLines = 4, animationDurationMS = 320;
    bool gradient = false, animationEnabled = true;
    QColor accentStart = QColor("#FF4FA3"), accentEnd = QColor("#FF8FCB");

    Appearance() = default;
    explicit Appearance(const QJsonObject &value) {
        const QString align = value.value("alignment").toString("left");
        alignment = align == "right" ? Qt::AlignRight : align == "center" ? Qt::AlignHCenter : Qt::AlignLeft;
        historyScale = value.value("history_scale").toDouble(1);
        latestScale = value.value("latest_scale").toDouble(1);
        lineSpacing = value.value("line_spacing").toDouble(3);
        messageSpacing = value.value("message_spacing").toDouble(8);
        historyAlpha = value.value("history_alpha").toDouble(.95);
        maxMessages = value.value("max_messages").toInt(6);
        maxLines = value.value("max_lines").toInt(4);
        gradient = value.value("gradient").toBool(false);
        accentStart = color(value, "accent_start", "#FF4FA3");
        accentEnd = color(value, "accent_end", "#FF8FCB");
        cornerRadius = value.value("corner_radius").toDouble(16);
        borderAlpha = value.value("border_alpha").toDouble(.18);
        shadowStrength = value.value("shadow_strength").toDouble(.3);
        animationEnabled = value.value("animation_enabled").toBool(true);
        animationDurationMS = value.value("animation_duration_ms").toInt(320);
        animationDistance = value.value("animation_distance").toDouble(10);
    }
};

class OverlayWindow final : public QWidget {
public:
    explicit OverlayWindow(QScreen *target) : screen_(target), animation_(this) {
        setWindowFlags(Qt::Tool | Qt::FramelessWindowHint | Qt::WindowStaysOnTopHint | Qt::WindowDoesNotAcceptFocus);
        setAttribute(Qt::WA_TranslucentBackground);
        setAttribute(Qt::WA_ShowWithoutActivating);
        setWindowTitle(QStringLiteral("Arcana World · 精美浮层 / Elegant overlay"));
        winId();
        awqt::prepareWindow(this, target);
        animation_.setStartValue(0.0);
        animation_.setEndValue(1.0);
        animation_.setEasingCurve(QEasingCurve::OutCubic);
        connect(&animation_, &QVariantAnimation::valueChanged, this, [this](const QVariant &value) {
            progress_ = value.toReal();
            update();
        });
        connect(&animation_, &QVariantAnimation::finished, this, [this] {
            rows_.erase(std::remove_if(rows_.begin(), rows_.end(),
                [](const VisualRow &row) { return row.toOpacity == 0; }), rows_.end());
            progress_ = 1;
            update();
        });
    }

    void apply(const QJsonObject &config, bool unlocked, bool reduced, bool reposition) {
        const bool first = config_.isEmpty();
        const bool policyChanged = first || unlocked_ != unlocked ||
            config_.value("qt").toObject().value("show_in_taskbar").toBool() !=
                config.value("qt").toObject().value("show_in_taskbar").toBool();
        const bool textChanged = config_.value("text") != config.value("text") || config_.value("text_roles") != config.value("text_roles");
        const bool styleChanged = first || config_.value("qt") != config.value("qt");
        const Appearance nextStyle = styleChanged ? Appearance(config.value("qt").toObject()) : style_;
        const bool layoutStyleChanged = style_.historyScale != nextStyle.historyScale ||
            style_.latestScale != nextStyle.latestScale || style_.lineSpacing != nextStyle.lineSpacing ||
            style_.messageSpacing != nextStyle.messageSpacing || style_.maxLines != nextStyle.maxLines;
        const bool geometryChanged = first || config_.value("position") != config.value("position") ||
            config_.value("width") != config.value("width") || config_.value("height") != config.value("height");
        const bool appearanceChanged = first || textChanged || styleChanged || config_.value("font") != config.value("font") ||
            config_.value("padding") != config.value("padding") || config_.value("colors") != config.value("colors") ||
            config_.value("text_alpha") != config.value("text_alpha") || config_.value("outline") != config.value("outline");
        const bool preserveHeights = !geometryChanged && !reposition && !layoutStyleChanged &&
            config_.value("font") == config.value("font") &&
            config_.value("padding") == config.value("padding") && cachedDpr_ == devicePixelRatioF();
        config_ = config;
        style_ = nextStyle;
        if ((geometryChanged || reposition) && screen_) {
            frame_ = frame(config, screen_);
            awqt::placeWindow(this, screen_, frame_);
        }
        if (first || unlocked_ != unlocked) {
            unlocked_ = unlocked;
            setCursor(unlocked ? Qt::SizeAllCursor : Qt::ArrowCursor);
        }
        if (appearanceChanged || geometryChanged || reposition || cachedDpr_ != devicePixelRatioF()) {
            const bool animate = textChanged && !first && !reduced && !geometryChanged &&
                style_.animationEnabled && style_.animationDurationMS > 0 && !config.value("text").toString().isEmpty();
            animation_.stop();
            rebuildText(animate, preserveHeights);
            progress_ = animate ? 0 : 1;
            if (animate) {
                animation_.setDuration(style_.animationDurationMS);
                animation_.start();
            }
        }
        if (reduced && animation_.state() == QAbstractAnimation::Running) {
            animation_.stop();
            progress_ = 1;
        }
        const bool wasVisible = isVisible();
        if (policyChanged) applyWindowPolicy();
        show();
        // Qt 在显示窗口时可能重写原生样式，显示后恢复非激活与任务栏策略。
        if (!wasVisible) applyWindowPolicy();
        update();
    }

protected:
    bool event(QEvent *event) override {
        const auto type = event->type();
        const bool result = QWidget::event(event);
        if ((type == QEvent::WinIdChange || type == QEvent::Show || type == QEvent::WindowStateChange)
            && !config_.isEmpty() && !policyPending_) {
            policyPending_ = true;
            QTimer::singleShot(0, this, [this] {
                policyPending_ = false;
                applyWindowPolicy();
            });
        }
        return result;
    }

    void paintEvent(QPaintEvent *) override {
        QPainter painter(this);
        painter.setRenderHints(QPainter::Antialiasing | QPainter::TextAntialiasing);
        const qreal alpha = config_.value("background_alpha").toDouble(.35);
        const QRectF panel = QRectF(rect()).adjusted(5, 5, -5, -5);
        // 阴影只绘制窗口内部，透明边缘不依赖桌面截图或系统模糊权限。
        painter.setPen(Qt::NoPen);
        for (int i = 5; i > 0; --i) {
            painter.setBrush(QColor(0, 0, 0, qRound(alpha * style_.shadowStrength * (6 - i) * 3)));
            painter.drawRoundedRect(panel.adjusted(-i, -i + 2, i, i + 2), style_.cornerRadius + i, style_.cornerRadius + i);
        }
        QColor background = color(config_.value("colors").toObject(), "background", "#181C24");
        background.setAlphaF(alpha);
        painter.setBrush(background);
        painter.drawRoundedRect(panel, style_.cornerRadius, style_.cornerRadius);
        QLinearGradient edge(panel.topLeft(), panel.bottomRight());
        QColor edgeStart = style_.accentStart, edgeEnd = style_.accentEnd;
        edgeStart.setAlphaF(alpha * style_.borderAlpha);
        edgeEnd.setAlphaF(alpha * style_.borderAlpha * .35);
        edge.setColorAt(0, edgeStart);
        edge.setColorAt(1, edgeEnd);
        painter.setBrush(Qt::NoBrush);
        painter.setPen(QPen(QBrush(edge), 1));
        const qreal edgeRadius = std::max(0.0, style_.cornerRadius - .5);
        painter.drawRoundedRect(panel.adjusted(.5, .5, -.5, -.5), edgeRadius, edgeRadius);
        painter.save();
        painter.setClipRect(content_);
        for (const auto &row : rows_) {
            painter.setOpacity(row.fromOpacity + (row.toOpacity - row.fromOpacity) * progress_);
            painter.drawPixmap(QPointF(content_.left(), row.fromY + (row.toY - row.fromY) * progress_), row.pixmap);
        }
        painter.restore();
        if (unlocked_) {
            painter.setPen(QPen(style_.accentEnd, 2, Qt::SolidLine, Qt::RoundCap));
            painter.drawLine(QPointF(width() / 2.0 - 12, 9), QPointF(width() / 2.0 + 12, 9));
        }
    }

    void mousePressEvent(QMouseEvent *event) override {
        if (unlocked_ && event->button() == Qt::LeftButton) {
            dragging_ = true;
            dragOffset_ = QGuiApplication::platformName().startsWith(QStringLiteral("wayland"))
                ? event->position().toPoint() : event->globalPosition().toPoint() - pos();
            event->accept();
        }
    }
    void mouseMoveEvent(QMouseEvent *event) override {
        if (unlocked_ && dragging_ && screen_) {
            const bool wayland = QGuiApplication::platformName().startsWith(QStringLiteral("wayland"));
            QPoint point = wayland ? frame_.topLeft() + event->position().toPoint() - dragOffset_
                                   : event->globalPosition().toPoint() - dragOffset_;
            const QRect bounds = screen_->geometry();
            point.setX(std::clamp(point.x(), bounds.x(), bounds.x() + bounds.width() - width()));
            point.setY(std::clamp(point.y(), bounds.y(), bounds.y() + bounds.height() - height()));
            frame_.moveTopLeft(point);
            awqt::placeWindow(this, screen_, frame_);
        }
    }
    void mouseReleaseEvent(QMouseEvent *) override { dragging_ = false; }

private:
    struct LogicalRow {
        QString key;
        quint64 identity;
        qreal height = 0;
    };
    struct VisualRow {
        quint64 identity;
        QPixmap pixmap;
        qreal fromY, toY, fromOpacity, toOpacity;
    };

    void rebuildText(bool animate, bool preserveHeights) {
        auto previous = std::move(rows_);
        rows_.clear();
        // 逐条继承当前动画位置，共有历史消息只绘制一次，避免整幅交叉淡化的重影。
        for (auto &row : previous) {
            row.fromY += (row.toY - row.fromY) * progress_;
            row.fromOpacity += (row.toOpacity - row.fromOpacity) * progress_;
        }
        const auto padding = config_.value("padding").toObject();
        const qreal left = std::min(padding.value("left").toDouble(16), double(width()));
        const qreal top = std::min(padding.value("top").toDouble(12), double(height()));
        content_ = QRectF(left, top,
            std::max(0.0, width() - left - padding.value("right").toDouble(16)),
            std::max(0.0, height() - top - padding.value("bottom").toDouble(12)));
        cachedDpr_ = devicePixelRatioF();
        if (config_.value("text").toString().isEmpty()) {
            logicalRows_.clear();
            return;
        }
        const auto fontConfig = config_.value("font").toObject();
        QFont font = QFontDatabase::systemFont(QFontDatabase::GeneralFont);
        QStringList families;
        const auto chosen = fontConfig.value("family").toString();
        families << (chosen.isEmpty() ? font.family() : chosen)
                 << "PingFang SC" << "Microsoft YaHei" << "Noto Sans CJK SC"
                 << "Noto Sans CJK JP" << "Noto Sans CJK KR" << "Source Han Sans SC" << "sans-serif";
        font.setFamilies(families);
        // 配置单位是逻辑点，不随屏幕物理 DPI 二次放大。
        font.setPixelSize(std::max(1, qRound(fontConfig.value("size").toDouble(16))));
        font.setWeight(QFont::Weight(fontConfig.value("weight").toInt(500)));
        font.setItalic(fontConfig.value("italic").toBool());
        const QString text = config_.value("text").toString();
        const QString roles = config_.value("text_roles").toString();
        // 无角色标注的普通文本是一个多行块；上游弹幕与演示显式标注逐条角色。
        const auto lines = roles.isEmpty() ? QList<QStringView>{QStringView(text)} : QStringView(text).split(u'\n');
        std::vector<LogicalRow> next;
        next.reserve(size_t(lines.size()));
        for (qsizetype i = 0; i < lines.size(); ++i)
            next.push_back({lines.at(i).toString() + (i < roles.size() ? roles.at(i) : QChar('n')), 0});
        // 只延续完整逻辑快照的后缀与新快照前缀的重叠，不能按文字在可见行中任意查找。
        // 前缀表使大量相同文本的匹配仍为线性复杂度；重复的新事件获得独立身份。
        std::vector<size_t> prefix(next.size(), 0);
        for (size_t i = 1, matched = 0; i < next.size(); ++i) {
            while (matched && next[i].key != next[matched].key) matched = prefix[matched - 1];
            if (next[i].key == next[matched].key) ++matched;
            prefix[i] = matched;
        }
        size_t overlap = 0;
        for (const auto &old : logicalRows_) {
            while (overlap && (overlap == next.size() || old.key != next[overlap].key))
                overlap = prefix[overlap - 1];
            if (old.key == next[overlap].key) ++overlap;
        }
        for (size_t i = 0; i < next.size(); ++i) {
            if (i < overlap) {
                const auto &old = logicalRows_[logicalRows_.size() - overlap + i];
                next[i].identity = old.identity;
                if (preserveHeights) next[i].height = old.height;
            } else {
                next[i].identity = ++nextIdentity_;
            }
        }
        logicalRows_ = std::move(next);
        if (content_.isEmpty()) return;
        const auto colors = config_.value("colors").toObject();
        const qreal textAlpha = config_.value("text_alpha").toDouble(1);
        struct Row { std::unique_ptr<QTextLayout> layout; qreal height; quint64 identity; qreal opacity; };
        std::vector<Row> rows;
        qreal total = 0;
        // 快照已经包含近期历史；从底部布局可见内容，绝不再次追加。
        const qsizetype firstVisible = std::max(qsizetype(0), lines.size() - style_.maxMessages);
        for (qsizetype index = lines.size(); index > firstVisible && total < content_.height() + 40; --index) {
            const qsizetype i = index - 1;
            const bool latest = i == lines.size() - 1;
            QColor foreground = color(colors, "text", "#FFFFFF");
            const QChar role = i < roles.size() ? roles.at(i) : QChar('n');
            if (role == 'c') foreground = color(colors, "captain", "#E88C8C");
            else if (role == 'a') foreground = color(colors, "admiral", "#E88C8C");
            else if (role == 'g') foreground = color(colors, "governor", "#E88C8C");
            else if (role == 's') foreground = color(colors, "super_chat", "#E6C77A");
            const qreal opacity = textAlpha * (latest ? 1.0 : style_.historyAlpha);
            QFont rowFont = font;
            rowFont.setPixelSize(std::max(1, qRound(font.pixelSize() * (latest ? style_.latestScale : style_.historyScale))));
            QTextCharFormat format;
            format.setForeground(foreground);
            if (latest && style_.gradient && role == 'n' && foreground.rgb() == QColor("#FFFFFF").rgb()) {
                // 默认普通文本使用玫瑰渐变；自定义文字颜色与角色颜色保持原样。
                QLinearGradient accent(0, 0, content_.width(), 0);
                QColor rose = style_.accentStart, pink = style_.accentEnd;
                accent.setColorAt(0, rose);
                accent.setColorAt(1, pink);
                format.setForeground(QBrush(accent));
            }
            const qreal spacing = latest ? 0 : style_.messageSpacing;
            auto measured = layoutMessage(lines.at(i).toString(), rowFont, format, content_.width(),
                content_.height() - spacing, style_.maxLines, style_.lineSpacing, style_.alignment);
            if (!measured.layout) continue;
            qreal y = measured.height + spacing;
            // 最新行变为小字号历史时保留其占位高度，换行收缩不能把更早消息向下推。
            auto &logical = logicalRows_[size_t(i)];
            logical.height = std::max(logical.height, y);
            y = logical.height;
            total += y;
            rows.push_back({std::move(measured.layout), y, logical.identity, opacity});
        }
        // 底部锚定使追加消息只会向上推动保留历史，与顶部裁切多少行无关。
        qreal y = content_.bottom() - total;
        for (auto it = rows.rbegin(); it != rows.rend(); ++it) {
            // 稳态只显示完整消息块，顶部空间不足的旧消息淡出退出，不留下半行。
            if (y < content_.top() - .01) {
                y += it->height;
                continue;
            }
            const qreal pixelHeight = std::min(it->height, content_.height());
            QPixmap image(qCeil(content_.width() * cachedDpr_), qCeil(pixelHeight * cachedDpr_));
            image.setDevicePixelRatio(cachedDpr_);
            image.fill(Qt::transparent);
            QPainter painter(&image);
            painter.setRenderHints(QPainter::Antialiasing | QPainter::TextAntialiasing);
            if (config_.value("outline").toBool(true)) {
                // 独立描边层保留正常填充和彩色表情字形，不让描边格式吞掉填充。
                auto outline = it->layout->formats();
                for (auto &range : outline) {
                    range.format.setForeground(Qt::transparent);
                    range.format.setTextOutline(QPen(QColor(0, 0, 0, 110), .7));
                }
                it->layout->draw(&painter, QPointF(), outline);
            }
            it->layout->draw(&painter, QPointF());
            painter.end();
            VisualRow visual{it->identity, std::move(image), animate ? y + style_.animationDistance : y,
                y, animate ? 0.0 : it->opacity, it->opacity};
            if (animate) {
                auto old = std::find_if(previous.begin(), previous.end(),
                    [&visual](const VisualRow &row) { return row.identity == visual.identity && row.toOpacity != 0; });
                if (old != previous.end()) {
                    visual.fromY = old->fromY;
                    visual.fromOpacity = old->fromOpacity;
                    previous.erase(old);
                }
            }
            rows_.push_back(std::move(visual));
            y += it->height;
        }
        if (animate) {
            for (auto &old : previous) {
                old.toY = old.fromY - style_.animationDistance;
                old.toOpacity = 0;
                rows_.push_back(std::move(old));
            }
        }
        // 动画只复用光栅化字形缓存；没有词级时间数据，因此不模拟卡拉 OK 扫光。
    }

    QPointer<QScreen> screen_;
    QJsonObject config_;
    Appearance style_;
    QVariantAnimation animation_;
    std::vector<VisualRow> rows_;
    std::vector<LogicalRow> logicalRows_;
    quint64 nextIdentity_ = 0;
    QRectF content_;
    qreal progress_ = 1, cachedDpr_ = 0;
    void applyWindowPolicy() {
        const QString error = awqt::configureWindow(this, !unlocked_,
            config_.value("qt").toObject().value("show_in_taskbar").toBool());
        if (!error.isEmpty()) failPolicy(error);
    }
    bool policyPending_ = false;
    bool unlocked_ = false, dragging_ = false;
    QPoint dragOffset_;
    QRect frame_;
};

class Controller final : public QObject {
public:
    explicit Controller(QJsonObject config) : config_(std::move(config)) {
#if defined(Q_OS_WIN)
        const QString error = resolveDisplaySelection(config_, resolvedDisplayId_);
        if (!error.isEmpty()) { failPolicy(error); return; }
#endif
        for (auto *screen : QGuiApplication::screens()) watch(screen);
        connect(qApp, &QGuiApplication::screenAdded, this, [this](QScreen *screen) { watch(screen); sync(true); });
        connect(qApp, &QGuiApplication::screenRemoved, this, [this](QScreen *screen) {
            windows_.erase(screen);
            sync(true);
        });
        connect(qApp, &QGuiApplication::primaryScreenChanged, this, [this] { sync(true); });
        sync(true);
        // 直接抓取真实 QWidget，自带透明通道，不读取桌面也不请求录屏权限。
        const QString capture = qEnvironmentVariable("ARCANA_OVERLAY_CAPTURE");
        if (!capture.isEmpty()) QTimer::singleShot(14000, this, [this, capture] { captureWidget(capture); });
        const QString directory = qEnvironmentVariable("ARCANA_OVERLAY_CAPTURE_DIR");
        if (!directory.isEmpty()) {
            if (!QDir().mkpath(directory)) fail(QStringLiteral("Elegant overlay: cannot create capture directory: ") + directory);
            else {
                captureTimeline_.setFileName(QDir(directory).filePath(QStringLiteral("timestamps.jsonl")));
                if (!captureTimeline_.open(QIODevice::WriteOnly | QIODevice::Truncate)) {
                    fail(QStringLiteral("Elegant overlay: cannot write capture timestamps"));
                    return;
                }
                captureClock_.start();
                auto *timer = new QTimer(this);
                timer->setTimerType(Qt::PreciseTimer);
                connect(timer, &QTimer::timeout, this, [this, directory] {
                    const QString file = QStringLiteral("frame-%1.png").arg(captureFrame_, 6, 10, QLatin1Char('0'));
                    const qint64 time = captureClock_.nsecsElapsed();
                    if (captureWidget(QDir(directory).filePath(file))) {
                        // 记录实际抓取时刻，编码时保留写 PNG 和显示休眠造成的可变采样间隔。
                        const auto record = QJsonDocument(QJsonObject{{"file", file}, {"time_ns", time}}).toJson(QJsonDocument::Compact) + '\n';
                        if (captureTimeline_.write(record) != record.size())
                            fail(QStringLiteral("Elegant overlay: cannot write capture timestamps"));
                        ++captureFrame_;
                    }
                });
                timer->start(33);
            }
        }
    }

    void apply(QJsonObject config) {
#if defined(Q_OS_WIN)
        if (config.value("displays") != config_.value("displays") ||
            config.value("output") != config_.value("output") ||
            config.value("display_id") != config_.value("display_id")) {
            QString resolvedDisplayId;
            const QString error = resolveDisplaySelection(config, resolvedDisplayId);
            if (!error.isEmpty()) { failPolicy(error); return; }
            resolvedDisplayId_ = std::move(resolvedDisplayId);
        }
#endif
        config_ = std::move(config);
        sync(false);
    }
private:
    void watch(QScreen *screen) {
        connect(screen, &QScreen::geometryChanged, this, [this] { sync(true); });
        connect(screen, &QScreen::logicalDotsPerInchChanged, this, [this] { sync(true); });
        connect(screen, &QScreen::physicalDotsPerInchChanged, this, [this] { sync(true); });
    }
    void sync(bool reposition) {
        const bool unlocked = config_.value("unlocked").toBool();
        const bool reduced = config_.value("reduced_motion").toBool() ||
            !config_.value("qt").toObject().value("animation_enabled").toBool(true);
        for (auto *screen : QGuiApplication::screens()) {
#if defined(Q_OS_WIN)
            if (!selected(config_, screen, resolvedDisplayId_)) { windows_.erase(screen); continue; }
#else
            if (!selected(config_, screen)) { windows_.erase(screen); continue; }
#endif
            auto &window = windows_[screen];
            if (!window) window = std::make_unique<OverlayWindow>(screen);
            window->apply(config_, unlocked, reduced, reposition);
        }
        // 即使所有显示器均关闭也同步应用级策略，避免留下空的 Dock 项。
        const QString error = awqt::setTaskbarVisibility(!windows_.empty() &&
            config_.value("qt").toObject().value("show_in_taskbar").toBool());
        if (!error.isEmpty()) failPolicy(error);
    }
    bool captureWidget(const QString &path) {
        for (const auto &entry : windows_) {
            if (!entry.second->isVisible()) continue;
            if (entry.second->grab().save(path, "PNG")) return true;
            fail(QStringLiteral("Elegant overlay: cannot save widget capture: ") + path);
            return false;
        }
        fail(QStringLiteral("Elegant overlay: no visible widget to capture"));
        return false;
    }
    QJsonObject config_;
#if defined(Q_OS_WIN)
    QString resolvedDisplayId_;
#endif
    std::map<QScreen *, std::unique_ptr<OverlayWindow>> windows_;
    quint64 captureFrame_ = 0;
    QElapsedTimer captureClock_;
    QFile captureTimeline_;
};
Controller *controller = nullptr;

bool ensureApplication() {
    if (finished) { fail(QStringLiteral("Elegant overlay: this process has already completed its event loop")); return false; }
    if (application) {
        if (QThread::currentThread() != application->thread()) {
            fail(QStringLiteral("Elegant overlay: create/run/displays require the application main thread"));
            return false;
        }
        return true;
    }
    if (QCoreApplication::instance()) { fail(QStringLiteral("Elegant overlay: an external application already exists")); return false; }
#if defined(Q_OS_MACOS)
    // 禁止 Qt 转换前台进程和强制激活；Qt 初始化前后均显式设置辅助应用策略。
    if (!qputenv("QT_MAC_DISABLE_FOREGROUND_APPLICATION_TRANSFORM", "1")) {
        fail(QStringLiteral("Elegant overlay: cannot disable Qt foreground application activation."));
        return false;
    }
    const QString preparationError = awqt::prepareApplication();
    if (!preparationError.isEmpty()) {
        fail(preparationError);
        return false;
    }
#endif
    application = new QApplication(argcValue, argvValue);
    application->setQuitOnLastWindowClosed(false);
    application->setApplicationName(QStringLiteral("Arcana World Overlay"));
    const QString policyError = awqt::setTaskbarVisibility(false);
    const QString limitation = policyError.isEmpty() ? awqt::platformLimitation() : policyError;
    if (!limitation.isEmpty()) {
        fail(limitation);
        delete application;
        application = nullptr;
        finished = true;
        return false;
    }
    return true;
}
} // namespace

extern "C" int awqt_create(const char *json) {
    std::lock_guard<std::mutex> lock(lifecycleMutex);
    QJsonObject config;
    if (!parse(json, config) || !ensureApplication()) return 0;
    if (controller || running) { fail(QStringLiteral("Elegant overlay: already created")); return 0; }
    controller = new Controller(std::move(config));
    // ready 之前交付首次绘制；不读取桌面，不启动额外渲染循环。
    QApplication::processEvents(QEventLoop::ExcludeUserInputEvents);
    for (auto *widget : QApplication::topLevelWidgets())
        if (widget->isVisible()) widget->repaint();
    if (!policyFailure.isEmpty()) {
        fail(policyFailure);
        delete controller;
        controller = nullptr;
        delete application;
        application = nullptr;
        finished = true;
        return 0;
    }
    return 1;
}

extern "C" int awqt_run(void) {
    {
        std::lock_guard<std::mutex> lock(lifecycleMutex);
        if (!application || !controller || running || QThread::currentThread() != application->thread()) {
            fail(QStringLiteral("Elegant overlay: run requires a created overlay on its main thread"));
            return 0;
        }
        running = true;
    }
    const int status = application->exec();
    {
        std::lock_guard<std::mutex> lock(lifecycleMutex);
        running = false;
        finished = true;
        delete controller;
        controller = nullptr;
        delete application;
        application = nullptr;
    }
    if (!policyFailure.isEmpty()) {
        fail(policyFailure);
        return 0;
    }
    if (status != 0) fail(QStringLiteral("Elegant overlay: event loop exited with status %1").arg(status));
    return status == 0 ? 1 : 0;
}

extern "C" void awqt_update(const char *json) {
    QJsonObject config;
    if (!parse(json, config)) return;
    std::lock_guard<std::mutex> lock(lifecycleMutex);
    if (!controller || finished) { fail(QStringLiteral("Elegant overlay: update requires a live overlay")); return; }
    Controller *target = controller;
    QMetaObject::invokeMethod(target, [target, config = std::move(config)]() mutable { target->apply(std::move(config)); }, Qt::QueuedConnection);
}

extern "C" void awqt_stop(void) {
    std::lock_guard<std::mutex> lock(lifecycleMutex);
    if (application && !finished) QMetaObject::invokeMethod(application, [] { QApplication::quit(); }, Qt::QueuedConnection);
}

extern "C" char *awqt_displays(const char *json) {
    std::lock_guard<std::mutex> lock(lifecycleMutex);
    QJsonObject config;
    if (!parse(json, config) || !ensureApplication()) return nullptr;
    QString resolvedDisplayId;
#if defined(Q_OS_WIN)
    const QString error = resolveDisplaySelection(config, resolvedDisplayId);
    if (!error.isEmpty()) { fail(error); return nullptr; }
#endif
    QJsonArray displays;
    for (auto *screen : QGuiApplication::screens()) {
        displays.append(QJsonObject{{"id", awqt::screenId(screen)}, {"name", screen->name()}, {"selected", selected(config, screen, resolvedDisplayId)}});
    }
    const QByteArray encoded = QJsonDocument(displays).toJson(QJsonDocument::Compact);
    auto *result = static_cast<char *>(std::malloc(size_t(encoded.size()) + 1));
    if (!result) { fail(QStringLiteral("Elegant overlay: cannot allocate display list")); return nullptr; }
    std::memcpy(result, encoded.constData(), size_t(encoded.size()));
    result[encoded.size()] = '\0';
    return result;
}

extern "C" void awqt_free(char *value) { std::free(value); }
extern "C" const char *awqt_error(void) {
    thread_local QByteArray snapshot;
    std::lock_guard<std::mutex> lock(errorMutex);
    snapshot = lastError;
    return snapshot.constData();
}
