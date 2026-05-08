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
  // If a book detail modal is open, popping history is required so the URL
  // returns to the underlying list. Outside-click + Escape also land here.
  const wasBookModal = modal.dataset.bookModal === "1";
  modal.classList.remove("is-open");
  modal.dataset.bookModal = "";
  modal.querySelector("[data-modal-card]").innerHTML = "";
  if (modalLastFocus && typeof modalLastFocus.focus === "function") {
    modalLastFocus.focus();
  }
  modalLastFocus = null;
  if (wasBookModal && history.state && history.state.bookModal) {
    history.back();
  }
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

  // Show/hide password toggle
  authForm.querySelectorAll("[data-toggle-password]").forEach((btn) => {
    const input = btn.parentElement?.querySelector('input[type="password"], input[type="text"]');
    btn.addEventListener("click", () => {
      if (!input) return;
      const showing = input.type === "text";
      input.type = showing ? "password" : "text";
      btn.textContent = showing ? "Show" : "Hide";
      btn.setAttribute("aria-pressed", String(!showing));
      btn.setAttribute("aria-label", showing ? "Show password" : "Hide password");
    });
  });

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

  const convertRow = uploadForm.querySelector("[data-convert-row]");
  const convertCheckbox = uploadForm.querySelector("[data-convert-checkbox]");
  const convertHint = uploadForm.querySelector("[data-convert-hint]");

  // Drop zone + reveal-on-pick metadata fields
  const dropzone        = uploadForm.querySelector("[data-dropzone]");
  const dropEmpty       = uploadForm.querySelector("[data-dropzone-empty]");
  const dropFilled      = uploadForm.querySelector("[data-dropzone-filled]");
  const dropFilename    = uploadForm.querySelector("[data-dropzone-filename]");
  const dropFilesize    = uploadForm.querySelector("[data-dropzone-filesize]");
  const dropFormat      = uploadForm.querySelector("[data-dropzone-format]");
  const dropRemove      = uploadForm.querySelector("[data-dropzone-remove]");
  const metaFields      = uploadForm.querySelector("[data-upload-meta]");
  const detectedMessage = uploadForm.querySelector("[data-upload-detected]");
  const titleInput      = uploadForm.querySelector("[data-upload-title]");
  const authorInput     = uploadForm.querySelector("[data-upload-author]");

  function updateConvertOption(file) {
    if (!convertRow) return;
    if (!file) {
      convertRow.hidden = true;
      if (convertCheckbox) convertCheckbox.checked = false;
      return;
    }
    const name = file.name.toLowerCase();
    const isPDF = name.endsWith(".pdf");
    const isMD = name.endsWith(".md") || name.endsWith(".markdown");
    if (!isPDF && !isMD) {
      convertRow.hidden = true;
      if (convertCheckbox) convertCheckbox.checked = false;
      return;
    }
    convertRow.hidden = false;
    if (convertHint) {
      convertHint.textContent = isPDF
        ? "Reflows the PDF text into an EPUB you can resize, theme, and dark-mode. Image fidelity depends on the original PDF — diagrams drawn as vector graphics may not transfer."
        : "Wraps the markdown into an EPUB with proper chapters and the full reading-style settings.";
    }
  }

  function clearFileSelection() {
    fileInput.value = "";
    if (dropEmpty) dropEmpty.hidden = false;
    if (dropFilled) dropFilled.hidden = true;
    if (metaFields) metaFields.hidden = true;
    if (titleInput) titleInput.value = "";
    if (authorInput) authorInput.value = "";
    if (detectedMessage) detectedMessage.textContent = "";
    if (submitBtn) submitBtn.disabled = true;
    updateConvertOption(null);
  }

  function applyFile(file) {
    if (!file) { clearFileSelection(); return; }
    if (!allowedExt.test(file.name)) {
      setMessage(message, "Only .pdf, .epub, and .md files are supported.", "error");
      clearFileSelection();
      return;
    }
    if (file.size > maxBytes) {
      setMessage(message, `File is too large (max ${Math.round(maxBytes / 1024 / 1024)}MB).`, "error");
      clearFileSelection();
      return;
    }

    // Sync into the actual file input via DataTransfer so form submit picks it up.
    const dt = new DataTransfer();
    dt.items.add(file);
    fileInput.files = dt.files;

    if (dropEmpty) dropEmpty.hidden = true;
    if (dropFilled) dropFilled.hidden = false;
    if (dropFilename) dropFilename.textContent = file.name;
    if (dropFilesize) dropFilesize.textContent = formatBytes(file.size);
    if (dropFormat) {
      const ext = file.name.split(".").pop().toUpperCase();
      dropFormat.textContent = ext === "MARKDOWN" ? "MD" : ext;
    }
    if (metaFields) metaFields.hidden = false;
    if (submitBtn) submitBtn.disabled = false;
    setMessage(message, "");
    updateConvertOption(file);

    // Inspect the file to auto-fill title and author.
    inspectFile(file);
  }

  async function inspectFile(file) {
    if (detectedMessage) detectedMessage.textContent = "Detecting title and author…";
    try {
      const fd = new FormData();
      fd.append("file", file);
      const csrf = getCookie("moco_csrf");
      const res = await fetch("/api/v1/books/inspect", {
        method: "POST",
        body: fd,
        headers: csrf ? { "X-CSRF-Token": csrf } : {},
        credentials: "same-origin",
      });
      if (!res.ok) throw new Error("inspect failed");
      const data = await res.json();
      if (titleInput && !titleInput.value) titleInput.value = data.title || "";
      if (authorInput && !authorInput.value) authorInput.value = data.author || "";
      if (detectedMessage) {
        const detected = [];
        if (data.title) detected.push("title");
        if (data.author) detected.push("author");
        detectedMessage.textContent = detected.length
          ? `Detected ${detected.join(" and ")} from the file — edit if needed.`
          : "We couldn't detect a title — please add one.";
      }
    } catch (_) {
      if (detectedMessage) {
        detectedMessage.textContent = "Could not auto-detect details — please fill them in.";
      }
      if (titleInput && !titleInput.value) {
        // Fallback: derive from filename
        titleInput.value = file.name.replace(/\.[^.]+$/, "").replace(/[_-]+/g, " ");
      }
    }
  }

  fileInput?.addEventListener("change", () => {
    const file = fileInput.files?.[0];
    applyFile(file || null);
  });

  dropRemove?.addEventListener("click", (event) => {
    event.preventDefault();
    clearFileSelection();
  });

  if (dropzone) {
    // Click anywhere on the empty zone opens the file picker
    dropEmpty?.addEventListener("click", () => fileInput.click());
    dropzone.addEventListener("keydown", (e) => {
      if (e.key === "Enter" || e.key === " ") {
        if (dropFilled?.hidden !== false) {
          e.preventDefault();
          fileInput.click();
        }
      }
    });
    ["dragenter", "dragover"].forEach((ev) => {
      dropzone.addEventListener(ev, (e) => {
        e.preventDefault();
        e.stopPropagation();
        dropzone.classList.add("is-dragging");
      });
    });
    ["dragleave", "drop"].forEach((ev) => {
      dropzone.addEventListener(ev, (e) => {
        if (ev === "dragleave" && dropzone.contains(e.relatedTarget)) return;
        dropzone.classList.remove("is-dragging");
      });
    });
    dropzone.addEventListener("drop", (e) => {
      e.preventDefault();
      const file = e.dataTransfer?.files?.[0];
      if (file) applyFile(file);
    });
  }

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
    xhr.upload.addEventListener("load", () => {
      // Upload finished; if conversion is requested the server may take a few
      // seconds to extract text and rebuild the EPUB. Tell the user.
      if (convertCheckbox?.checked) {
        setMessage(message, "Converting to EPUB… this can take a few seconds for large books.");
      } else {
        setMessage(message, "Saving…");
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
function attachDeleteBook(button) {
  if (button.dataset.wired === "1") return;
  button.dataset.wired = "1";
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
}
document.querySelectorAll("[data-delete-book]").forEach(attachDeleteBook);

// ---------- Toggle visibility ----------
function attachToggleVisibility(button) {
  if (button.dataset.wired === "1") return;
  button.dataset.wired = "1";
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
}
document.querySelectorAll("[data-toggle-visibility]").forEach(attachToggleVisibility);

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
      // Also strip the EPUB visual overlay if this is an EPUB
      if (window.__mocoEpubAnnotations?.remove) window.__mocoEpubAnnotations.remove(id);
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
  const isGuest = readerRoot.getAttribute("data-guest") === "1";

  function guestBlock(action) {
    toast(`Sign in to ${action}.`, "error");
    setTimeout(() => { window.location.href = "/login"; }, 1200);
  }

  // Shared bookmark state — each reader-kind block updates these on every
  // page/scroll/relocate event so the "Bookmark current page" button always
  // captures wherever the reader currently is. jumpToLocator is set by each
  // reader so the bookmarks list can navigate cross-format.
  let latestLocator = "";
  let latestLabel = "";
  let jumpToLocator = null;
  function setReaderPosition(locator, label) {
    if (locator) latestLocator = locator;
    if (label) latestLabel = label;
  }
  const progressStatus = document.querySelector("[data-progress-status]");
  const highlightsList = document.querySelector("[data-highlights-list]");
  const saveProgress = isGuest
    ? async () => {} // guests don't have an account to save progress to
    : saveReaderProgressFactory(bookID, progressStatus);
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

  // ----- Immersive mode -----
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
  const floatingClose = document.querySelector(".reader-floating-close");
  function syncFloatingClose() {
    if (!floatingClose) return;
    floatingClose.classList.toggle("is-visible", readerApp.classList.contains("is-immersive"));
  }
  function clearImmersiveTimer() {
    if (immersiveTimer) { clearTimeout(immersiveTimer); immersiveTimer = null; }
  }
  function scheduleHide(delay = IMMERSIVE_DELAY) {
    clearImmersiveTimer();
    immersiveTimer = setTimeout(() => {
      if (panelOrModalOpen() || hasSelection()) return;
      readerApp.classList.add("is-immersive");
      syncFloatingClose();
    }, delay);
  }
  function showChrome({ keepShown = false } = {}) {
    readerApp.classList.remove("is-immersive");
    syncFloatingClose();
    if (keepShown) clearImmersiveTimer();
    else scheduleHide();
  }
  function toggleImmersive() {
    if (readerApp.classList.contains("is-immersive")) {
      showChrome();
    } else {
      clearImmersiveTimer();
      readerApp.classList.add("is-immersive");
      syncFloatingClose();
    }
  }
  syncFloatingClose();

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
    // EPUB page layout: "auto" = two-up on wide screens, "single" = one page always.
    spread: "auto",
  };
  const settings = { ...defaultSettings, ...safeJSON(localStorage.getItem(SETTINGS_KEY)) };

  function safeJSON(raw) { try { return raw ? JSON.parse(raw) : {}; } catch (_) { return {}; } }

  // Re-applied to the EPUB iframe theme below; defined so applySettings can call it.
  let applyEpubTheme = () => {};
  // Re-applied to the EPUB rendition spread mode below.
  let applyEpubSpread = () => {};

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
    document.querySelectorAll(".spread-swatch").forEach((btn) => {
      const on = btn.dataset.spread === settings.spread;
      btn.setAttribute("aria-pressed", String(on));
      btn.setAttribute("aria-checked", String(on));
    });

    const range = document.querySelector("[data-font-scale]");
    if (range) range.value = String(settings.fontScale);

    // Push changes into the EPUB iframe (no-op for non-EPUB readers)
    applyEpubTheme();
    applyEpubSpread();
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
  document.querySelectorAll(".spread-swatch").forEach((btn) => {
    btn.addEventListener("click", () => {
      settings.spread = btn.dataset.spread;
      applySettings();
      persistSettings();
    });
  });

  // Hide the EPUB-only settings rows on non-EPUB readers (PDF, Markdown).
  if (readerKind !== "epub") {
    document.querySelectorAll("[data-epub-only]").forEach((el) => { el.style.display = "none"; });
  }

  // Expose the settings + helpers so the EPUB block can build its theme.
  window.__mocoReaderSettings = {
    settings, FONT_FAMILIES, LINE_HEIGHTS,
    registerEpubThemeHook:  (fn) => { applyEpubTheme  = fn; },
    registerEpubSpreadHook: (fn) => { applyEpubSpread = fn; },
  };

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
    jumpToLocator = (locator) => {
      if (!locator) return;
      if (locator === "start" || locator === "top") {
        window.scrollTo({ top: 0, behavior: "smooth" });
        return;
      }
      const target = document.getElementById(locator);
      if (target) target.scrollIntoView({ block: "start", behavior: "smooth" });
    };
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
        const locator = active?.id || "start";
        const label = active ? (active.textContent || "").trim().slice(0, 80) : "Top of book";
        setReaderPosition(locator, label || locator);
        await saveProgress(locator, progressPercent);
        showJumpButtonIfRelevant();
      }, 300);
    }, { passive: true });

    // Selection popover for highlights — same UX as the EPUB reader.
    ensureSelectionPopover();
    let mdPendingSelection = null;

    function locatorFromNode(node) {
      let el = node && node.nodeType === Node.TEXT_NODE ? node.parentElement : node;
      if (!(el instanceof Element)) return "start";
      const anchor = el.closest("[id]");
      return anchor?.id || "start";
    }

    // Show the popover only AFTER the user releases the pointer — not on
    // every selectionchange tick. Otherwise the popover jitters during drag
    // and lingers when the selection is dismissed via keyboard / focus.
    function settleMarkdownSelection() {
      const sel = window.getSelection();
      const text = sel ? sel.toString().trim() : "";
      const inReader = !!(text && sel.anchorNode && readerContent.contains(sel.anchorNode));
      if (!inReader || sel.rangeCount === 0) {
        hideSelectionPopover();
        mdPendingSelection = null;
        return;
      }
      const range = sel.getRangeAt(0);
      const rect = range.getBoundingClientRect();
      mdPendingSelection = { text, locator: locatorFromNode(sel.anchorNode) };
      showSelectionPopoverNear(rect);
    }

    document.addEventListener("mouseup", () => setTimeout(settleMarkdownSelection, 10));
    document.addEventListener("touchend", () => setTimeout(settleMarkdownSelection, 10), { passive: true });

    // Hard hide whenever the document selection is cleared (catches keyboard
    // deselect, focus changes, page navigation).
    document.addEventListener("selectionchange", () => {
      const sel = window.getSelection();
      if (!sel || !sel.toString().trim()) {
        hideSelectionPopover();
        mdPendingSelection = null;
      }
    });

    attachPopoverHighlight(async () => {
      if (isGuest) {
        hideSelectionPopover();
        guestBlock("save highlights");
        return;
      }
      if (!mdPendingSelection) return;
      const { locator, text } = mdPendingSelection;
      const color = window.__mocoActiveHighlightColor?.() || "amber";
      try {
        const payload = await requestJSON(`/api/v1/books/${bookID}/highlights`, {
          method: "POST",
          body: JSON.stringify({ locator, selectedText: text, color }),
        });
        clearEmptyHighlightState();
        highlightsList?.prepend(buildHighlightCard(payload.highlight));
        currentHighlights.unshift(payload.highlight);
        renderMarkdownHighlights(readerContent);
        toast("Highlighted.", "success");
      } catch (err) {
        toast(err.message || "Could not save highlight.", "error");
      } finally {
        window.getSelection()?.removeAllRanges();
        hideSelectionPopover();
        mdPendingSelection = null;
      }
    });

    // The bottom Save Highlight button is replaced by the popover.
    if (highlightButton) {
      highlightButton.style.display = "none";
      const noticeEl = document.querySelector("[data-highlight-message]");
      if (noticeEl) noticeEl.textContent = "Select text in the book to highlight it.";
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

      // "single" forces one page even on wide screens; "auto" keeps the
      // two-up spread above minSpreadWidth.
      const initialSpread = (window.__mocoReaderSettings?.settings?.spread === "single") ? "none" : "auto";
      try {
        rendition = book.renderTo("reader-content", {
          width: "100%",
          height: "100%",
          flow: "paginated",          // page-turn layout (no scroll)
          spread: initialSpread,
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
        if (s.theme === "paper") { bodyColor = "#3c3225"; headingColor = "#2a241c"; linkColor = "#8a5a30"; }
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

      // Live spread mode updates: "single" = always one page, "auto" = two-up
      // when the viewport is wider than minSpreadWidth.
      settingsBag?.registerEpubSpreadHook(() => {
        if (!rendition) return;
        const value = (settingsBag.settings.spread === "single") ? "none" : "auto";
        try {
          rendition.spread(value, 900);
        } catch (err) {
          console.warn("EPUB spread update failed:", err);
        }
      });

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
        // Render top-level + nested entries so users can jump to subsections
        // straight from the Contents sidebar.
        const renderItems = (items, depth = 0) => {
          items.forEach((item) => {
            const li = document.createElement("li");
            const button = document.createElement("button");
            button.type = "button";
            button.textContent = item.label.trim();
            if (depth > 0) button.style.paddingLeft = `${14 + depth * 14}px`;
            button.addEventListener("click", () => rendition.display(item.href));
            li.appendChild(button);
            tocList.appendChild(li);
            if (item.subitems?.length) renderItems(item.subitems, depth + 1);
          });
        };
        renderItems(navigation.toc);
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

      // EPUBs don't have intrinsic page numbers — epub.js builds "locations"
      // from the spine. We pre-generate them so the user gets a jump-to-page
      // input similar to the PDF reader. ~1024 chars per location is the
      // standard heuristic.
      const epubPageInput = document.querySelector("[data-epub-page-input]");
      const epubPageTotal = document.querySelector("[data-epub-page-total]");
      let epubLocationsReady = false;
      book.locations.generate(1024).then(() => {
        epubLocationsReady = true;
        if (epubPageTotal) epubPageTotal.textContent = `/ ${book.locations.length()}`;
        if (epubPageInput) {
          epubPageInput.disabled = false;
          epubPageInput.max = book.locations.length();
        }
      }).catch(() => {
        if (epubPageTotal) epubPageTotal.textContent = "/ —";
      });

      rendition.on("relocated", (location) => {
        const locator = location?.start?.cfi || "";
        const progressPercent = location?.start?.percentage ? location.start.percentage * 100 : 0;
        saveProgress(locator, progressPercent);
        if (prev) prev.disabled = !!location?.atStart;
        if (next) next.disabled = !!location?.atEnd;
        let pageIdx = 0;
        if (epubLocationsReady) {
          pageIdx = book.locations.locationFromCfi(locator);
          if (pageIdx > 0 && epubPageInput && document.activeElement !== epubPageInput) {
            epubPageInput.value = pageIdx;
          }
        }
        const label = pageIdx > 0 ? `Page ${pageIdx}` : "";
        setReaderPosition(locator, label);
      });
      jumpToLocator = (locator) => {
        if (!locator) return;
        try { rendition.display(locator); } catch (_) {}
      };

      function jumpToEpubPage() {
        if (!epubLocationsReady) return;
        const total = book.locations.length();
        let n = parseInt(epubPageInput.value, 10);
        if (isNaN(n) || n < 1) n = 1;
        if (n > total) n = total;
        epubPageInput.value = n;
        const cfi = book.locations.cfiFromLocation(n);
        if (cfi) rendition.display(cfi);
      }
      epubPageInput?.addEventListener("change", jumpToEpubPage);
      epubPageInput?.addEventListener("keydown", (event) => {
        if (event.key === "Enter") { event.preventDefault(); jumpToEpubPage(); }
      });

      prev?.addEventListener("click", () => { if (!prev.disabled) rendition.prev(); });
      next?.addEventListener("click", () => { if (!next.disabled) rendition.next(); });

      document.addEventListener("keydown", (event) => {
        if (event.target.matches("input,textarea,select")) return;
        if (event.key === "ArrowLeft") rendition.prev();
        if (event.key === "ArrowRight") rendition.next();
      });

      // Taps inside the iframe never reach the parent document, so wire the
      // chrome-toggle and swipe-to-page-turn through epub.js's content hooks.
      rendition.hooks.content.register((contents) => {
        const doc = contents.document;
        const win = contents.window;
        let sx = 0, sy = 0, st = 0, moved = false;
        const SWIPE_MIN = 50;
        const VERTICAL_TOLERANCE = 60;
        const TIME_LIMIT = 600;

        // Any new pointer interaction inside the page tears the popover down.
        // A fresh selection will re-summon it via the `selected` event.
        doc.addEventListener("mousedown", hideSelectionPopover);

        doc.addEventListener("touchstart", (e) => {
          const t = e.touches[0];
          sx = t.clientX; sy = t.clientY; st = Date.now(); moved = false;
          hideSelectionPopover();
        }, { passive: true });

        doc.addEventListener("touchmove", (e) => {
          const t = e.touches[0];
          if (Math.abs(t.clientX - sx) > 6 || Math.abs(t.clientY - sy) > 6) moved = true;
        }, { passive: true });

        doc.addEventListener("touchend", (e) => {
          const elapsed = Date.now() - st;
          const t = e.changedTouches[0];
          const dx = t.clientX - sx;
          const dy = Math.abs(t.clientY - sy);
          // Swipe → page turn
          if (elapsed < TIME_LIMIT && Math.abs(dx) >= SWIPE_MIN && dy <= VERTICAL_TOLERANCE) {
            if (dx > 0) rendition.prev();
            else rendition.next();
            return;
          }
          // Plain tap (no movement, no selection) → toggle chrome
          if (!moved && elapsed < 350) {
            const sel = win.getSelection();
            if (sel && sel.toString().trim()) return;
            const target = e.target.closest("a, button, input, textarea, select");
            if (target) return;
            toggleImmersive();
          }
        }, { passive: true });

        // Desktop: clicks inside the iframe should also toggle chrome
        doc.addEventListener("click", (e) => {
          // Touch handler already toggled — avoid double-toggle on devices
          // that fire both touchend and click.
          if (st && Date.now() - st < 600) return;
          const sel = win.getSelection();
          if (sel && sel.toString().trim()) return;
          if (e.target.closest("a, button, input, textarea, select")) return;
          toggleImmersive();
        });

        // The "selected" event only fires when text becomes selected. Clearing
        // the selection produces no event, so the popover would linger. Watch
        // for selection changes inside the iframe and hide it when empty.
        doc.addEventListener("selectionchange", () => {
          const sel = win.getSelection();
          if (!sel || !sel.toString().trim()) hideSelectionPopover();
        });
      });

      // ---- Highlight overlay ----
      const HIGHLIGHT_COLORS = {
        amber: "#d6ad68",
        sage:  "#8d9b77",
        rose:  "#c98072",
      };
      const stylesFor = (color) => ({
        fill: HIGHLIGHT_COLORS[color] || HIGHLIGHT_COLORS.amber,
        "fill-opacity": "0.42",
        "mix-blend-mode": "multiply",
      });
      const HIGHLIGHT_STYLES = stylesFor("amber");
      // id → cfiRange so we can remove the visual overlay on delete
      const annotationIndex = new Map();
      window.__mocoEpubAnnotations = {
        rendition,
        index: annotationIndex,
        remove(id) {
          const cfi = annotationIndex.get(id);
          if (!cfi) return;
          try { rendition.annotations.remove(cfi, "highlight"); } catch (_) {}
          annotationIndex.delete(id);
        },
      };

      // ---- Selection popover ----
      ensureSelectionPopover();
      let pendingSelection = null;

      rendition.on("selected", (cfiRange, contents) => {
        const sel = contents.window.getSelection();
        const text = sel?.toString().trim() || "";
        if (!text || sel.rangeCount === 0) {
          hideSelectionPopover();
          pendingSelection = null;
          return;
        }
        pendingSelection = { cfiRange, text, contents };

        const range = sel.getRangeAt(0);
        const rect = range.getBoundingClientRect();
        const frame = contents.document.defaultView.frameElement;
        const frameRect = frame ? frame.getBoundingClientRect() : { left: 0, top: 0 };
        showSelectionPopoverNear(rect, { left: frameRect.left, top: frameRect.top });
      });

      rendition.on("relocated", hideSelectionPopover);

      attachPopoverHighlight(async () => {
        if (isGuest) {
          hideSelectionPopover();
          guestBlock("save highlights");
          return;
        }
        if (!pendingSelection) return;
        const { cfiRange, text, contents } = pendingSelection;
        const color = window.__mocoActiveHighlightColor?.() || "amber";
        try {
          const payload = await requestJSON(`/api/v1/books/${bookID}/highlights`, {
            method: "POST",
            body: JSON.stringify({ locator: cfiRange, selectedText: text, color }),
          });
          try {
            rendition.annotations.highlight(
              cfiRange,
              { id: payload.highlight.id },
              null,
              "moco-highlight",
              stylesFor(color),
            );
            annotationIndex.set(payload.highlight.id, cfiRange);
          } catch (annErr) {
            console.warn("Could not paint highlight overlay:", annErr);
          }
          const onlyCard = highlightsList?.querySelector(".note-card");
          if (onlyCard && onlyCard.textContent.includes("No highlights")) highlightsList.innerHTML = "";
          highlightsList?.prepend(buildHighlightCard(payload.highlight));
          toast("Highlighted.", "success");
          contents.window.getSelection()?.removeAllRanges();
          hideSelectionPopover();
          pendingSelection = null;
        } catch (err) {
          toast(err.message || "Could not save highlight.", "error");
        }
      });

      // Restore previously saved highlights as visual overlays once the
      // book is open. CFIs that don't resolve (deleted chapters, etc.) are
      // skipped silently.
      try {
        const data = await requestJSON(`/api/v1/books/${bookID}/highlights`);
        (data.items || []).forEach((item) => {
          if (!item.locator || !item.locator.startsWith("epubcfi")) return;
          try {
            rendition.annotations.highlight(
              item.locator,
              { id: item.id },
              null,
              "moco-highlight",
              stylesFor(item.color),
            );
            annotationIndex.set(item.id, item.locator);
          } catch (_) { /* invalid CFI — skip */ }
        });
      } catch (_) {}

      // Disable the manual "Save highlight" button — the popover handles it now.
      if (highlightButton) {
        highlightButton.style.display = "none";
        const noticeEl = document.querySelector("[data-highlight-message]");
        if (noticeEl) noticeEl.textContent = "Select text in the book to highlight it.";
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
      const pageInput = document.querySelector("[data-pdf-page-input]");
      const pageTotal = document.querySelector("[data-pdf-page-total]");
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
          // scrolling; one page at a time via the Next button.
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
          if (pageInput) {
            pageInput.value = String(num);
            pageInput.max = String(pdf.numPages);
          }
          if (pageTotal) pageTotal.textContent = `/ ${pdf.numPages}`;
          if (prev) prev.disabled = num <= 1;
          if (next) next.disabled = num >= pdf.numPages;
          setReaderPosition(`page:${num}`, `Page ${num}`);
          await saveProgress(`page:${num}`, (num / pdf.numPages) * 100);
        } finally {
          rendering = false;
        }
      };
      jumpToLocator = (locator) => {
        if (!locator) return;
        const m = /^page:(\d+)$/.exec(locator);
        if (!m) return;
        const target = Math.min(Math.max(parseInt(m[1], 10) || 1, 1), pdf.numPages);
        pageNum = target;
        renderPage(pageNum);
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

      const goToPage = (target) => {
        const n = Math.max(1, Math.min(pdf.numPages, target | 0));
        if (n === pageNum) return;
        pageNum = n;
        renderPage(pageNum);
      };
      pageInput?.addEventListener("change", () => goToPage(Number(pageInput.value)));
      pageInput?.addEventListener("keydown", (e) => {
        if (e.key === "Enter") {
          e.preventDefault();
          goToPage(Number(pageInput.value));
          pageInput.blur();
        }
      });

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

  // ---------- Bookmarks panel ----------
  // Backend already exposes /api/v1/books/{id}/bookmarks (GET/POST) and
  // DELETE /api/v1/bookmarks/{id}. Reader-kind blocks above keep latestLocator
  // / latestLabel up-to-date and set jumpToLocator so this module can be
  // format-agnostic.
  const bookmarksList = document.querySelector("[data-bookmarks-list]");
  const addBookmarkBtn = document.querySelector("[data-add-bookmark]");

  function renderBookmarks(items) {
    if (!bookmarksList) return;
    if (!items.length) {
      bookmarksList.innerHTML = `<article class="note-card" data-bookmarks-empty><p>No bookmarks yet.</p></article>`;
      return;
    }
    bookmarksList.innerHTML = items.map((b) => {
      const safeLabel = (b.label || b.locator || "Bookmark").replace(/[<>&"]/g, (c) => ({"<":"&lt;",">":"&gt;","&":"&amp;","\"":"&quot;"})[c]);
      const safeLocator = (b.locator || "").replace(/"/g, "&quot;");
      const when = b.createdAt ? new Date(b.createdAt).toLocaleDateString() : "";
      return `
        <article class="note-card" data-bookmark-id="${b.id}">
          <p><strong>${safeLabel}</strong>${when ? ` <span class="note-meta">· ${when}</span>` : ""}</p>
          <div class="book-panel-actions">
            <button type="button" class="text-link" data-jump-bookmark="${safeLocator}">Jump to bookmark</button>
            <button type="button" class="text-link destructive" data-delete-bookmark="${b.id}">Delete</button>
          </div>
        </article>`;
    }).join("");
  }

  async function loadBookmarks() {
    if (!bookmarksList || isGuest) return;
    try {
      const data = await requestJSON(`/api/v1/books/${bookID}/bookmarks`);
      renderBookmarks(data.bookmarks || []);
    } catch (_) {
      bookmarksList.innerHTML = `<article class="note-card"><p>Couldn't load bookmarks.</p></article>`;
    }
  }

  addBookmarkBtn?.addEventListener("click", async () => {
    if (isGuest) { guestBlock("save bookmarks"); return; }
    if (!latestLocator) { toast("Open a page first.", "error"); return; }
    try {
      await requestJSON(`/api/v1/books/${bookID}/bookmarks`, {
        method: "POST",
        body: JSON.stringify({ locator: latestLocator, label: latestLabel || latestLocator }),
      });
      toast("Bookmarked.", "success");
      loadBookmarks();
    } catch (_) {
      toast("Couldn't save bookmark.", "error");
    }
  });

  bookmarksList?.addEventListener("click", async (event) => {
    const jump = event.target.closest("[data-jump-bookmark]");
    if (jump) {
      const locator = jump.getAttribute("data-jump-bookmark");
      if (typeof jumpToLocator === "function") {
        jumpToLocator(locator);
      } else {
        toast("Reader not ready.", "error");
      }
      // Close the panel after jumping so the user sees the page they landed on.
      document.querySelectorAll("[data-reader-panel]").forEach((p) => p.classList.remove("is-open"));
      return;
    }
    const del = event.target.closest("[data-delete-bookmark]");
    if (del) {
      const ok = await openConfirm({
        title: "Delete bookmark?",
        body: "This can't be undone.",
        confirmLabel: "Delete",
        danger: true,
      });
      if (!ok) return;
      try {
        await requestJSON(`/api/v1/bookmarks/${del.getAttribute("data-delete-bookmark")}`, { method: "DELETE" });
        loadBookmarks();
      } catch (_) {
        toast("Couldn't delete bookmark.", "error");
      }
    }
  });

  // Refresh the list when the panel is opened so newly added bookmarks from
  // another tab/device show up without a hard reload.
  document.querySelectorAll('[data-reader-toggle="bookmarks"]').forEach((btn) => {
    btn.addEventListener("click", () => loadBookmarks());
  });
  loadBookmarks();
}

// ---------- Service Worker registration ----------
// Self-healing strategy so users with a stale SW pick up new deploys without
// manual unregister. Three pieces:
//  1. updateViaCache:"none" → browser never serves /sw.js from HTTP cache.
//  2. Active update() on each page load → forces an immediate freshness check.
//  3. controllerchange listener + one-shot reload → when the new SW takes
//     over (skipWaiting + clients.claim already in the SW), the page reloads
//     itself once so the user sees the fresh CSS/JS without lifting a finger.
if ("serviceWorker" in navigator && location.protocol !== "http:") {
  window.addEventListener("load", () => {
    navigator.serviceWorker
      .register("/sw.js", { updateViaCache: "none" })
      .then((reg) => {
        // Force an update check on every load. Cheap on the wire (sw.js is
        // tiny and Cache-Control: no-cache means a 304 most of the time).
        reg.update().catch(() => {});

        // When a freshly installed SW finishes installing while another SW
        // is controlling the page, ours calls skipWaiting in install — so
        // it'll progress to "activated" quickly. Listen so we can prompt a
        // reload (or just reload) the moment it takes control.
        reg.addEventListener("updatefound", () => {
          const installing = reg.installing;
          if (!installing) return;
          installing.addEventListener("statechange", () => {
            if (installing.state === "activated" && navigator.serviceWorker.controller) {
              // controllerchange handles the actual reload; this state hook
              // is just defensive in case controllerchange doesn't fire on
              // some browsers.
            }
          });
        });
      })
      .catch(() => {});

    // One-shot reload on controller change. The session flag stops an infinite
    // loop on browsers that fire controllerchange more than once per claim.
    let reloaded = false;
    navigator.serviceWorker.addEventListener("controllerchange", () => {
      if (reloaded) return;
      reloaded = true;
      window.location.reload();
    });
  });
}

// ---------- Settings page: profile, password, delete-account ----------
const accountForm = document.querySelector("[data-account-form]");
if (accountForm) {
  const message = accountForm.querySelector("[data-account-message]");
  const submit = accountForm.querySelector('button[type="submit"]');
  accountForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    const data = new FormData(accountForm);
    const name = String(data.get("displayName") || "").trim();
    if (!name) { setMessage(message, "Display name is required.", "error"); return; }
    setButtonLoading(submit, true, "Saving…");
    try {
      await requestJSON("/api/v1/auth/me", { method: "PUT", body: JSON.stringify({ displayName: name }) });
      setMessage(message, "Saved.", "success");
      toast("Profile updated.", "success");
      // Reflect new display name in the topbar without a reload
      const chip = document.querySelector(".session-chip");
      if (chip) chip.textContent = name;
    } catch (err) {
      setMessage(message, err.message || "Could not save.", "error");
    } finally {
      setButtonLoading(submit, false);
    }
  });
}

const passwordForm = document.querySelector("[data-password-form]");
if (passwordForm) {
  const message = passwordForm.querySelector("[data-password-message]");
  const submit = passwordForm.querySelector('button[type="submit"]');
  passwordForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    const data = new FormData(passwordForm);
    const cur = String(data.get("currentPassword") || "");
    const nxt = String(data.get("newPassword") || "");
    if (nxt.length < 10) { setMessage(message, "New password must be at least 10 characters.", "error"); return; }
    if (cur === nxt)     { setMessage(message, "New password must differ from the current one.", "error"); return; }
    setButtonLoading(submit, true, "Changing…");
    try {
      await requestJSON("/api/v1/auth/password", {
        method: "PUT",
        body: JSON.stringify({ currentPassword: cur, newPassword: nxt }),
      });
      setMessage(message, "Password changed.", "success");
      toast("Password updated.", "success");
      passwordForm.reset();
    } catch (err) {
      setMessage(message, err.message || "Could not change password.", "error");
    } finally {
      setButtonLoading(submit, false);
    }
  });
  // Show/hide toggles for both fields
  passwordForm.querySelectorAll("[data-toggle-password]").forEach((btn) => {
    const input = btn.parentElement?.querySelector('input[type="password"], input[type="text"]');
    btn.addEventListener("click", () => {
      if (!input) return;
      const showing = input.type === "text";
      input.type = showing ? "password" : "text";
      btn.textContent = showing ? "Show" : "Hide";
      btn.setAttribute("aria-pressed", String(!showing));
    });
  });
}

const deleteAccountForm = document.querySelector("[data-delete-account-form]");
if (deleteAccountForm) {
  const message = deleteAccountForm.querySelector("[data-delete-account-message]");
  const submit = deleteAccountForm.querySelector('button[type="submit"]');
  deleteAccountForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    const confirmed = await openConfirm({
      title: "Delete your account?",
      body: "This permanently removes your account, all uploaded books, highlights, bookmarks, and tags. This cannot be undone.",
      confirmLabel: "Delete account",
      danger: true,
    });
    if (!confirmed) return;
    const data = new FormData(deleteAccountForm);
    const password = String(data.get("password") || "");
    setButtonLoading(submit, true, "Deleting…");
    try {
      await requestJSON("/api/v1/auth/me", { method: "DELETE", body: JSON.stringify({ password }) });
      window.location.href = "/";
    } catch (err) {
      setMessage(message, err.message || "Could not delete account.", "error");
      setButtonLoading(submit, false);
    }
  });
  deleteAccountForm.querySelectorAll("[data-toggle-password]").forEach((btn) => {
    const input = btn.parentElement?.querySelector('input[type="password"], input[type="text"]');
    btn.addEventListener("click", () => {
      if (!input) return;
      const showing = input.type === "text";
      input.type = showing ? "password" : "text";
      btn.textContent = showing ? "Show" : "Hide";
      btn.setAttribute("aria-pressed", String(!showing));
    });
  });
}

// ---------- Library: tag chips on book cards ----------
async function addTagToBook(bookID, tag) {
  const cleaned = String(tag || "").trim().toLowerCase().slice(0, 32);
  if (!cleaned) return null;
  const data = await requestJSON(`/api/v1/books/${bookID}/tags`, {
    method: "POST",
    body: JSON.stringify({ tag: cleaned }),
  });
  return data.tags || [];
}

function renderTagChips(container, bookID, tags) {
  if (!container) return;
  container.querySelectorAll(".tag-chip--removable").forEach((c) => c.remove());
  const addBtn = container.querySelector("[data-add-tag]");
  (tags || []).forEach((tag) => {
    const wrap = document.createElement("span");
    wrap.className = "tag-chip tag-chip--removable";
    wrap.innerHTML =
      `<a class="tag-chip-label" href="/app?tag=${encodeURIComponent(tag)}">#${escapeHTML(tag)}</a>` +
      `<button type="button" class="tag-chip-remove" data-remove-tag="${escapeHTML(tag)}" data-book-id="${escapeHTML(bookID)}" aria-label="Remove tag ${escapeHTML(tag)} from this book" title="Remove tag">×</button>`;
    container.insertBefore(wrap, addBtn);
    attachRemoveTag(wrap.querySelector("[data-remove-tag]"));
  });
}

function escapeHTML(s) {
  return String(s).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]);
}

function attachRemoveTag(button) {
  if (button.dataset.wired === "1") return;
  button.dataset.wired = "1";
  button.addEventListener("click", async () => {
    const bookID = button.dataset.bookId;
    const tag = button.dataset.removeTag;
    if (!bookID || !tag) return;
    try {
      await requestJSON(`/api/v1/books/${bookID}/tags/${encodeURIComponent(tag)}`, {
        method: "DELETE",
        body: "{}",
      });
      const chip = button.closest(".tag-chip--removable") || button;
      chip.remove();
    } catch (err) {
      toast(err.message || "Could not remove tag.", "error");
    }
  });
}

function attachAddTag(btn) {
  if (btn.dataset.wired === "1") return;
  btn.dataset.wired = "1";
  btn.addEventListener("click", async () => {
    const bookID = btn.dataset.bookId;
    if (!bookID) return;
    const value = await openTextPrompt({
      title: "Add a tag",
      body: "Tags are lowercase, up to 32 characters. Used for sorting and filtering your library.",
      placeholder: "e.g. fiction, tech, in-progress",
      confirmLabel: "Add",
    });
    if (!value) return;
    try {
      const tags = await addTagToBook(bookID, value);
      const card = btn.closest(".book-panel");
      const container = card?.querySelector("[data-book-tags]");
      renderTagChips(container, bookID, tags);
    } catch (err) {
      toast(err.message || "Could not add tag.", "error");
    }
  });
}

function attachShareBook(btn) {
  if (btn.dataset.wired === "1") return;
  btn.dataset.wired = "1";
  btn.addEventListener("click", () => openShareModal(btn.getAttribute("data-share-book")));
}

async function openShareModal(bookID) {
  if (!bookID) return;
  const modal = ensureModal();
  const card = modal.querySelector("[data-modal-card]");
  modalLastFocus = document.activeElement;
  card.innerHTML = `
    <h2>Share this book</h2>
    <p style="margin:0 0 14px;color:var(--muted);font-size:0.95rem">Anyone you share with needs a Moco account using the same email. They'll see this book in their "Shared with you" section.</p>
    <form data-share-form>
      <label style="display:block;margin-bottom:14px">
        <span style="display:block;font-size:0.85rem;color:var(--muted);margin-bottom:6px">Email address</span>
        <input type="email" name="email" autocomplete="email" required style="width:100%;padding:10px 12px;border:1px solid var(--line);border-radius:10px;background:rgba(255,255,255,0.6);color:var(--text)" />
      </label>
      <p class="form-message" data-share-message aria-live="polite"></p>
      <div class="modal-actions">
        <button type="button" class="button subtle" data-share-close>Close</button>
        <button type="submit" class="button primary">Share</button>
      </div>
    </form>
    <h3 style="font-size:0.95rem;margin:18px 0 8px">Currently shared with</h3>
    <div data-share-list><p class="form-message">Loading…</p></div>
  `;
  modal.classList.add("is-open");
  card.querySelector("input[name=email]").focus();

  const list = card.querySelector("[data-share-list]");
  const refreshList = async () => {
    try {
      const data = await requestJSON(`/api/v1/books/${bookID}/shares`);
      const items = data.items || [];
      if (items.length === 0) {
        list.innerHTML = `<p class="form-message">Not shared with anyone yet.</p>`;
        return;
      }
      list.innerHTML = `<ul style="list-style:none;padding:0;margin:0;display:grid;gap:8px">${items.map((s) => `
        <li style="display:flex;justify-content:space-between;align-items:center;padding:8px 12px;background:var(--accent-soft);border-radius:10px">
          <span>${escapeHTML(s.withUserEmail)}</span>
          <button type="button" class="text-link destructive" data-revoke="${escapeHTML(s.withUserId)}">Remove</button>
        </li>`).join("")}</ul>`;
      list.querySelectorAll("[data-revoke]").forEach((b) => {
        b.addEventListener("click", async () => {
          try {
            await requestJSON(`/api/v1/books/${bookID}/shares/${b.dataset.revoke}`, { method: "DELETE", body: "{}" });
            refreshList();
          } catch (err) {
            toast(err.message || "Could not remove.", "error");
          }
        });
      });
    } catch (err) {
      list.innerHTML = `<p class="form-message">Could not load shares.</p>`;
    }
  };
  refreshList();

  card.querySelector("[data-share-close]").addEventListener("click", closeModal);
  const form = card.querySelector("[data-share-form]");
  const message = card.querySelector("[data-share-message]");
  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    const email = form.email.value.trim();
    if (!email) return;
    setMessage(message, "Sending invite…");
    try {
      await requestJSON(`/api/v1/books/${bookID}/shares`, {
        method: "POST",
        body: JSON.stringify({ email }),
      });
      setMessage(message, `Shared with ${email}.`, "success");
      form.email.value = "";
      refreshList();
    } catch (err) {
      setMessage(message, err.message || "Could not share.", "error");
    }
  });
}

function attachWishlistToggle(btn) {
  if (btn.dataset.wired === "1") return;
  btn.dataset.wired = "1";
  btn.addEventListener("click", async () => {
    const bookID = btn.getAttribute("data-wishlist-toggle");
    if (!bookID) return;
    const onList = btn.getAttribute("data-wishlisted") === "1";
    setButtonLoading(btn, true);
    try {
      await requestJSON(`/api/v1/wishlist/${bookID}`, {
        method: onList ? "DELETE" : "POST",
        body: "{}",
      });
      const nowOnList = !onList;
      btn.setAttribute("data-wishlisted", nowOnList ? "1" : "0");
      btn.setAttribute("aria-pressed", String(nowOnList));
      btn.textContent = nowOnList ? "✓ On list" : "+ Want to read";
      toast(nowOnList ? "Added to your reading list." : "Removed from list.", "success");
    } catch (err) {
      toast(err.message || "Could not update list.", "error");
    } finally {
      setButtonLoading(btn, false);
    }
  });
}

function wireBookCardActions(scope) {
  scope.querySelectorAll("[data-delete-book]").forEach(attachDeleteBook);
  scope.querySelectorAll("[data-toggle-visibility]").forEach(attachToggleVisibility);
  scope.querySelectorAll("[data-remove-tag]").forEach(attachRemoveTag);
  scope.querySelectorAll("[data-add-tag]").forEach(attachAddTag);
  scope.querySelectorAll("[data-wishlist-toggle]").forEach(attachWishlistToggle);
  scope.querySelectorAll("[data-share-book]").forEach(attachShareBook);
}

document.querySelectorAll("[data-remove-tag]").forEach(attachRemoveTag);
document.querySelectorAll("[data-add-tag]").forEach(attachAddTag);
document.querySelectorAll("[data-wishlist-toggle]").forEach(attachWishlistToggle);
document.querySelectorAll("[data-share-book]").forEach(attachShareBook);

// ---------- Share-link button (book detail page) ----------
// Delegated on document so it survives any earlier scripting error and works
// for nodes injected later. Web Share API on supported devices (mobile share
// sheet); clipboard copy elsewhere; window.prompt as last resort.
document.addEventListener("click", async (event) => {
  const btn = event.target.closest("[data-share-button]");
  if (!btn) return;
  event.preventDefault();
  const url = btn.getAttribute("data-share-url") || window.location.href;
  const title = btn.getAttribute("data-share-title") || document.title;
  const toast = document.querySelector("[data-share-toast]");
  const showToast = (msg) => {
    if (toast) {
      toast.textContent = msg;
      toast.hidden = false;
      setTimeout(() => { toast.hidden = true; }, 2400);
    }
  };
  if (navigator.share) {
    try {
      await navigator.share({ title, url });
      return;
    } catch (_) {
      // user cancelled or share failed — fall through to copy
    }
  }
  if (navigator.clipboard && window.isSecureContext) {
    try {
      await navigator.clipboard.writeText(url);
      showToast("Link copied to clipboard.");
      return;
    } catch (_) {
      // fall through
    }
  }
  // Last-resort fallback for non-HTTPS contexts (clipboard API blocked).
  window.prompt("Copy this link:", url);
});

// ---------- Book detail modal (SPA-style) ----------
// Intercept clicks on book covers / row links that point at /books/{id} and
// open the detail card as a modal instead of navigating. URL is pushed to
// history so the entry remains shareable; popstate closes the modal so the
// browser back button works as expected.
const bookDetailURLRE = /^\/books\/([A-Za-z0-9_-]+)\/?$/;

function modalCardClass(extra = "") {
  const modal = ensureModal();
  const card = modal.querySelector("[data-modal-card]");
  card.className = "modal-card" + (extra ? " " + extra : "");
  return card;
}

async function openBookModal(href, { push = true } = {}) {
  const modal = ensureModal();
  const card = modalCardClass("is-wide");
  modalLastFocus = document.activeElement;

  // Loading placeholder while we fetch.
  card.innerHTML = `<div class="modal-loading" role="status" aria-live="polite">Loading…</div>`;
  modal.classList.add("is-open");
  modal.dataset.bookModal = "1";

  if (push && window.location.pathname + window.location.search !== href) {
    history.pushState({ bookModal: href }, "", href);
  }

  try {
    const res = await fetch(href + (href.includes("?") ? "&" : "?") + "fragment=1", {
      headers: { "X-Fragment": "1", Accept: "text/html" },
      credentials: "same-origin",
    });
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const html = await res.text();
    card.innerHTML = `
      <button type="button" class="modal-close" aria-label="Close" data-book-modal-close>×</button>
      ${html}
    `;
    // Re-wire any interactive elements inside the injected card so the
    // wishlist toggle, share button, etc. behave the same as on the full
    // page. The share button is delegated globally so no wiring needed; the
    // wishlist toggle is per-element.
    if (typeof wireBookCardActions === "function") {
      wireBookCardActions(card);
    }
  } catch (err) {
    card.innerHTML = `<p class="form-message">Couldn't load book. <a href="${href}">Open the page instead.</a></p>`;
  }
}

function closeBookModal({ pop = true } = {}) {
  const modal = document.querySelector("[data-modal-root]");
  if (!modal || modal.dataset.bookModal !== "1") return;
  if (!pop) {
    // Direct close without history pop (e.g. driven by popstate).
    modal.dataset.bookModal = "";
  }
  closeModal();
}

document.addEventListener("click", (event) => {
  // Close button inside modal.
  if (event.target.closest("[data-book-modal-close]")) {
    event.preventDefault();
    closeBookModal();
    return;
  }
  // Modifier-aware: let cmd/ctrl/middle clicks open in a new tab normally.
  if (event.metaKey || event.ctrlKey || event.shiftKey || event.button === 1) return;

  const link = event.target.closest("a[href]");
  if (!link) return;
  const url = new URL(link.href, window.location.origin);
  if (url.origin !== window.location.origin) return;
  const m = url.pathname.match(bookDetailURLRE);
  if (!m) return;

  // Skip the "Read now" / cover-on-detail-page links — those go to /read.
  // Only intercept links that target the bare /books/{id} URL.
  event.preventDefault();
  openBookModal(url.pathname + url.search);
});

window.addEventListener("popstate", (event) => {
  const modal = document.querySelector("[data-modal-root]");
  const open = modal && modal.classList.contains("is-open") && modal.dataset.bookModal === "1";
  // If user navigated forward to /books/{id} again, reopen.
  if (bookDetailURLRE.test(window.location.pathname)) {
    if (!open) openBookModal(window.location.pathname + window.location.search, { push: false });
    return;
  }
  // URL no longer matches a book — close the modal without pushing more state.
  if (open) closeBookModal({ pop: false });
});

// ---------- Auto-submitting filter forms (AJAX swap) ----------
// Forms with data-results-target="<selector>" fetch ?fragment=1 and replace
// the matching container's innerHTML instead of reloading the whole page.
function wireAutoSubmitForm(form) {
  if (form.dataset.wired === "1") return;
  form.dataset.wired = "1";
  const targetSelector = form.dataset.resultsTarget;
  let inflight = null;

  async function applyFilters() {
    const action = form.getAttribute("action") || window.location.pathname;
    const params = new URLSearchParams(new FormData(form));
    const url = `${action}?${params.toString()}`;

    if (!targetSelector) {
      window.location.assign(url);
      return;
    }
    const target = document.querySelector(targetSelector);
    if (!target) {
      window.location.assign(url);
      return;
    }

    history.replaceState(null, "", url);

    if (inflight) inflight.abort();
    inflight = new AbortController();
    target.classList.add("is-loading");
    try {
      const res = await fetch(`${url}${url.includes("?") ? "&" : "?"}fragment=1`, {
        headers: { "X-Fragment": "1", Accept: "text/html" },
        signal: inflight.signal,
        credentials: "same-origin",
      });
      if (!res.ok) throw new Error(`Request failed (${res.status})`);
      const html = await res.text();
      target.innerHTML = html;
      wireBookCardActions(target);
    } catch (err) {
      if (err.name !== "AbortError") {
        toast(err.message || "Could not update results.", "error");
      }
    } finally {
      target.classList.remove("is-loading");
    }
  }

  form.addEventListener("submit", (event) => {
    event.preventDefault();
    applyFilters();
  });
  form.querySelectorAll("select").forEach((select) => {
    select.addEventListener("change", applyFilters);
  });
  form.querySelectorAll('input[type="search"], input[type="text"]').forEach((input) => {
    let timer = null;
    input.addEventListener("input", () => {
      clearTimeout(timer);
      timer = setTimeout(applyFilters, 350);
    });
  });
}

document.querySelectorAll("form[data-auto-submit]").forEach(wireAutoSubmitForm);

// ---------- SPA-style top-nav navigation ----------
// Intercepts clicks on top-nav links between dashboard/quotes/stats/discover
// and swaps the <main> content via fetch instead of a full reload. The
// reader, settings, and login pages are excluded — they keep full-page loads.
const SPA_PATHS = ["/app", "/quotes", "/stats", "/discover"];

function isSpaPath(pathname) {
  return SPA_PATHS.some((p) => pathname === p || pathname.startsWith(p + "/") || pathname.startsWith(p + "?"));
}

async function spaNavigate(target, push) {
  const main = document.querySelector("main#main");
  if (!main) { window.location.assign(target); return; }
  main.classList.add("is-spa-loading");
  try {
    const res = await fetch(target, {
      headers: { Accept: "text/html" },
      credentials: "same-origin",
    });
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const html = await res.text();
    const doc = new DOMParser().parseFromString(html, "text/html");
    const newMain = doc.querySelector("main#main");
    if (!newMain) throw new Error("destination has no <main>");
    main.innerHTML = newMain.innerHTML;
    document.title = doc.title;

    // Sync the topbar's active-page state without losing event listeners by
    // mirroring aria-current attributes from the response.
    const newLinks = doc.querySelectorAll(".topnav a");
    document.querySelectorAll(".topnav a").forEach((link, i) => {
      const fresh = newLinks[i];
      if (!fresh) return;
      if (fresh.hasAttribute("aria-current")) {
        link.setAttribute("aria-current", fresh.getAttribute("aria-current"));
      } else {
        link.removeAttribute("aria-current");
      }
    });

    // Re-wire dynamic handlers in the swapped content.
    wireBookCardActions(main);
    main.querySelectorAll("form[data-auto-submit]").forEach(wireAutoSubmitForm);

    if (push) history.pushState({ spa: true }, "", target);
    window.scrollTo(0, 0);
  } catch (err) {
    // On failure, fall back to a full navigation so the user isn't stuck.
    console.warn("SPA navigation failed, falling back:", err);
    window.location.assign(target);
  } finally {
    main.classList.remove("is-spa-loading");
  }
}

document.addEventListener("click", (event) => {
  if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.button !== 0) return;
  const link = event.target.closest("a");
  if (!link) return;
  if (link.target === "_blank" || link.hasAttribute("download")) return;
  if (link.getAttribute("data-spa") === "off") return;
  let url;
  try { url = new URL(link.href, window.location.origin); }
  catch (_) { return; }
  if (url.origin !== window.location.origin) return;
  if (!isSpaPath(url.pathname)) return;
  // Don't intercept hash-only links.
  if (url.pathname === window.location.pathname && url.hash) return;
  event.preventDefault();
  spaNavigate(url.pathname + url.search + url.hash, true);
});

window.addEventListener("popstate", () => {
  if (isSpaPath(window.location.pathname)) {
    spaNavigate(window.location.pathname + window.location.search, false);
  }
});

// ---------- Selection popover (shared by EPUB + Markdown readers) ----------
// One instance is built lazily and reused. Helpers below handle positioning,
// color selection, and click-to-highlight binding for whichever reader is live.
let __mocoPopoverActiveColor = "amber";
window.__mocoActiveHighlightColor = () => __mocoPopoverActiveColor;

function ensureSelectionPopover() {
  let popover = document.querySelector(".selection-popover");
  if (popover) return popover;

  popover = document.createElement("div");
  popover.className = "selection-popover";
  popover.setAttribute("role", "menu");

  const btn = document.createElement("button");
  btn.type = "button";
  btn.dataset.popoverAction = "highlight";
  const swatch = document.createElement("span");
  swatch.className = "swatch";
  const label = document.createElement("span");
  label.textContent = "Highlight";
  btn.appendChild(swatch);
  btn.appendChild(label);
  popover.appendChild(btn);

  const colorRow = document.createElement("div");
  colorRow.className = "popover-colors";
  ["amber", "sage", "rose"].forEach((c) => {
    const dot = document.createElement("button");
    dot.type = "button";
    dot.className = `swatch swatch-${c}`;
    dot.dataset.color = c;
    dot.setAttribute("aria-label", `Highlight ${c}`);
    if (c === __mocoPopoverActiveColor) dot.setAttribute("aria-pressed", "true");
    dot.addEventListener("click", (e) => {
      e.stopPropagation();
      __mocoPopoverActiveColor = dot.dataset.color;
      colorRow.querySelectorAll(".swatch").forEach((d) => {
        d.setAttribute("aria-pressed", String(d === dot));
      });
    });
    colorRow.appendChild(dot);
  });
  popover.appendChild(colorRow);

  document.body.appendChild(popover);

  // Outside-click + global resets
  document.addEventListener("pointerdown", (event) => {
    if (popover.contains(event.target)) return;
    hideSelectionPopover();
  });
  window.addEventListener("resize", hideSelectionPopover);
  window.addEventListener("scroll", hideSelectionPopover, true);

  return popover;
}

function hideSelectionPopover() {
  const popover = document.querySelector(".selection-popover");
  if (popover) popover.classList.remove("is-open");
}

function showSelectionPopoverNear(rect, frameOffset = { left: 0, top: 0 }) {
  const popover = ensureSelectionPopover();
  const x = frameOffset.left + (rect.left + rect.right) / 2;
  let y = frameOffset.top + rect.top - 50;
  if (y < 12) y = frameOffset.top + rect.bottom + 14; // flip below if too close to top

  popover.style.visibility = "hidden";
  popover.classList.add("is-open");
  const popoverWidth = popover.offsetWidth;
  const left = Math.max(8, Math.min(window.innerWidth - popoverWidth - 8, x - popoverWidth / 2));
  popover.style.left = `${left}px`;
  popover.style.top = `${y}px`;
  popover.style.visibility = "";
}

function attachPopoverHighlight(handler) {
  const popover = ensureSelectionPopover();
  const btn = popover.querySelector('[data-popover-action="highlight"]');
  if (!btn) return;
  // Replace the node to clear any prior listener — readers re-bind on init.
  const fresh = btn.cloneNode(true);
  btn.replaceWith(fresh);
  fresh.addEventListener("click", (event) => {
    event.preventDefault();
    handler();
  });
}

// ---------- Reader: in-book search (Cmd/Ctrl+F is browser-native; we offer "/" as a quick command) ----------
// We open a simple search modal. For markdown reader: highlight matches in the
// DOM. For EPUB: use book.search() and jump to results. For PDF: skip (pdf.js
// has built-in search via its viewer).
(function wireInBookSearch() {
  const root = document.querySelector("[data-reader]");
  if (!root) return;
  const kind = root.getAttribute("data-reader-kind");
  if (kind === "pdf") return; // skip; pdf.js has its own

  document.addEventListener("keydown", (event) => {
    if (event.key !== "/" && !(event.key === "f" && (event.metaKey || event.ctrlKey))) return;
    if (event.target.matches("input,textarea,select")) return;
    event.preventDefault();
    runReaderSearch(kind);
  });
})();

async function runReaderSearch(kind) {
  const query = await openTextPrompt({
    title: "Search this book",
    body: kind === "epub" ? "Searches across loaded chapters." : "Find any text in the markdown.",
    placeholder: "What are you looking for?",
    confirmLabel: "Search",
  });
  if (!query) return;
  if (kind === "md") return mdSearch(query);
  if (kind === "epub") return epubSearch(query);
}

function mdSearch(query) {
  const root = document.getElementById("reader-content");
  if (!root) return;
  // Strip any prior highlights
  root.querySelectorAll(".search-hit").forEach((m) => {
    const t = document.createTextNode(m.textContent);
    m.replaceWith(t);
  });
  root.normalize();

  const needle = query.toLowerCase();
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, null);
  let count = 0;
  const hits = [];
  while (walker.nextNode()) {
    const node = walker.currentNode;
    const value = node.nodeValue || "";
    const lower = value.toLowerCase();
    if (!lower.includes(needle)) continue;
    const parts = lower.split(needle);
    const frag = document.createDocumentFragment();
    let cursor = 0;
    parts.forEach((part, i) => {
      frag.appendChild(document.createTextNode(value.slice(cursor, cursor + part.length)));
      cursor += part.length;
      if (i < parts.length - 1) {
        const span = document.createElement("mark");
        span.className = "search-hit";
        span.textContent = value.slice(cursor, cursor + needle.length);
        frag.appendChild(span);
        cursor += needle.length;
        count += 1;
      }
    });
    node.replaceWith(frag);
  }
  root.querySelectorAll(".search-hit").forEach((m) => hits.push(m));
  if (hits.length === 0) { toast(`No matches for "${query}"`, "error"); return; }
  hits[0].scrollIntoView({ block: "center", behavior: "smooth" });
  toast(`${count} match${count === 1 ? "" : "es"} for "${query}"`);
}

async function epubSearch(query) {
  const reg = window.__mocoEpubAnnotations;
  const rendition = reg?.rendition;
  if (!rendition) { toast("Reader not ready yet.", "error"); return; }
  const book = rendition.book;
  if (!book?.spine) { toast("This book doesn't expose a searchable spine.", "error"); return; }
  try {
    const all = [];
    // Search across all spine items — concurrency-light
    for (const section of book.spine.spineItems) {
      const item = await section.load(book.load.bind(book));
      const result = section.find(query);
      all.push(...(result || []));
      section.unload();
    }
    if (all.length === 0) { toast(`No matches for "${query}"`, "error"); return; }
    rendition.display(all[0].cfi);
    toast(`${all.length} match${all.length === 1 ? "" : "es"} — jumped to first.`, "success");
  } catch (err) {
    toast("Search failed.", "error");
    console.error(err);
  }
}
