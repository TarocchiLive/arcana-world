"use strict";
(() => {
  const $ = (id) => document.getElementById(id);
  const node = (tag, cls, text) => {
    const n = document.createElement(tag);
    if (cls) n.className = cls;
    if (text != null) n.textContent = text;
    return n;
  };
  let language = navigator.language.startsWith("zh") ? "zh" : "en";
  const t = (en, zh) => (language === "zh" ? zh : en);
  const routes = ["studio", "chat", "outputs", "accounts", "settings", "help"];
  const labels = () => ({
    studio: t("Studio", "直播台"),
    chat: t("Chat", "弹幕"),
    outputs: t("Outputs", "输出"),
    accounts: t("Accounts", "账号"),
    settings: t("Settings", "设置"),
    help: t("Help", "帮助"),
  });
  let page = "studio",
    state = null,
    timer,
    controller,
    generation = 0,
    sending = false,
    fetching = false;
  let pageKey = "",
    tab = "",
    stale = false,
    commandError = "",
    notificationKey = "",
    chatDraft = "",
    pendingSend = null;
  let pausedChat = false,
    pausedLogs = false,
    feedSignature = "",
    logsSignature = "",
    interactionKey = "",
    interactionRevision = 0,
    focusReturn = null,
    chatFocusReturn = null,
    chatFocusMessage = "",
    commandFocus = null;
  let helpQuery = "",
    helpTopic = "",
    helpSignature = "";
  const drafts = new Map(),
    interactionDrafts = new Map(),
    fieldNodes = new Map();
  function storage(key, value) {
    try {
      if (value === undefined) return localStorage.getItem(key);
      localStorage.setItem(key, value);
    } catch (_) {}
    return "";
  }
  function safeURL(value, image = false) {
    if (!value) return "";
    if (
      image &&
      /^data:image\/(png|jpeg|webp);base64,[a-z0-9+/=\s]+$/i.test(value)
    )
      return value;
    try {
      const u = new URL(value, location.href);
      return ["http:", "https:"].includes(u.protocol) ? u.href : "";
    } catch (_) {
      return "";
    }
  }
  function button(text, action, cls = "", value = "") {
    const b = node("button", cls, text);
    b.type = "button";
    if (action) {
      b.dataset.command = action;
      b.onclick = () => command(action, value);
    }
    return b;
  }
  function actionRow(...buttons) {
    const row = node("div", "actions");
    row.append(...buttons);
    return row;
  }
  function heading(title, actions) {
    const h = node("div", "page-heading");
    h.append(node("h1", "", title));
    if (actions) h.append(actions);
    return h;
  }
  function note(text) {
    return node("p", "note", text);
  }
  function setText(id, text) {
    const n = $(id);
    if (n && n.textContent !== text) n.textContent = text;
  }
  function localize() {
    document.documentElement.lang = language;
    $("navigation").replaceChildren(
      ...routes.map((id) => {
        const a = node("a", "", labels()[id]);
        a.href = "#" + id;
        a.onclick = (e) => {
          e.preventDefault();
          navigate(id);
        };
        if (id === page) a.setAttribute("aria-current", "page");
        return a;
      }),
    );
    $("navigation").setAttribute("aria-label", t("Workspace", "工作空间"));
    setText("retry", t("Refresh connection", "刷新连接"));
    document.querySelector(".skip").textContent = t(
      "Skip to content",
      "跳至主要内容",
    );
    const theme = storage("arcana-theme") || "system";
    $("theme").replaceChildren(
      ...[
        ["system", t("System theme", "跟随系统")],
        ["light", t("Light", "浅色")],
        ["dark", t("Dark", "深色")],
      ].map(([id, label]) => {
        const o = node("option", "", label);
        o.value = id;
        return o;
      }),
    );
    $("theme").value = theme;
    $("theme").setAttribute("aria-label", t("Appearance", "外观"));
    document.documentElement.dataset.theme = theme;
  }
  function schedule() {
    clearTimeout(timer);
    if (!document.hidden) timer = setTimeout(poll, 1000);
  }
  function renderConnection() {
    $("main").replaceChildren(
      heading(labels()[page]),
      note(
        stale
          ? t(
              "Cannot reach Arcana World. Check that it is running, then refresh the connection. Retrying automatically.",
              "无法连接 Arcana World。请确认程序正在运行，然后刷新连接。页面也会自动重试。",
            )
          : t("Loading studio…", "正在加载工作台…"),
      ),
    );
    renderStatus();
  }
  async function request(path, body, signal) {
    const response = await fetch(path, {
      method: body ? "POST" : "GET",
      headers: {
        ...(body ? { "Content-Type": "application/json" } : {}),
      },
      body: body ? JSON.stringify(body) : undefined,
      cache: "no-store",
      credentials: "omit",
      redirect: "error",
      signal,
    });
    const data = await response.json();
    if (!response.ok) {
      const e = new Error(data.error || t("Request failed", "请求失败"));
      e.status = response.status;
      throw e;
    }
    return data;
  }
  async function poll() {
    clearTimeout(timer);
    if (fetching || sending || document.hidden) return;
    fetching = true;
    $("retry").disabled = true;
    const current = generation;
    const own = new AbortController();
    controller = own;
    const deadline = setTimeout(() => own.abort(), 15000);
    try {
      const next = await request("/api/state?page=" + page, null, own.signal);
      if (current === generation) accept(next);
    } catch (e) {
      if (current !== generation) return;
      stale = true;
      hideChatMenu();
      commandError =
        e.name === "AbortError"
          ? state
            ? t(
                "Connection timed out. Last known state is shown.",
                "连接超时，当前显示上次已知状态。",
              )
            : t("Connection timed out.", "连接超时。")
          : e.message;
      if (state) renderStatus();
      else renderConnection();
    } finally {
      clearTimeout(deadline);
      fetching = false;
      if (controller === own) controller = null;
      $("retry").disabled = false;
      if (current !== generation && !sending && !document.hidden)
        queueMicrotask(poll);
      else schedule();
    }
  }
  async function command(action, value = "", fromInteraction = false) {
    if (sending || !state || (state.busy && action !== "cancel")) return;
    if (!state.interaction) commandFocus = document.activeElement;
    hideChatMenu();
    sending = true;
    clearTimeout(timer);
    generation++;
    controller?.abort();
    const current = generation;
    const own = new AbortController();
    controller = own;
    const deadline = setTimeout(() => own.abort(), 30000);
    commandError = "";
    renderStatus();
    updateBusy();
    try {
      const next = await request(
        "/api/command",
        {
          page,
          action,
          value,
          revision: fromInteraction ? interactionRevision : state.revision,
        },
        own.signal,
      );
      if (current === generation) {
        if (action === "send" && pendingSend) pendingSend.acknowledged = true;
        if (action.startsWith("set:")) {
          const draft = drafts.get(action.slice(4));
          if (draft && draft.value === value) draft.acknowledged = true;
        }
        if (fromInteraction && next.interaction)
          interactionRevision = next.interaction.revision || next.revision;
        accept(next);
      }
    } catch (e) {
      if (current !== generation) return;
      stale = !e.status;
      commandError =
        e.name === "AbortError"
          ? t(
              "The request timed out. Check the result before retrying.",
              "请求超时，请确认操作结果后再重试。",
            )
          : e.message;
      if (action === "send" && e.status) pendingSend = null;
      renderStatus();
    } finally {
      clearTimeout(deadline);
      sending = false;
      if (controller === own) controller = null;
      updateBusy();
      schedule();
    }
  }
  function matchesSavedValue(field, submitted) {
    const saved = String(field.value ?? "");
    if (field.kind === "color")
      return saved.toLowerCase() === submitted.toLowerCase();
    if (field.kind === "number" || field.kind === "range") {
      return (
        submitted.trim() !== "" &&
        Number.isFinite(Number(submitted)) &&
        Number(saved) === Number(submitted)
      );
    }
    return (
      saved === submitted ||
      (field.kind === "text" && saved === submitted.trim())
    );
  }
  function accept(next) {
    const nextLanguage = String(next.language || language).startsWith("zh")
      ? "zh"
      : "en";
    if (nextLanguage !== language || next.busy || next.interaction ||
        (state && (state.accountUID !== next.accountUID || state.room?.id !== next.room?.id)))
      hideChatMenu();
    if (nextLanguage !== language) {
      language = nextLanguage;
      pageKey = "";
      localize();
    }
    if (state && state.accountUID !== next.accountUID) {
      drafts.delete("title");
      drafts.delete("announcement");
      chatDraft = "";
      pendingSend = null;
    }
    if (state && (state.accountUID !== next.accountUID || state.room?.id !== next.room?.id))
      feedSignature = "";
    if (!next.interaction) interactionDrafts.clear();
    if (stale) commandError = "";
    state = next;
    stale = false;
    if (pendingSend && next.chatDraft === pendingSend.value)
      pendingSend.acknowledged = true;
    if (pendingSend && !next.busy) {
      if (
        pendingSend.acknowledged &&
        next.chatDraft === "" &&
        !next.warning &&
        chatDraft === pendingSend.value
      )
        chatDraft = "";
      pendingSend = null;
    }
    for (const f of next.fields || []) {
      const d = drafts.get(f.id);
      if (
        d?.acknowledged &&
        !next.busy &&
        !next.warning &&
        (matchesSavedValue(f, d.value) ||
          f.kind === "password" ||
          f.id === "proxy")
      ) {
        drafts.delete(f.id);
      }
    }
    render();
  }
  function navigate(next) {
    if (!routes.includes(next)) next = "studio";
    if (next === page && pageKey) return;
    hideChatMenu();
    page = next;
    history.replaceState(
      null,
      "",
      location.pathname + location.search + "#" + page,
    );
    tab = "";
    pageKey = "";
    generation++;
    controller?.abort();
    localize();
    if (state) render();
    else renderConnection();
    poll();
  }
  function notify(message) {
    const host = $("notifications");
    if (Array.from(host.children).some((toast) => toast.notificationMessage === message)) return;
    const toast = node("section", "toast");
    toast.notificationMessage = message;
    const content = node("div", "toast-content");
    const title = node("strong", "", t("Operation failed", "操作失败"));
    const summary = message.split(/[:：\n]/, 1)[0];
    const detailed = message.length > 160;
    content.append(
      title,
      node("p", "", detailed && summary.length < 80 ? summary : detailed
        ? t("See details for the full error.", "展开详情查看完整错误。")
        : message),
    );
    if (detailed) {
      const details = node("details");
      details.append(
        node("summary", "", t("Error details", "错误详情")),
        node("p", "toast-details", message),
      );
      content.append(details);
    }
    const close = button("×", null, "toast-close");
    close.setAttribute("aria-label", t("Dismiss notification", "关闭通知"));
    close.onclick = () => toast.remove();
    toast.append(content, close);
    host.append(toast);
    while (host.children.length > 3) host.firstElementChild.remove();
  }
  function renderStatus() {
    setText(
      "identity",
      state?.account ? state.account : t("No account", "未登录账号"),
    );
    setText(
      "connection",
      !state
        ? stale
          ? t("Disconnected", "连接已断开")
          : t("Connecting…", "正在连接…")
        : stale
          ? t("Last known state", "上次已知状态")
          : t("Connected", "已连接"),
    );
    $("connection").classList.toggle("stale", stale);
    setText(
      "status",
      state?.busy ? t("Working…", "处理中…") : state?.warning ? "" : state?.status || "",
    );
    const host = $("notifications");
    const parent = $("confirm").open ? $("confirm")
      : $("chat-dialog").open ? $("chat-dialog") : document.body;
    if (host.parentElement !== parent) parent.append(host);
    const error = commandError || (!state?.busy && state?.warning ? state.status : "");
    if (error && error !== notificationKey) notify(error);
    notificationKey = error;
  }
  function updateBusy() {
    const busy = sending || state?.busy;
    document.querySelectorAll("[data-command]").forEach((n) => {
      n.disabled = !!(
        n.dataset.disabled === "true" ||
        sending || (state?.busy && n.dataset.command !== "cancel") ||
        (state?.interaction &&
          !n.dataset.interaction &&
          n.dataset.command !== "cancel")
      );
    });
    for (const [id, entry] of fieldNodes) {
      const f = (state?.fields || []).find((x) => x.id === id);
      entry.input.disabled = !!(f?.disabled || busy || state?.interaction);
      if (entry.clear) entry.clear.disabled = entry.input.disabled;
      if (entry.save) {
        entry.save.disabled = entry.input.disabled;
        entry.save.hidden = !drafts.has(id);
      }
    }
    const send = $("chat-send");
    if (send)
      send.disabled = !!(
        busy ||
        pendingSend ||
        state?.interaction ||
        !state?.chatConnected ||
        !chatDraft.trim() ||
        Array.from(chatDraft).length > 40
      );
  }
  function render() {
    if (state?.page !== page) {
      if ($("main").dataset.loading !== page) {
        const loading = node(
          "p",
          "page-loading",
          t("Loading workspace…", "正在载入工作区…"),
        );
        loading.setAttribute("role", "status");
        $("main").replaceChildren(loading);
        $("main").dataset.loading = page;
      }
      renderInteraction();
      renderStatus();
      updateBusy();
      return;
    }
    delete $("main").dataset.loading;
    const key = page + ":" + language;
    if (pageKey !== key) {
      pageKey = key;
      fieldNodes.clear();
      feedSignature = "";
      logsSignature = "";
      helpSignature = "";
      $("main").replaceChildren(buildPage());
    }
    if (page === "studio") updateStudio();
    if (page === "chat") updateChat();
    if (page === "accounts") updateAccounts();
    if (page === "help") updateHelp();
    if (page === "settings" && tab === "diagnostics") updateLogs();
    const settingsOBS = $("settings-obs-action");
    if (settingsOBS) {
      settingsOBS.textContent = state.obs
        ? t("Disconnect OBS", "断开 OBS")
        : state.obsConnecting
          ? t("Connecting…", "连接中…")
          : t("Connect OBS", "连接 OBS");
      settingsOBS.dataset.command = state.obs
        ? "obs-disconnect-confirm"
        : "obs-connect";
      settingsOBS.dataset.disabled = String(!!state.obsConnecting);
      settingsOBS.onclick = () => command(settingsOBS.dataset.command);
    }
    updateFields();
    renderInteraction();
    renderStatus();
    updateBusy();
  }
  function buildPage() {
    const root = node("div", "page " + page + "-page");
    if (page === "studio") buildStudio(root);
    else if (page === "chat") buildChat(root);
    else if (page === "outputs") buildOutputs(root);
    else if (page === "accounts") buildAccounts(root);
    else if (page === "settings") buildSettings(root);
    else buildHelp(root);
    return root;
  }
  function fieldsRoot(id, cls = "fields") {
    const root = node("div", cls);
    root.id = id;
    return root;
  }
  function renderFields(root, fields) {
    const ids = fields.map((f) => f.id).join("|");
    if (root.dataset.fields !== ids) {
      for (const n of root.querySelectorAll("[data-field]"))
        fieldNodes.delete(n.dataset.field);
      root.replaceChildren(...fields.map(makeField));
      root.dataset.fields = ids;
    }
  }
  function makeField(f) {
    const wrap = node("div", "field" + (f.kind === "switch" ? " switch" : ""));
    wrap.dataset.field = f.id;
    const label = node("label", "", f.label);
    label.htmlFor = "field-" + f.id;
    const line = node("div", "field-line");
    let input;
    if (f.kind === "select") {
      input = node("select");
    } else {
      input = node("input");
      input.type =
        {
          switch: "checkbox",
          password: "password",
          number: "number",
          range: "range",
          color: "color",
        }[f.kind] || "text";
    }
    input.id = "field-" + f.id;
    input.autocomplete = "off";
    let save = null;
    const direct = ["select", "switch"].includes(f.kind);
    const value = () =>
      f.kind === "switch" ? String(input.checked) : input.value;
    const changed = () => {
      drafts.set(f.id, { value: value(), submitted: false });
      if (save) {
        save.hidden = false;
        save.disabled = input.disabled;
      }
    };
    input.addEventListener("input", changed);
    if (direct)
      input.addEventListener("change", () => {
        changed();
        drafts.get(f.id).submitted = true;
        command("set:" + f.id, value());
      });
    else {
      save = button(t("Save", "保存"));
      save.hidden = true;
      save.dataset.command = "set:" + f.id;
      save.onclick = () => {
        drafts.set(f.id, { value: value(), submitted: true });
        command("set:" + f.id, value());
      };
      input.addEventListener("keydown", (e) => {
        if (e.key === "Enter") {
          e.preventDefault();
          save.click();
        }
      });
    }
    line.append(input);
    if (save) line.append(save);
    if (f.kind === "range") {
      const output = node("output");
      output.htmlFor = input.id;
      output.textContent = f.value;
      line.append(output);
      input.addEventListener("input", () => {
        output.textContent = input.value;
      });
    }
    let clear = null;
    if (f.kind === "password" || f.id === "proxy") {
      input.placeholder = t("Enter a replacement value", "输入新值以替换");
      clear = button(t("Clear", "清空"));
      clear.dataset.command = "set:" + f.id;
      clear.onclick = () => {
        input.value = "";
        drafts.set(f.id, { value: "", submitted: true });
        command("set:" + f.id, "");
      };
      line.append(clear);
    }
    wrap.append(label, line);
    fieldNodes.set(f.id, { input, label, save, clear });
    return wrap;
  }
  function updateFields() {
    const fields = state?.fields || [];
    if (page === "studio") {
      renderFields(
        $("room-fields"),
        fields.filter((f) => f.group === "room"),
      );
      renderFields(
        $("connection-fields"),
        fields.filter((f) => f.group === "connection"),
      );
    }
    if (page === "outputs") {
      renderFields(
        $("output-switches"),
        fields.filter((f) => ["overlay-enabled", "tts-enabled"].includes(f.id)),
      );
      renderFields($("output-fields"), fields.filter(outputFilter));
    }
    if (page === "settings" && $("settings-fields"))
      renderFields(
        $("settings-fields"),
        fields.filter((f) => f.group === tab),
      );
    for (const f of fields) {
      const entry = fieldNodes.get(f.id);
      if (!entry) continue;
      const { input, label } = entry;
      label.textContent = f.label;
      const value = drafts.get(f.id)?.value ?? f.value ?? "";
      if (f.kind === "select") {
        const signature = JSON.stringify(f.options || []);
        if (input.dataset.options !== signature) {
          input.replaceChildren(
            ...(f.options || []).map((o) => {
              const n = node("option", "", o.label);
              n.value = o.id;
              n.disabled = !!o.disabled;
              return n;
            }),
          );
          input.dataset.options = signature;
        }
      }
      if (f.kind === "text" && f.options?.length) {
        let list = $("options-" + f.id);
        if (!list) {
          list = node("datalist");
          list.id = "options-" + f.id;
          input.parentElement.append(list);
          input.setAttribute("list", list.id);
        }
        const signature = JSON.stringify(f.options);
        if (list.dataset.options !== signature) {
          list.replaceChildren(
            ...f.options.map((o) => {
              const n = node("option");
              n.value = o.id;
              n.label = o.label;
              return n;
            }),
          );
          list.dataset.options = signature;
        }
      }
      if (f.kind === "switch") input.checked = value === "true";
      else if (input.value !== value) input.value = value;
      if (f.kind === "range") {
        const output = input.parentElement.querySelector("output");
        if (output) output.textContent = value;
      }
      for (const key of ["min", "max", "step"]) {
        if (f[key] !== undefined) input.setAttribute(key, f[key]);
        else if (key === "step" && f.kind === "number") input.step = "any";
        else input.removeAttribute(key);
      }
    }
  }
  function buildStudio(root) {
    const session = node("section", "session");
    session.setAttribute("aria-label", t("Broadcast session", "直播会话"));
    const statuses = node("div", "session-state");
    for (const id of ["live-state", "obs-state"]) {
      const n = node("span", "badge");
      n.id = id;
      statuses.append(n);
    }
    const live = button("", "start-confirm", "primary");
    live.id = "live-action";
    session.append(
      statuses,
      actionRow(button(t("Refresh room", "刷新房间"), "refresh"), live),
    );
    const desk = node("div", "studio-layout"),
      room = node("section", "room-desk");
    const roomHead = node("div", "section-head");
    roomHead.append(
      node("h2", "", t("Live room", "直播间")),
      button(t("Help", "帮助")),
    );
    roomHead.lastChild.onclick = () => openHelp("room");
    const summary = node("div", "room-summary");
    summary.id = "room-summary";
    const roomFields = fieldsRoot("room-fields");
    const roomActions = actionRow(
      button(t("Category", "分区"), "areas"),
      button(t("Cover", "封面"), "cover"),
      button(t("Delay", "延迟"), "delay"),
      button(t("Stream key", "推流信息"), "reveal"),
    );
    const connection = node("section", "connection-fields");
    const ch = node("div", "section-head");
    ch.append(node("h2", "", "OBS Studio"));
    const connect = button("", "obs-connect");
    connect.id = "obs-action";
    ch.append(connect);
    connection.append(ch, fieldsRoot("connection-fields"));
    const flow = node("div");
    flow.id = "studio-flow";
    room.append(roomHead, summary, roomFields, roomActions, flow);
    desk.append(room, connection);
    root.append(session, desk);
  }
  function buildChat(root) {
    const status = node("div", "chat-session");
    const connection = node("span", "badge");
    connection.id = "chat-state";
    const audience = node("span", "note");
    audience.id = "chat-audience";
    status.append(connection, audience);
    const title = heading(t("Chat", "弹幕"), status);
    const room = node("p", "note");
    room.id = "chat-room";
    const tools = actionRow(
      button(t("Audience", "观众"), "audience"),
      button(t("Fleet", "大航海"), "fleet"),
      button(t("Moderators", "房管"), "moderation-admins"),
      button(t("Blacklist", "黑名单"), "moderation-blocks"),
      button(t("Muted users", "禁言名单"), "moderation-mutes"),
    );
    tools.classList.add("chat-tools");
    const help = button(t("Help", "帮助"));
    help.onclick = () => openHelp("chat");
    tools.append(help);
    const chat = node("section", "chat-desk");
    const chatHead = node("div", "chat-head");
    const pause = button(pausedChat ? t("Resume updates", "恢复更新") : t("Pause updates", "暂停更新"));
    pause.onclick = () => {
      pausedChat = !pausedChat;
      pause.textContent = pausedChat
        ? t("Resume updates", "恢复更新")
        : t("Pause updates", "暂停更新");
      setText(
        "chat-pause-note",
        pausedChat
          ? t(
              "Updates paused; controls remain live.",
              "消息更新已暂停，控制仍保持实时。",
            )
          : "",
      );
      if (!pausedChat) updateMessages();
    };
    const latest = button(t("Latest messages", "最新消息"));
    latest.onclick = () => {
      const feed = $("chat-feed");
      feed.scrollTop = feed.scrollHeight;
    };
    chatHead.append(node("h2", "", t("Live messages", "实时消息")), actionRow(latest, pause));
    const feed = node("div", "feed");
    feed.id = "chat-feed";
    feed.setAttribute("role", "log");
    feed.setAttribute("aria-label", t("Live messages", "实时弹幕"));
    feed.setAttribute("aria-live", "off");
    feed.tabIndex = 0;
    const composer = node("form", "composer");
    composer.id = "chat-form";
    const label = node(
      "label",
      "",
      t("Send to your live room", "发送到直播间"),
    );
    label.htmlFor = "chat-draft";
    const input = node("textarea");
    input.id = "chat-draft";
    input.rows = 2;
    input.value = chatDraft;
    input.setAttribute("aria-describedby", "chat-note");
    input.oninput = () => {
      chatDraft = input.value;
      updateComposer();
    };
    input.onkeydown = (e) => {
      if (e.key === "Enter" && !e.shiftKey && !e.isComposing) {
        e.preventDefault();
        composer.requestSubmit();
      }
    };
    const send = button(t("Send", "发送"), null, "primary");
    send.type = "submit";
    send.id = "chat-send";
    const row = node("div", "composer-row");
    row.append(input, send);
    const hint = node("p", "note");
    hint.id = "chat-note";
    const paused = node("p", "note");
    paused.id = "chat-pause-note";
    composer.append(label, row, hint, paused);
    composer.onsubmit = (e) => {
      e.preventDefault();
      if (send.disabled) return;
      pendingSend = { value: chatDraft, acknowledged: false };
      command("send", chatDraft);
    };
    chat.append(chatHead, feed, composer);
    root.append(title, room, tools, chat);
  }
  function updateStudio() {
    const room = state.room || {};
    setText(
      "live-state",
      !state.room
        ? t("Room not loaded", "房间未载入")
        : room.live
          ? t("On air", "正在直播")
          : t("Off air", "未开播"),
    );
    $("live-state").className = "badge" + (room.live ? " live" : "");
    setText(
      "obs-state",
      state.obs
        ? t("OBS connected", "OBS 已连接")
        : state.obsConnecting
          ? t("OBS connecting…", "OBS 连接中…")
          : t("OBS disconnected", "OBS 未连接"),
    );
    $("obs-state").className = "badge" + (state.obs ? " good" : "");
    const live = $("live-action");
    if (!state.accountUID) {
      live.textContent = t("Connect an account", "连接账号");
      delete live.dataset.command;
      delete live.dataset.disabled;
      live.disabled = false;
      live.onclick = () => navigate("accounts");
    } else {
      live.textContent = !room.id
        ? t("Load live room", "载入直播间")
        : room.live
          ? t("End broadcast", "结束直播")
          : t("Start broadcast", "开始直播");
      live.dataset.command = !room.id
        ? "refresh"
        : room.live
          ? "stop-confirm"
          : "start-confirm";
      live.dataset.disabled = "false";
      live.onclick = () => command(live.dataset.command);
    }
    const connect = $("obs-action");
    connect.textContent = state.obs
      ? t("Disconnect", "断开连接")
      : state.obsConnecting
        ? t("Connecting…", "连接中…")
        : t("Connect", "连接");
    connect.dataset.command = state.obs
      ? "obs-disconnect-confirm"
      : "obs-connect";
    connect.dataset.disabled = String(!!state.obsConnecting);
    connect.onclick = () => command(connect.dataset.command);
    const summary = $("room-summary");
    const signature = JSON.stringify([
      room.id,
      room.title,
      room.area,
      room.cover,
      room.coverStatus,
      state.account,
    ]);
    if (summary.dataset.state !== signature) {
      summary.replaceChildren();
      if (room.id) {
        const url = safeURL(room.cover, true);
        if (url) {
          const img = node("img", "room-cover");
          img.alt = t("Current room cover", "当前直播间封面");
          img.referrerPolicy = "no-referrer";
          img.hidden = true;
          img.onload = () => {
            img.hidden = false;
          };
          img.onerror = () => img.remove();
          summary.append(img);
          img.src = url;
        }
        const text = node("div");
        text.append(
          node("strong", "", room.title || t("Untitled room", "未设置标题")),
          node(
            "p",
            "room-meta",
            t("Room ", "房间 ") +
              room.id +
              " · " +
              (room.area || t("No category", "未设置分区")),
          ),
        );
        if (room.coverStatus) text.append(note(room.coverStatus));
        summary.append(text);
      } else {
        const empty = node("div");
        empty.append(
          note(
            t(
              "Sign in to load your room, then connect OBS to prepare your broadcast.",
              "登录账号载入直播间，再连接 OBS 准备开播。",
            ),
          ),
        );
        const b = button(t("Manage accounts", "管理账号"));
        b.onclick = () => navigate("accounts");
        empty.append(b);
        summary.append(empty);
      }
      summary.dataset.state = signature;
    }
    document
      .querySelectorAll(
        '[data-command="areas"],[data-command="cover"],[data-command="delay"]',
      )
      .forEach((n) => (n.dataset.disabled = String(!room.id)));
    const reveal = document.querySelector('[data-command="reveal"]');
    if (reveal) reveal.dataset.disabled = String(!state.streamAvailable);
    document.querySelectorAll('[data-command="refresh"]').forEach((button) => {
      button.dataset.disabled = String(!state.accountUID);
    });
  }
  function updateChat() {
    setText("chat-room", state.room
      ? `${state.room.title || t("Untitled room", "未设置标题")} · ${t("Room ", "房间 ")}${state.room.id}`
      : t("Load your room in Studio to receive and send messages.", "请先在直播台载入房间，再收发弹幕。"));
    setText("chat-state", state.chatConnected
      ? t("Chat connected", "弹幕已连接")
      : t("Chat disconnected", "弹幕未连接"));
    $("chat-state").className = "badge" + (state.chatConnected ? " good" : "");
    setText("chat-audience", state.audience || "");
    document.querySelectorAll(".chat-tools [data-command]").forEach((control) => {
      control.dataset.disabled = String(!state.accountUID || !state.room);
    });
    setText("chat-pause-note", pausedChat
      ? t("Updates paused; receiving continues.", "消息更新已暂停，后台仍在接收。")
      : "");
    updateMessages();
    updateComposer();
  }
  function updateComposer() {
    if (!$("chat-draft")) return;
    if ($("chat-draft").value !== chatDraft) $("chat-draft").value = chatDraft;
    setText(
      "chat-note",
      t("Draft kept until sending succeeds.", "发送成功前保留草稿。") +
        "  " +
        Array.from(chatDraft).length +
        " / 40",
    );
    setText(
      "chat-send",
      pendingSend ? t("Sending…", "发送中…") : t("Send", "发送"),
    );
    updateBusy();
  }
  const messageKinds = {
    chat: ["Chat", "弹幕"],
    gift: ["Gift", "礼物"],
    sc: ["Super Chat", "醒目留言"],
    guard: ["Guard", "大航海"],
    enter: ["Joined", "进入直播间"],
    follow: ["Follow", "关注"],
    mutual_follow: ["Mutual follow", "互相关注"],
    special_follow: ["Special follow", "特别关注"],
    share: ["Share", "分享"],
    like: ["Like", "点赞"],
    likes: ["Likes", "点赞统计"],
    watched: ["Views", "观看统计"],
    rank_count: ["Ranking", "榜单人数"],
    notice: ["Notice", "公告"],
    recommendation: ["Recommendation", "推荐"],
    stop_rooms: ["Stream ended", "停播"],
    live: ["Live", "开播"],
    preparing: ["Offline", "下播"],
    room_change: ["Room update", "房间更新"],
    room_block: ["Muted", "禁言"],
    cut_off: ["Interrupted", "直播切断"],
    delete: ["Removed", "消息撤回"],
    detail: ["Event", "互动事件"],
    gap: ["Connection", "连接记录"],
  };
  let chatMenu = null;
  const messageUserKinds = new Set([
    "chat", "gift", "sc", "guard", "enter", "follow", "mutual_follow",
    "special_follow", "share", "like", "room_block", "detail",
  ]);
  function messageUID(m) {
    return !m.mystery && messageUserKinds.has(m.kind) &&
      typeof m.uid === "string" && /^[1-9]\d*$/.test(m.uid) ? m.uid : "";
  }
  function messageMenuTrigger(id) {
    return Array.from($("chat-feed")?.children || [])
      .find((row) => row.dataset.messageId === String(id))
      ?.querySelector(".message-menu-trigger");
  }
  function hideChatMenu({ restoreFocus = false } = {}) {
    if (!chatMenu) return;
    const current = chatMenu;
    chatMenu = null;
    current.listeners.abort();
    current.menu.remove();
    const trigger = messageMenuTrigger(current.message.id) || current.trigger;
    trigger?.setAttribute("aria-expanded", "false");
    if (restoreFocus && trigger?.isConnected) trigger.focus({ preventScroll: true });
  }
  async function copyMessageText(value) {
    try {
      await navigator.clipboard.writeText(value);
    } catch (_) {
      notify(t("Could not copy. Check clipboard permission in your browser.",
        "复制失败，请检查浏览器的剪贴板权限。"));
    }
  }
  function showChatMenu(m, trigger, event) {
    event?.preventDefault();
    if (sending || stale || state?.busy || state?.interaction || !m.id) return;
    const live = (state.messages || []).find((entry) => entry.id === m.id);
    if (!live || JSON.stringify(live) !== JSON.stringify(m)) return;
    hideChatMenu();
    const menu = node("div", "context-menu");
    menu.id = "chat-context-menu";
    menu.setAttribute("role", "menu");
    menu.setAttribute("aria-label", t("Message actions", "消息操作"));
    const listeners = new AbortController();
    chatMenu = { menu, trigger, message: m, listeners, feedScrollTop: $("chat-feed")?.scrollTop };
    trigger.setAttribute("aria-expanded", "true");
    const add = (label, run) => {
      const item = button(label, "", "context-menu-item");
      item.setAttribute("role", "menuitem");
      item.tabIndex = -1;
      item.onclick = () => {
        hideChatMenu({ restoreFocus: true });
        run();
      };
      menu.append(item);
    };
    add(t("Copy message", "复制正文"), () => copyMessageText(m.text || ""));
    const uid = messageUID(m);
    if (uid) {
      add(t("Copy UID", "复制 UID"), () => copyMessageText(uid));
      add(t("View user", "查看用户"), () => command("speaker-open", m.id));
      const link = node("a", "context-menu-item", t("Open homepage", "打开主页"));
      link.href = "https://space.bilibili.com/" + uid;
      link.target = "_blank";
      link.rel = "noopener noreferrer";
      link.tabIndex = -1;
      link.setAttribute("role", "menuitem");
      link.onclick = () => hideChatMenu({ restoreFocus: true });
      menu.append(link);
      add(t("Mute", "禁言"), () => command("speaker-mute", m.id));
      add(t("Add to blacklist", "加入黑名单"), () => command("speaker-block", m.id));
      add(t("Set as moderator", "设为房管"), () => command("speaker-admin", m.id));
    }
    document.body.append(menu);
    const rect = trigger.getBoundingClientRect();
    const pointer = event?.type === "contextmenu" && (event.clientX || event.clientY);
    const x = pointer ? event.clientX : rect.left;
    const y = pointer ? event.clientY : rect.bottom;
    menu.style.left = Math.max(8, Math.min(x, window.innerWidth - menu.offsetWidth - 8)) + "px";
    menu.style.top = Math.max(8, Math.min(y, window.innerHeight - menu.offsetHeight - 8)) + "px";
    const items = Array.from(menu.children);
    items[0].focus({ preventScroll: true });
    menu.addEventListener("keydown", (e) => {
      const index = items.indexOf(document.activeElement);
      let next;
      if (e.key === "ArrowDown") next = (index + 1) % items.length;
      else if (e.key === "ArrowUp") next = (index + items.length - 1) % items.length;
      else if (e.key === "Home") next = 0;
      else if (e.key === "End") next = items.length - 1;
      else if (e.key === " " && document.activeElement?.tagName === "A") {
        e.preventDefault();
        document.activeElement.click();
      } else if (e.key === "Tab") {
        hideChatMenu({ restoreFocus: true });
      }
      if (next !== undefined) {
        e.preventDefault();
        items[next].focus({ preventScroll: true });
      }
    });
    const options = { capture: true, signal: listeners.signal };
    document.addEventListener("keydown", (e) => {
      if (e.key !== "Escape") return;
      e.preventDefault();
      e.stopPropagation();
      hideChatMenu({ restoreFocus: true });
    }, options);
    document.addEventListener("pointerdown", (e) => {
      if (!menu.contains(e.target) && !trigger.contains(e.target)) hideChatMenu();
    }, options);
    document.addEventListener("focusin", (e) => {
      if (!menu.contains(e.target)) hideChatMenu();
    }, options);
    document.addEventListener("scroll", (e) => {
      // 新消息的自动跟随不关闭菜单，读者主动滚动才关闭。
      if (e.target === $("chat-feed") && e.target.scrollTop === chatMenu?.feedScrollTop) return;
      if (!menu.contains(e.target)) hideChatMenu({ restoreFocus: true });
    }, options);
    window.addEventListener("resize", () => hideChatMenu({ restoreFocus: true }), options);
  }
  function bindMessageMenu(row, m, heading) {
    if (!m.id) return;
    const trigger = button(t("Actions", "操作"), "", "message-menu-trigger");
    trigger.setAttribute("aria-label", t("Message actions", "消息操作") +
      (m.name ? ": " + m.name : ""));
    trigger.setAttribute("aria-haspopup", "menu");
    trigger.setAttribute("aria-controls", "chat-context-menu");
    trigger.setAttribute("aria-expanded", String(chatMenu?.message.id === m.id));
    trigger.onclick = (event) => {
      if (chatMenu?.message.id === m.id) hideChatMenu({ restoreFocus: true });
      else showChatMenu(m, trigger, event);
    };
    row.oncontextmenu = (event) => showChatMenu(m, trigger, event);
    row.onkeydown = (event) => {
      if (event.key === "ContextMenu" || (event.shiftKey && event.key === "F10"))
        showChatMenu(m, trigger, event);
    };
    heading.append(trigger);
  }
  function renderChatMembers(root, i) {
    const members = i.members || {};
    let view = root.chatMembersView;
    if (!view || view.id !== i.id || view.language !== language ||
        !root.contains(view.section)) {
      const section = node("section", "chat-members");
      const title = node("h2");
      const status = node("p", "chat-members-status");
      status.setAttribute("role", "status");
      const refresh = button(t("Refresh", "刷新"), "", "secondary");
      refresh.dataset.command = "members-refresh";
      refresh.dataset.interaction = "true";
      refresh.onclick = () => command("members-refresh", "", true);
      const back = button(t("Back", "返回"), "", "secondary");
      back.dataset.command = "back";
      back.dataset.interaction = "true";
      back.onclick = () => command("back", "", true);
      const close = button(t("Close", "关闭"), "", "secondary");
      close.dataset.command = "cancel";
      close.dataset.interaction = "true";
      close.onclick = () => command("cancel", "", true);
      const viewport = node("div", "chat-members-scroll");
      viewport.tabIndex = 0;
      viewport.setAttribute("role", "region");
      const table = node("table", "chat-members-table");
      const head = node("thead"), row = node("tr"), body = node("tbody");
      for (const text of [t("Rank", "排名"), t("Viewer", "观众"),
        t("Guard", "大航海"), t("Contribution", "贡献"), "UID", t("Medal", "粉丝牌")]) {
        const cell = node("th", "", text);
        cell.scope = "col";
        row.append(cell);
      }
      head.append(row);
      table.append(head, body);
      viewport.append(table);
      const empty = node("p", "chat-members-empty");
      section.append(title, actionRow(refresh, back, close), status, viewport, empty);
      root.replaceChildren(section);
      view = { id: i.id, language, section, title, status, refresh, back, close,
        viewport, body, empty, entries: "" };
      root.chatMembersView = view;
    }
    view.title.textContent = i.title || t("Viewers", "观众");
    view.viewport.setAttribute("aria-label", view.title.textContent);
    // 禁用原因交给统一的 busy 更新合并，避免随后被重新启用。
    view.refresh.dataset.disabled = String(!!members.loading || stale);
    view.back.hidden = !i.back;
    view.back.dataset.disabled = String(stale);
    view.section.setAttribute("aria-busy", String(!!members.loading));
    const total = members.total ?? 0;
    view.status.textContent = [
      t("Total: ", "共 ") + total + t("", " 人"),
      t("Top 20", "前 20 名"),
      members.loading ? t("Loading…", "正在加载…") : "",
      members.previous ? t("Showing previous data", "显示上次数据") : "",
    ].filter(Boolean).join(" · ");
    const entries = members.entries || [];
    view.empty.hidden = entries.length > 0;
    view.viewport.hidden = !entries.length;
    view.empty.textContent = members.loading ? t("Loading…", "正在加载…") :
      members.loaded ? t("No viewers to display", "暂无观众") :
        t("No data loaded. Refresh to try again.", "尚未加载数据，请刷新重试。");
    const signature = JSON.stringify(entries);
    if (signature === view.entries) return;
    view.entries = signature;
    const scrollTop = view.viewport.scrollTop, scrollLeft = view.viewport.scrollLeft;
    view.body.replaceChildren();
    const columns = Array.from(view.body.closest("table").querySelectorAll("th"), (cell) => cell.textContent);
    for (const member of entries) {
      const row = node("tr");
      const uid = !member.mystery && /^[1-9]\d*$/.test(member.uid || "") ? member.uid : "";
      for (const [index, text] of [member.rank ?? "", member.name || t("Mystery user", "神秘用户"),
        member.role || "", member.score || "",
        member.mystery ? t("Mystery user", "神秘用户") : uid,
        member.medalName && member.medalLevel > 0 ?
          member.medalName + " · Lv." + member.medalLevel : ""].entries()) {
        const cell = node("td", "", text);
        cell.dataset.label = columns[index];
        row.append(cell);
      }
      view.body.append(row);
    }
    view.viewport.scrollTop = scrollTop;
    view.viewport.scrollLeft = scrollLeft;
  }
  function updateMessages() {
    const messages = state.messages || [];
    if (chatMenu) {
      const current = messages.find((m) => m.id === chatMenu.message.id);
      if (!current || JSON.stringify(current) !== JSON.stringify(chatMenu.message))
        hideChatMenu({ restoreFocus: true });
    }
    if ((pausedChat && feedSignature) || !$("chat-feed")) return;
    const signature = JSON.stringify([state.language, messages]);
    if (signature === feedSignature) return;
    feedSignature = signature;
    const feed = $("chat-feed");
    const atEnd = feed.scrollHeight - feed.scrollTop - feed.clientHeight < 70;
    const scrollTop = feed.scrollTop;
    const feedTop = feed.getBoundingClientRect().top;
    const anchor = atEnd ? null : Array.from(feed.children).find(
      (row) => row.dataset.messageId && row.getBoundingClientRect().bottom > feedTop,
    );
    const anchorID = anchor?.dataset.messageId;
    const anchorOffset = anchor ? anchor.getBoundingClientRect().top - feedTop : 0;
    const focusedMessage = document.activeElement?.closest(".message")?.dataset.messageId;
    feed.replaceChildren();
    if (!messages.length)
      feed.append(
        node(
          "div",
          "empty",
          t(
            "Your audience appears here. Keep the conversation open while you run the show.",
            "观众消息将在这里显示。控制直播时，也不错过每一次互动。",
          ),
        ),
      );
    else
      for (const m of messages) {
        const row = node("article", "message"),
          time = node("time", "message-time");
        row.dataset.messageId = m.id;
        if (m.kind === "sc" || m.kind === "guard")
          row.classList.add("message-highlight");
        else if (m.kind === "gift" || m.kind === "follow" || m.kind === "live")
          row.classList.add("message-positive");
        else if (m.kind === "cut_off" || m.kind === "room_block")
          row.classList.add("message-warning");
        if (m.deleted) row.classList.add("message-deleted");
        const date = new Date(m.time);
        time.textContent = Number.isNaN(date.getTime())
          ? ""
          : date.toLocaleTimeString([], {
              hour: "2-digit",
              minute: "2-digit",
              second: "2-digit",
              hour12: false,
            });
        time.dateTime = m.time || "";
        time.title = Number.isNaN(date.getTime()) ? "" : date.toLocaleString();
        const body = node("div", "message-body"),
          heading = node("div", "message-heading"),
          identity = node("div", "message-identity");
        heading.append(node("strong", "message-name", m.name || t("System", "系统")));
        const label = Object.hasOwn(messageKinds, m.kind)
          ? t(...messageKinds[m.kind])
          : t("Event", "事件");
        heading.append(node("span", "message-kind", label));
        if (m.mystery)
          identity.append(node("span", "message-badge", t("Mystery user", "神秘用户")));
        else if (m.uid)
          identity.append(node("span", "message-uid", "UID " + m.uid));
        if (m.medalName && m.medalLevel > 0)
          identity.append(node("span", "message-medal", m.medalName + " · Lv." + m.medalLevel));
        if (m.deleted)
          identity.append(node("span", "message-badge", t("Removed", "已撤回")));
        body.append(heading);
        if (identity.childElementCount) heading.append(identity);
        bindMessageMenu(row, m, heading);
        body.append(node("p", "message-text", m.text || ""));
        row.append(time, body);
        feed.append(row);
      }
    if (atEnd) feed.scrollTop = feed.scrollHeight;
    else {
      const restored = anchorID && Array.from(feed.children).find(
        (row) => row.dataset.messageId === anchorID,
      );
      feed.scrollTop = restored
        ? feed.scrollTop + restored.getBoundingClientRect().top - feedTop - anchorOffset
        : scrollTop;
    }
    if (chatMenu) chatMenu.feedScrollTop = feed.scrollTop;
    if (focusedMessage) (messageMenuTrigger(focusedMessage) || feed).focus({ preventScroll: true });
  }
  function tabs(items, onSelect) {
    const root = node("div", "tabs");
    root.setAttribute("role", "tablist");
    for (const [id, label] of items) {
      const b = button(label);
      b.setAttribute("role", "tab");
      b.setAttribute("aria-selected", String(tab === id));
      b.tabIndex = tab === id ? 0 : -1;
      b.onclick = () => onSelect(id);
      b.onkeydown = (e) => {
        const index = items.findIndex((x) => x[0] === id);
        let next;
        if (e.key === "ArrowRight") next = (index + 1) % items.length;
        if (e.key === "ArrowLeft")
          next = (index + items.length - 1) % items.length;
        if (e.key === "Home") next = 0;
        if (e.key === "End") next = items.length - 1;
        if (next !== undefined) {
          e.preventDefault();
          onSelect(items[next][0]);
          document.querySelector('.tabs [aria-selected="true"]')?.focus();
        }
      };
      root.append(b);
    }
    return root;
  }
  function switchTab(id) {
    tab = id;
    pageKey = "";
    render();
  }
  function buildOutputs(root) {
    if (!tab) tab = "layout";
    root.append(
      heading(
        t("Audience outputs", "观众消息输出"),
        fieldsRoot("output-switches", "output-switches"),
      ),
    );
    const editor = node("section", "editor");
    editor.append(
      tabs(
        [
          ["layout", t("Position & size", "位置与尺寸")],
          ["spacing", t("Padding", "内边距")],
          ["type", t("Typography", "文字")],
          ["color", t("Colors", "配色")],
          ["events", t("Overlay events", "弹幕姬事件")],
          ["voice", t("Voice", "语音")],
          ["voice-events", t("Voice events", "语音事件")],
        ],
        switchTab,
      ),
    );
    const main = node("div", "editor-main");
    main.append(
      fieldsRoot(
        "output-fields",
        tab.includes("events") ? "fields event-fields" : "fields",
      ),
    );
    const actions = actionRow();
    if (tab === "layout")
      actions.append(
        button(t("Choose displays", "选择显示器"), "overlay-displays"),
        button(
          t("Restore overlay defaults", "恢复弹幕姬默认值"),
          "overlay-restore-confirm",
        ),
      );
    if (tab === "voice")
      actions.append(
        button(t("Preview voice", "试听语音"), "tts-preview"),
        button(t("Stop preview", "停止试听"), "tts-stop"),
      );
    main.append(actions);
    const flow = node("div");
    flow.id = "outputs-flow";
    main.append(flow);
    if (tab.startsWith("voice"))
      main.append(note(t(
        "Voice preview plays on the computer running Arcana World, not in this browser.",
        "试听将在运行 Arcana World 的电脑上播放，而不是当前浏览器。",
      )));
    const help = button(t("Output guide", "输出使用指南"));
    help.onclick = () => openHelp("outputs");
    actions.append(help);
    editor.append(main);
    root.append(editor);
  }
  function outputFilter(f) {
    if (["overlay-enabled", "tts-enabled"].includes(f.id)) return false;
    if (tab === "voice-events") return f.id.startsWith("tts-event:");
    if (tab === "events") return f.id.startsWith("overlay-event:");
    if (tab === "voice")
      return f.id.startsWith("tts-") && !f.id.startsWith("tts-event:");
    if (!f.id.startsWith("overlay-") || f.id.startsWith("overlay-event:"))
      return false;
    if (tab === "type") return f.group === "overlay-type";
    if (tab === "color") return f.group === "overlay-color";
    if (tab === "spacing") return f.id.startsWith("overlay-padding-");
    return (
      !["overlay-type", "overlay-color"].includes(f.group) &&
      !f.id.startsWith("overlay-padding-")
    );
  }
  function buildAccounts(root) {
    root.append(
      heading(
        t("Accounts", "账号"),
        actionRow(button(t("Sign in with QR", "扫码登录"), "login", "primary")),
      ),
    );
    const layout = node("div", "accounts-layout"),
      list = node("section", "account-list");
    list.id = "account-list";
    list.setAttribute("aria-label", t("Saved accounts", "已保存账号"));
    const canvas = node("section", "login-canvas");
    canvas.id = "accounts-flow";
    layout.append(list, canvas);
    root.append(layout);
  }
  function updateAccounts() {
    const root = $("account-list");
    const signature = JSON.stringify(state.accounts || []);
    if (root.dataset.state !== signature) {
      root.replaceChildren();
      if (!(state.accounts || []).length)
        root.append(
          node(
            "div",
            "empty",
            t(
              "No saved accounts. Sign in to connect your live room.",
              "尚未保存账号。登录后即可连接直播间。",
            ),
          ),
        );
      for (const a of state.accounts || []) {
        const row = node("div", "account-row"),
          name = node("div", "account-name");
        name.append(
          node("strong", "", a.name),
          node(
            "small",
            "",
            a.uid + (a.active ? t(" · Current account", " · 当前账号") : ""),
          ),
        );
        row.append(name);
        if (!a.active)
          row.append(button(t("Use account", "切换账号"), "account:" + a.uid));
        else row.append(node("span", "badge good", t("Active", "使用中")));
        row.append(button(t("Remove", "移除"), "delete:" + a.uid));
        root.append(row);
      }
      root.dataset.state = signature;
    }
  }
  function buildSettings(root) {
    if (!tab) tab = "connection";
    root.append(heading(t("Studio settings", "工作台设置")));
    const editor = node("section", "editor");
    editor.append(
      tabs(
        [
          ["connection", t("Connection", "连接")],
          ["behavior", t("Behavior", "行为")],
          ["maintenance", t("Maintenance", "维护")],
          ["diagnostics", t("Diagnostics", "诊断")],
        ],
        switchTab,
      ),
    );
    if (["connection", "behavior"].includes(tab)) {
      editor.append(fieldsRoot("settings-fields"));
      if (tab === "connection") {
        const b = button(
          state.obs
            ? t("Disconnect OBS", "断开 OBS")
            : t("Connect OBS", "连接 OBS"),
          state.obs ? "obs-disconnect-confirm" : "obs-connect",
        );
        b.id = "settings-obs-action";
        editor.append(
          actionRow(b),
          note(
            t(
              "Passwords are never returned to this browser. Leave the password unchanged unless replacing it. Session overrides are read-only.",
              "密码不会回传到浏览器，无需修改时请保持不变。启动时覆盖的设置为只读。",
            ),
          ),
        );
      }
    }
    if (tab === "maintenance") {
      const sections = node("div", "maintenance");
      for (const [title, text, label, action] of [
        [
          t("Reset preferences", "重置偏好设置"),
          t(
            "Restore application preferences. You will review the operation before it runs.",
            "恢复应用偏好设置，执行前需要确认。",
          ),
          t("Reset settings…", "重置设置…"),
          "settings-reset-confirm",
        ],
        [
          t("Remove local data", "清除本地数据"),
          t(
            "Review the stored data you want to remove. Account data can affect your next sign-in.",
            "选择需要清理的本地数据。清理账号数据可能影响下次登录。",
          ),
          t("Choose data…", "选择数据…"),
          "clear-data",
        ],
      ]) {
        const s = node("section");
        s.append(
          node("h2", "", title),
          note(text),
          button(label, action, "danger"),
        );
        sections.append(s);
      }
      editor.append(sections);
    }
    if (tab === "diagnostics") {
      const pause = button(
        pausedLogs ? t("Resume logs", "恢复日志") : t("Pause logs", "暂停日志"),
      );
      pause.onclick = () => {
        pausedLogs = !pausedLogs;
        pause.textContent = pausedLogs
          ? t("Resume logs", "恢复日志")
          : t("Pause logs", "暂停日志");
        setText(
          "logs-note",
          pausedLogs
            ? t(
                "Updates paused. Displayed entries may be out of date.",
                "日志更新已暂停，显示内容可能不是最新。",
              )
            : t(
                "Recent application activity. Review before sharing: logs can contain private room information.",
                "最近的应用活动。日志可能包含房间隐私信息，分享前请检查。",
              ),
        );
        if (!pausedLogs) updateLogs();
      };
      const help = button(t("Troubleshooting", "故障排查"));
      help.onclick = () => openHelp("troubleshooting");
      editor.append(actionRow(pause, help));
      const n = note("");
      n.id = "logs-note";
      const logs = node("div", "logs");
      logs.id = "logs";
      logs.tabIndex = 0;
      logs.setAttribute("role", "log");
      logs.setAttribute("aria-label", t("Application logs", "应用日志"));
      editor.append(n, logs);
    }
    const flow = node("div");
    flow.id = "settings-flow";
    editor.append(flow);
    root.append(editor);
  }
  function updateLogs() {
    if (pausedLogs || !$("logs")) return;
    const logs = state.logs || [];
    const signature = JSON.stringify(logs);
    if (signature === logsSignature) return;
    logsSignature = signature;
    const root = $("logs");
    const end = root.scrollHeight - root.scrollTop - root.clientHeight < 70;
    root.textContent = logs.length
      ? logs
          .map((l) =>
            typeof l === "string"
              ? l
              : [l.time, l.level, l.text || l.message]
                  .filter(Boolean)
                  .join(" "),
          )
          .join("\n")
      : t("No activity yet.", "暂无运行日志。");
    if (end) root.scrollTop = root.scrollHeight;
    setText(
      "logs-note",
      t(
        "Recent application activity. Review before sharing: logs can contain private room information.",
        "最近的应用活动。日志可能包含房间隐私信息，分享前请检查。",
      ),
    );
  }
  function buildHelp(root) {
    root.append(heading(t("Studio guide", "工作台指南")));
    const layout = node("div", "help-layout"),
      list = node("aside", "help-list"),
      label = node("label", "sr-only", t("Search help", "搜索帮助"));
    label.htmlFor = "help-search";
    const input = node("input");
    input.id = "help-search";
    input.type = "search";
    input.placeholder = t("Search topics…", "搜索帮助主题…");
    input.value = helpQuery;
    input.oninput = () => {
      helpQuery = input.value;
      helpSignature = "";
      updateHelp();
    };
    const topics = node("div", "topics");
    topics.id = "help-topics";
    const selector = node("select", "help-select");
    selector.id = "help-select";
    selector.setAttribute("aria-label", t("Help topic", "帮助主题"));
    selector.onchange = () => {
      helpTopic = selector.value;
      helpSignature = "";
      updateHelp();
    };
    list.append(label, input, selector, topics);
    const article = node("article", "article");
    article.id = "help-article";
    article.tabIndex = 0;
    layout.append(list, article);
    root.append(layout);
  }
  function openHelp(id) {
    helpTopic = id;
    helpQuery = "";
    navigate("help");
  }
  function updateHelp() {
    const articles = state.help || [];
    const signature = JSON.stringify([articles, helpQuery, helpTopic]);
    if (signature === helpSignature) return;
    helpSignature = signature;
    const q = helpQuery.trim().toLocaleLowerCase();
    const matches = articles.filter((a) =>
      (a.title + " " + a.summary + " " + a.body)
        .toLocaleLowerCase()
        .includes(q),
    );
    let active = matches.find((a) => a.id === helpTopic) || matches[0];
    const topics = $("help-topics");
    topics.replaceChildren();
    const selector = $("help-select");
    selector.replaceChildren(
      ...matches.map((article) => {
        const option = node("option", "", article.title);
        option.value = article.id;
        return option;
      }),
    );
    selector.disabled = !matches.length;
    if (active) selector.value = active.id;
    for (const a of matches) {
      const b = button(a.title);
      b.setAttribute("aria-current", String(a === active));
      b.onclick = () => {
        helpTopic = a.id;
        helpSignature = "";
        updateHelp();
        $("help-article").focus();
      };
      topics.append(b);
    }
    const article = $("help-article");
    article.replaceChildren();
    if (active) {
      article.append(
        node("h2", "", active.title),
        node("p", "summary", active.summary),
        node("div", "article-body", active.body),
      );
    } else
      article.append(
        node("h2", "", t("No matching topics", "没有匹配的主题")),
        note(
          t(
            "Try another search or clear the search field.",
            "尝试其他关键词，或清空搜索框。",
          ),
        ),
      );
  }
  function interactionPage(i) {
    if (i.target?.startsWith("account") || i.kind === "qr") return "accounts";
    if (i.target?.startsWith("chat-")) return "chat";
    if (i.target?.startsWith("overlay")) return "outputs";
    if (i.target === "maintenance") return "settings";
    return "studio";
  }
  function openChatDialog(changed) {
    const dialog = $("chat-dialog");
    const root = $("chat-flow");
    const title = root.querySelector("h2, h3");
    if (title) title.id = "chat-dialog-title";
    const opening = !dialog.open;
    if (opening) {
      if (!chatFocusReturn) {
        chatFocusReturn = commandFocus?.isConnected ? commandFocus
          : focusReturn?.isConnected ? focusReturn : document.activeElement;
        chatFocusMessage = chatFocusReturn?.closest(".message")?.dataset.messageId || "";
      }
      commandFocus = null;
      dialog.showModal();
    }
    if (opening || changed)
      queueMicrotask(() => {
        if (!dialog.open || $("confirm").open) return;
        const control = root.querySelector('input:not(:disabled), textarea:not(:disabled)') ||
          root.querySelector('[data-command="cancel"]');
        control?.focus({ preventScroll: true });
      });
  }
  function restoreChatFocus() {
    const previous = chatFocusReturn;
    const message = chatFocusMessage;
    chatFocusReturn = null;
    chatFocusMessage = "";
    queueMicrotask(() => {
      if ($("chat-dialog").open || $("confirm").open || $("app").hidden) return;
      const target = previous?.isConnected ? previous : messageMenuTrigger(message);
      if (target && !target.disabled) target.focus({ preventScroll: true });
      else $("main").focus({ preventScroll: true });
    });
  }
  function renderInteraction() {
    const i = state.interaction;
    const owner = i ? interactionPage(i) : "";
    const key = i ? i.id + ":" + i.kind : "";
    const changed = key !== interactionKey;
    if (changed) interactionKey = key;
    interactionRevision = i ? i.revision || state.revision : 0;
    const modal = $("confirm");
    const chatDialog = $("chat-dialog");
    const restoringChatFocus = !i && !!chatFocusReturn;
    const chatOpen = i && i.kind !== "confirm" && owner === "chat" && page === "chat";
    if (!chatOpen && chatDialog.open) chatDialog.close();
    if (!i && chatFocusReturn) restoreChatFocus();
    if (!i || i.kind !== "confirm") {
      if (modal.open) {
        modal.close();
        queueMicrotask(() => {
          if (!restoringChatFocus && !modal.open && !chatDialog.open && !$("app").hidden) {
            if (focusReturn?.isConnected && !focusReturn.disabled)
              focusReturn.focus();
            else $("main").focus();
          }
        });
      }
    }
    for (const id of [
      "studio-flow",
      "chat-flow",
      "outputs-flow",
      "accounts-flow",
      "settings-flow",
    ]) {
      const root = $(id);
      if (root && (!i || id !== owner + "-flow" ||
          owner !== page || i.kind === "confirm")) {
        if (root.dataset.interaction) {
          root.replaceChildren();
          delete root.dataset.interaction;
        }
        if (id === "accounts-flow" && !root.children.length) {
          const empty = node("div", "empty");
          empty.append(
            node("h2", "", t("A secure place to sign in", "在这里安全登录")),
            note(
              t(
                "Choose “Sign in with QR” and scan with the Bilibili app. Switch accounts without mixing their room state.",
                "点击「扫码登录」，使用哔哩哔哩客户端扫码。切换账号时，直播间状态相互隔离。",
              ),
            ),
          );
          root.append(empty);
        }
      }
    }
    $("pending").hidden = true;
    if (!i) return;
    if (i.kind === "confirm") {
      if (changed || !modal.open) {
        focusReturn = commandFocus?.isConnected
          ? commandFocus
          : document.activeElement;
        commandFocus = null;
        setText("confirm-title", i.title);
        setText("confirm-text", i.text || "");
        $("confirm-image")?.remove();
        const source = safeURL(i.image, true);
        if (source) {
          const image = node("img", "flow-image");
          image.id = "confirm-image";
          image.alt = i.title;
          image.src = source;
          modal.insertBefore(image, $("confirm-actions"));
        }
        const actions = $("confirm-actions");
        actions.replaceChildren();
        const cancel = interactionButton(t("Cancel", "取消"), "cancel");
        actions.append(cancel);
        for (const c of i.choices || []) {
          actions.append(interactionButton(c.label, c.id, "danger", c.id));
        }
        if (!modal.open) modal.showModal();
        cancel.focus();
      }
      return;
    }
    if (owner !== page) {
      const pending = $("pending");
      pending.hidden = false;
      pending.replaceChildren(node("span", "", i.title));
      const go = button(t("Continue in ", "前往") + labels()[owner]);
      go.onclick = () => navigate(owner);
      pending.append(go, interactionButton(t("Close", "关闭"), "cancel"));
      return;
    }
    const root = $(owner + "-flow");
    if (!root) return;
    if (i.kind === "members") {
      renderChatMembers(root, i);
      root.dataset.interaction = key + ":" + pageKey;
      openChatDialog(changed);
      return;
    }
    const flowKey = key + ":" + pageKey;
    if (root.dataset.interaction !== flowKey) {
      root.replaceChildren();
      root.dataset.interaction = flowKey;
      const panel = node("section", "inline-flow");
      panel.id = "inline-flow";
      panel.tabIndex = -1;
      panel.append(node("h3", "", i.title));
      const text = node("p", "note");
      text.id = "flow-text";
      panel.append(text);
      if (["form", "cover", "selection"].includes(i.kind)) {
        const form = node("form", "flow-form"),
          label = node(
            "label",
            "",
            i.kind === "cover"
              ? t("Image path on the studio computer", "工作台电脑上的图片路径")
              : i.kind === "selection"
                ? t("Search categories", "搜索分区")
                : i.title,
          );
        label.htmlFor = "flow-input";
        const input = node("input");
        input.id = "flow-input";
        input.type = i.secret ? "password" : "text";
        input.value = interactionDrafts.get(key) ?? i.value ?? "";
        input.oninput = () => interactionDrafts.set(key, input.value);
        input.autocomplete = "off";
        const submit = button(
          i.kind === "selection"
            ? t("Search", "搜索")
            : i.kind === "cover"
              ? t("Preview cover", "预览封面")
              : t("Apply", "应用"),
        );
        submit.type = "submit";
        submit.dataset.command = i.kind === "selection" ? "search" : "submit";
        submit.dataset.interaction = "true";
        form.append(input, submit);
        form.onsubmit = (e) => {
          e.preventDefault();
          command(
            i.kind === "selection" ? "search" : "submit",
            input.value,
            true,
          );
        };
        panel.append(label, form);
        if (i.kind === "cover")
          panel.append(
            note(
              t(
                "Not a browser upload: use the full path on the computer running Arcana World.",
                "这不是浏览器上传：请输入运行 Arcana World 的电脑上的完整路径。",
              ),
            ),
          );
        if (i.secret) {
          const show = button(t("Show value", "显示内容"));
          show.onclick = () => {
            input.type = input.type === "password" ? "text" : "password";
            show.textContent =
              input.type === "password"
                ? t("Show value", "显示内容")
                : t("Hide value", "隐藏内容");
          };
          panel.append(show);
        }
      }
      const img = node("img", "flow-image");
      img.id = "flow-image";
      img.alt = i.title;
      img.hidden = true;
      const link = node("a", "", i.kind === "speaker"
        ? t("Open homepage", "打开主页")
        : t("Open verification link", "打开验证链接"));
      link.id = "flow-link";
      link.target = "_blank";
      link.rel = "noopener noreferrer";
      link.hidden = true;
      const choices = node("div", "choices");
      choices.id = "flow-choices";
      const navigation = actionRow();
      if (i.back) navigation.append(interactionButton(t("Back", "返回"), "back"));
      navigation.append(interactionButton(t("Close", "关闭"), "cancel"));
      panel.append(
        img,
        link,
        choices,
        navigation,
      );
      if (i.kind === "qr")
        panel.append(
          note(
            t(
              "Scan with the mobile app. Sign-in status updates automatically.",
              "请使用手机客户端扫码，登录状态会自动更新。",
            ),
          ),
        );
      root.append(panel);
    }
    setText("flow-text", i.text || "");
    const image = safeURL(i.image, true);
    $("flow-image").hidden = !image;
    if (image && $("flow-image").getAttribute("src") !== image)
      $("flow-image").src = image;
    if (!image) $("flow-image").removeAttribute("src");
    const url = safeURL(i.url);
    $("flow-link").hidden = !url;
    if (url) $("flow-link").href = url;
    else $("flow-link").removeAttribute("href");
    const choices = $("flow-choices");
    const signature = JSON.stringify(i.choices || []);
    if (choices.dataset.choices !== signature) {
      choices.replaceChildren(
        ...(i.choices || []).map((c) => {
          const b = interactionButton(
            c.label,
            ["pick", "selection", "form", "cover"].includes(i.kind)
              ? "choose"
              : c.id,
            "",
            c.id,
          );
          b.setAttribute("aria-pressed", String(!!c.selected));
          b.dataset.disabled = String(!!c.disabled);
          return b;
        }),
      );
      choices.dataset.choices = signature;
    }
    if (owner === "chat") openChatDialog(changed);
  }
  function interactionButton(label, action, cls = "", value = "") {
    const b = button(label, null, cls);
    b.dataset.command = action;
    b.dataset.interaction = "true";
    b.onclick = () => {
      const i = state?.interaction;
      // 返回时也提交未确认的表单文本，由服务端保存所属步骤的草稿。
      const draft = action === "back" && i?.kind === "form" &&
        interactionPage(i) === "chat" ? $("flow-input")?.value : undefined;
      command(action, draft ?? value, true);
    };
    return b;
  }
  for (const dialog of [$("confirm"), $("chat-dialog")]) {
    dialog.addEventListener("cancel", (e) => {
      e.preventDefault();
      command("cancel", "", true);
    });
    dialog.addEventListener("keydown", (event) => {
      if (event.key !== "Tab") return;
      const controls = [
        ...dialog.querySelectorAll("button, input, textarea, select, a[href], summary, [tabindex]"),
      ].filter(
        (n) => !n.disabled && n.tabIndex >= 0 && n.getClientRects().length,
      );
      if (!controls.length) {
        event.preventDefault();
        return;
      }
      const first = controls[0],
        last = controls[controls.length - 1];
      if (
        event.shiftKey &&
        (document.activeElement === first ||
          !controls.includes(document.activeElement))
      ) {
        event.preventDefault();
        last.focus();
      } else if (
        !event.shiftKey &&
        (document.activeElement === last ||
          !controls.includes(document.activeElement))
      ) {
        event.preventDefault();
        first.focus();
      }
    });
  }
  $("retry").onclick = () => poll();
  $("theme").onchange = () => {
    const value = $("theme").value;
    storage("arcana-theme", value);
    document.documentElement.dataset.theme = value;
  };
  document.querySelector(".appbar .brand").onclick = (e) => {
    e.preventDefault();
    navigate("studio");
  };
  document.addEventListener("visibilitychange", () => {
    if (!document.hidden) poll();
    else clearTimeout(timer);
  });
  window.addEventListener("hashchange", () => {
    const next = location.hash.slice(1);
    if (routes.includes(next)) navigate(next);
  });
  if (routes.includes(location.hash.slice(1))) page = location.hash.slice(1);
  localize();
  renderConnection();
  poll();
})();
