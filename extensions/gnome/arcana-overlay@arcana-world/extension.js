import Gio from 'gi://Gio';
import GLib from 'gi://GLib';
import {Extension} from 'resource:///org/gnome/shell/extensions/extension.js';
import * as Main from 'resource:///org/gnome/shell/ui/main.js';

const NAME = 'org.arcana.World.ElegantOverlay';
const PATH = '/org/arcana/World/ElegantOverlay';
const TITLE = 'Arcana Elegant ';
const TIMEOUT_MS = 5000;
const MAX_WINDOWS = 128;
const XML = `<node><interface name="${NAME}">
<method name="Begin"><arg type="u" direction="in" name="version"/></method>
<method name="Place">
<arg type="s" direction="in" name="token"/>
<arg type="s" direction="in" name="connector"/>
<arg type="i" direction="in" name="x"/><arg type="i" direction="in" name="y"/>
<arg type="b" direction="in" name="locked"/>
</method>
<method name="Remove"><arg type="s" direction="in" name="token"/></method>
<method name="End"/>
</interface></node>`;

function requireValue(condition, message) {
    if (!condition)
        throw new Error(message);
}

function validateToken(token) {
    requireValue(typeof token === 'string' &&
        /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(token),
    'Invalid window token');
}

export default class ArcanaOverlay extends Extension {
    enable() {
        this._enabled = true;
        this._owner = null;
        this._pid = 0;
        this._placements = new Map();
        this._windows = new Map();
        this._sessionSignal = Main.sessionMode.connect('updated', () => this._syncSession());
        this._syncSession();
    }

    _syncSession() {
        const userSession = Main.sessionMode.currentMode === 'user' ||
            Main.sessionMode.parentMode === 'user';
        if (Main.sessionMode.isLocked || !userSession) {
            this._stop('Session locked');
        } else if (this._enabled && !this._busObject) {
            this._connection = Gio.DBus.session;
            this._busObject = Gio.DBusExportedObject.wrapJSObject(XML, this);
            this._busObject.export(this._connection, PATH);
            this._subscription = this._connection.signal_subscribe('org.freedesktop.DBus',
                'org.freedesktop.DBus', 'NameOwnerChanged', '/org/freedesktop/DBus', null,
                Gio.DBusSignalFlags.NONE, (_c, _s, _p, _i, _m, parameters) => {
                    const [name, , owner] = parameters.deep_unpack();
                    if (name === this._owner && owner === '')
                        this._clear('Owner disconnected');
                });
            this._nameId = Gio.bus_own_name_on_connection(this._connection, NAME,
                Gio.BusNameOwnerFlags.NONE, null, () => this._stop('Service name lost'));
        }
    }

    _error(invocation, message) {
        invocation.return_dbus_error(`${NAME}.Error`, message);
    }

    _reply(invocation) {
        invocation.return_value(new GLib.Variant('()', []));
    }

    _authorize(invocation) {
        requireValue(this._owner === invocation.get_sender() && this._pid > 0,
            'Begin must authenticate this caller first');
    }

    BeginAsync([version], invocation) {
        try {
            requireValue(version === 1, 'Unsupported protocol version');
            requireValue(!this._owner, 'An overlay owner is already active');
            this._owner = invocation.get_sender();
            this._rememberFocus(global.display.focus_window);
            const pending = {invocation, cancellable: new Gio.Cancellable()};
            this._begin = pending;
            this._connection.call('org.freedesktop.DBus', '/org/freedesktop/DBus',
                'org.freedesktop.DBus', 'GetConnectionUnixProcessID',
                new GLib.Variant('(s)', [this._owner]), new GLib.VariantType('(u)'),
                Gio.DBusCallFlags.NONE, TIMEOUT_MS, pending.cancellable, (connection, result) => {
                    try {
                        const [pid] = connection.call_finish(result).deep_unpack();
                        if (this._begin !== pending)
                            return;
                        requireValue(pid > 0, 'Owner has no Unix process ID');
                        this._pid = pid;
                        this._startWatching();
                        this._begin = null;
                        this._reply(invocation);
                    } catch (error) {
                        if (this._begin === pending)
                            this._clear(`Owner authentication failed: ${error.message}`);
                    }
                });
        } catch (error) {
            this._error(invocation, error.message);
        }
    }

    PlaceAsync([token, connector, x, y, locked], invocation) {
        try {
            this._authorize(invocation);
            validateToken(token);
            requireValue(typeof connector === 'string' && connector.length > 0 &&
                connector.length <= 256 && !/[\x00-\x1f\x7f]/.test(connector), 'Invalid connector');
            // 尺寸由 Qt 按自身缩放决定，Shell 只负责位置，避免两侧坐标单位不一致。
            requireValue([x, y].every(Number.isInteger) &&
                Math.abs(x) <= 1048576 && Math.abs(y) <= 1048576, 'Invalid position');
            requireValue(typeof locked === 'boolean', 'Invalid locked state');
            const monitor = this._monitor(connector);
            let placement = this._placements.get(token);
            requireValue(!placement?.invocation, 'Placement is already pending');
            requireValue(placement || this._placements.size < MAX_WINDOWS, 'Too many overlay windows');
            if (!placement) {
                placement = {token, window: null, original: null, source: 0};
                this._placements.set(token, placement);
            }
            Object.assign(placement, {connector, monitor, x, y, locked,
                invocation, deadline: GLib.get_monotonic_time() + TIMEOUT_MS * 1000});
            this._syncFocus();
            // Qt 自己设置输入区域；Shell 不替换 actor，也不改变点击穿透策略。
            this._advance(placement);
            if (placement.invocation) {
                placement.source = GLib.timeout_add(GLib.PRIORITY_DEFAULT, TIMEOUT_MS, () => {
                    placement.source = 0;
                    this._remove(placement, 'Window placement timed out');
                    return GLib.SOURCE_REMOVE;
                });
            }
        } catch (error) {
            this._error(invocation, error.message);
        }
    }

    RemoveAsync([token], invocation) {
        try {
            this._authorize(invocation);
            validateToken(token);
            const placement = this._placements.get(token);
            if (placement)
                this._remove(placement, 'Window removed');
            this._reply(invocation);
        } catch (error) {
            this._error(invocation, error.message);
        }
    }

    EndAsync(_args, invocation) {
        try {
            this._authorize(invocation);
            this._clear('Session ended');
            this._reply(invocation);
        } catch (error) {
            this._error(invocation, error.message);
        }
    }

    _monitor(connector) {
        const monitor = global.backend.get_monitor_manager().get_monitor_for_connector(connector);
        requireValue(monitor >= 0, `Monitor is unavailable: ${connector}`);
        return monitor;
    }

    _startWatching() {
        this._focusSignal = global.display.connect('notify::focus-window', () => this._syncFocus());
        this._createdSignal = global.display.connect('window-created', (_display, window) =>
            this._watchWindow(window));
        this._restackedSignal = global.display.connect('restacked', () => this._raiseActors());
        this._mapSignal = global.window_manager.connect('map', (_manager, actor) => {
            this._watchWindow(actor.meta_window);
            this._watchActor(actor.meta_window);
            this._queueAdvance();
        });
        this._monitorSignal = Main.layoutManager.connect('monitors-changed', () => {
            // Qt 按 QScreen 生命周期移除、重建窗口；拔出显示器不终止整个会话。
            for (const placement of this._placements.values()) {
                const monitor = global.backend.get_monitor_manager()
                    .get_monitor_for_connector(placement.connector);
                if (monitor < 0)
                    continue;
                placement.monitor = monitor;
                if (placement.window)
                    placement.window.move_to_monitor(monitor);
            }
        });
        for (const actor of global.get_window_actors())
            this._watchWindow(actor.meta_window);
        this._syncFocus();
    }

    _rememberFocus(window) {
        if (this._previousFocus === window)
            return;
        if (this._previousFocus && this._previousFocusSignal)
            this._previousFocus.disconnect(this._previousFocusSignal);
        this._previousFocus = window;
        this._previousFocusSignal = window ? window.connect('unmanaged', () => {
            this._previousFocus = null;
            this._previousFocusSignal = 0;
        }) : 0;
    }

    _syncFocus() {
        const focused = global.display.focus_window;
        if (!focused)
            return;
        const title = focused.get_title() ?? '';
        const token = title.startsWith(TITLE) ? title.slice(TITLE.length) : '';
        const owned = focused.get_pid() === this._pid &&
            /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(token);
        if (!owned) {
            // 创建初期的默认标题不能覆盖最近的其他应用焦点。
            if (focused.get_pid() !== this._pid)
                this._rememberFocus(focused);
            return;
        }
        // Place 可晚于窗口映射；身份已匹配而策略尚未到达时，默认不接受焦点。
        if ((this._placements.get(token)?.locked ?? true) && this._previousFocus)
            this._previousFocus.focus(global.get_current_time());
    }

    _watchWindow(window) {
        if (this._windows.has(window) || window.get_pid() !== this._pid)
            return;
        const title = window.connect('notify::title', () => {
            this._syncFocus();
            for (const placement of this._placements.values()) {
                if (placement.window === window && !this._matches(window, placement.token))
                    this._remove(placement, 'Window title changed');
                else if (placement.invocation)
                    this._queueAdvance();
            }
        });
        const unmanaged = window.connect('unmanaged', () => {
            for (const placement of this._placements.values()) {
                if (placement.window === window) {
                    placement.window = null;
                    this._remove(placement, 'Window was destroyed');
                }
            }
            this._unwatchWindow(window);
        });
        this._windows.set(window, {
            signals: [title, unmanaged],
            actor: null, actorSignals: [],
        });
        this._watchActor(window);
        this._queueAdvance();
    }

    _watchActor(window) {
        const watched = this._windows.get(window);
        const actor = window.get_compositor_private();
        if (!watched || !actor || watched.actor === actor)
            return;
        if (watched.actor) {
            for (const signal of watched.actorSignals)
                watched.actor.disconnect(signal);
        }
        watched.actor = actor;
        watched.actorSignals = [
            actor.connect('notify::mapped', () => this._queueAdvance()),
            actor.connect('first-frame', () => this._queueAdvance()),
        ];
    }

    _queueAdvance() {
        if (this._advanceSource)
            return;
        this._advanceSource = GLib.idle_add(GLib.PRIORITY_DEFAULT_IDLE, () => {
            this._advanceSource = 0;
            for (const window of this._windows.keys())
                this._watchActor(window);
            for (const placement of this._placements.values())
                this._advance(placement);
            return GLib.SOURCE_REMOVE;
        });
    }

    _unwatchWindow(window) {
        const watched = this._windows.get(window);
        if (watched?.actor) {
            for (const signal of watched.actorSignals)
                watched.actor.disconnect(signal);
        }
        for (const id of watched?.signals ?? [])
            window.disconnect(id);
        this._windows.delete(window);
    }

    _matches(window, token) {
        return window.get_pid() === this._pid && window.get_title() === TITLE + token;
    }

    _advance(placement) {
        if (!placement.invocation)
            return;
        try {
            requireValue(GLib.get_monotonic_time() < placement.deadline, 'Window placement timed out');
            if (!placement.window) {
                const matches = [...this._windows.keys()].filter(window => this._matches(window, placement.token));
                requireValue(matches.length <= 1, 'Ambiguous window token');
                if (!matches.length)
                    return;
                placement.window = matches[0];
            }
            const window = placement.window;
            requireValue(this._matches(window, placement.token), 'Window identity changed');
            const actor = window.get_compositor_private();
            // 概览通过克隆显示窗口，原 actor 可未 mapped；真实缓冲区才是就绪依据。
            if (!actor?.get_texture()?.get_texture())
                return;
            if (!placement.original) {
                placement.original = {
                    above: window.is_above(), sticky: window.is_on_all_workspaces(),
                    frame: window.get_frame_rect(), monitor: window.get_monitor(),
                    nextActor: actor.get_next_sibling(),
                };
                window.make_above();
                window.stick();
            }
            // Mutter 约束可能调整最终位置（如避开顶栏）；接受合成器结果，不把约束视为失败。
            window.move_frame(false, placement.x, placement.y);
            this._raiseActors();
            const invocation = placement.invocation;
            if (placement.source) {
                GLib.Source.remove(placement.source);
                placement.source = 0;
            }
            placement.invocation = null;
            this._reply(invocation);
        } catch (error) {
            this._remove(placement, error.message);
        }
    }

    _raiseActors() {
        // 保留真实窗口 actor 和原有父节点，仅恢复全屏窗口之上的视觉顺序。
        for (const placement of this._placements.values()) {
            const actor = placement.window?.get_compositor_private();
            if (placement.original && actor?.mapped)
                actor.get_parent()?.set_child_above_sibling(actor, null);
        }
    }

    _remove(placement, reason) {
        this._placements.delete(placement.token);
        if (placement.source) {
            GLib.Source.remove(placement.source);
            placement.source = 0;
        }
        if (placement.invocation) {
            this._error(placement.invocation, reason);
            placement.invocation = null;
        }
        const window = placement.window;
        const original = placement.original;
        if (window && original) {
            try {
                if (!original.above)
                    window.unmake_above();
                if (!original.sticky)
                    window.unstick();
                if (original.monitor >= 0 && original.monitor < global.display.get_n_monitors())
                    window.move_to_monitor(original.monitor);
                const frame = original.frame;
                window.move_frame(false, frame.x, frame.y);
                const actor = window.get_compositor_private();
                const parent = actor?.get_parent();
                if (parent && original.nextActor?.get_parent() === parent)
                    parent.set_child_below_sibling(actor, original.nextActor);
            } catch (error) {
                console.warn(`Arcana window restore failed: ${error.message}`);
            }
        }
    }

    _clear(reason) {
        if (this._focusSignal) {
            global.display.disconnect(this._focusSignal);
            this._focusSignal = 0;
        }
        this._rememberFocus(null);
        if (this._advanceSource) {
            GLib.Source.remove(this._advanceSource);
            this._advanceSource = 0;
        }
        if (this._mapSignal) {
            global.window_manager.disconnect(this._mapSignal);
            this._mapSignal = 0;
        }
        if (this._begin) {
            const pending = this._begin;
            this._begin = null;
            pending.cancellable.cancel();
            this._error(pending.invocation, reason);
        }
        for (const window of this._windows.keys())
            this._unwatchWindow(window);
        for (const placement of this._placements.values())
            this._remove(placement, reason);
        for (const key of ['_createdSignal', '_restackedSignal']) {
            if (this[key]) {
                global.display.disconnect(this[key]);
                this[key] = 0;
            }
        }
        if (this._monitorSignal) {
            Main.layoutManager.disconnect(this._monitorSignal);
            this._monitorSignal = 0;
        }
        this._owner = null;
        this._pid = 0;
    }

    _stop(reason) {
        this._clear(reason);
        if (this._subscription) {
            this._connection.signal_unsubscribe(this._subscription);
            this._subscription = 0;
        }
        if (this._busObject) {
            this._busObject.unexport();
            this._busObject = null;
        }
        if (this._nameId) {
            const id = this._nameId;
            this._nameId = 0;
            Gio.bus_unown_name(id);
        }
    }

    disable() {
        this._enabled = false;
        Main.sessionMode.disconnect(this._sessionSignal);
        this._sessionSignal = 0;
        this._stop('Extension disabled');
    }
}
