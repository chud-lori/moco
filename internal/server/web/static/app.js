// ---------- Toast / notification system ----------
function ensureToastStack() {
  let stack = document.querySelector(".toast-stack");
  if (!stack) {
    stack = document.createElement("div");
    stack.className = "toast-stack";
    stack.setAttribute("role", "status");
    stack.setAttribute("aria-live", "polite");
    document.body.appendChild(stack);
  }
  return stack;
}

function toast(message, kind = "info", { autoDismiss = true } = {}) {
  const stack = ensureToastStack();
  const node = document.createElement("div");
  node.className = "toast" + (kind === "error" ? " is-error" : kind === "success" ? " is-success" : "");
  node.setAttribute("role", kind === "error" ? "alert" : "status");

  const text = document.createElement("span");
  text.textContent = message;
  node.appendChild(text);

  const close = document.createElement("button");
  close.type = "button";
  close.setAttribute("aria-label", "Dismiss");
  close.textContent = "×";
  close.addEventListener("click", () => node.remove());
  node.appendChild(close);

  stack.appendChild(node);

  if (autoDismiss) {
    const ms = kind === "error" ? 6000 : 3000;
    setTimeout(() => node.remove(), ms);
  }
  return node;
}

// ---------- Modal helpers ----------
function ensureModal() {
  let modal = document.querySelector("[data-modal-root]");
  if (!modal) {
    modal = document.createElement("div");
    modal.className = "modal";
    modal.setAttribute("data-modal-root", "");
    modal.setAttribute("role", "dialog");
    modal.setAttribute("aria-modal", "true");
    const card = document.createElement("div");
    card.className = "modal-card";
    card.setAttribute("data-modal-card", "");
    modal.appendChild(card);
    document.body.appendChild(modal);
    modal.addEventListener("click", (event) => {
      if (event.target === modal) closeModal();
    });
    document.addEventListener("keydown", (event) => {
      if (event.key === "Escape" && modal.classList.contains("is-open")) closeModal();
    });
  }
  return modal;
}

let modalLastFocus = null;
function closeModal() {
  const modal = document.querySelector("[data-modal-root]");
  if (!modal) return;
  modal.classList.remove("is-open");
  modal.querySelector("[data-modal-card]").innerHTML = "";
  if (modalLastFocus && typeof modalLastFocus.focus === "function") {
    modalLastFocus.focus();
  }
  modalLastFocus = null;
}

function openConfirm({ title, body, confirmLabel = "Confirm", cancelLabel = "Cancel", danger = false }) {
  return new Promise((resolve) => {
    const modal = ensureModal();
    const card = modal.querySelector("[data-modal-card]");
    modalLastFocus = document.activeElement;

    const titleEl = document.createElement("h2");
    titleEl.textContent = title;

    const bodyEl = document.createElement("p");
    bodyEl.textContent = body;

    const actions = document.createElement("div");
    actions.className = "modal-actions";

    const cancelBtn = document.createElement("button");
    cancelBtn.type = "button";
    cancelBtn.className = "button subtle";
    cancelBtn.textContent = cancelLabel;
    cancelBtn.addEventListener("click", () => { closeModal(); resolve(false); });

    const confirmBtn = document.createElement("button");
    confirmBtn.type = "button";
    confirmBtn.className = "button " + (danger ? "danger" : "primary");
    confirmBtn.textContent = confirmLabel;
    confirmBtn.addEventListener("click", () => { closeModal(); resolve(true); });

    actions.appendChild(cancelBtn);
    actions.appendChild(confirmBtn);

    card.appendChild(titleEl);
    card.appendChild(bodyEl);
    card.appendChild(actions);

    modal.classList.add("is-open");
    setTimeout(() => confirmBtn.focus(), 0);
  });
}

function openTextPrompt({ title, body, placeholder = "", confirmLabel = "Save", cancelLabel = "Cancel" }) {
  return new Promise((resolve) => {
    const modal = ensureModal();
    const card = modal.querySelector("[data-modal-card]");
    modalLastFocus = document.activeElement;

    const titleEl = document.createElement("h2");
    titleEl.textContent = title;

    const bodyEl = document.createElement("p");
    bodyEl.textContent = body;

    const textarea = document.createElement("textarea");
    textarea.placeholder = placeholder;
    textarea.setAttribute("aria-label", title);

    const actions = document.createElement("div");
    actions.className = "modal-actions";

    const cancelBtn = document.createElement("button");
    cancelBtn.type = "button";
    cancelBtn.className = "button subtle";
    cancelBtn.textContent = cancelLabel;
    cancelBtn.addEventListener("click", () => { closeModal(); resolve(null); });

    const confirmBtn = document.createElement("button");
    confirmBtn.type = "button";
    confirmBtn.className = "button primary";
    confirmBtn.textContent = confirmLabel;
    confirmBtn.addEventListener("click", () => {
      const value = textarea.value.trim();
      closeModal();
      resolve(value || null);
    });

    actions.appendChild(cancelBtn);
    actions.appendChild(confirmBtn);

    card.appendChild(titleEl);
    card.appendChild(bodyEl);
    card.appendChild(textarea);
    card.appendChild(actions);

    modal.classList.add("is-open");
    setTimeout(() => textarea.focus(), 0);
  });
}

// ---------- HTTP helpers ----------
function getCookie(name) {
  const needle = `${name}=`;
  return document.cookie
    .split(";")
    .map((part) => part.trim())
    .find((part) => part.startsWith(needle))
    ?.slice(needle.length) || "";
}

function csrfHeaders(method, headers = {}) {
  const upper = String(method || "GET").toUpperCase();
  if (["GET", "HEAD", "OPTIONS"].includes(upper)) return headers;
  const token = getCookie("moco_csrf");
  return token ? { ...headers, "X-CSRF-Token": token } : headers;
}

async function requestJSON(url, options = {}) {
  const method = options.method || "GET";
  const response = await fetch(url, {
    credentials: "same-origin",
    headers: {
      Accept: "application/json",
      ...(options.body instanceof FormData ? {} : { "Content-Type": "application/json" }),
      ...csrfHeaders(method, options.headers || {}),
    },
    ...options,
  });
  const data = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(data.error || data.message || "Request failed");
  return data;
}

function setButtonLoading(button, loading, loadingText) {
  if (!button) return;
  if (loading) {
    if (!button.dataset.originalLabel) button.dataset.originalLabel = button.textContent;
    button.disabled = true;
    button.classList.add("is-loading");
    if (loadingText) {
      button.textContent = "";
      const spinner = document.createElement("span");
      spinner.className = "spinner";
      spinner.setAttribute("aria-hidden", "true");
      const label = document.createElement("span");
      label.textContent = loadingText;
      button.appendChild(spinner);
      button.appendChild(label);
    }
  } else {
    button.disabled = false;
    button.classList.remove("is-loading");
    if (button.dataset.originalLabel) {
      button.textContent = button.dataset.originalLabel;
      delete button.dataset.originalLabel;
    }
  }
}

function setMessage(node, text, variant = "") {
  if (!node) return;
  node.textContent = text || "";
  node.classList.remove("is-error", "is-success");
  if (variant === "error") node.classList.add("is-error");
  if (variant === "success") node.classList.add("is-success");
}

function truncateFilename(name, maxLen = 48) {
  if (!name || name.length <= maxLen) return name || "";
  const dot = name.lastIndexOf(".");
  const ext = dot > 0 && name.length - dot <= 8 ? name.slice(dot) : "";
  const stem = ext ? name.slice(0, name.length - ext.length) : name;
  const keep = Math.max(8, maxLen - ext.length - 1);
  return stem.slice(0, keep) + "…" + ext;
}

function formatBytes(bytes) {
  if (bytes < 1024) return `${bytes}B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)}KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)}MB`;
}

// ---------- Auth form ----------
const authForm = document.querySelector("[data-auth-form]");
if (authForm) {
  const message = document.querySelector("[data-form-message]");
  if (message) {
    message.setAttribute("aria-live", "polite");
    if (!message.id) message.id = "auth-form-message";
    authForm.querySelectorAll("input").forEach((input) => {
      input.setAttribute("aria-describedby", message.id);
    });
  }

  const submitBtn = authForm.querySelector('button[type="submit"]');

  authForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    const formData = new FormData(authForm);
    const email = String(formData.get("email") || "").trim();
    const password = String(formData.get("password") || "");

    authForm.querySelectorAll("input").forEach((i) => i.removeAttribute("aria-invalid"));
    if (!email) {
      setMessage(message, "Email is required.", "error");
      authForm.querySelector('input[name="email"]')?.setAttribute("aria-invalid", "true");
      return;
    }
    if (password.length < 10) {
      setMessage(message, "Password must be at least 10 characters.", "error");
      authForm.querySelector('input[name="password"]')?.setAttribute("aria-invalid", "true");
      return;
    }

    setMessage(message, "Working…");
    setButtonLoading(submitBtn, true, "Working…");
    try {
      await requestJSON(`/api/v1/auth/${authForm.dataset.mode}`, {
        method: "POST",
        body: JSON.stringify({ email, password }),
      });
      window.location.href = "/app";
    } catch (error) {
      setMessage(message, error.message || "Something went wrong.", "error");
      setButtonLoading(submitBtn, false);
    }
  });

  authForm.querySelectorAll("input").forEach((input) => {
    input.addEventListener("input", () => {
      input.removeAttribute("aria-invalid");
      if (message && message.classList.contains("is-error")) setMessage(message, "");
    });
  });
}

// ---------- Logout ----------
const logoutButton = document.querySelector("[data-logout-button]");
if (logoutButton) {
  logoutButton.addEventListener("click", async () => {
    setButtonLoading(logoutButton, true, "Signing out…");
    try {
      await requestJSON("/api/v1/auth/logout", { method: "POST", body: "{}" });
      window.location.href = "/";
    } catch (error) {
      setButtonLoading(logoutButton, false);
      toast(error.message || "Logout failed.", "error");
    }
  });
}

// ---------- Upload (XHR with progress, validation) ----------
const uploadForm = document.querySelector("[data-upload-form]");
if (uploadForm) {
  const message = document.querySelector("[data-upload-message]");
  const submitBtn = uploadForm.querySelector('button[type="submit"]');
  const fileInput = uploadForm.querySelector('input[type="file"]');
  const allowedExt = /\.(pdf|epub|md|markdown)$/i;
  const maxBytes = 100 * 1024 * 1024;

  let progressEl = uploadForm.querySelector("[data-upload-progress]");
  if (!progressEl) {
    progressEl = document.createElement("div");
    progressEl.className = "upload-progress";
    progressEl.setAttribute("data-upload-progress", "");
    progressEl.setAttribute("role", "progressbar");
    progressEl.setAttribute("aria-valuemin", "0");
    progressEl.setAttribute("aria-valuemax", "100");
    const bar = document.createElement("div");
    bar.className = "upload-progress-bar";
    bar.setAttribute("data-upload-progress-bar", "");
    progressEl.appendChild(bar);
    submitBtn?.before(progressEl);
  }
  const progressBar = progressEl.querySelector("[data-upload-progress-bar]");

  fileInput?.addEventListener("change", () => {
    const file = fileInput.files?.[0];
    if (!file) return;
    if (!allowedExt.test(file.name)) {
      setMessage(message, "Only .pdf, .epub, and .md files are supported.", "error");
      fileInput.value = "";
      return;
    }
    if (file.size > maxBytes) {
      setMessage(message, `File is too large (max ${Math.round(maxBytes / 1024 / 1024)}MB).`, "error");
      fileInput.value = "";
      return;
    }
    setMessage(message, `Selected ${truncateFilename(file.name, 48)} (${formatBytes(file.size)})`);
  });

  uploadForm.addEventListener("submit", (event) => {
    event.preventDefault();
    const file = fileInput?.files?.[0];
    if (!file) {
      setMessage(message, "Pick a file first.", "error");
      return;
    }

    setMessage(message, "Uploading…");
    setButtonLoading(submitBtn, true, "Uploading…");
    progressEl.classList.add("is-active");
    progressBar.style.width = "0%";

    const xhr = new XMLHttpRequest();
    xhr.open("POST", "/api/v1/books/upload");
    xhr.responseType = "json";
    const csrf = getCookie("moco_csrf");
    if (csrf) xhr.setRequestHeader("X-CSRF-Token", csrf);
    xhr.upload.addEventListener("progress", (e) => {
      if (e.lengthComputable) {
        const pct = (e.loaded / e.total) * 100;
        progressBar.style.width = `${pct}%`;
        progressEl.setAttribute("aria-valuenow", String(Math.round(pct)));
        setMessage(message, `Uploading ${Math.round(pct)}%…`);
      }
    });
    xhr.addEventListener("load", () => {
      const ok = xhr.status >= 200 && xhr.status < 300;
      const data = xhr.response || {};
      if (ok) {
        progressBar.style.width = "100%";
        setMessage(message, "Uploaded. Refreshing library…", "success");
        toast("Book added to your library.", "success");
        setTimeout(() => window.location.reload(), 400);
      } else {
        progressEl.classList.remove("is-active");
        setButtonLoading(submitBtn, false);
        const error = data.error || "Upload failed.";
        setMessage(message, error, "error");
        toast(error, "error");
      }
    });
    xhr.addEventListener("error", () => {
      progressEl.classList.remove("is-active");
      setButtonLoading(submitBtn, false);
      setMessage(message, "Network error. Please try again.", "error");
      toast("Network error during upload.", "error");
    });
    xhr.addEventListener("abort", () => {
      progressEl.classList.remove("is-active");
      setButtonLoading(submitBtn, false);
      setMessage(message, "Upload cancelled.", "error");
    });

    xhr.send(new FormData(uploadForm));
  });
}

// ---------- Delete book ----------
document.querySelectorAll("[data-delete-book]").forEach((button) => {
  button.addEventListener("click", async () => {
    const bookID = button.getAttribute("data-delete-book");
    if (!bookID) return;
    const confirmed = await openConfirm({
      title: "Remove this book?",
      body: "This permanently deletes the file, your highlights, and reading progress.",
      confirmLabel: "Remove",
      danger: true,
    });
    if (!confirmed) return;
    setButtonLoading(button, true, "Removing…");
    try {
      await requestJSON(`/api/v1/books/${bookID}`, { method: "DELETE", body: "{}" });
      toast("Book removed.", "success");
      window.location.reload();
    } catch (error) {
      setButtonLoading(button, false);
      toast(error.message || "Could not remove book.", "error");
    }
  });
});

// ---------- Toggle visibility ----------
document.querySelectorAll("[data-toggle-visibility]").forEach((button) => {
  button.addEventListener("click", async () => {
    const bookID = button.getAttribute("data-toggle-visibility");
    const visibility = button.getAttribute("data-next-visibility");
    if (!bookID || !visibility) return;
    const goingPublic = visibility === "public";
    const confirmed = await openConfirm({
      title: goingPublic ? "Publish to the public shelf?" : "Make this book private?",
      body: goingPublic
        ? "Anyone with the link will be able to read this book."
        : "It will only be visible to you.",
      confirmLabel: goingPublic ? "Publish" : "Make private",
    });
    if (!confirmed) return;
    setButtonLoading(button, true, "Updating…");
    try {
      await requestJSON(`/api/v1/books/${bookID}/visibility`, {
        method: "PUT",
        body: JSON.stringify({ visibility }),
      });
      toast(goingPublic ? "Now public." : "Now private.", "success");
      window.location.reload();
    } catch (error) {
      setButtonLoading(button, false);
      toast(error.message || "Could not update visibility.", "error");
    }
  });
});

// ---------- Highlight delete ----------
function attachHighlightDelete(button) {
  if (!button) return;
  button.addEventListener("click", async () => {
    const id = button.getAttribute("data-delete-highlight");
    if (!id) return;
    const confirmed = await openConfirm({
      title: "Delete this highlight?",
      body: "You can re-highlight the same passage later.",
      confirmLabel: "Delete",
      danger: true,
    });
    if (!confirmed) return;
    try {
      await requestJSON(`/api/v1/highlights/${id}`, { method: "DELETE", body: "{}" });
      if (window.__mocoReaderState?.removeHighlight) window.__mocoReaderState.removeHighlight(id);
      button.closest(".note-card")?.remove();
      if (window.__mocoReaderState?.ensureHighlightListState) window.__mocoReaderState.ensureHighlightListState();
      toast("Highlight deleted.", "success");
    } catch (error) {
      toast(error.message || "Could not delete highlight.", "error");
    }
  });
}
document.querySelectorAll("[data-delete-highlight]").forEach(attachHighlightDelete);

// ---------- Reader ----------
function saveReaderProgressFactory(bookID, progressStatus) {
  return async (locator, progressPercent) => {
    try {
      await requestJSON(`/api/v1/books/${bookID}/progress`, {
        method: "PUT",
        body: JSON.stringify({ locator, progressPercent }),
      });
      if (progressStatus) progressStatus.textContent = `${Math.round(progressPercent)}% read`;
    } catch (_) {
      // progress saves are silent — flaky-tick toasts would be obnoxious
    }
  };
}

// Adds a Copy button to each fenced code block and re-runs Prism if it's loaded.
// Idempotent — calling it twice on the same root won't double-attach.
function enrichCodeBlocks(root) {
  if (!root) return;
  root.querySelectorAll("pre > code").forEach((code) => {
    const pre = code.parentElement;
    if (!pre || pre.dataset.enriched === "1") return;
    pre.dataset.enriched = "1";

    const copy = document.createElement("button");
    copy.type = "button";
    copy.className = "code-copy";
    copy.textContent = "Copy";
    copy.setAttribute("aria-label", "Copy code to clipboard");
    copy.addEventListener("click", async () => {
      const text = code.innerText;
      try {
        await navigator.clipboard.writeText(text);
        copy.textContent = "Copied";
        copy.classList.add("is-copied");
      } catch (_) {
        copy.textContent = "Press ⌘C";
      }
      setTimeout(() => {
        copy.textContent = "Copy";
        copy.classList.remove("is-copied");
      }, 1600);
    });
    pre.appendChild(copy);
  });

  // Trigger Prism if its bundle has loaded; otherwise wait briefly.
  const tryHighlight = () => {
    if (window.Prism && typeof window.Prism.highlightAllUnder === "function") {
      window.Prism.highlightAllUnder(root);
      return true;
    }
    return false;
  };
  if (!tryHighlight()) {
    let attempts = 0;
    const interval = setInterval(() => {
      attempts += 1;
      if (tryHighlight() || attempts > 40) clearInterval(interval);
    }, 100);
  }
}

function buildHighlightCard(item) {
  const card = document.createElement("article");
  card.className = "note-card";
  card.setAttribute("data-highlight-id", item.id);

  const tag = document.createElement("span");
  tag.className = "note-tag amber";
  tag.textContent = item.color || "amber";

  const text = document.createElement("p");
  text.textContent = item.selectedText || "";

  const actions = document.createElement("div");
  actions.className = "book-panel-actions";

  const jump = document.createElement("a");
  jump.className = "text-link";
  jump.href = "#" + encodeURIComponent(item.locator || "");
  jump.textContent = "Jump to passage";

  const del = document.createElement("button");
  del.type = "button";
  del.className = "text-link destructive";
  del.setAttribute("data-delete-highlight", item.id);
  del.textContent = "Delete";

  actions.appendChild(jump);
  actions.appendChild(del);
  card.appendChild(tag);
  card.appendChild(text);
  card.appendChild(actions);

  attachHighlightDelete(del);
  return card;
}

const readerRoot = document.querySelector("[data-reader]");
if (readerRoot) {
  const bookID = readerRoot.getAttribute("data-book-id");
  const readerKind = readerRoot.getAttribute("data-reader-kind");
  const fileURL = readerRoot.getAttribute("data-file-url");
  const progressStatus = document.querySelector("[data-progress-status]");
  const highlightsList = document.querySelector("[data-highlights-list]");
  const saveProgress = saveReaderProgressFactory(bookID, progressStatus);
  const currentHighlights = (() => {
    const raw = document.getElementById("reader-highlights-data")?.textContent?.trim();
    if (!raw) return [];
    try {
      return JSON.parse(raw);
    } catch (_) {
      return [];
    }
  })();

  function ensureHighlightListState() {
    if (!highlightsList) return;
    const cards = highlightsList.querySelectorAll(".note-card");
    if (cards.length > 0) return;
    const card = document.createElement("article");
    card.className = "note-card";
    card.innerHTML = "<p>No highlights yet.</p>";
    highlightsList.appendChild(card);
  }

  function clearEmptyHighlightState() {
    const emptyCard = Array.from(highlightsList?.querySelectorAll(".note-card") || []).find((card) => {
      return card.querySelector("[data-delete-highlight]") === null && /No highlights yet\./i.test(card.textContent || "");
    });
    emptyCard?.remove();
  }

  function unwrapMark(mark) {
    const parent = mark.parentNode;
    if (!parent) return;
    while (mark.firstChild) parent.insertBefore(mark.firstChild, mark);
    parent.removeChild(mark);
    parent.normalize();
  }

  function sectionScope(locator, readerContent) {
    return readerContent.querySelector(`[data-section="${CSS.escape(locator)}"]`) || readerContent;
  }

  function highlightNode(scope, item) {
    const query = String(item.selectedText || "").trim();
    if (!query) return false;
    const walker = document.createTreeWalker(scope, NodeFilter.SHOW_TEXT, {
      acceptNode(node) {
        const parent = node.parentElement;
        if (!parent) return NodeFilter.FILTER_REJECT;
        if (parent.closest("pre, code, mark, script, style")) return NodeFilter.FILTER_REJECT;
        if (!node.nodeValue || !node.nodeValue.includes(query)) return NodeFilter.FILTER_SKIP;
        return NodeFilter.FILTER_ACCEPT;
      },
    });

    while (walker.nextNode()) {
      const node = walker.currentNode;
      const value = node.nodeValue || "";
      const start = value.indexOf(query);
      if (start < 0) continue;
      const range = document.createRange();
      range.setStart(node, start);
      range.setEnd(node, start + query.length);
      const mark = document.createElement("mark");
      mark.className = `saved-highlight saved-highlight-${item.color || "amber"}`;
      mark.setAttribute("data-rendered-highlight", item.id);
      try {
        range.surroundContents(mark);
        return true;
      } catch (_) {
        return false;
      }
    }
    return false;
  }

  function renderMarkdownHighlights(readerContent) {
    if (!readerContent) return;
    readerContent.querySelectorAll("[data-rendered-highlight]").forEach(unwrapMark);
    currentHighlights.forEach((item) => {
      const scope = sectionScope(item.locator || "start", readerContent);
      highlightNode(scope, item);
    });
  }

  function removeHighlight(id) {
    const idx = currentHighlights.findIndex((item) => item.id === id);
    if (idx >= 0) currentHighlights.splice(idx, 1);
    const readerContent = document.getElementById("reader-content");
    if (readerKind === "md" && readerContent) renderMarkdownHighlights(readerContent);
  }

  window.__mocoReaderState = { removeHighlight, ensureHighlightListState };

  // ----- Sidebar panels -----
  let overlay = document.querySelector(".reader-overlay");
  if (!overlay) {
    overlay = document.createElement("div");
    overlay.className = "reader-overlay";
    document.body.appendChild(overlay);
    overlay.addEventListener("click", closeAllPanels);
  }

  function closeAllPanels() {
    document.querySelectorAll("[data-reader-panel]").forEach((p) => p.classList.remove("is-open"));
    overlay.classList.remove("is-open");
  }

  function openPanel(target) {
    document.querySelectorAll("[data-reader-panel]").forEach((panel) => {
      const matches = panel.getAttribute("data-reader-panel") === target;
      const wasOpen = panel.classList.contains("is-open");
      panel.classList.toggle("is-open", matches ? !wasOpen : false);
    });
    const anyOpen = document.querySelector("[data-reader-panel].is-open");
    overlay.classList.toggle("is-open", !!anyOpen);
  }

  document.querySelectorAll("[data-reader-toggle]").forEach((button) => {
    button.addEventListener("click", () => openPanel(button.getAttribute("data-reader-toggle")));
  });

  // Inject close buttons into each panel for mobile/tablet
  document.querySelectorAll("[data-reader-panel]").forEach((panel) => {
    if (panel.querySelector(".reader-sidebar-close")) return;
    const close = document.createElement("button");
    close.type = "button";
    close.className = "reader-sidebar-close";
    close.setAttribute("aria-label", "Close panel");
    close.textContent = "×";
    close.addEventListener("click", closeAllPanels);
    panel.appendChild(close);
  });

  document.addEventListener("keydown", (event) => {
    if (event.key === "Escape") {
      const anyOpen = document.querySelector("[data-reader-panel].is-open");
      if (anyOpen) closeAllPanels();
    }
  });

  // ----- Kindle-style immersive mode -----
  const readerApp = readerRoot;
  let immersiveTimer = null;
  const IMMERSIVE_DELAY = 2800;

  function panelOrModalOpen() {
    return !!document.querySelector("[data-reader-panel].is-open")
      || !!document.querySelector("[data-modal-root].is-open");
  }
  function hasSelection() {
    const sel = window.getSelection();
    return !!(sel && sel.toString().trim());
  }
  function clearImmersiveTimer() {
    if (immersiveTimer) { clearTimeout(immersiveTimer); immersiveTimer = null; }
  }
  function scheduleHide(delay = IMMERSIVE_DELAY) {
    clearImmersiveTimer();
    immersiveTimer = setTimeout(() => {
      if (panelOrModalOpen() || hasSelection()) return;
      readerApp.classList.add("is-immersive");
    }, delay);
  }
  function showChrome({ keepShown = false } = {}) {
    readerApp.classList.remove("is-immersive");
    if (keepShown) clearImmersiveTimer();
    else scheduleHide();
  }
  function toggleImmersive() {
    if (readerApp.classList.contains("is-immersive")) {
      showChrome();
    } else {
      clearImmersiveTimer();
      readerApp.classList.add("is-immersive");
    }
  }

  // Tap on the reading area toggles chrome (skip interactive elements / sidebar / overlay)
  readerRoot.addEventListener("click", (event) => {
    if (event.target.closest("button, a, input, textarea, select, .reader-sidebar, .reader-overlay, [data-modal-root]")) return;
    if (hasSelection()) return;
    toggleImmersive();
  });

  // Mouse near the top of the screen reveals chrome
  document.addEventListener("mousemove", (event) => {
    if (event.clientY < 80) showChrome();
  }, { passive: true });

  // Selection forces chrome on (so the Save Highlight button is reachable)
  document.addEventListener("selectionchange", () => {
    if (hasSelection()) showChrome({ keepShown: true });
  });

  // Opening a panel keeps chrome visible; closing it re-arms the timer
  const panelObserver = new MutationObserver(() => {
    if (panelOrModalOpen()) showChrome({ keepShown: true });
    else scheduleHide();
  });
  document.querySelectorAll("[data-reader-panel]").forEach((panel) => {
    panelObserver.observe(panel, { attributes: true, attributeFilter: ["class"] });
  });

  // Show chrome briefly on load so users see the controls, then auto-hide.
  // First-time users also get a one-shot toast hint.
  scheduleHide(3000);
  if (!localStorage.getItem("moco-reader-hint-seen")) {
    setTimeout(() => {
      toast("Tap anywhere to show controls.", "info");
      localStorage.setItem("moco-reader-hint-seen", "1");
    }, 800);
  }

  // ----- Swipe-to-page-turn (EPUB / PDF only) -----
  if (readerKind === "epub" || readerKind === "pdf") {
    let touchStartX = 0, touchStartY = 0, touchStartT = 0;
    const SWIPE_MIN = 50;
    const VERTICAL_TOLERANCE = 60;
    const TIME_LIMIT = 600;

    readerRoot.addEventListener("touchstart", (event) => {
      if (event.target.closest("button, a, input, textarea, select, .reader-sidebar, [data-modal-root]")) return;
      const t = event.touches[0];
      touchStartX = t.clientX;
      touchStartY = t.clientY;
      touchStartT = Date.now();
    }, { passive: true });

    readerRoot.addEventListener("touchend", (event) => {
      if (Date.now() - touchStartT > TIME_LIMIT) return;
      const t = event.changedTouches[0];
      const dx = t.clientX - touchStartX;
      const dy = Math.abs(t.clientY - touchStartY);
      if (Math.abs(dx) < SWIPE_MIN || dy > VERTICAL_TOLERANCE) return;
      const prevSel = readerKind === "epub" ? "[data-epub-prev]" : "[data-pdf-prev]";
      const nextSel = readerKind === "epub" ? "[data-epub-next]" : "[data-pdf-next]";
      if (dx > 0) document.querySelector(prevSel)?.click();
      else document.querySelector(nextSel)?.click();
    }, { passive: true });
  }

  // ----- Reading settings (font size + theme + family + line height) -----
  const SETTINGS_KEY = "moco-reader-settings";
  const FONT_FAMILIES = {
    serif:    '"Cormorant Garamond", Georgia, serif',
    sans:     '"Manrope", system-ui, -apple-system, "Segoe UI", sans-serif',
    original: "",
  };
  const LINE_HEIGHTS = { compact: 0.88, normal: 1, spacious: 1.18 };
  const defaultSettings = {
    fontScale: 1,
    theme: "system",
    fontFamily: "serif",
    lineHeight: "normal",
  };
  const settings = { ...defaultSettings, ...safeJSON(localStorage.getItem(SETTINGS_KEY)) };

  function safeJSON(raw) { try { return raw ? JSON.parse(raw) : {}; } catch (_) { return {}; } }

  // Re-applied to the EPUB iframe theme below; defined so applySettings can call it.
  let applyEpubTheme = () => {};

  function applySettings() {
    document.documentElement.style.setProperty("--reader-font-scale", String(settings.fontScale));

    const lineScale = LINE_HEIGHTS[settings.lineHeight] ?? 1;
    document.documentElement.style.setProperty("--reader-line-scale", String(lineScale));

    const family = FONT_FAMILIES[settings.fontFamily];
    document.documentElement.style.setProperty(
      "--reader-font",
      family && family.length > 0 ? family : "inherit"
    );

    if (settings.theme === "system") {
      document.body.removeAttribute("data-reader-theme");
    } else {
      document.body.setAttribute("data-reader-theme", settings.theme);
    }

    const display = document.querySelector("[data-font-scale-display]");
    if (display) display.textContent = `${Math.round(settings.fontScale * 100)}%`;

    document.querySelectorAll(".theme-swatch").forEach((btn) => {
      const on = btn.dataset.theme === settings.theme;
      btn.setAttribute("aria-pressed", String(on));
      btn.setAttribute("aria-checked", String(on));
    });
    document.querySelectorAll(".font-swatch").forEach((btn) => {
      const on = btn.dataset.fontFamily === settings.fontFamily;
      btn.setAttribute("aria-pressed", String(on));
      btn.setAttribute("aria-checked", String(on));
    });
    document.querySelectorAll(".line-swatch").forEach((btn) => {
      const on = btn.dataset.lineHeight === settings.lineHeight;
      btn.setAttribute("aria-pressed", String(on));
      btn.setAttribute("aria-checked", String(on));
    });

    const range = document.querySelector("[data-font-scale]");
    if (range) range.value = String(settings.fontScale);

    // Push changes into the EPUB iframe (no-op for non-EPUB readers)
    applyEpubTheme();
  }
  function persistSettings() {
    localStorage.setItem(SETTINGS_KEY, JSON.stringify(settings));
  }
  applySettings();

  document.querySelector("[data-font-scale]")?.addEventListener("input", (event) => {
    settings.fontScale = Number(event.target.value);
    applySettings();
    persistSettings();
  });
  document.querySelectorAll(".theme-swatch").forEach((btn) => {
    btn.addEventListener("click", () => {
      settings.theme = btn.dataset.theme;
      applySettings();
      persistSettings();
    });
  });
  document.querySelectorAll(".font-swatch").forEach((btn) => {
    btn.addEventListener("click", () => {
      settings.fontFamily = btn.dataset.fontFamily;
      applySettings();
      persistSettings();
    });
  });
  document.querySelectorAll(".line-swatch").forEach((btn) => {
    btn.addEventListener("click", () => {
      settings.lineHeight = btn.dataset.lineHeight;
      applySettings();
      persistSettings();
    });
  });

  // Expose the settings + helpers so the EPUB block can build its theme.
  window.__mocoReaderSettings = { settings, FONT_FAMILIES, LINE_HEIGHTS, registerEpubThemeHook: (fn) => { applyEpubTheme = fn; } };

  // ----- Fullscreen API -----
  const fullscreenButton = document.querySelector("[data-fullscreen-toggle]");
  if (fullscreenButton) {
    fullscreenButton.addEventListener("click", async () => {
      try {
        if (document.fullscreenElement) {
          await document.exitFullscreen();
        } else {
          await document.documentElement.requestFullscreen();
        }
      } catch (_) {
        toast("Fullscreen isn't available in this browser.", "error");
      }
    });
    document.addEventListener("fullscreenchange", () => {
      const on = !!document.fullscreenElement;
      fullscreenButton.setAttribute("aria-pressed", String(on));
      fullscreenButton.title = on ? "Exit fullscreen" : "Enter fullscreen";
      fullscreenButton.textContent = on ? "⤧" : "⛶";
    });
  }

  // ----- Save highlight (selection-gated) -----
  const highlightButton = document.querySelector("[data-save-highlight]");
  if (highlightButton) {
    if (readerKind === "pdf") {
      highlightButton.disabled = false;
      highlightButton.textContent = "Add page note";
      highlightButton.title = "Add a note for the current PDF page";
    } else {
      highlightButton.disabled = true;
      highlightButton.title = "Select text in the reader to enable.";
    }
  }
  function setHighlightEnabled(enabled) {
    if (!highlightButton || readerKind === "pdf") return;
    highlightButton.disabled = !enabled;
    highlightButton.title = enabled ? "Save the current selection" : "Select text in the reader to enable.";
  }

  const saveHighlight = async (locator, text) => {
    if (!text) {
      toast("Select some text in the reader first.", "error");
      return;
    }
    try {
      const payload = await requestJSON(`/api/v1/books/${bookID}/highlights`, {
        method: "POST",
        body: JSON.stringify({ locator, selectedText: text, color: "amber" }),
      });
      clearEmptyHighlightState();
      highlightsList?.prepend(buildHighlightCard(payload.highlight));
      currentHighlights.unshift(payload.highlight);
      if (readerKind === "md") {
        const readerContent = document.getElementById("reader-content");
        renderMarkdownHighlights(readerContent);
      }
      toast("Highlight saved.", "success");
    } catch (error) {
      toast(error.message || "Could not save highlight.", "error");
    }
  };

  // ----- Markdown reader -----
  if (readerKind === "md") {
    const readerContent = document.getElementById("reader-content");
    renderMarkdownHighlights(readerContent);
    enrichCodeBlocks(readerContent);
    let lastProgressLocator = null;
    let activeHeadingId = null;

    const tocLinks = Array.from(document.querySelectorAll(".toc-list a[href^='#']"));

    function setActiveTOC(id) {
      if (!id || id === activeHeadingId) return;
      activeHeadingId = id;
      tocLinks.forEach((a) => {
        const li = a.closest("li");
        if (!li) return;
        li.classList.toggle("current", a.getAttribute("href") === "#" + id);
      });
    }

    requestJSON(`/api/v1/books/${bookID}/progress`).then((data) => {
      const locator = data.progress?.locator;
      if (locator) {
        lastProgressLocator = locator;
        const target = document.getElementById(locator);
        if (target) target.scrollIntoView({ block: "start" });
        showJumpButtonIfRelevant();
      }
    }).catch(() => {});

    let progressTimer = null;
    window.addEventListener("scroll", () => {
      window.clearTimeout(progressTimer);
      progressTimer = window.setTimeout(async () => {
        const headings = [...readerContent.querySelectorAll("h1,h2,h3,h4,h5,h6")];
        let active = headings[0];
        for (const heading of headings) {
          if (heading.getBoundingClientRect().top <= 120) active = heading;
        }
        const scrollHeight = document.documentElement.scrollHeight - window.innerHeight;
        const progressPercent = scrollHeight > 0
          ? Math.max(0, Math.min(100, window.scrollY / scrollHeight * 100))
          : 0;
        if (active) setActiveTOC(active.id);
        await saveProgress(active?.id || "start", progressPercent);
        showJumpButtonIfRelevant();
      }, 300);
    }, { passive: true });

    document.addEventListener("selectionchange", () => {
      const sel = window.getSelection();
      const text = sel ? sel.toString().trim() : "";
      const inReader = !!(text && sel.anchorNode && readerContent.contains(sel.anchorNode));
      setHighlightEnabled(inReader);
    });

    if (highlightButton) {
      highlightButton.addEventListener("click", async () => {
        const selection = window.getSelection();
        const text = selection ? selection.toString().trim() : "";
        if (!text) {
          toast("Select some text in the reader first.", "error");
          return;
        }
        let container = selection.anchorNode && selection.anchorNode.nodeType === Node.TEXT_NODE
          ? selection.anchorNode.parentElement
          : selection.anchorNode;
        if (!(container instanceof Element)) container = readerContent;
        const section = container.closest("[data-section]");
        const locator = section?.getAttribute("data-section") || "start";
        await saveHighlight(locator, text);
        selection.removeAllRanges();
        setHighlightEnabled(false);
      });
    }

    // Floating "jump to last position" button
    const jumpBtn = document.createElement("button");
    jumpBtn.type = "button";
    jumpBtn.className = "jump-to-progress";
    jumpBtn.setAttribute("aria-label", "Jump to last reading position");
    const arrow = document.createElement("span");
    arrow.setAttribute("aria-hidden", "true");
    arrow.textContent = "↧";
    const label = document.createElement("span");
    label.textContent = "Resume reading";
    jumpBtn.appendChild(arrow);
    jumpBtn.appendChild(label);
    jumpBtn.addEventListener("click", () => {
      if (!lastProgressLocator) return;
      const target = document.getElementById(lastProgressLocator);
      if (target) target.scrollIntoView({ block: "start", behavior: "smooth" });
    });
    document.body.appendChild(jumpBtn);

    function showJumpButtonIfRelevant() {
      if (!lastProgressLocator) return;
      const target = document.getElementById(lastProgressLocator);
      if (!target) return;
      const rect = target.getBoundingClientRect();
      const offscreen = rect.bottom < 0 || rect.top > window.innerHeight;
      jumpBtn.classList.toggle("is-visible", offscreen);
    }
  }

  // ----- EPUB reader -----
  if (readerKind === "epub") {
    const initEpub = async () => {
      // Wait for the epub.js + jszip CDN scripts to land. Both are deferred so
      // they may not be ready the instant app.js starts. Give them ~6 seconds.
      let attempts = 0;
      while ((!window.ePub || !window.JSZip) && attempts < 60) {
        await new Promise((r) => setTimeout(r, 100));
        attempts += 1;
      }
      if (!window.ePub) {
        toast("EPUB reader failed to load. Check your network or any content blocker.", "error");
        const stage = document.getElementById("reader-content");
        if (stage) stage.innerHTML = '<p class="reader-loading">Could not load the EPUB reader.</p>';
        return;
      }

      const tocList = document.querySelector("[data-dynamic-toc]");
      const prev = document.querySelector("[data-epub-prev]");
      const next = document.querySelector("[data-epub-next]");
      const stage = document.getElementById("reader-content");
      const loadingEl = stage?.querySelector("[data-reader-loading]");
      const showLoadingError = (msg) => {
        if (stage) stage.innerHTML = `<p class="reader-loading">${msg}</p>`;
      };
      const removeLoading = () => loadingEl?.remove();

      // Fetch the EPUB ourselves so we can surface network errors and pin a
      // 30s timeout. Letting epub.js do its own fetch can silently hang.
      let buffer;
      try {
        const controller = new AbortController();
        const timeout = setTimeout(() => controller.abort(), 30000);
        const response = await fetch(fileURL, {
          credentials: "same-origin",
          signal: controller.signal,
        });
        clearTimeout(timeout);
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        buffer = await response.arrayBuffer();
      } catch (error) {
        const reason = error.name === "AbortError" ? "Network timed out." : "Could not download the EPUB file.";
        showLoadingError(reason);
        toast(reason, "error");
        console.error("EPUB fetch error:", error);
        return;
      }

      let book, rendition;
      try {
        book = window.ePub(buffer);
      } catch (error) {
        showLoadingError("This EPUB file looks malformed.");
        toast("This EPUB file looks malformed.", "error");
        console.error("EPUB parse error:", error);
        return;
      }

      // Surface metadata-load failures (corrupt zip, missing OPF, etc.)
      book.opened.catch((err) => {
        showLoadingError("Could not open this EPUB file.");
        toast("Could not open this EPUB file.", "error");
        console.error("EPUB open error:", err);
      });

      // Wipe placeholder before handing the container to epub.js — it APPENDS
      // iframes, doesn't replace, so the placeholder would otherwise stay.
      removeLoading();

      try {
        rendition = book.renderTo("reader-content", {
          width: "100%",
          height: "100%",
          flow: "paginated",          // Kindle-style page turns
          spread: "auto",              // two-up on wide screens
          minSpreadWidth: 900,         // single-page below ~iPad portrait
          allowScriptedContent: false,
        });
      } catch (error) {
        showLoadingError("Could not initialize EPUB renderer.");
        toast("Could not initialize EPUB reader.", "error");
        console.error("EPUB renderTo error:", error);
        return;
      }

      // Build a theme from the current reader settings and push it into the
      // iframe. Code elements always force monospace so an EPUB that uses bare
      // <p> for shell commands still reads as code rather than serif prose.
      const settingsBag = window.__mocoReaderSettings;
      const buildTheme = () => {
        const s = settingsBag?.settings || {};
        const family = settingsBag?.FONT_FAMILIES?.[s.fontFamily];
        const lineScale = settingsBag?.LINE_HEIGHTS?.[s.lineHeight] ?? 1;
        const useFamily = family && family.length > 0; // "original" → leave EPUB CSS alone

        // Pick text colors per theme so the iframe matches the page chrome.
        let bodyColor = "#46382c", linkColor = "#905f36", headingColor = "#30261d";
        if (s.theme === "sepia") { bodyColor = "#5b4830"; headingColor = "#4a3a25"; linkColor = "#a0683a"; }
        if (s.theme === "dark")  { bodyColor = "#e7dcc7"; headingColor = "#f1e6d3"; linkColor = "#e9b27d"; }

        const theme = {
          "html, body": {
            background: "transparent !important",
            margin: "0",
            padding: "0",
            color: bodyColor,
            "-webkit-font-smoothing": "antialiased",
          },
          "body": { padding: "8px 18px" },
          "p, li, td, blockquote": {
            "line-height": String(1.55 * lineScale),
          },
          "h1, h2, h3, h4, h5, h6": { color: headingColor },
          "img": { "max-width": "100%", "height": "auto" },
          "a": { color: linkColor, "text-decoration": "underline" },
          // Force mono on actual code elements regardless of font family choice
          "pre, code, kbd, samp, tt, var": {
            "font-family": '"JetBrains Mono", "Fira Code", ui-monospace, SFMono-Regular, Menlo, Consolas, monospace !important',
            "font-size": "0.92em",
          },
          "pre": {
            background: s.theme === "dark" ? "rgba(0,0,0,0.35)" : "rgba(85, 66, 43, 0.06)",
            padding: "12px 14px",
            "border-radius": "8px",
            "overflow-x": "auto",
            "white-space": "pre-wrap",
            "word-break": "break-word",
          },
          "::-webkit-scrollbar": { display: "none" },
        };
        if (useFamily) {
          theme["html, body"]["font-family"] = family;
          theme["h1, h2, h3, h4, h5, h6"]["font-family"] = family;
        }
        return theme;
      };

      const refreshTheme = () => {
        if (!rendition) return;
        try {
          rendition.themes.register("moco", buildTheme());
          rendition.themes.select("moco");
        } catch (_) { /* themes are optional */ }
      };
      refreshTheme();
      // Wire so future setting changes re-paint the iframe.
      settingsBag?.registerEpubThemeHook(refreshTheme);

      // Belt-and-braces: even if the renderer never fires "rendered",
      // make sure the placeholder is gone and we don't show a frozen UI.
      const safety = setTimeout(removeLoading, 6000);
      rendition.on("rendered", () => clearTimeout(safety));

      try {
        await rendition.display();
      } catch (err) {
        toast("Could not display the EPUB.", "error");
        console.error("EPUB display error:", err);
      }

      book.loaded.navigation.then((navigation) => {
        tocList.innerHTML = "";
        if (!navigation.toc || navigation.toc.length === 0) {
          const li = document.createElement("li");
          li.innerHTML = '<span style="padding:12px 14px;display:block">No chapters found.</span>';
          tocList.appendChild(li);
          return;
        }
        navigation.toc.forEach((item) => {
          const li = document.createElement("li");
          const button = document.createElement("button");
          button.type = "button";
          button.textContent = item.label;
          button.addEventListener("click", () => rendition.display(item.href));
          li.appendChild(button);
          tocList.appendChild(li);
        });
      }).catch(() => {
        tocList.innerHTML = "";
        const li = document.createElement("li");
        li.textContent = "No table of contents.";
        tocList.appendChild(li);
      });

      requestJSON(`/api/v1/books/${bookID}/progress`).then((data) => {
        if (data.progress?.locator) {
          try { rendition.display(data.progress.locator); }
          catch (_) { /* invalid CFI from old data — ignore */ }
        }
      }).catch(() => {});

      rendition.on("relocated", (location) => {
        const locator = location?.start?.cfi || "";
        const progressPercent = location?.start?.percentage ? location.start.percentage * 100 : 0;
        saveProgress(locator, progressPercent);
      });

      prev?.addEventListener("click", () => rendition.prev());
      next?.addEventListener("click", () => rendition.next());

      document.addEventListener("keydown", (event) => {
        if (event.target.matches("input,textarea,select")) return;
        if (event.key === "ArrowLeft") rendition.prev();
        if (event.key === "ArrowRight") rendition.next();
      });

      if (highlightButton) {
        rendition.on("selected", (cfiRange, contents) => {
          const text = contents.window.getSelection()?.toString().trim() || "";
          setHighlightEnabled(!!text);
        });

        highlightButton.addEventListener("click", async () => {
          const contents = rendition.getContents()[0];
          const selection = contents?.window?.getSelection?.();
          const text = selection ? selection.toString().trim() : "";
          const locator = rendition.currentLocation()?.start?.cfi || "start";
          if (!text) {
            toast("Select some EPUB text first.", "error");
            return;
          }
          await saveHighlight(locator, text);
          selection.removeAllRanges();
          setHighlightEnabled(false);
        });
      }
    };

    initEpub();
  }

  // ----- PDF reader -----
  if (readerKind === "pdf") {
    const initPdf = async () => {
      while (!window.__mocoPdfjs) {
        await new Promise((resolve) => setTimeout(resolve, 50));
      }
      const pdfjsLib = window.__mocoPdfjs;
      const canvas = document.getElementById("pdf-canvas");
      const ctx = canvas.getContext("2d");
      const tocList = document.querySelector("[data-dynamic-toc]");
      const prev = document.querySelector("[data-pdf-prev]");
      const next = document.querySelector("[data-pdf-next]");
      const pageLabel = document.querySelector("[data-pdf-page]");
      let pdf;
      try {
        pdf = await pdfjsLib.getDocument(fileURL).promise;
      } catch (error) {
        toast("Could not load PDF.", "error");
        return;
      }
      let pageNum = 1;
      let rendering = false;

      const stage = document.querySelector(".pdf-stage");

      const renderPage = async (num) => {
        if (rendering) return;
        rendering = true;
        try {
          const page = await pdf.getPage(num);
          const dpr = window.devicePixelRatio || 1;

          // Fit the page to the available stage area — no horizontal/vertical
          // scrolling, Kindle-style "next page" via the Next button.
          const base = page.getViewport({ scale: 1 });
          const stageW = Math.max(stage.clientWidth - 24, 200);
          const stageH = Math.max(stage.clientHeight - 24, 200);
          const fitScale = Math.min(stageW / base.width, stageH / base.height);

          const viewport = page.getViewport({ scale: fitScale * dpr });
          canvas.width = viewport.width;
          canvas.height = viewport.height;
          canvas.style.width = `${viewport.width / dpr}px`;
          canvas.style.height = `${viewport.height / dpr}px`;
          await page.render({ canvasContext: ctx, viewport }).promise;
          if (pageLabel) pageLabel.textContent = `Page ${num} / ${pdf.numPages}`;
          await saveProgress(`page:${num}`, (num / pdf.numPages) * 100);
        } finally {
          rendering = false;
        }
      };

      // Re-render the current page when the viewport changes (rotation,
      // window resize, immersive toggle changing the stage height).
      let resizeTimer = null;
      window.addEventListener("resize", () => {
        clearTimeout(resizeTimer);
        resizeTimer = setTimeout(() => renderPage(pageNum), 150);
      });

      try {
        const data = await requestJSON(`/api/v1/books/${bookID}/progress`);
        const locator = data.progress?.locator || "";
        if (locator.startsWith("page:")) {
          const parsed = Number(locator.split(":")[1]);
          if (parsed > 0) pageNum = parsed;
        }
      } catch (_) { /* fresh book */ }
      await renderPage(pageNum);

      const goPrev = () => {
        if (pageNum <= 1) return;
        pageNum -= 1;
        renderPage(pageNum);
      };
      const goNext = () => {
        if (pageNum >= pdf.numPages) return;
        pageNum += 1;
        renderPage(pageNum);
      };
      prev?.addEventListener("click", goPrev);
      next?.addEventListener("click", goNext);

      document.addEventListener("keydown", (event) => {
        if (event.target.matches("input,textarea,select")) return;
        if (event.key === "ArrowLeft") goPrev();
        if (event.key === "ArrowRight") goNext();
      });

      try {
        const outline = await pdf.getOutline();
        tocList.innerHTML = "";
        if (!outline || outline.length === 0) {
          const li = document.createElement("li");
          const span = document.createElement("span");
          span.style.padding = "12px 14px";
          span.style.display = "block";
          span.textContent = "No outline available.";
          li.appendChild(span);
          tocList.appendChild(li);
        } else {
          outline.forEach((item) => {
            const li = document.createElement("li");
            const span = document.createElement("span");
            span.style.padding = "12px 14px";
            span.style.display = "block";
            span.textContent = item.title || "Untitled";
            li.appendChild(span);
            tocList.appendChild(li);
          });
        }
      } catch (_) {
        tocList.innerHTML = "";
        const li = document.createElement("li");
        li.textContent = "No outline available.";
        tocList.appendChild(li);
      }

      if (highlightButton) {
        highlightButton.addEventListener("click", async () => {
          const note = await openTextPrompt({
            title: `Add a note for page ${pageNum}`,
            body: "Text selection inside PDFs isn't supported in this build. Type a short note instead — it will be saved against this page.",
            placeholder: "What stood out on this page?",
            confirmLabel: "Save note",
          });
          if (!note) return;
          await saveHighlight(`page:${pageNum}`, note);
        });
      }
    };
    initPdf();
  }
}
