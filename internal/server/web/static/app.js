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
async function requestJSON(url, options = {}) {
  const response = await fetch(url, {
    credentials: "same-origin",
    headers: {
      Accept: "application/json",
      ...(options.body instanceof FormData ? {} : { "Content-Type": "application/json" }),
      ...(options.headers || {}),
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
    setMessage(message, `Selected ${file.name} (${formatBytes(file.size)})`);
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
      button.closest(".note-card")?.remove();
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
      const onlyCard = highlightsList?.querySelector(".note-card");
      if (onlyCard && onlyCard.textContent.includes("No highlights")) highlightsList.innerHTML = "";
      highlightsList?.prepend(buildHighlightCard(payload.highlight));
      toast("Highlight saved.", "success");
    } catch (error) {
      toast(error.message || "Could not save highlight.", "error");
    }
  };

  // ----- Markdown reader -----
  if (readerKind === "md") {
    const readerContent = document.getElementById("reader-content");
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
        const headings = [...readerContent.querySelectorAll("h1,h2,h3")];
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
        const section = container.closest("h1,h2,h3,[id]");
        const locator = section?.id || "start";
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
  if (readerKind === "epub" && window.ePub) {
    const tocList = document.querySelector("[data-dynamic-toc]");
    const prev = document.querySelector("[data-epub-prev]");
    const next = document.querySelector("[data-epub-next]");
    const book = window.ePub(fileURL);
    const rendition = book.renderTo("reader-content", {
      width: "100%",
      height: "100%",
      flow: "scrolled-doc",
    });
    rendition.display();

    book.loaded.navigation.then((navigation) => {
      tocList.innerHTML = "";
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
      if (data.progress?.locator) rendition.display(data.progress.locator);
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

      const renderPage = async (num) => {
        if (rendering) return;
        rendering = true;
        try {
          const page = await pdf.getPage(num);
          const dpr = window.devicePixelRatio || 1;
          const viewport = page.getViewport({ scale: 1.25 * dpr });
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
