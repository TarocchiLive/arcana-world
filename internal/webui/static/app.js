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
    notificationRevision = 0;
  let pausedChat = false,
    pausedLogs = false,
    feedSignature = "",
    feedLastID = "",
    logsSignature = "",
    interactionKey = "",
    interactionRevision = 0,
    flowKey = "",
    focusReturn = null,
    focusReturnMessage = "",
    commandFocus = null;
  let helpQuery = "",
    helpTopic = "",
    helpSignature = "";
  const fieldNodes = new Map();
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
    const b = node("button", cls);
    b.type = "button";
    b.append(node("span", "label", text));
    if (action) {
      b.dataset.command = action;
      b.onclick = () => command(action, value);
    }
    return b;
  }
  function setLabel(b, text) {
    const label = b.querySelector(".label");
    if (label && label.textContent !== text) label.textContent = text;
  }
  function actionRow(...buttons) {
    const row = node("div", "actions");
    row.append(...buttons);
    return row;
  }
  function pageHeader(title, description, actions) {
    const h = node("header", "page-header");
    const text = node("div", "page-title");
    text.append(node("h1", "", title));
    if (description) text.append(node("p", "page-description", description));
    h.append(text);
    if (actions) h.append(actions);
    return h;
  }
  function card(title, extra, cls = "") {
    const el = node("section", "card " + cls);
    const head = node("header", "card-head");
    const h = node("h2", "", title);
    head.append(h);
    if (extra) head.append(extra);
    const body = node("div", "card-body");
    el.append(head, body);
    return { el, head, body };
  }
  function helpButton(id) {
    const b = button(t("Guide", "指南"), "", "ghost small");
    b.onclick = () => openHelp(id);
    return b;
  }
  function note(text, cls = "") {
    return node("p", "note " + cls, text);
  }
  function setText(id, text) {
    const n = $(id);
    if (n && n.textContent !== text) n.textContent = text;
  }
  function setPill(id, tone, text) {
    const n = $(id);
    if (!n) return;
    const cls = "pill " + tone;
    if (n.className !== cls) n.className = cls;
    if (n.textContent !== text) n.textContent = text;
  }
  function localize() {
    document.documentElement.lang = language;
    $("navigation").replaceChildren(
      ...routes.map((id) => {
        const a = node("a", "nav-item");
        a.append(node("span", "label", labels()[id]));
        a.href = "#" + id;
        a.onclick = (e) => {
          e.preventDefault();
          navigate(id);
        };
        if (id === page) a.setAttribute("aria-current", "page");
        return a;
      }),
    );
    $("navigation").setAttribute("aria-label", t("Workspace", "工作区"));
    document.querySelector(".skip").textContent = t(
      "Skip to content",
      "跳至主要内容",
    );
    document.querySelector(".brand").setAttribute(
      "aria-label",
      "Arcana World · " + labels().studio,
    );
    setLabel($("retry"), t("Retry now", "立即重试"));
    setLabel($("flow-close"), t("Close", "关闭"));
    const theme = storage("arcana-theme") || "system";
    const themes = [
      ["system", t("System", "系统")],
      ["light", t("Light", "浅色")],
      ["dark", t("Dark", "深色")],
    ];
    $("theme").setAttribute("aria-label", t("Appearance", "外观"));
    $("theme").replaceChildren(
      ...themes.map(([id, label]) => {
        const b = button(label, "", "ghost small");
        b.setAttribute("aria-pressed", String(id === theme));
        b.onclick = () => {
          storage("arcana-theme", id);
          document.documentElement.dataset.theme = id;
          for (const other of $("theme").children)
            other.setAttribute("aria-pressed", String(other === b));
        };
        return b;
      }),
    );
    document.documentElement.dataset.theme = theme;
  }
  function schedule() {
    clearTimeout(timer);
    if (!document.hidden) timer = setTimeout(poll, 1000);
  }
  function renderConnection() {
    const box = node("div", "page connection-page");
    const panel = node("div", "empty-state");
    panel.append(
      node(
        "h1",
        "",
        stale
          ? t("Cannot reach Arcana World", "无法连接 Arcana World")
          : t("Loading studio…", "正在加载工作台…"),
      ),
    );
    if (stale)
      panel.append(
        note(
          t(
            "Check that it is still running. This page retries automatically.",
            "请确认程序仍在运行，页面会自动重试。",
          ),
        ),
      );
    box.append(panel);
    $("main").replaceChildren(box);
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
    if (action.startsWith("set:")) {
      const entry = fieldNodes.get(action.slice(4));
      if (entry) setDirty(entry, false);
    }
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
        action === "cover-upload" ? "/api/cover" : "/api/command",
        {
          page,
          action,
          value,
          revision:
            action === "cover-upload"
              ? state.revision
              : fromInteraction
                ? interactionRevision
                : state.revision,
        },
        own.signal,
      );
      if (current === generation) {
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
      renderStatus();
    } finally {
      clearTimeout(deadline);
      sending = false;
      if (controller === own) controller = null;
      if (state) updateFields();
      updateBusy();
      schedule();
    }
  }
  function accept(next) {
    const nextLanguage = String(next.language || language).startsWith("zh")
      ? "zh"
      : "en";
    const identityChanged =
      state &&
      (state.accountUID !== next.accountUID || state.room?.id !== next.room?.id);
    if (nextLanguage !== language || next.busy || next.interaction || identityChanged)
      hideChatMenu();
    if (nextLanguage !== language) {
      language = nextLanguage;
      pageKey = "";
      flowKey = "";
      localize();
    }
    if (identityChanged) feedSignature = "";
    if (identityChanged || next.interaction?.target !== "room-cover")
      clearCoverDraft();
    if (stale) {
      commandError = "";
      pageKey = "";
      flowKey = "";
    }
    state = next;
    stale = false;
    render();
  }
  function navigate(next) {
    if (!routes.includes(next)) next = "studio";
    if (next === page && pageKey) return;
    hideChatMenu();
    clearCoverDraft();
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
    $("main").scrollTop = 0;
    poll();
  }
  function notify(message, tone = "error") {
    const host = $("notifications");
    if (
      Array.from(host.children).some(
        (toast) => toast.notificationMessage === message,
      )
    )
      return;
    const toast = node("section", "toast toast-" + tone);
    toast.notificationMessage = message;
    toast.setAttribute("role", tone === "error" ? "alert" : "status");
    const content = node("div", "toast-content");
    const summary = message.split(/[:：\n]/, 1)[0];
    const detailed = message.length > 160;
    if (tone === "error")
      content.append(node("strong", "", t("Notice", "提示")));
    content.append(
      node(
        "p",
        "",
        detailed && summary.length < 80
          ? summary
          : detailed
            ? t("See details for the full error.", "展开详情查看完整错误。")
            : message,
      ),
    );
    if (detailed) {
      const details = node("details");
      details.append(
        node("summary", "", t("Error details", "错误详情")),
        node("p", "toast-details", message),
      );
      content.append(details);
    }
    const close = button(t("Close", "关闭"), "", "ghost small");
    close.setAttribute("aria-label", t("Dismiss notification", "关闭通知"));
    close.onclick = () => toast.remove();
    toast.append(content, close);
    // 简短通知自动消失；悬停或聚焦时暂停，详细错误保留到手动关闭。
    if (!detailed) {
      let hide;
      const arm = () => {
        clearTimeout(hide);
        hide = setTimeout(() => toast.remove(), tone === "error" ? 9000 : 2500);
      };
      toast.addEventListener("pointerenter", () => clearTimeout(hide));
      toast.addEventListener("pointerleave", arm);
      toast.addEventListener("focusin", () => clearTimeout(hide));
      toast.addEventListener("focusout", arm);
      arm();
    }
    host.append(toast);
    while (host.children.length > 3) host.firstElementChild.remove();
  }
  function renderIdentity() {
    const root = $("identity");
    const signature = JSON.stringify([language, state?.account, state?.accountUID]);
    if (root.dataset.state === signature) return;
    root.dataset.state = signature;
    root.replaceChildren();
    const [name, uid] = String(state?.account || "").split(" · ");
    const avatar = node("span", "avatar", (name || "?").slice(0, 1).toUpperCase());
    avatar.setAttribute("aria-hidden", "true");
    const text = node("span", "identity-text");
    text.append(
      node("strong", "", state?.accountUID ? name : t("Not signed in", "未登录")),
      node(
        "small",
        "",
        state?.accountUID ? "UID " + (uid || state.accountUID) : t("Open Accounts to sign in", "前往账号页登录"),
      ),
    );
    const link = node("a", "identity-link");
    link.href = "#accounts";
    link.onclick = (e) => {
      e.preventDefault();
      navigate("accounts");
    };
    link.append(avatar, text);
    root.append(link);
  }
  function renderStatus() {
    renderIdentity();
    const room = state?.room;
    setPill(
      "live-pill",
      room?.live ? "live" : room ? "idle" : "off",
      !state
        ? t("Studio", "直播台")
        : !room
          ? t("No room", "房间未载入")
          : room.live
            ? t("On air", "直播中")
            : t("Off air", "未开播"),
    );
    setPill(
      "obs-pill",
      state?.obs ? "good" : state?.obsConnecting ? "wait" : "off",
      state?.obs
        ? t("OBS connected", "OBS 已连接")
        : state?.obsConnecting
          ? t("OBS connecting…", "OBS 连接中…")
          : t("OBS offline", "OBS 未连接"),
    );
    setPill(
      "chat-pill",
      state?.chatConnected ? "good" : "off",
      state?.chatConnected
        ? t("Chat connected", "弹幕已连接")
        : t("Chat offline", "弹幕未连接"),
    );
    setText(
      "status",
      state?.busy
        ? t("Working…", "处理中…")
        : state?.warning
          ? ""
          : state?.status || "",
    );
    setText(
      "connection",
      !state
        ? stale
          ? t("Disconnected", "连接已断开")
          : t("Connecting…", "正在连接…")
        : stale
          ? t("Connection lost · showing last state", "连接中断 · 显示上次状态")
          : "",
    );
    $("connection").hidden = !!state && !stale;
    $("retry").hidden = !stale;
    const live = $("live-action");
    if (!state?.accountUID) {
      setLabel(live, t("Sign in", "登录账号"));
      live.className = "primary";
      delete live.dataset.command;
      delete live.dataset.disabled;
      live.disabled = !state;
      live.onclick = () => navigate("accounts");
    } else {
      setLabel(
        live,
        !room?.id
          ? t("Load room", "载入直播间")
          : room.live
            ? t("End broadcast", "结束直播")
            : t("Go live", "开始直播"),
      );
      live.className = room?.live ? "danger-solid" : "primary";
      live.dataset.command = !room?.id
        ? "refresh"
        : room.live
          ? "stop-confirm"
          : "start-confirm";
      live.dataset.disabled = "false";
      live.onclick = () => command(live.dataset.command);
    }
    const obs = $("obs-action");
    setLabel(
      obs,
      state?.obs
        ? t("Disconnect OBS", "断开 OBS")
        : state?.obsConnecting
          ? t("Connecting…", "连接中…")
          : t("Connect OBS", "连接 OBS"),
    );
    obs.dataset.command = state?.obs ? "obs-disconnect-confirm" : "obs-connect";
    obs.dataset.disabled = String(!!state?.obsConnecting);
    obs.onclick = () => command(obs.dataset.command);
    const host = $("notifications");
    const parent = $("confirm").open
      ? $("confirm")
      : $("flow-dialog").open
        ? $("flow-dialog")
        : document.body;
    if (host.parentElement !== parent) parent.append(host);
    const error =
      commandError || (!state?.busy && state?.warning ? state.status : "");
    const notice = state?.notification;
    const newNotice = notice && notice.revision !== notificationRevision;
    if (error && error !== notificationKey && (!newNotice || error !== notice.text))
      notify(error);
    notificationKey = error;
    if (newNotice) {
      notificationRevision = notice.revision;
      notify(notice.text, notice.warning ? "error" : "info");
    }
  }
  function updateBusy() {
    const busy = !!state?.busy;
    const interaction = !!state?.interaction;
    document.querySelectorAll("[data-command]").forEach((n) => {
      n.disabled = !!(
        !state ||
        n.dataset.disabled === "true" ||
        sending ||
        (busy && n.dataset.command !== "cancel") ||
        (interaction && !n.dataset.interaction && n.dataset.command !== "cancel")
      );
    });
    // 请求进行中仍允许继续输入，避免禁用输入框导致焦点丢失。
    for (const [id, entry] of fieldNodes) {
      const f = (state?.fields || []).find((x) => x.id === id);
      const locked = !!(!state || f?.disabled || busy || interaction);
      entry.input.disabled = locked;
      entry.wrap.classList.toggle("is-disabled", locked);
      if (entry.save) entry.save.disabled = locked || sending;
      if (entry.revert) entry.revert.disabled = locked;
      if (entry.clear) entry.clear.disabled = locked || sending;
      if (entry.words)
        entry.wrap.querySelectorAll("button").forEach((b) => (b.disabled = locked || sending));
    }
    const send = $("chat-send");
    if (send)
      send.disabled = !!(
        busy ||
        sending ||
        interaction ||
        !state?.chatConnected ||
        !$("chat-input")?.value.trim() ||
        Array.from($("chat-input")?.value || "").length > 40
      );
    document.body.classList.toggle("is-busy", busy || sending);
    $("status").classList.toggle("busy", busy || sending);
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
    const key =
      page + ":" + language + ":" + state.accountUID + ":" + state.room?.id +
      (page === "outputs" ? ":" + state.ttsSettingsTitle : "");
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
    if (page === "settings" && tab === "activity") updateLogs();
    if (page === "outputs" && $("tts-model-status"))
      $("tts-model-status").textContent = state.ttsModelText || "";
    if (page === "outputs" && $("tts-model-actions")) {
      const actions = $("tts-model-actions");
      const options = state.ttsModelActions || [];
      const signature = JSON.stringify(options);
      if (actions.dataset.state !== signature) {
        actions.replaceChildren(
          ...options.map((action) => button(action.label, action.id, "ghost")),
        );
        actions.dataset.state = signature;
      }
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

  // ───────────── 字段 ─────────────
  function fieldsRoot(id, cls = "fields") {
    const root = node("div", cls);
    root.id = id;
    return root;
  }
  function renderFields(root, fields) {
    if (!root) return;
    const ids = fields.map((f) => f.id).join("|");
    if (root.dataset.fields !== ids) {
      for (const n of root.querySelectorAll("[data-field]"))
        fieldNodes.delete(n.dataset.field);
      root.replaceChildren(...fields.map(makeField));
      root.dataset.fields = ids;
    }
  }
  function setDirty(entry, dirty) {
    entry.dirty = dirty;
    entry.wrap.classList.toggle("is-dirty", dirty);
    if (entry.save) entry.save.hidden = !dirty;
    if (entry.revert) entry.revert.hidden = !dirty;
  }
  // 词列表：输入框一次可添加多个词，已有词以标签展示并可单独移除；每次提交完整列表。
  function makeWordsField(f) {
    const wrap = node("div", "field field-words");
    wrap.dataset.field = f.id;
    const label = node("label", "field-label", f.label);
    label.htmlFor = "field-" + f.id;
    const hint = node("p", "field-hint", f.hint || "");
    const line = node("div", "field-control");
    const input = node("input");
    input.type = "text";
    input.id = "field-" + f.id;
    input.autocomplete = "off";
    input.spellcheck = false;
    input.placeholder = t(
      "Type a word and press Enter; separate several with commas",
      "输入屏蔽词后回车，多个词用逗号分隔",
    );
    const add = button(t("Add", "添加"), "", "primary small");
    const list = node("ul", "word-list");
    list.setAttribute("aria-label", f.label);
    const count = node("small", "word-count");
    const entry = { wrap, input, label, text: label, save: null, revert: null, clear: null, dirty: false, words: true };
    const current = () => {
      try {
        const value = (state?.fields || []).find((x) => x.id === f.id)?.value;
        return JSON.parse(value || "[]") || [];
      } catch (_) {
        return [];
      }
    };
    const send = async (words) => {
      if (sending || !state || state.busy || state.interaction) return false;
      await command("set:" + f.id, JSON.stringify(words));
      return !commandError;
    };
    const submit = async () => {
      const value = input.value.trim();
      if (!value) return;
      // 原始输入由服务端按分隔符拆分并去重；失败时保留输入，便于修改后重试。
      if (await send([...current(), value])) {
        input.value = "";
        setDirty(entry, false);
        input.focus();
      }
    };
    add.onclick = submit;
    input.addEventListener("input", () => setDirty(entry, input.value.trim() !== ""));
    input.addEventListener("keydown", (e) => {
      if (e.key === "Enter" && !e.isComposing) {
        e.preventDefault();
        submit();
      } else if (e.key === "Escape" && input.value) {
        e.preventDefault();
        e.stopPropagation();
        input.value = "";
        setDirty(entry, false);
      }
    });
    entry.renderWords = (value) => {
      const signature = language + "|" + (value || "[]");
      if (list.dataset.value === signature) return;
      list.dataset.value = signature;
      let words = [];
      try {
        words = JSON.parse(value || "[]") || [];
      } catch (_) {}
      list.replaceChildren(
        ...words.map((word) => {
          const item = node("li", "word-chip");
          const remove = button("×", "", "word-remove");
          remove.setAttribute("aria-label", t("Remove ", "移除 ") + word);
          remove.title = t("Remove", "移除");
          remove.onclick = () => send(current().filter((w) => w !== word));
          item.append(node("span", "word-text", word), remove);
          return item;
        }),
      );
      if (!words.length) list.append(node("li", "word-empty", t("No words yet", "暂无屏蔽词")));
      count.textContent = t(words.length + " words", "共 " + words.length + " 个");
    };
    line.append(input, add);
    const head = node("div", "word-head");
    head.append(label, count);
    wrap.append(head, hint, line, list);
    entry.renderWords(f.value);
    fieldNodes.set(f.id, entry);
    return wrap;
  }
  function makeField(f) {
    if (f.kind === "words") return makeWordsField(f);
    const wrap = node("div", "field field-" + f.kind);
    wrap.dataset.field = f.id;
    const label = node("label", "field-label", f.label);
    label.htmlFor = "field-" + f.id;
    const line = node("div", "field-control");
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
    if (f.kind === "switch") input.setAttribute("role", "switch");
    input.id = "field-" + f.id;
    input.autocomplete = "off";
    input.spellcheck = false;
    const entry = { wrap, input, label, save: null, revert: null, clear: null, dirty: false };
    const value = () =>
      f.kind === "switch" ? String(input.checked) : input.value;
    const commit = () => {
      // 请求进行中无法提交，恢复为服务端值以免显示未保存的状态。
      if (sending || !state) {
        setDirty(entry, false);
        updateFields();
        return;
      }
      command("set:" + f.id, value());
    };
    const revert = () => {
      setDirty(entry, false);
      updateFields();
    };
    if (["select", "switch"].includes(f.kind)) {
      input.addEventListener("change", commit);
    } else if (["number", "range", "color"].includes(f.kind)) {
      // 数值类在完成调整（失焦、回车、松开滑块）时立即保存。
      input.addEventListener("input", () => setDirty(entry, true));
      input.addEventListener("change", () => {
        if (entry.dirty) commit();
      });
      input.addEventListener("keydown", (e) => {
        if (e.key === "Escape" && entry.dirty) {
          e.preventDefault();
          e.stopPropagation();
          revert();
        }
      });
    } else {
      entry.save = button(t("Save", "保存"), "", "primary small");
      entry.save.onclick = commit;
      entry.revert = button(t("Undo", "撤销"), "", "ghost small");
      entry.revert.onclick = () => {
        revert();
        input.focus();
      };
      input.addEventListener("input", () => setDirty(entry, true));
      input.addEventListener("keydown", (e) => {
        if (e.key === "Enter" && !e.isComposing) {
          e.preventDefault();
          if (entry.dirty) commit();
        } else if (e.key === "Escape" && entry.dirty) {
          e.preventDefault();
          e.stopPropagation();
          revert();
        }
      });
    }
    line.append(input);
    if (f.kind === "range") {
      const output = node("output");
      output.htmlFor = input.id;
      output.textContent = f.value;
      line.append(output);
      input.addEventListener("input", () => {
        output.textContent = input.value;
      });
    }
    if (f.kind === "color") {
      const code = node("code", "color-code", f.value);
      line.append(code);
      input.addEventListener("input", () => {
        code.textContent = input.value;
      });
    }
    if (entry.revert) line.append(entry.revert, entry.save);
    if (f.kind === "password" || f.id === "proxy") {
      input.placeholder = t("Enter a new value to replace", "输入新值以替换");
      entry.clear = button(t("Clear", "清空"), "", "ghost small");
      entry.clear.onclick = () => {
        input.value = "";
        setDirty(entry, false);
        command("set:" + f.id, "");
      };
      line.append(entry.clear);
    }
    // 开关整行均为标签，点击文字即可切换。
    if (f.kind === "switch") {
      const track = node("span", "switch-track");
      track.setAttribute("aria-hidden", "true");
      entry.text = node("span", "switch-text", f.label);
      label.replaceChildren(entry.text, input, track);
      wrap.append(label);
    } else {
      entry.text = label;
      wrap.append(label, line);
    }
    setDirty(entry, false);
    fieldNodes.set(f.id, entry);
    return wrap;
  }
  function updateFields() {
    const fields = state?.fields || [];
    if (page === "studio") {
      renderFields($("room-fields"), fields.filter((f) => f.group === "room"));
      renderFields($("blocklist-fields"), fields.filter((f) => f.group === "blocklist"));
      renderFields(
        $("connection-fields"),
        fields.filter((f) => f.group === "connection"),
      );
    }
    if (page === "outputs") {
      renderFields(
        $("output-switches"),
        fields.filter((f) => outputSwitches.includes(f.id)),
      );
      renderFields($("output-fields"), fields.filter(outputFilter));
    }
    if (page === "settings")
      renderFields(
        $("settings-fields"),
        fields.filter((f) => f.group === tab),
      );
    for (const f of fields) {
      const entry = fieldNodes.get(f.id);
      if (!entry) continue;
      const { input } = entry;
      if (entry.text.textContent !== f.label) entry.text.textContent = f.label;
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
          entry.wrap.append(list);
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
      if (f.kind === "words") {
        entry.renderWords(f.value);
        const hint = entry.wrap.querySelector(".field-hint");
        if (hint && hint.textContent !== (f.hint || "")) hint.textContent = f.hint || "";
        continue;
      }
      if (entry.dirty) continue;
      const value = f.value ?? "";
      if (f.kind === "switch") input.checked = value === "true";
      else if (input.value !== value) input.value = value;
      if (f.kind === "range") {
        const output = entry.wrap.querySelector("output");
        if (output) output.textContent = value;
      }
      if (f.kind === "color") {
        const code = entry.wrap.querySelector(".color-code");
        if (code) code.textContent = value;
      }
      for (const key of ["min", "max", "step"]) {
        if (f[key] !== undefined) input.setAttribute(key, f[key]);
        else if (key === "step" && f.kind === "number") input.step = "any";
        else input.removeAttribute(key);
      }
    }
  }

  // ───────────── 直播台 ─────────────
  function buildStudio(root) {
    root.append(
      pageHeader(
        t("Studio", "直播台"),
        t(
          "Prepare the room and OBS. Go live from the top bar on any page.",
          "准备直播间与 OBS；任何页面都可在顶栏开播或停播。",
        ),
        actionRow(button(t("Refresh room", "刷新房间"), "refresh")),
      ),
    );
    const grid = node("div", "studio-grid");
    const room = card(t("Live room", "直播间"), helpButton("room"), "room-card");
    const hero = node("div", "room-hero");
    hero.id = "room-summary";
    const tools = node("div", "tool-grid");
    for (const [action, label] of [
      ["areas", t("Category", "分区")],
      ["cover", t("Cover", "封面")],
      ["delay", t("Delay", "延迟")],
      ["reveal", t("Stream key", "推流信息")],
    ])
      tools.append(button(label, action, "tool"));
    room.body.append(hero, tools, fieldsRoot("room-fields", "fields single"));
    const side = node("div", "studio-side");
    const check = card(t("Ready to go live", "开播准备"), null, "checklist-card");
    const list = node("ul", "checklist");
    list.id = "checklist";
    check.body.append(list);
    const obs = card("OBS Studio", helpButton("obs"), "obs-card");
    obs.body.append(fieldsRoot("connection-fields", "fields"));
    side.append(check.el, obs.el);
    const words = card(t("Block words", "屏蔽词"), null, "blocklist-card");
    words.body.append(note(state?.blocklistRules || ""), fieldsRoot("blocklist-fields", "fields word-fields"));
    const main = node("div", "studio-main");
    main.append(room.el, words.el);
    grid.append(main, side);
    root.append(grid);
  }
  function areaSet(room) {
    return !!room?.area && room.area.replace(/[\s/]/g, "") !== "";
  }
  function updateStudio() {
    const room = state.room || {};
    const summary = $("room-summary");
    const signature = JSON.stringify([
      language,
      room.id,
      room.title,
      room.area,
      room.cover,
      room.coverStatus,
      room.live,
      state.accountUID,
    ]);
    if (summary.dataset.state !== signature) {
      summary.replaceChildren();
      if (room.id) {
        const frame = node("div", "room-cover");
        frame.append(node("span", "placeholder", t("No cover", "暂无封面")));
        const url = safeURL(room.cover, true);
        if (url) {
          const img = node("img");
          img.alt = t("Current room cover", "当前直播间封面");
          img.referrerPolicy = "no-referrer";
          img.hidden = true;
          img.onload = () => {
            img.hidden = false;
          };
          img.onerror = () => img.remove();
          frame.append(img);
          img.src = url;
        }
        if (room.live) frame.append(node("span", "live-tag", t("LIVE", "直播中")));
        const text = node("div", "room-info");
        text.append(
          node("h3", "room-title", room.title || t("Untitled room", "未设置标题")),
        );
        const meta = node("dl", "room-meta");
        for (const [k, v] of [
          [t("Room", "房间号"), String(room.id)],
          [t("Category", "分区"), areaSet(room) ? room.area : t("Not set", "未设置")],
        ]) {
          const row = node("div");
          row.append(node("dt", "", k), node("dd", "", v));
          meta.append(row);
        }
        text.append(meta);
        if (room.coverStatus) text.append(note(room.coverStatus));
        summary.append(frame, text);
      } else {
        const empty = node("div", "empty-inline");
        empty.append(
          note(
            state.accountUID
              ? t(
                  "Room not loaded yet. Refresh to load it.",
                  "尚未载入直播间，刷新后即可编辑。",
                )
              : t(
                  "Sign in to load your room, then connect OBS.",
                  "登录账号后载入直播间，再连接 OBS。",
                ),
          ),
        );
        if (!state.accountUID) {
          const b = button(t("Go to Accounts", "前往账号"));
          b.onclick = () => navigate("accounts");
          empty.append(b);
        } else empty.append(button(t("Refresh room", "刷新房间"), "refresh"));
        summary.append(empty);
      }
      summary.dataset.state = signature;
    }
    document
      .querySelectorAll('.tool[data-command="areas"],.tool[data-command="cover"],.tool[data-command="delay"]')
      .forEach((n) => (n.dataset.disabled = String(!room.id)));
    const reveal = document.querySelector('.tool[data-command="reveal"]');
    if (reveal) reveal.dataset.disabled = String(!state.streamAvailable);
    document.querySelectorAll('[data-command="refresh"]').forEach((b) => {
      b.dataset.disabled = String(!state.accountUID);
    });
    const items = [
      {
        done: !!state.accountUID,
        label: t("Account", "账号"),
        detail: state.accountUID ? state.account : t("Not signed in", "未登录"),
        fix: () => button(t("Sign in", "登录"), "", "small"),
        go: "accounts",
      },
      {
        done: !!room.id,
        label: t("Live room", "直播间"),
        detail: room.id ? t("Room ", "房间 ") + room.id : t("Not loaded", "未载入"),
        fix: () => button(t("Load", "载入"), "refresh", "small"),
        when: !!state.accountUID,
      },
      {
        done: areaSet(room),
        label: t("Category", "分区"),
        detail: areaSet(room) ? room.area : t("Required before going live", "开播前必须设置"),
        fix: () => button(t("Choose", "选择"), "areas", "small"),
        when: !!room.id,
      },
      {
        done: !!state.obs,
        label: "OBS",
        detail: state.obs
          ? t("Connected", "已连接")
          : t("Optional, needed for auto streaming", "可选，自动推流时需要"),
        fix: () => button(t("Connect", "连接"), "obs-connect", "small"),
        when: !state.obsConnecting,
        optional: true,
      },
      {
        done: !!state.chatConnected,
        label: t("Chat", "弹幕"),
        detail: state.chatConnected ? t("Receiving", "接收中") : t("Not connected", "未连接"),
        optional: true,
      },
    ];
    const list = $("checklist");
    const listSignature = JSON.stringify([language, items.map((i) => [i.done, i.detail, i.when])]);
    if (list.dataset.state !== listSignature) {
      list.dataset.state = listSignature;
      list.replaceChildren(
        ...items.map((item) => {
          const li = node("li", item.done ? "done" : item.optional ? "optional" : "todo");
          const mark = node("span", "check-mark");
          const text = node("div", "check-text");
          text.append(node("strong", "", item.label), node("small", "", item.detail));
          li.append(mark, text);
          if (!item.done && item.fix && item.when !== false) {
            const b = item.fix();
            if (item.go) b.onclick = () => navigate(item.go);
            li.append(b);
          }
          return li;
        }),
      );
    }
  }

  // ───────────── 弹幕 ─────────────
  function buildChat(root) {
    const layout = node("div", "chat-layout");
    const panel = node("section", "chat-panel");
    panel.setAttribute("aria-labelledby", "chat-heading");
    const head = node("header", "chat-head");
    const titleBox = node("div", "chat-title");
    const h = node("h1", "", t("Live chat", "实时弹幕"));
    h.id = "chat-heading";
    const room = node("p", "note");
    room.id = "chat-room";
    titleBox.append(h, room);
    const pause = button("", "", "ghost small");
    pause.id = "chat-pause";
    pause.setAttribute("aria-pressed", String(pausedChat));
    const syncPause = () => {
      setLabel(pause, pausedChat ? t("Resume updates", "恢复更新") : t("Pause updates", "暂停更新"));
      pause.setAttribute("aria-pressed", String(pausedChat));
      panel.classList.toggle("is-paused", pausedChat);
    };
    pause.onclick = () => {
      pausedChat = !pausedChat;
      syncPause();
      setText(
        "chat-pause-note",
        pausedChat
          ? t("Updates paused; receiving continues.", "已暂停显示，后台仍在接收。")
          : "",
      );
      if (!pausedChat) updateMessages();
    };
    syncPause();
    head.append(titleBox, actionRow(pause, helpButton("chat")));
    const feedWrap = node("div", "feed-wrap");
    const feed = node("div", "feed");
    feed.id = "chat-feed";
    feed.setAttribute("role", "log");
    feed.setAttribute("aria-label", t("Live messages", "实时弹幕"));
    feed.setAttribute("aria-live", "off");
    feed.tabIndex = 0;
    const jump = button(t("Latest messages", "最新消息"), "", "jump");
    jump.id = "feed-jump";
    jump.hidden = true;
    jump.onclick = () => {
      feed.scrollTop = feed.scrollHeight;
      jump.hidden = true;
    };
    feed.addEventListener("scroll", () => {
      if (feed.scrollHeight - feed.scrollTop - feed.clientHeight < 70) jump.hidden = true;
    });
    const paused = node("p", "paused-note");
    paused.id = "chat-pause-note";
    paused.setAttribute("role", "status");
    feedWrap.append(feed, jump, paused);
    const composer = node("form", "composer");
    composer.id = "chat-form";
    const label = node("label", "sr-only", t("Send to your live room", "发送到直播间"));
    label.htmlFor = "chat-input";
    const input = node("textarea");
    input.id = "chat-input";
    input.rows = 1;
    input.setAttribute("aria-describedby", "chat-note");
    input.oninput = updateComposer;
    input.onkeydown = (e) => {
      if (e.key === "Enter" && !e.shiftKey && !e.isComposing) {
        e.preventDefault();
        composer.requestSubmit();
      }
    };
    const counter = node("span", "counter");
    counter.id = "chat-note";
    const send = button(t("Send", "发送"), null, "primary");
    send.type = "submit";
    send.id = "chat-send";
    const box = node("div", "composer-box");
    box.append(input, counter, send);
    composer.append(label, box);
    composer.onsubmit = (e) => {
      e.preventDefault();
      if (send.disabled) return;
      const value = input.value;
      input.value = "";
      command("send", value);
      updateComposer();
    };
    panel.append(head, feedWrap, composer);
    const side = node("aside", "chat-side");
    side.setAttribute("aria-label", t("Room tools", "房间工具"));
    const stats = node("div", "chat-stats");
    const audience = node("p", "audience");
    audience.id = "chat-audience";
    stats.append(audience);
    const toolsTitle = node("h2", "side-title", t("Viewers & moderation", "观众与管理"));
    const tools = node("div", "chat-tools");
    for (const [action, text] of [
      ["audience", t("Audience", "观众")],
      ["fleet", t("Fleet", "大航海")],
      ["moderation-admins", t("Moderators", "房管")],
      ["moderation-blocks", t("Blacklist", "黑名单")],
      ["moderation-mutes", t("Muted users", "禁言名单")],
    ])
      tools.append(button(text, action, "tool-row"));
    side.append(stats, toolsTitle, tools);
    layout.append(panel, side);
    root.append(layout);
  }
  function updateChat() {
    setText(
      "chat-room",
      state.room
        ? `${state.room.title || t("Untitled room", "未设置标题")} · ${t("Room ", "房间 ")}${state.room.id}`
        : t(
            "Load your room in Studio to receive and send messages.",
            "请先在直播台载入房间，再收发弹幕。",
          ),
    );
    setText("chat-audience", state.audience || "");
    document.querySelectorAll(".chat-tools [data-command]").forEach((control) => {
      control.dataset.disabled = String(!state.accountUID || !state.room);
    });
    setText(
      "chat-pause-note",
      pausedChat
        ? t("Updates paused; receiving continues.", "已暂停显示，后台仍在接收。")
        : "",
    );
    updateMessages();
    updateComposer();
  }
  function updateComposer() {
    const input = $("chat-input");
    if (!input) return;
    const length = Array.from(input.value).length;
    setText("chat-note", length + " / 40");
    $("chat-note").classList.toggle("over", length > 40);
    input.placeholder = !state?.room
      ? t("Load your room first", "请先载入直播间")
      : !state.chatConnected
        ? t("Chat is not connected", "弹幕未连接")
        : t("Message · Enter to send", "发送弹幕，Enter 发送");
    input.style.height = "auto";
    input.style.height = Math.min(input.scrollHeight, 120) + "px";
    updateBusy();
  }
  const messageKinds = {
    chat: ["Chat", "弹幕"],
    gift: ["Gift", "礼物"],
    sc: ["Super Chat", "醒目留言"],
    guard: ["Guard", "大航海"],
    enter: ["Joined", "进场"],
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
  function messageTone(kind) {
    if (kind === "chat") return "chat";
    if (kind === "sc" || kind === "guard") return "highlight";
    if (["gift", "follow", "mutual_follow", "special_follow", "share", "live"].includes(kind))
      return "positive";
    if (kind === "cut_off" || kind === "room_block") return "warning";
    return "system";
  }
  let chatMenu = null;
  const messageUserKinds = new Set([
    "chat", "gift", "sc", "guard", "enter", "follow", "mutual_follow",
    "special_follow", "share", "like", "room_block", "detail",
  ]);
  function messageUID(m) {
    return !m.mystery &&
      messageUserKinds.has(m.kind) &&
      typeof m.uid === "string" &&
      /^[1-9]\d*$/.test(m.uid)
      ? m.uid
      : "";
  }
  function messageMenuTrigger(id) {
    return Array.from($("chat-feed")?.children || [])
      .find((row) => row.dataset.messageId === String(id))
      ?.querySelector(".msg-more");
  }
  function hideChatMenu({ restoreFocus = false } = {}) {
    if (!chatMenu) return;
    const current = chatMenu;
    chatMenu = null;
    current.listeners.abort();
    current.menu.remove();
    const trigger = messageMenuTrigger(current.message.id) || current.trigger;
    trigger?.setAttribute("aria-expanded", "false");
    trigger?.closest(".msg")?.classList.remove("menu-open");
    if (restoreFocus && trigger?.isConnected) trigger.focus({ preventScroll: true });
  }
  async function copyText(value, fallback) {
    try {
      await navigator.clipboard.writeText(value);
      if (fallback?.isConnected && fallback.value === value) {
        fallback.hidden = true;
        setText("verification-status", t("Link copied.", "链接已复制。"));
      } else notify(t("Copied.", "已复制。"), "info");
    } catch (_) {
      if (fallback?.isConnected) {
        fallback.hidden = false;
        fallback.value = value;
        fallback.focus();
        fallback.select();
        setText(
          "verification-status",
          t("Select and copy this link manually.", "请选中并手动复制此链接。"),
        );
      } else
        notify(
          t(
            "Could not copy. Check clipboard permission in your browser.",
            "复制失败，请检查浏览器的剪贴板权限。",
          ),
        );
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
    chatMenu = {
      menu,
      trigger,
      message: m,
      listeners,
      feedScrollTop: $("chat-feed")?.scrollTop,
    };
    trigger.setAttribute("aria-expanded", "true");
    trigger.closest(".msg")?.classList.add("menu-open");
    const add = (label, run, cls = "") => {
      const item = button(label, "", "context-menu-item " + cls);
      item.setAttribute("role", "menuitem");
      item.tabIndex = -1;
      item.onclick = () => {
        hideChatMenu({ restoreFocus: true });
        run();
      };
      menu.append(item);
    };
    add(t("Copy message", "复制正文"), () => copyText(m.text || ""));
    const uid = messageUID(m);
    if (uid) {
      add(t("Copy UID", "复制 UID"), () => copyText(uid));
      add(t("View user", "查看用户"), () => command("speaker-open", m.id));
      const link = node("a", "context-menu-item");
      link.append(node("span", "label", t("Open homepage", "打开主页")));
      link.href = "https://space.bilibili.com/" + uid;
      link.target = "_blank";
      link.rel = "noopener noreferrer";
      link.tabIndex = -1;
      link.setAttribute("role", "menuitem");
      link.onclick = () => hideChatMenu({ restoreFocus: true });
      menu.append(link);
      menu.append(node("hr"));
      add(t("Mute", "禁言"), () => command("speaker-mute", m.id), "danger");
      add(t("Add to blacklist", "加入黑名单"), () => command("speaker-block", m.id), "danger");
      add(t("Set as moderator", "设为房管"), () => command("speaker-admin", m.id));
    }
    document.body.append(menu);
    const rect = trigger.getBoundingClientRect();
    const pointer =
      event?.type === "contextmenu" && (event.clientX || event.clientY);
    const x = pointer ? event.clientX : rect.right - menu.offsetWidth;
    const y = pointer ? event.clientY : rect.bottom + 4;
    menu.style.left =
      Math.max(8, Math.min(x, window.innerWidth - menu.offsetWidth - 8)) + "px";
    menu.style.top =
      Math.max(8, Math.min(y, window.innerHeight - menu.offsetHeight - 8)) + "px";
    const items = Array.from(menu.querySelectorAll('[role="menuitem"]'));
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
    document.addEventListener(
      "keydown",
      (e) => {
        if (e.key !== "Escape") return;
        e.preventDefault();
        e.stopPropagation();
        hideChatMenu({ restoreFocus: true });
      },
      options,
    );
    document.addEventListener(
      "pointerdown",
      (e) => {
        if (!menu.contains(e.target) && !trigger.contains(e.target)) hideChatMenu();
      },
      options,
    );
    document.addEventListener(
      "focusin",
      (e) => {
        if (!menu.contains(e.target)) hideChatMenu();
      },
      options,
    );
    document.addEventListener(
      "scroll",
      (e) => {
        // 新消息的自动跟随不关闭菜单，读者主动滚动才关闭。
        if (e.target === $("chat-feed") && e.target.scrollTop === chatMenu?.feedScrollTop)
          return;
        if (!menu.contains(e.target)) hideChatMenu({ restoreFocus: true });
      },
      options,
    );
    window.addEventListener(
      "resize",
      () => hideChatMenu({ restoreFocus: true }),
      options,
    );
  }
  function bindMessageMenu(row, m) {
    if (!m.id) return;
    const trigger = button(t("Actions", "操作"), "", "ghost small");
    trigger.setAttribute(
      "aria-label",
      t("Message actions", "消息操作") + (m.name ? ": " + m.name : ""),
    );
    trigger.classList.add("msg-more");
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
    row.append(trigger);
  }
  function messageRow(m) {
    const tone = messageTone(m.kind);
    const row = node("article", "msg msg-" + tone);
    row.dataset.messageId = m.id;
    if (m.deleted) row.classList.add("msg-deleted");
    const time = node("time", "msg-time");
    const date = new Date(m.time);
    const valid = !Number.isNaN(date.getTime());
    time.textContent = valid
      ? date.toLocaleTimeString([], {
          hour: "2-digit",
          minute: "2-digit",
          second: "2-digit",
          hour12: false,
        })
      : "";
    time.dateTime = m.time || "";
    time.title = valid ? date.toLocaleString() : "";
    const main = node("div", "msg-main");
    const uid = messageUID(m);
    if (m.kind === "chat") {
      let name;
      if (uid) {
        name = button(m.name || "UID " + uid, "speaker-open", "msg-name", m.id);
        name.title = t("View user", "查看用户") + " · UID " + uid;
      } else name = node("span", "msg-name", m.name || t("System", "系统"));
      main.append(name);
    } else {
      const label = Object.hasOwn(messageKinds, m.kind)
        ? t(...messageKinds[m.kind])
        : t("Event", "事件");
      main.append(node("span", "chip chip-kind", label));
    }
    if (m.mystery) main.append(node("span", "chip", t("Mystery user", "神秘用户")));
    if (m.medalName && m.medalLevel > 0) {
      const medal = node("span", "chip chip-medal");
      medal.append(node("span", "", m.medalName), node("b", "", String(m.medalLevel)));
      medal.title = m.medalName + " · Lv." + m.medalLevel;
      main.append(medal);
    }
    if (m.deleted) main.append(node("span", "chip", t("Removed", "已撤回")));
    main.append(node("span", "msg-text", m.text || ""));
    row.append(time, main);
    bindMessageMenu(row, m);
    return row;
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
    const first = !feedSignature;
    feedSignature = signature;
    const feed = $("chat-feed");
    const atEnd = feed.scrollHeight - feed.scrollTop - feed.clientHeight < 70;
    const scrollTop = feed.scrollTop;
    const feedTop = feed.getBoundingClientRect().top;
    const anchor = atEnd
      ? null
      : Array.from(feed.children).find(
          (row) =>
            row.dataset.messageId && row.getBoundingClientRect().bottom > feedTop,
        );
    const anchorID = anchor?.dataset.messageId;
    const anchorOffset = anchor ? anchor.getBoundingClientRect().top - feedTop : 0;
    const focusedMessage = document.activeElement?.closest(".msg")?.dataset.messageId;
    const lastID = messages.at(-1)?.id || "";
    feed.replaceChildren();
    if (!messages.length) {
      const empty = node("div", "empty-state compact");
      empty.append(
        node(
          "p",
          "",
          state.room
            ? t(
                "Your audience appears here. Keep this page open while you run the show.",
                "观众消息会显示在这里。开播时保持此页打开，不错过每一次互动。",
              )
            : t("Load your room in Studio first.", "请先在直播台载入直播间。"),
        ),
      );
      feed.append(empty);
    } else for (const m of messages) feed.append(messageRow(m));
    if (atEnd || first) feed.scrollTop = feed.scrollHeight;
    else {
      const restored =
        anchorID &&
        Array.from(feed.children).find((row) => row.dataset.messageId === anchorID);
      feed.scrollTop = restored
        ? feed.scrollTop + restored.getBoundingClientRect().top - feedTop - anchorOffset
        : scrollTop;
      if (lastID && lastID !== feedLastID) $("feed-jump").hidden = false;
    }
    feedLastID = lastID;
    if (chatMenu) chatMenu.feedScrollTop = feed.scrollTop;
    if (focusedMessage)
      (messageMenuTrigger(focusedMessage) || feed).focus({ preventScroll: true });
  }
  function renderChatMembers(root, i) {
    const members = i.members || {};
    let view = root.chatMembersView;
    if (!view || view.id !== i.id || view.language !== language || !root.contains(view.section)) {
      const section = node("section", "members");
      const bar = node("div", "members-bar");
      const status = node("p", "members-status");
      status.setAttribute("role", "status");
      const refresh = button(t("Refresh", "刷新"), "", "small");
      refresh.dataset.command = "members-refresh";
      refresh.dataset.interaction = "true";
      refresh.onclick = () => command("members-refresh", "", true);
      bar.append(status, refresh);
      const viewport = node("div", "members-scroll");
      viewport.tabIndex = 0;
      viewport.setAttribute("role", "region");
      const table = node("table", "members-table");
      const head = node("thead"),
        row = node("tr"),
        body = node("tbody");
      for (const text of [
        t("Rank", "排名"),
        t("Viewer", "观众"),
        t("Guard", "大航海"),
        t("Contribution", "贡献"),
        "UID",
        t("Medal", "粉丝牌"),
      ]) {
        const cell = node("th", "", text);
        cell.scope = "col";
        row.append(cell);
      }
      head.append(row);
      table.append(head, body);
      viewport.append(table);
      const empty = node("p", "members-empty");
      section.append(bar, viewport, empty);
      root.replaceChildren(section);
      view = { id: i.id, language, section, status, refresh, viewport, body, empty, entries: "" };
      root.chatMembersView = view;
    }
    view.viewport.setAttribute("aria-label", i.title || t("Viewers", "观众"));
    // 禁用原因交给统一的 busy 更新合并，避免随后被重新启用。
    view.refresh.dataset.disabled = String(!!members.loading || stale);
    view.section.setAttribute("aria-busy", String(!!members.loading));
    const total = members.total ?? 0;
    view.status.textContent = [
      t("Total: ", "共 ") + total + t("", " 人"),
      t("Top 20", "前 20 名"),
      members.loading ? t("Loading…", "正在加载…") : "",
      members.previous ? t("Showing previous data", "显示上次数据") : "",
    ]
      .filter(Boolean)
      .join(" · ");
    const entries = members.entries || [];
    view.empty.hidden = entries.length > 0;
    view.viewport.hidden = !entries.length;
    view.empty.textContent = members.loading
      ? t("Loading…", "正在加载…")
      : members.loaded
        ? t("No viewers to display", "暂无观众")
        : t("No data loaded. Refresh to try again.", "尚未加载数据，请刷新重试。");
    const signature = JSON.stringify(entries);
    if (signature === view.entries) return;
    view.entries = signature;
    const scrollTop = view.viewport.scrollTop,
      scrollLeft = view.viewport.scrollLeft;
    view.body.replaceChildren();
    const columns = Array.from(
      view.body.closest("table").querySelectorAll("th"),
      (cell) => cell.textContent,
    );
    for (const member of entries) {
      const row = node("tr");
      const uid =
        !member.mystery && /^[1-9]\d*$/.test(member.uid || "") ? member.uid : "";
      for (const [index, text] of [
        member.rank ?? "",
        member.name || t("Mystery user", "神秘用户"),
        member.role || "",
        member.score || "",
        member.mystery ? t("Mystery user", "神秘用户") : uid,
        member.medalName && member.medalLevel > 0
          ? member.medalName + " · Lv." + member.medalLevel
          : "",
      ].entries()) {
        const cell = node("td", "", text);
        cell.dataset.label = columns[index];
        row.append(cell);
      }
      view.body.append(row);
    }
    view.viewport.scrollTop = scrollTop;
    view.viewport.scrollLeft = scrollLeft;
  }

  // ───────────── 分栏与输出 ─────────────
  function subnav(items, label) {
    const root = node("div", "subnav");
    root.setAttribute("role", "tablist");
    root.setAttribute("aria-label", label);
    root.setAttribute("aria-orientation", "vertical");
    for (const item of items) {
      if (item.group) {
        root.append(node("p", "subnav-group", item.group));
        continue;
      }
      const [id, text] = item;
      const b = button(text, "", "subnav-item");
      b.setAttribute("role", "tab");
      b.setAttribute("aria-selected", String(tab === id));
      b.tabIndex = tab === id ? 0 : -1;
      b.onclick = () => switchTab(id);
      b.onkeydown = (e) => {
        const ids = items.filter((x) => !x.group).map((x) => x[0]);
        const index = ids.indexOf(id);
        let next;
        if (e.key === "ArrowDown" || e.key === "ArrowRight") next = (index + 1) % ids.length;
        if (e.key === "ArrowUp" || e.key === "ArrowLeft")
          next = (index + ids.length - 1) % ids.length;
        if (e.key === "Home") next = 0;
        if (e.key === "End") next = ids.length - 1;
        if (next !== undefined) {
          e.preventDefault();
          switchTab(ids[next]);
          document.querySelector('.subnav [aria-selected="true"]')?.focus();
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
  const outputSwitches = ["overlay-enabled", "overlay-preview", "tts-enabled"];
  function buildOutputs(root) {
    if (!tab) tab = "layout";
    root.append(
      pageHeader(
        t("Outputs", "输出"),
        t(
          "Overlay and voice play on the computer running Arcana World. Preview shows samples there until you turn it off or leave this page.",
          "浮层和语音在运行 Arcana World 的电脑上输出；示例预览在关闭或离开本页后恢复实际内容。",
        ),
        actionRow(button(t("Output guide", "输出指南"), "", "ghost")),
      ),
    );
    root.querySelector(".page-header .actions button").onclick = () => openHelp("outputs");
    root.append(fieldsRoot("output-switches", "switch-tiles"));
    const tabs = [
      { group: t("Overlay", "浮层") },
      ["layout", t("Position & size", "位置与尺寸")],
      ["spacing", t("Padding", "内边距")],
      ["type", t("Typography", "文字")],
      ["color", t("Colors", "配色")],
      ["events", t("Overlay events", "弹幕姬事件")],
      { group: t("Voice", "语音") },
      ["voice", t("Voice", "声音")],
      ["voice-settings", state.ttsSettingsTitle],
      ["voice-events", t("Voice events", "语音事件")],
    ];
    const split = node("div", "split");
    const panel = node("section", "split-panel");
    const current = tabs.find((x) => x[0] === tab);
    panel.append(node("h2", "split-title", current ? current[1] : ""));
    panel.append(
      fieldsRoot("output-fields", tab.includes("events") ? "fields event-fields" : "fields"),
    );
    const actions = actionRow();
    if (tab === "layout")
      actions.append(
        button(t("Choose displays", "选择显示器"), "overlay-displays"),
        button(t("Restore defaults", "恢复默认值"), "overlay-restore-confirm", "ghost"),
      );
    if (tab === "voice")
      actions.append(
        button(t("Preview voice", "试听语音"), "tts-preview"),
        button(t("Stop preview", "停止试听"), "tts-stop", "ghost"),
      );
    if (tab === "voice-settings") {
      actions.id = "tts-model-actions";
      if (state.ttsModelText) {
        const status = node("pre", "note", state.ttsModelText);
        status.id = "tts-model-status";
        panel.append(status);
      }
    }
    if (actions.children.length || tab === "voice-settings") panel.append(actions);
    if (tab.startsWith("voice"))
      panel.append(
        note(
          t(
            "Voice preview plays on the computer running Arcana World, not in this browser.",
            "试听在运行 Arcana World 的电脑上播放，而不是当前浏览器。",
          ),
        ),
      );
    split.append(subnav(tabs, t("Output sections", "输出分类")), panel);
    root.append(split);
  }
  function outputFilter(f) {
    if (outputSwitches.includes(f.id)) return false;
    if (tab === "voice-events") return f.id.startsWith("tts-event:");
    if (tab === "events") return f.id.startsWith("overlay-event:");
    if (tab === "voice-settings") return f.group === "tts-provider-settings";
    if (tab === "voice") return f.group === "tts";
    if (!f.id.startsWith("overlay-") || f.id.startsWith("overlay-event:")) return false;
    if (tab === "type") return f.group === "overlay-type";
    if (tab === "color") return f.group === "overlay-color";
    if (tab === "spacing") return f.id.startsWith("overlay-padding-");
    return (
      !["overlay-type", "overlay-color"].includes(f.group) &&
      !f.id.startsWith("overlay-padding-")
    );
  }

  // ───────────── 账号 ─────────────
  function buildAccounts(root) {
    root.append(
      pageHeader(
        t("Accounts", "账号"),
        t(
          "Sign in by scanning a QR code with the Bilibili app. Each account keeps its own room state.",
          "使用哔哩哔哩 App 扫码登录；切换账号时，各自的直播间状态相互隔离。",
        ),
        actionRow(button(t("Sign in with QR", "扫码登录"), "login", "primary")),
      ),
    );
    const list = node("section", "account-list");
    list.id = "account-list";
    list.setAttribute("aria-label", t("Saved accounts", "已保存账号"));
    root.append(list);
  }
  function updateAccounts() {
    const root = $("account-list");
    const signature = JSON.stringify([language, state.accounts || []]);
    if (root.dataset.state === signature) return;
    root.replaceChildren();
    if (!(state.accounts || []).length) {
      const empty = node("div", "empty-state");
      empty.append(
        node("h2", "", t("No saved accounts", "尚未保存账号")),
        note(
          t(
            "Choose “Sign in with QR” and scan with the Bilibili app.",
            "点击「扫码登录」，使用哔哩哔哩手机客户端扫码。",
          ),
        ),
        button(t("Sign in with QR", "扫码登录"), "login", "primary"),
      );
      root.append(empty);
    }
    for (const a of state.accounts || []) {
      const row = node("article", "account-card" + (a.active ? " active" : ""));
      const avatar = node("span", "avatar large", (a.name || "?").slice(0, 1).toUpperCase());
      avatar.setAttribute("aria-hidden", "true");
      const name = node("div", "account-name");
      name.append(node("strong", "", a.name), node("small", "", "UID " + a.uid));
      row.append(avatar, name);
      if (a.active) row.append(node("span", "pill good", t("In use", "使用中")));
      else row.append(button(t("Switch", "切换"), "account:" + a.uid));
      const remove = button(t("Remove", "移除"), "delete:" + a.uid, "ghost small");
      remove.setAttribute("aria-label", t("Remove ", "移除 ") + a.name);
      remove.classList.add("danger-ghost");
      row.append(remove);
      root.append(row);
    }
    root.dataset.state = signature;
  }

  // ───────────── 设置 ─────────────
  function buildSettings(root) {
    if (!tab) tab = "connection";
    root.append(pageHeader(t("Settings", "设置")));
    const tabs = [
      ["connection", t("Connection", "连接")],
      ["behavior", t("Behavior", "行为")],
      ["maintenance", t("Maintenance", "维护")],
      ["activity", t("Activity", "操作记录")],
    ];
    const split = node("div", "split");
    const panel = node("section", "split-panel");
    panel.append(node("h2", "split-title", tabs.find((x) => x[0] === tab)[1]));
    if (["connection", "behavior"].includes(tab)) {
      panel.append(fieldsRoot("settings-fields", "fields"));
      if (tab === "connection")
        panel.append(
          note(
            t(
              "Passwords are never sent back to this browser; leave them blank unless replacing. Startup overrides are read-only.",
              "密码不会回传到浏览器，无需修改时保持为空。启动参数覆盖的设置为只读。",
            ),
          ),
        );
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
            "Choose stored data to remove. Removing account data affects your next sign-in.",
            "选择需要清理的本地数据。清理账号数据会影响下次登录。",
          ),
          t("Choose data…", "选择数据…"),
          "clear-data",
        ],
      ]) {
        const s = node("section", "danger-zone");
        s.append(node("h3", "", title), note(text), button(label, action, "danger"));
        sections.append(s);
      }
      panel.append(sections);
    }
    if (tab === "activity") {
      const pause = button(
        pausedLogs ? t("Resume updates", "恢复更新") : t("Pause updates", "暂停更新"),
        "",
        "small",
      );
      pause.onclick = () => {
        pausedLogs = !pausedLogs;
        setLabel(pause, pausedLogs ? t("Resume updates", "恢复更新") : t("Pause updates", "暂停更新"));
        setText(
          "logs-note",
          pausedLogs
            ? t(
                "Updates paused. Displayed entries may be out of date.",
                "更新已暂停，显示内容可能不是最新。",
              )
            : logsNote(),
        );
        if (!pausedLogs) updateLogs();
      };
      const copy = button(t("Copy records", "复制记录"), "", "small");
      copy.onclick = () => copyText((state?.logs || []).join("\n"));
      const help = button(t("Troubleshooting", "故障排查"), "", "small ghost");
      help.onclick = () => openHelp("troubleshooting");
      const n = note(logsNote());
      n.id = "logs-note";
      const logs = node("div", "logs");
      logs.id = "logs";
      logs.tabIndex = 0;
      logs.setAttribute("role", "log");
      logs.setAttribute("aria-label", t("Activity", "操作记录"));
      panel.append(actionRow(pause, copy, help), n, logs);
    }
    split.append(subnav(tabs, t("Settings sections", "设置分类")), panel);
    root.append(split);
  }
  function logsNote() {
    return t(
      "Recent operations, last 200 entries. Detailed diagnostics are in logs/arcana-world.log in the data directory. Review before sharing: records can contain private room information.",
      "最近 200 条操作。详细诊断信息位于数据目录下的 logs/arcana-world.log。记录可能包含房间隐私信息，分享前请检查。",
    );
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
              : [l.time, l.level, l.text || l.message].filter(Boolean).join(" "),
          )
          .join("\n")
      : t("No operations recorded.", "暂无操作记录。");
    if (end) root.scrollTop = root.scrollHeight;
  }

  // ───────────── 帮助 ─────────────
  function buildHelp(root) {
    root.append(pageHeader(t("Guide", "使用指南")));
    const split = node("div", "split help-split");
    const list = node("aside", "help-list");
    const label = node("label", "sr-only", t("Search help", "搜索帮助"));
    label.htmlFor = "help-search";
    const search = node("div", "search");
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
    search.append(input);
    const topics = node("div", "subnav help-topics");
    topics.id = "help-topics";
    const selector = node("select", "help-select");
    selector.id = "help-select";
    selector.setAttribute("aria-label", t("Help topic", "帮助主题"));
    selector.onchange = () => {
      helpTopic = selector.value;
      helpSignature = "";
      updateHelp();
    };
    list.append(label, search, selector, topics);
    const article = node("article", "split-panel article");
    article.id = "help-article";
    article.tabIndex = 0;
    split.append(list, article);
    root.append(split);
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
      (a.title + " " + a.summary + " " + a.body).toLocaleLowerCase().includes(q),
    );
    const active = matches.find((a) => a.id === helpTopic) || matches[0];
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
      const b = node("button", "subnav-item help-topic");
      b.type = "button";
      b.append(node("span", "label", a.title), node("small", "", a.summary));
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
        note(t("Try another search or clear the search field.", "尝试其他关键词，或清空搜索框。")),
      );
  }

  // ───────────── 交互弹窗 ─────────────
  function interactionPage(i) {
    if (i.target?.startsWith("account") || i.kind === "qr") return "accounts";
    if (i.target?.startsWith("chat-")) return "chat";
    if (i.target?.startsWith("overlay")) return "outputs";
    if (i.target === "maintenance") return "settings";
    return "studio";
  }
  function interactionButton(label, action, cls = "", value = "") {
    const b = button(label, null, cls, value);
    b.dataset.command = action;
    b.dataset.interaction = "true";
    b.onclick = () => command(action, value, true);
    return b;
  }
  function restoreFocus() {
    const previous = focusReturn;
    const message = focusReturnMessage;
    focusReturn = null;
    focusReturnMessage = "";
    queueMicrotask(() => {
      if ($("flow-dialog").open || $("confirm").open) return;
      const target = previous?.isConnected ? previous : messageMenuTrigger(message);
      if (target && !target.disabled) target.focus({ preventScroll: true });
      else $("main").focus({ preventScroll: true });
    });
  }
  function renderInteraction() {
    const i = state.interaction;
    const owner = i ? interactionPage(i) : "";
    const key = i
      ? state.accountUID + ":" + state.room?.id + ":" + i.id + ":" + i.kind
      : "";
    const changed = key !== interactionKey;
    if (changed) {
      // 仅在交互开始时记录入口，多步流程结束后焦点回到最初的控件。
      if (i && !interactionKey) {
        focusReturn = commandFocus?.isConnected ? commandFocus : document.activeElement;
        focusReturnMessage = focusReturn?.closest?.(".msg")?.dataset.messageId || "";
      }
      interactionKey = key;
    }
    commandFocus = null;
    interactionRevision = i ? i.revision || state.revision : 0;
    const confirm = $("confirm");
    const flow = $("flow-dialog");
    const flowOpen = i && i.kind !== "confirm" && owner === page;
    if (!flowOpen && flow.open) flow.close();
    if (!flowOpen) {
      flowKey = "";
      if ($("flow-body").childElementCount) {
        $("flow-body").replaceChildren();
        $("flow-foot").replaceChildren();
      }
    }
    if ((!i || i.kind !== "confirm") && confirm.open) confirm.close();
    const pending = $("pending");
    pending.hidden = !(i && i.kind !== "confirm" && owner !== page);
    if (!i) {
      if (focusReturn || focusReturnMessage) restoreFocus();
      return;
    }
    if (i.kind === "confirm") {
      renderConfirm(i, changed);
      return;
    }
    if (owner !== page) {
      const signature = JSON.stringify([language, i.title, owner]);
      if (pending.dataset.state !== signature) {
        pending.dataset.state = signature;
        pending.replaceChildren(node("span", "pending-text", i.title));
        const go = button(t("Continue in ", "前往") + labels()[owner], "", "small primary");
        go.onclick = () => navigate(owner);
        pending.append(go, interactionButton(t("Close", "关闭"), "cancel", "small ghost"));
      }
      return;
    }
    renderFlow(i, changed);
  }
  function renderConfirm(i, changed) {
    const modal = $("confirm");
    if (!changed && modal.open) return;
    setText("confirm-title", i.title);
    setText("confirm-text", i.text || "");
    $("confirm-text").hidden = !i.text || i.text === i.title;
    $("confirm-image")?.remove();
    const source = safeURL(i.image, true);
    if (source) {
      const image = node("img", "confirm-image");
      image.id = "confirm-image";
      image.alt = i.title;
      image.src = source;
      modal.querySelector(".sheet-body").append(image);
    }
    const actions = $("confirm-actions");
    actions.replaceChildren();
    const cancel = interactionButton(t("Cancel", "取消"), "cancel", "ghost");
    actions.append(cancel);
    for (const c of i.choices || [])
      actions.append(interactionButton(c.label, c.id, "primary", c.id));
    if (!modal.open) modal.showModal();
    cancel.focus();
  }
  function renderFlow(i, changed) {
    const dialog = $("flow-dialog");
    const opening = !dialog.open;
    const key = interactionKey + ":" + language;
    if (flowKey !== key) {
      flowKey = key;
      buildFlow(i);
    }
    updateFlow(i);
    if (opening) dialog.showModal();
    if (opening || changed)
      queueMicrotask(() => {
        if (!dialog.open || $("confirm").open) return;
        const control =
          dialog.querySelector("#flow-input:not(:disabled)") ||
          dialog.querySelector(".choice:not(:disabled)") ||
          dialog.querySelector('.sheet-foot [data-command="cancel"]');
        control?.focus({ preventScroll: true });
      });
  }
  function buildFlow(i) {
    const dialog = $("flow-dialog");
    const body = $("flow-body");
    const foot = $("flow-foot");
    dialog.dataset.kind = i.kind;
    dialog.dataset.target = i.target || "";
    body.chatMembersView = null;
    body.replaceChildren();
    foot.replaceChildren();
    const close = $("flow-close");
    close.dataset.command = "cancel";
    close.dataset.interaction = "true";
    const back = interactionButton(t("Back", "返回"), "back", "ghost");
    back.id = "flow-back";
    foot.append(back, node("span", "spacer"));
    foot.append(interactionButton(t("Close", "关闭"), "cancel", "ghost"));
    if (i.kind === "members") return;
    if (i.kind === "speaker") {
      const head = node("div", "speaker");
      const img = node("img", "speaker-avatar");
      img.id = "flow-image";
      img.alt = "";
      img.hidden = true;
      const text = node("p", "speaker-text");
      text.id = "flow-text";
      head.append(img, text);
      const link = node("a", "button ghost small");
      link.id = "flow-link";
      link.target = "_blank";
      link.rel = "noopener noreferrer";
      link.append(node("span", "label", t("Open homepage", "打开主页")));
      link.hidden = true;
      body.append(head, link);
    } else {
      const text = node("p", "flow-text");
      text.id = "flow-text";
      body.append(text);
      if (i.target === "stream-credentials") {
        const creds = node("div", "credentials");
        creds.id = "flow-credentials";
        body.append(creds);
        text.hidden = true;
      }
    }
    if (i.kind === "cover") body.append(coverEditor());
    if (["form", "selection"].includes(i.kind)) {
      const form = node("form", "flow-form");
      const label = node(
        "label",
        i.kind === "selection" ? "sr-only" : "field-label",
        i.kind === "selection" ? t("Search categories", "搜索分区") : i.title,
      );
      label.htmlFor = "flow-input";
      const row = node("div", "flow-input-row");
      const input = node("input");
      input.id = "flow-input";
      input.type = i.secret ? "password" : i.kind === "selection" ? "search" : "text";
      input.value = i.value ?? "";
      input.autocomplete = "off";
      if (i.kind === "selection") input.placeholder = t("Search categories", "搜索分区");
      const submit = button(
        i.kind === "selection" ? t("Search", "搜索") : t("Apply", "应用"),
        "",
        i.kind === "selection" ? "" : "primary",
      );
      submit.type = "submit";
      submit.dataset.command = i.kind === "selection" ? "search" : "submit";
      submit.dataset.interaction = "true";
      row.append(input);
      if (i.secret) {
        const show = button(t("Show value", "显示内容"), "", "ghost small");
        show.onclick = () => {
          input.type = input.type === "password" ? "text" : "password";
          setLabel(show, input.type === "password" ? t("Show value", "显示内容") : t("Hide value", "隐藏内容"));
        };
        row.append(show);
      }
      row.append(submit);
      form.append(label, row);
      form.onsubmit = (e) => {
        e.preventDefault();
        command(i.kind === "selection" ? "search" : "submit", input.value, true);
      };
      body.append(form);
    }
    if (i.kind !== "speaker") {
      const img = node("img", "flow-image");
      img.id = "flow-image";
      img.alt = i.title;
      img.hidden = true;
      body.append(img);
    }
    if (i.kind !== "speaker") {
      const hint = note(
        t(
          "Scan with the Bilibili app. If the QR code is unavailable, copy the link and open it in your phone’s browser.",
          "使用哔哩哔哩手机客户端扫码；二维码不可用时，复制链接在手机浏览器中打开验证。",
        ),
      );
      hint.id = "verification-hint";
      const link = button(t("Copy verification link", "复制验证链接"), "", "small");
      link.id = "flow-link";
      link.hidden = true;
      const fallback = node("input");
      fallback.id = "verification-url";
      fallback.readOnly = true;
      fallback.hidden = true;
      fallback.setAttribute("aria-label", t("Verification link for manual copying", "手动复制验证链接"));
      const status = node("p", "note");
      status.id = "verification-status";
      status.setAttribute("role", "status");
      link.onclick = () => {
        const currentURL = safeURL(state?.interaction?.url);
        if (currentURL) copyText(currentURL, fallback);
      };
      body.append(hint, link, fallback, status);
    }
    const choices = node("div", "choices");
    choices.id = "flow-choices";
    body.append(choices);
  }
  function updateFlow(i) {
    setText("flow-title", i.title);
    $("flow-back").hidden = !i.back;
    const body = $("flow-body");
    if (i.kind === "members") {
      renderChatMembers(body, i);
      return;
    }
    setText("flow-text", i.text || "");
    $("flow-text").hidden = !i.text || i.target === "stream-credentials";
    const creds = $("flow-credentials");
    if (creds && creds.dataset.text !== i.text) {
      creds.dataset.text = i.text || "";
      creds.replaceChildren(
        ...(i.text || "")
          .split("\n")
          .filter(Boolean)
          .map((line, index) => {
            const row = node("div", "credential");
            const label = node("span", "credential-label", index === 0 ? t("Server", "服务器地址") : t("Stream key", "推流码"));
            const value = node("code", "", line);
            const copy = button(t("Copy", "复制"), "", "ghost small");
            copy.onclick = () => copyText(line);
            row.append(label, value, copy);
            return row;
          }),
      );
    }
    const image = i.kind === "cover" ? "" : safeURL(i.image, true);
    const img = $("flow-image");
    if (img) {
      img.hidden = !image;
      if (image && img.getAttribute("src") !== image) img.src = image;
      if (!image) img.removeAttribute("src");
    }
    const url = safeURL(i.url);
    const link = $("flow-link");
    if (link) link.hidden = !url;
    if (i.kind === "speaker") {
      if (url) link.href = url;
      else link.removeAttribute("href");
    } else if ($("verification-url")) {
      $("verification-hint").hidden = !url;
      $("verification-status").hidden = !url;
      const fallback = $("verification-url");
      if (fallback.value !== url) {
        fallback.value = url;
        setText("verification-status", "");
      }
      if (!url) fallback.hidden = true;
    }
    const choices = $("flow-choices");
    const signature = JSON.stringify(i.choices || []);
    if (choices.dataset.choices !== signature) {
      choices.replaceChildren(
        ...(i.choices || []).map((c) => {
          const b = interactionButton(
            c.label,
            ["pick", "selection", "form", "cover"].includes(i.kind) ? "choose" : c.id,
            "choice",
            c.id,
            c.selected ? "check" : "",
          );
          b.setAttribute("aria-pressed", String(!!c.selected));
          b.dataset.disabled = String(!!c.disabled);
          return b;
        }),
      );
      choices.dataset.choices = signature;
    }
    choices.hidden = !(i.choices || []).length;
  }

  // ───────────── 封面编辑 ─────────────
  let coverDraft = null,
    coverSelection = 0;
  function clearCoverDraft() {
    coverSelection++;
    if (coverDraft?.image) coverDraft.image.removeAttribute("src");
    coverDraft = null;
  }
  // 在解码前读取尺寸，限制压缩图片的内存膨胀；仅接受 PNG 和 JPEG。
  function coverDimensions(buffer) {
    const view = new DataView(buffer);
    if (
      view.byteLength >= 24 &&
      view.getUint32(0) === 0x89504e47 &&
      view.getUint32(4) === 0x0d0a1a0a &&
      view.getUint32(12) === 0x49484452
    )
      return [view.getUint32(16), view.getUint32(20)];
    if (view.byteLength < 4 || view.getUint16(0) !== 0xffd8) return null;
    let offset = 2;
    while (offset + 4 <= view.byteLength) {
      if (view.getUint8(offset++) !== 0xff) return null;
      while (offset < view.byteLength && view.getUint8(offset) === 0xff) offset++;
      if (offset + 3 > view.byteLength) return null;
      const marker = view.getUint8(offset++);
      if (marker === 0xda || marker === 0xd9) return null;
      if (marker === 0x01 || (marker >= 0xd0 && marker <= 0xd7)) continue;
      const length = view.getUint16(offset);
      if (length < 2 || offset + length > view.byteLength) return null;
      if ([0xc0, 0xc1, 0xc2].includes(marker) && length >= 8)
        return [view.getUint16(offset + 5), view.getUint16(offset + 3)];
      offset += length;
    }
    return null;
  }
  function coverEditor() {
    const draft = coverDraft || (coverDraft = { image: null, zoom: 1, x: 0.5, y: 0.5 });
    const editor = node("div", "cover-editor");
    const picker = node("input", "sr-only");
    picker.id = "cover-file";
    picker.type = "file";
    picker.accept = "image/png,image/jpeg,.png,.jpg,.jpeg";
    const drop = node("label", "cover-dropzone");
    drop.id = "cover-dropzone";
    drop.htmlFor = "cover-file";
    const prompt = node("span", "drop-prompt");
    prompt.append(
      node("strong", "", t("Choose a cover image", "选择封面图片")),
      node(
        "small",
        "",
        t(
          "Or drop a PNG/JPEG here · up to 12 MiB, 24 megapixels",
          "或拖入 PNG/JPEG · 最大 12 MiB、2400 万像素",
        ),
      ),
    );
    drop.append(prompt);
    const canvas = node("canvas", "cover-canvas");
    canvas.id = "cover-canvas";
    canvas.width = 720;
    canvas.height = 540;
    canvas.tabIndex = 0;
    canvas.setAttribute("role", "img");
    canvas.setAttribute(
      "aria-label",
      t(
        "Cover crop preview. Drag or use arrow keys to position.",
        "封面裁剪预览，可拖动或使用方向键调整位置。",
      ),
    );
    const status = node("p", "note");
    status.id = "cover-status";
    status.setAttribute("role", "status");
    const controls = node("div", "cover-controls");
    const ranges = {};
    let loading = false;
    const replace = button(t("Choose another", "更换图片"), "", "ghost small");
    replace.onclick = () => picker.click();
    const reset = button(t("Reset crop", "重置裁剪"), "", "ghost small");
    reset.id = "cover-reset";
    const preview = button(t("Preview cover", "预览封面"), "", "primary");
    preview.id = "cover-preview";
    preview.dataset.command = "cover-upload";
    preview.dataset.interaction = "true";
    const locked = () => sending || state?.busy || state?.interaction?.kind !== "cover";
    const geometry = () => {
      const image = draft.image;
      const scale =
        Math.max(canvas.width / image.naturalWidth, canvas.height / image.naturalHeight) *
        draft.zoom;
      return [image.naturalWidth * scale, image.naturalHeight * scale];
    };
    const draw = () => {
      canvas.hidden = !draft.image;
      drop.hidden = !!draft.image;
      controls.hidden = !draft.image;
      reset.hidden = !draft.image;
      replace.hidden = !draft.image;
      preview.dataset.disabled = String(!draft.image || loading);
      preview.disabled = !draft.image || loading || locked();
      if (!draft.image) return;
      const [width, height] = geometry();
      const context = canvas.getContext("2d");
      context.fillStyle = "#fff";
      context.fillRect(0, 0, canvas.width, canvas.height);
      context.drawImage(
        draft.image,
        -(width - canvas.width) * draft.x,
        -(height - canvas.height) * draft.y,
        width,
        height,
      );
      for (const key of ["zoom", "x", "y"]) ranges[key].value = draft[key];
    };
    for (const [key, label, min, max, step] of [
      ["zoom", t("Zoom", "缩放"), 1, 4, 0.01],
      ["x", t("Horizontal", "水平"), 0, 1, 0.01],
      ["y", t("Vertical", "垂直"), 0, 1, 0.01],
    ]) {
      const labelNode = node("label", "", label);
      const range = node("input");
      range.type = "range";
      range.id = "cover-" + key;
      labelNode.htmlFor = range.id;
      range.min = min;
      range.max = max;
      range.step = step;
      range.value = draft[key];
      range.oninput = () => {
        if (!locked()) draft[key] = Number(range.value);
        draw();
      };
      ranges[key] = range;
      controls.append(labelNode, range);
    }
    reset.onclick = () => {
      if (locked()) return;
      Object.assign(draft, { zoom: 1, x: 0.5, y: 0.5 });
      draw();
    };
    const select = async (file) => {
      if (!file || locked()) return;
      const token = ++coverSelection;
      loading = true;
      draw();
      status.textContent = t("Loading image…", "正在加载图片…");
      let image, url;
      const current = () =>
        token === coverSelection && coverDraft === draft && editor.isConnected;
      try {
        if (!file.size || file.size > 12 * 1024 * 1024)
          throw new Error(t("Choose an image no larger than 12 MiB.", "请选择不超过 12 MiB 的图片。"));
        const dimensions = coverDimensions(await file.arrayBuffer());
        if (!current()) return;
        if (
          !dimensions ||
          dimensions.some((n) => !n || n > 16384) ||
          dimensions[0] * dimensions[1] > 24000000
        )
          throw new Error(
            t(
              "Use a valid PNG/JPEG, at most 24 megapixels and 16384 pixels per side.",
              "请使用有效的 PNG/JPEG，最多 2400 万像素，单边不超过 16384 像素。",
            ),
          );
        image = new Image();
        url = URL.createObjectURL(file);
        image.src = url;
        await image.decode();
        if (!current()) return;
        if (
          !image.naturalWidth ||
          !image.naturalHeight ||
          image.naturalWidth * image.naturalHeight > 24000000 ||
          Math.max(image.naturalWidth, image.naturalHeight) > 16384
        )
          throw new Error(t("Image dimensions exceed the limit.", "图片尺寸超出限制。"));
        draft.image?.removeAttribute("src");
        Object.assign(draft, { image, zoom: 1, x: 0.5, y: 0.5 });
        status.textContent = t(
          "Drag to adjust the crop, then preview. Upload to Bilibili requires final confirmation.",
          "拖动调整取景后预览，最终确认后才会上传到哔哩哔哩。",
        );
        draw();
      } catch (error) {
        if (current())
          status.textContent =
            error instanceof Error && error.name !== "EncodingError"
              ? error.message
              : t(
                  "Cannot decode this image. Choose another PNG/JPEG.",
                  "无法解码此图片，请选择其他 PNG/JPEG。",
                );
      } finally {
        if (url) URL.revokeObjectURL(url);
        if (image && draft.image !== image) image.removeAttribute("src");
        if (current()) {
          loading = false;
          picker.value = "";
          draw();
        }
      }
    };
    picker.onchange = () => select(picker.files[0]);
    for (const target of [drop, canvas])
      for (const eventName of ["dragenter", "dragover", "dragleave", "drop"])
        target.addEventListener(eventName, (event) => {
          event.preventDefault();
          event.stopPropagation();
          editor.classList.toggle(
            "dragging",
            eventName === "dragenter" || eventName === "dragover",
          );
          if (eventName === "drop") {
            if (event.dataTransfer.files.length !== 1)
              status.textContent = t("Choose one image at a time.", "请每次选择一张图片。");
            else select(event.dataTransfer.files[0]);
          }
        });
    let drag = null;
    canvas.onpointerdown = (event) => {
      if (!draft.image || locked() || event.button !== 0) return;
      drag = { id: event.pointerId, x: event.clientX, y: event.clientY };
      canvas.setPointerCapture(event.pointerId);
      canvas.focus();
    };
    canvas.onpointermove = (event) => {
      if (!drag || drag.id !== event.pointerId || locked()) return;
      const [width, height] = geometry();
      const scale = canvas.width / canvas.getBoundingClientRect().width;
      if (width > canvas.width)
        draft.x = Math.max(0, Math.min(1, draft.x - ((event.clientX - drag.x) * scale) / (width - canvas.width)));
      if (height > canvas.height)
        draft.y = Math.max(0, Math.min(1, draft.y - ((event.clientY - drag.y) * scale) / (height - canvas.height)));
      drag.x = event.clientX;
      drag.y = event.clientY;
      draw();
    };
    canvas.onpointerup = canvas.onpointercancel = canvas.onlostpointercapture = () => {
      drag = null;
    };
    canvas.onwheel = (event) => {
      if (!draft.image || locked()) return;
      event.preventDefault();
      draft.zoom = Math.max(1, Math.min(4, draft.zoom - event.deltaY * 0.002));
      draw();
    };
    canvas.onkeydown = (event) => {
      if (
        !draft.image ||
        locked() ||
        !["ArrowLeft", "ArrowRight", "ArrowUp", "ArrowDown"].includes(event.key)
      )
        return;
      event.preventDefault();
      const key = ["ArrowLeft", "ArrowRight"].includes(event.key) ? "x" : "y";
      const delta = ["ArrowLeft", "ArrowUp"].includes(event.key) ? -0.02 : 0.02;
      draft[key] = Math.max(0, Math.min(1, draft[key] + delta));
      draw();
    };
    preview.onclick = () => {
      if (!draft.image || loading || locked()) return;
      coverSelection++;
      command("cover-upload", canvas.toDataURL("image/png"), true);
    };
    const stage = node("div", "cover-stage");
    stage.append(drop, canvas);
    editor.append(picker, stage, status, controls, actionRow(replace, reset, node("span", "spacer"), preview));
    draw();
    return editor;
  }
  // 在工作台内丢弃文件也不能触发浏览器跳转。
  for (const eventName of ["dragover", "drop"])
    document.addEventListener(eventName, (event) => {
      if (Array.from(event.dataTransfer?.types || []).includes("Files"))
        event.preventDefault();
    });

  // ───────────── 初始化 ─────────────
  for (const id of ["retry", "obs-action", "live-action", "flow-close"])
    $(id).replaceChildren(node("span", "label"));
  $("flow-close").onclick = () => command("cancel", "", true);
  for (const dialog of [$("confirm"), $("flow-dialog")]) {
    dialog.addEventListener("cancel", (e) => {
      e.preventDefault();
      command("cancel", "", true);
    });
    dialog.addEventListener("keydown", (event) => {
      if (event.key !== "Tab") return;
      const controls = [
        ...dialog.querySelectorAll(
          "button, input, textarea, select, a[href], summary, [tabindex]",
        ),
      ].filter((n) => !n.disabled && n.tabIndex >= 0 && n.getClientRects().length);
      if (!controls.length) {
        event.preventDefault();
        return;
      }
      const first = controls[0],
        last = controls[controls.length - 1];
      if (
        event.shiftKey &&
        (document.activeElement === first || !controls.includes(document.activeElement))
      ) {
        event.preventDefault();
        last.focus();
      } else if (
        !event.shiftKey &&
        (document.activeElement === last || !controls.includes(document.activeElement))
      ) {
        event.preventDefault();
        first.focus();
      }
    });
  }
  $("retry").onclick = () => poll();
  document.querySelector(".brand").onclick = (e) => {
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
