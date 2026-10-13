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
  const routes = ["studio", "outputs", "accounts", "settings", "help"];
  const labels = () => ({
    studio: t("Studio", "直播台"),
    outputs: t("Outputs", "输出"),
    accounts: t("Accounts", "账号"),
    settings: t("Settings", "设置"),
    help: t("Help", "帮助"),
  });
  let token = "",
    page = "studio",
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
    errorTarget = "",
    chatDraft = "",
    pendingSend = null;
  let pausedChat = false,
    pausedLogs = false,
    feedSignature = "",
    logsSignature = "",
    interactionKey = "",
    interactionRevision = 0,
    focusReturn = null,
    commandFocus = null;
  let helpQuery = "",
    helpTopic = "",
    helpSignature = "";
  let studioPane = "chat";
  const drafts = new Map(),
    fieldNodes = new Map();
  function storage(name, key, value) {
    try {
      if (value === undefined) return window[name].getItem(key);
      if (value === null) window[name].removeItem(key);
      else window[name].setItem(key, value);
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
    setText("auth-title", t("Connect to your studio", "连接你的直播工作台"));
    setText(
      "auth-note",
      t(
        "Enter the access token from your running Arcana World. It stays in this browser tab only.",
        "输入正在运行的 Arcana World 提供的访问令牌。令牌仅保存在当前浏览器标签页。",
      ),
    );
    setText("token-label", t("Access token", "访问令牌"));
    setText("auth-submit", t("Connect", "连接"));
    setText("lock", t("Lock", "锁定"));
    setText("retry", t("Refresh connection", "刷新连接"));
    document.querySelector(".skip").textContent = t(
      "Skip to content",
      "跳至主要内容",
    );
    const theme = storage("localStorage", "arcana-theme") || "system";
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
    if (token && !document.hidden) timer = setTimeout(poll, 1000);
  }
  function authGate(message = "") {
    generation++;
    controller?.abort();
    clearTimeout(timer);
    token = "";
    storage("sessionStorage", "arcana-token", null);
    state = null;
    drafts.clear();
    chatDraft = "";
    pendingSend = null;
    commandError = "";
    errorTarget = "";
    pageKey = "";
    interactionKey = "";
    if ($("confirm").open) $("confirm").close();
    $("main").replaceChildren();
    $("app").hidden = true;
    $("auth").hidden = false;
    setText("auth-error", message);
    $("auth-submit").disabled = false;
    $("token").focus();
  }
  async function request(path, body, signal) {
    const response = await fetch(path, {
      method: body ? "POST" : "GET",
      headers: {
        Authorization: "Bearer " + token,
        ...(body ? { "Content-Type": "application/json" } : {}),
      },
      body: body ? JSON.stringify(body) : undefined,
      cache: "no-store",
      credentials: "omit",
      redirect: "error",
      signal,
    });
    if (response.status === 401) {
      const e = new Error(
        t(
          "Token missing or expired. Enter the token for your running studio.",
          "令牌缺失或已失效，请输入当前工作台的访问令牌。",
        ),
      );
      e.auth = true;
      throw e;
    }
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
    if (!token || fetching || sending || document.hidden) return;
    fetching = true;
    const current = generation;
    const own = new AbortController();
    controller = own;
    const deadline = setTimeout(() => own.abort(), 15000);
    try {
      const next = await request("/api/state?page=" + page, null, own.signal);
      if (current === generation) accept(next);
    } catch (e) {
      if (current !== generation) return;
      if (e.auth) authGate(e.message);
      else {
        stale = true;
        const message =
          e.name === "AbortError"
            ? t(
                "Connection timed out. Last known state is shown.",
                "连接超时，当前显示上次已知状态。",
              )
            : e.message;
        if (state) {
          commandError = message;
          renderStatus();
        } else setText("auth-error", message);
      }
    } finally {
      clearTimeout(deadline);
      fetching = false;
      if (controller === own) controller = null;
      $("auth-submit").disabled = false;
      if (current !== generation && token && !sending && !document.hidden)
        queueMicrotask(poll);
      else schedule();
    }
  }
  async function command(
    action,
    value = "",
    fromInteraction = false,
    target = "",
  ) {
    if (sending || !state || (state.busy && action !== "cancel")) return;
    if (!state.interaction) commandFocus = document.activeElement;
    sending = true;
    clearTimeout(timer);
    generation++;
    controller?.abort();
    const current = generation;
    const own = new AbortController();
    controller = own;
    const deadline = setTimeout(() => own.abort(), 30000);
    commandError = "";
    errorTarget = target;
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
      if (e.auth) authGate(e.message);
      else {
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
      }
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
    if (stale && !errorTarget) commandError = "";
    state = next;
    stale = false;
    $("auth").hidden = true;
    $("app").hidden = false;
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
    poll();
  }
  function renderStatus() {
    setText(
      "identity",
      state?.account ? state.account : t("No account", "未登录账号"),
    );
    setText(
      "connection",
      stale
        ? t("Last known state", "上次已知状态")
        : t("Local connection", "本机连接"),
    );
    $("connection").classList.toggle("stale", stale);
    setText(
      "status",
      state?.busy ? t("Working…", "处理中…") : state?.status || "",
    );
    $("status").classList.toggle("warning", !!state?.warning);
    const error =
      commandError ||
      (errorTarget && !sending && !state?.busy && state?.warning
        ? state.status
        : "");
    const inline = errorTarget && $(errorTarget);
    $("global-error").hidden = !error || !!inline || $("confirm").open;
    setText("global-error", error);
    document.querySelectorAll(".field-error,.flow-error").forEach((n) => {
      n.textContent = n === inline ? error : "";
    });
    setText("confirm-error", $("confirm").open ? error : "");
  }
  function updateBusy() {
    const busy = sending || state?.busy;
    document.querySelectorAll("[data-command]").forEach((n) => {
      n.disabled = !!(
        n.dataset.disabled === "true" ||
        (busy && n.dataset.command !== "cancel") ||
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
      renderStatus();
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
    if (page === "outputs") updateOutputPreview();
    renderInteraction();
    renderStatus();
    updateBusy();
  }
  function buildPage() {
    const root = node("div", "page " + page + "-page");
    if (page === "studio") buildStudio(root);
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
    const err = node("p", "error field-error");
    err.id = "error-" + f.id;
    input.setAttribute("aria-describedby", err.id);
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
      if (page === "outputs") updateOutputPreview();
    };
    input.addEventListener("input", changed);
    if (direct)
      input.addEventListener("change", () => {
        changed();
        drafts.get(f.id).submitted = true;
        command("set:" + f.id, value(), false, err.id);
      });
    else {
      save = button(t("Save", "保存"));
      save.hidden = true;
      save.dataset.command = "set:" + f.id;
      save.onclick = () => {
        drafts.set(f.id, { value: value(), submitted: true });
        command("set:" + f.id, value(), false, err.id);
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
        command("set:" + f.id, "", false, err.id);
      };
      line.append(clear);
    }
    wrap.append(label, line, err);
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
  function selectStudioPane(id) {
    studioPane = id;
    const studio = document.querySelector(".studio-page");
    if (studio) studio.dataset.mobilePane = id;
    document.querySelectorAll("[data-studio-pane]").forEach((button) => {
      button.setAttribute(
        "aria-pressed",
        String(button.dataset.studioPane === id),
      );
    });
  }
  function buildStudio(root) {
    root.dataset.mobilePane = studioPane;
    const views = node("div", "studio-views");
    views.setAttribute("role", "group");
    views.setAttribute("aria-label", t("Studio view", "直播台视图"));
    for (const [id, label] of [
      ["chat", t("Conversation", "实时互动")],
      ["room", t("Room & connection", "房间与连接")],
    ]) {
      const choice = button(label);
      choice.dataset.studioPane = id;
      choice.setAttribute("aria-pressed", String(studioPane === id));
      choice.onclick = () => selectStudioPane(id);
      views.append(choice);
    }
    const session = node("section", "session");
    session.setAttribute("aria-label", t("Broadcast session", "直播会话"));
    const statuses = node("div", "session-state");
    for (const id of ["live-state", "obs-state", "chat-state"]) {
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
    const desk = node("div", "desk"),
      room = node("section", "room-desk"),
      chat = node("section", "chat-desk");
    const roomHead = node("div", "section-head");
    roomHead.append(
      node("h2", "", t("Room & connection", "房间与连接")),
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
    ch.append(node("h3", "", t("OBS Studio", "OBS Studio")));
    const connect = button("", "obs-connect");
    connect.id = "obs-action";
    ch.append(connect);
    connection.append(ch, fieldsRoot("connection-fields"));
    const flow = node("div");
    flow.id = "studio-flow";
    room.append(roomHead, summary, roomFields, roomActions, connection, flow);
    const chatHead = node("div", "chat-head");
    const pause = button(t("Pause updates", "暂停更新"));
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
    chatHead.append(node("h2", "", t("Conversation", "实时互动")), pause);
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
    input.rows = 1;
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
    const err = node("p", "error field-error");
    err.id = "chat-error";
    composer.append(label, row, hint, paused, err);
    composer.onsubmit = (e) => {
      e.preventDefault();
      if (send.disabled) return;
      pendingSend = { value: chatDraft, acknowledged: false };
      command("send", chatDraft, false, "chat-error");
    };
    chat.append(chatHead, feed, composer);
    desk.append(room, chat);
    root.append(session, views, desk);
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
    setText(
      "chat-state",
      state.chatConnected
        ? t("Chat connected", "弹幕已连接")
        : t("Chat disconnected", "弹幕未连接"),
    );
    $("chat-state").className = "badge" + (state.chatConnected ? " good" : "");
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
          img.src = url;
          img.alt = t("Current room cover", "当前直播间封面");
          summary.append(img);
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
  function updateMessages() {
    if (pausedChat || !$("chat-feed")) return;
    const messages = state.messages || [];
    const signature = JSON.stringify(messages);
    if (signature === feedSignature) return;
    feedSignature = signature;
    const feed = $("chat-feed");
    const atEnd = feed.scrollHeight - feed.scrollTop - feed.clientHeight < 70;
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
        const row = node("div", "message"),
          time = node("time");
        const date = new Date(m.time);
        time.textContent = Number.isNaN(date.getTime())
          ? ""
          : date.toLocaleTimeString([], {
              hour: "2-digit",
              minute: "2-digit",
              hour12: false,
            });
        time.dateTime = m.time || "";
        const body = node("div"),
          name = node("div");
        name.append(
          node("strong", "", m.name || ""),
          node("span", "message-kind", m.kind || ""),
        );
        body.append(name, node("p", "", m.text || ""));
        row.append(time, body);
        feed.append(row);
      }
    if (atEnd) feed.scrollTop = feed.scrollHeight;
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
    const layout = node("div", "editor-layout"),
      main = node("div", "editor-main");
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
    const aside = node("details", "output-note");
    aside.open = !matchMedia("(max-width: 1150px)").matches;
    aside.append(
      node(
        "summary",
        "",
        tab.startsWith("voice")
          ? t("Playback guide", "播放说明")
          : t("Appearance preview", "外观预览"),
      ),
      note(
        tab.startsWith("voice")
          ? t(
              "Voice preview plays on the computer running Arcana World, not in this browser.",
              "试听将在运行 Arcana World 的电脑上播放，而不是当前浏览器。",
            )
          : t(
              "Changes apply to the overlay on the studio computer. Use layout, typography and colors to keep messages readable over your stream.",
              "更改会应用到工作台电脑上的弹幕姬。通过布局、文字和配色，让消息在直播画面上清晰可读。",
            ),
      ),
    );
    const help = button(t("Output guide", "输出使用指南"));
    help.onclick = () => openHelp("outputs");
    aside.append(help);
    if (!tab.startsWith("voice")) {
      const preview = node("figure", "output-preview");
      const caption = node(
        "figcaption",
        "",
        t("Appearance sample · scaled illustration", "外观样本 · 缩略示意"),
      );
      const stage = node("div", "preview-stage");
      stage.id = "preview-stage";
      const bubble = node("div", "preview-bubble");
      bubble.id = "preview-bubble";
      bubble.append(
        node("strong", "", t("Viewer", "观众")),
        node(
          "p",
          "",
          t("Good evening. Welcome to the stream.", "晚上好，欢迎来到直播间。"),
        ),
      );
      stage.append(bubble);
      const dimensions = node("p", "note");
      dimensions.id = "preview-dimensions";
      preview.append(caption, stage, dimensions);
      aside.insertBefore(preview, aside.children[1]);
    }
    layout.append(main, aside);
    editor.append(layout);
    root.append(editor);
  }
  function updateOutputPreview() {
    if (!$("preview-stage")) return;
    const values = new Map(
      (state?.fields || []).map((f) => [
        f.id,
        drafts.get(f.id)?.value ?? f.value,
      ]),
    );
    const number = (key, fallback) => {
      const value = Number(values.get("overlay-" + key));
      return Number.isFinite(value) ? value : fallback;
    };
    const color = (key, fallback) => {
      const value = values.get("overlay-" + key) || "";
      return /^#[0-9a-f]{6}$/i.test(value) ? value : fallback;
    };
    const alpha = (key) => Math.max(0, Math.min(1, number(key, 1)));
    const stage = $("preview-stage"),
      bubble = $("preview-bubble");
    const anchor = values.get("overlay-anchor") || "left";
    stage.style.alignItems = anchor.includes("top")
      ? "flex-start"
      : anchor.includes("bottom")
        ? "flex-end"
        : "center";
    stage.style.justifyContent = anchor.includes("left")
      ? "flex-start"
      : anchor.includes("right")
        ? "flex-end"
        : "center";
    bubble.style.backgroundColor =
      color("background-color", "#181C24") +
      Math.round(alpha("background-alpha") * 255)
        .toString(16)
        .padStart(2, "0");
    bubble.style.color =
      color("text-color", "#FFFFFF") +
      Math.round(alpha("text-alpha") * 255)
        .toString(16)
        .padStart(2, "0");
    bubble.style.fontSize =
      Math.max(12, Math.min(26, number("font-size", 16))) + "px";
    bubble.style.fontWeight = Math.max(
      100,
      Math.min(900, number("font-weight", 500)),
    );
    bubble.style.fontStyle =
      values.get("overlay-italic") === "true" ? "italic" : "normal";
    bubble.style.fontFamily = values.get("overlay-font-family") || "inherit";
    bubble.style.textShadow =
      values.get("overlay-outline") === "true" ? "0 1px 2px #000" : "none";
    setText(
      "preview-dimensions",
      `${number("width", 420)} × ${number("height", 180)} · ${number("font-size", 16)} px`,
    );
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
    if (i.target?.startsWith("overlay")) return "outputs";
    if (i.target === "maintenance") return "settings";
    return "studio";
  }
  function renderInteraction() {
    const i = state.interaction;
    const key = i ? i.id + ":" + i.kind : "";
    const changed = key !== interactionKey;
    if (changed) {
      interactionKey = key;
      interactionRevision = i ? i.revision || state.revision : 0;
    }
    const modal = $("confirm");
    if (!i || i.kind !== "confirm") {
      if (modal.open) {
        modal.close();
        queueMicrotask(() => {
          if (!modal.open && !$("app").hidden) {
            if (focusReturn?.isConnected && !focusReturn.disabled)
              focusReturn.focus();
            else $("main").focus();
          }
        });
      }
    }
    for (const id of [
      "studio-flow",
      "outputs-flow",
      "accounts-flow",
      "settings-flow",
    ]) {
      const root = $(id);
      if (root && (!i || interactionPage(i) !== page || i.kind === "confirm")) {
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
          modal.insertBefore(image, $("confirm-error"));
        }
        const actions = $("confirm-actions");
        actions.replaceChildren();
        const cancel = interactionButton(t("Cancel", "取消"), "cancel");
        actions.append(cancel);
        const choices = i.choices?.length
          ? i.choices
          : [{ id: "confirm", label: t("Confirm", "确认") }];
        for (const c of choices) {
          if (c.id === "cancel") continue;
          actions.append(interactionButton(c.label, c.id, "danger", c.id));
        }
        if (!modal.open) modal.showModal();
        cancel.focus();
      }
      return;
    }
    const owner = interactionPage(i);
    if (owner !== page) {
      const pending = $("pending");
      pending.hidden = false;
      pending.replaceChildren(node("span", "", i.title));
      const go = button(t("Continue in ", "前往") + labels()[owner]);
      go.onclick = () => navigate(owner);
      pending.append(go, interactionButton(t("Cancel", "取消"), "cancel"));
      return;
    }
    const root = $(owner + "-flow");
    if (!root) return;
    const flowKey = key + ":" + pageKey;
    if (root.dataset.interaction !== flowKey) {
      if (owner === "studio") selectStudioPane("room");
      root.replaceChildren();
      root.dataset.interaction = flowKey;
      const panel = node("section", "inline-flow");
      panel.id = "inline-flow";
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
        input.value = i.value || "";
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
            "flow-error",
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
      const link = node("a", "", t("Open verification link", "打开验证链接"));
      link.id = "flow-link";
      link.target = "_blank";
      link.rel = "noopener noreferrer";
      link.hidden = true;
      const choices = node("div", "choices");
      choices.id = "flow-choices";
      const error = node("p", "error flow-error");
      error.id = "flow-error";
      panel.append(
        img,
        link,
        choices,
        error,
        actionRow(interactionButton(t("Close", "关闭"), "cancel")),
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
  }
  function interactionButton(label, action, cls = "", value = "") {
    const b = button(label, null, cls);
    b.dataset.command = action;
    b.dataset.interaction = "true";
    b.onclick = () => command(action, value, true, "flow-error");
    return b;
  }
  $("confirm").addEventListener("cancel", (e) => {
    e.preventDefault();
    command("cancel", "", true);
  });
  $("confirm").addEventListener("keydown", (event) => {
    if (event.key !== "Tab") return;
    const controls = [
      ...$("confirm").querySelectorAll("button, input, a[href], [tabindex]"),
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
  $("auth-form").onsubmit = (e) => {
    e.preventDefault();
    const value = $("token").value.trim();
    if (!value || fetching || sending) return;
    token = value;
    $("token").value = "";
    storage("sessionStorage", "arcana-token", token);
    setText("auth-error", "");
    $("auth-submit").disabled = true;
    poll();
  };
  $("lock").onclick = () => authGate();
  $("retry").onclick = () => poll();
  $("theme").onchange = () => {
    const value = $("theme").value;
    storage("localStorage", "arcana-theme", value);
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
  const fragment = new URLSearchParams(location.hash.slice(1));
  token =
    fragment.get("token") || storage("sessionStorage", "arcana-token") || "";
  if (routes.includes(location.hash.slice(1))) page = location.hash.slice(1);
  if (location.hash)
    history.replaceState(null, "", location.pathname + location.search);
  if (token) storage("sessionStorage", "arcana-token", token);
  localize();
  if (token) poll();
})();
