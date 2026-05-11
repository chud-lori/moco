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
  const card = modal.querySelector("[data-modal-card]");
  card.innerHTML = "";
  // Reset card class — otherwise variants like `is-wide` (set by
  // openBookModal) leak into the next consumer (e.g. openConfirm), which
  // would otherwise render with 640px width + zero padding.
  card.className = "modal-card";
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
    card.className = "modal-card";
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
    card.className = "modal-card";
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
// Delegated on document so the listener survives SPA topbar swaps. The
// landing page's custom topbar gets replaced with the primary_topbar
// after login → /app, and a directly-bound listener on the original
// element wouldn't carry over to the new button.
document.addEventListener("click", async (event) => {
  const btn = event.target.closest("[data-logout-button]");
  if (!btn) return;
  setButtonLoading(btn, true, "Signing out…");
  try {
    await requestJSON("/api/v1/auth/logout", { method: "POST", body: "{}" });
    window.location.href = "/";
  } catch (error) {
    setButtonLoading(btn, false);
    toast(error.message || "Logout failed.", "error");
  }
});

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
  const convertDetails = uploadForm.querySelector("[data-convert-details]");
  const convertDetailsBody = uploadForm.querySelector("[data-convert-details-body]");

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
  const descInput       = uploadForm.querySelector("[data-upload-description]");
  const coverFileInput  = uploadForm.querySelector("[data-upload-cover]");
  const coverPreview    = uploadForm.querySelector("[data-cover-preview]");
  const coverPreviewImg = uploadForm.querySelector("[data-cover-preview-img]");
  // If the preview SVG fails (transient network blip, auth-cookie race
  // immediately after login, etc.) the browser would show its default
  // broken-image glyph + alt text. Swap in an inline cream placeholder
  // SVG so the preview area at least reads as "cover area" rather than
  // "something's broken." Re-enabled on every src change.
  if (coverPreviewImg) {
    const fallback = "data:image/svg+xml;utf8,"
      + encodeURIComponent(
        '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 120 180">'
        + '<rect width="120" height="180" fill="#ebe2cd"/>'
        + '<text x="60" y="95" font-family="serif" font-size="11" fill="#8a7a5e" text-anchor="middle">Cover preview</text>'
        + '</svg>'
      );
    coverPreviewImg.addEventListener("error", () => {
      if (coverPreviewImg.src !== fallback) coverPreviewImg.src = fallback;
    });
  }
  const coverSaltInput  = uploadForm.querySelector("[data-cover-salt-input]");
  const coverRerollBtn  = uploadForm.querySelector("[data-cover-preview-reroll]");
  const coverForceInput = uploadForm.querySelector("[data-cover-force-input]");
  const renderPdfBtn    = uploadForm.querySelector("[data-cover-render-pdf]");
  const useGeneratedBtn = uploadForm.querySelector("[data-cover-use-generated]");
  // True when the cover preview was rendered locally via PDF.js (vs.
  // server-side extraction). When true we ship the data URL as a Blob in
  // the upload's `cover` field, since the server can't re-render it
  // without mutool.
  let coverRenderedClientSide = false;
  // Tracks whether the user explicitly switched back to generated when an
  // extracted cover was available. Toggled by the two opposite buttons
  // below. Drives the hidden coverForce input which the server reads.
  let useGeneratedOverride = false;

  // Auto-generated cover preview state. The salt is what makes a re-roll
  // produce a different palette/variant; a random alphanumeric is enough
  // since it just feeds a hash. The preview <img> uses the /cover/preview
  // endpoint (no DB / storage write). When user uploads their own cover
  // file, the preview is hidden and salt is cleared so the upload handler
  // doesn't persist a generated cover.
  let coverSalt = randomShortSalt();
  function randomShortSalt() {
    return Math.random().toString(36).slice(2, 10);
  }
  // Pull static refs to the label/buttons so we can swap text + visibility
  // depending on whether we're previewing extracted vs generated.
  const coverPreviewLabel = uploadForm.querySelector(".cover-preview-label");

  // Belt-and-suspenders visibility: the [hidden] attribute alone is
  // overridden by `display: flex` on the parent, so we set inline display
  // too. Use this for any button inside .cover-preview-buttons.
  function setBtnVisible(el, visible) {
    if (!el) return;
    el.hidden = !visible;
    el.style.display = visible ? "" : "none";
  }

  function refreshCoverPreview() {
    if (!coverPreview || !coverPreviewImg || !coverSaltInput) return;
    // The user picking their own cover image overrides everything else —
    // show *their* image as the preview so they get visual confirmation
    // the file was accepted. The regenerate / first-page buttons don't
    // apply to a manually picked cover, so they're hidden here.
    const pickedCover = coverFileInput?.files?.[0];
    if (pickedCover) {
      // Revoke any prior object URL we created so we don't leak blobs as
      // the user picks different files in a row.
      if (coverPreviewImg.dataset.objectUrl) {
        URL.revokeObjectURL(coverPreviewImg.dataset.objectUrl);
        coverPreviewImg.dataset.objectUrl = "";
      }
      const url = URL.createObjectURL(pickedCover);
      coverPreviewImg.src = url;
      coverPreviewImg.dataset.objectUrl = url;
      coverSaltInput.value = "";
      if (coverForceInput) coverForceInput.value = "";
      useGeneratedOverride = false;
      if (coverPreviewLabel) {
        coverPreviewLabel.textContent = "Using the cover you uploaded.";
      }
      setBtnVisible(coverRerollBtn, false);
      setBtnVisible(renderPdfBtn, false);
      setBtnVisible(useGeneratedBtn, false);
      coverPreview.hidden = false;
      return;
    }
    // Two mutually-exclusive states drive what the preview shows:
    //   - "extracted": there's a real first-page cover (server extracted
    //     or client-rendered) and the user hasn't asked to override.
    //   - "generated": no extracted cover available, OR the user
    //     explicitly switched to a stylized generated cover.
    const showingExtracted = !!extractedCoverDataURL && !useGeneratedOverride;
    const f = fileInput?.files?.[0];
    const isPdf = !!(f && f.name.toLowerCase().endsWith(".pdf"));
    // First-page is reachable if we already extracted one OR we can render
    // a PDF page client-side. For MD/EPUB without a server-extracted cover
    // there is no first page to swap to.
    const canUseFirstPage = !!extractedCoverDataURL || isPdf;

    if (showingExtracted) {
      coverPreviewImg.src = extractedCoverDataURL;
      coverSaltInput.value = "";
      if (coverForceInput) coverForceInput.value = "";
      if (coverPreviewLabel) {
        coverPreviewLabel.textContent = coverRenderedClientSide
          ? "Using your book's first page (rendered from the PDF)."
          : "Using your book's first page.";
      }
      // Mutually exclusive: only the toggle to generated is offered here.
      setBtnVisible(coverRerollBtn, false);
      setBtnVisible(renderPdfBtn, false);
      setBtnVisible(useGeneratedBtn, true);
      coverPreview.hidden = false;
      return;
    }

    // Generated state.
    const title = (titleInput?.value || "").trim();
    if (!title) {
      coverPreview.hidden = true;
      coverSaltInput.value = "";
      return;
    }
    const ext = f ? f.name.split(".").pop().toLowerCase() : "";
    const params = new URLSearchParams({
      title,
      author: (authorInput?.value || "").trim(),
      format: ext === "markdown" ? "md" : ext,
      salt: coverSalt,
    });
    coverPreviewImg.src = `/api/v1/cover/preview?${params.toString()}`;
    coverSaltInput.value = coverSalt;
    // Tell the server to skip extraction and persist the generated SVG
    // when the user explicitly chose generated even though an extracted
    // cover is available. Otherwise (no extracted cover at all) the
    // override is unnecessary — server extraction would fail anyway.
    if (coverForceInput) coverForceInput.value = useGeneratedOverride ? "1" : "";
    if (coverPreviewLabel) {
      coverPreviewLabel.textContent = canUseFirstPage
        ? "Auto-generated from the title."
        : "Auto-generated from the title — your book doesn't have its own cover.";
    }
    // Re-roll always works in generated mode. "Use first page" appears only
    // when reachable. The "switch to generated" button is irrelevant here.
    setBtnVisible(coverRerollBtn, true);
    setBtnVisible(renderPdfBtn, canUseFirstPage);
    setBtnVisible(useGeneratedBtn, false);
    coverPreview.hidden = false;
  }

  // Switch back to generated when an extracted cover is currently shown.
  useGeneratedBtn?.addEventListener("click", () => {
    useGeneratedOverride = true;
    refreshCoverPreview();
  });
  coverRerollBtn?.addEventListener("click", () => {
    coverSalt = randomShortSalt();
    refreshCoverPreview();
  });

  // Client-side PDF first-page render. Used when server-side extraction
  // isn't available (mutool not installed) so the user still has a way to
  // pick the actual first page as the cover. Lazy-loads PDF.js so it
  // doesn't bloat the upload form for users who don't click the button.
  async function renderPdfFirstPageDataURL(file) {
    if (!window.__mocoPdfjs) {
      const mod = await import("https://cdn.jsdelivr.net/npm/pdfjs-dist@5.4.624/build/pdf.min.mjs");
      mod.GlobalWorkerOptions.workerSrc = "https://cdn.jsdelivr.net/npm/pdfjs-dist@5.4.624/build/pdf.worker.min.mjs";
      window.__mocoPdfjs = mod;
    }
    const buf = await file.arrayBuffer();
    const pdf = await window.__mocoPdfjs.getDocument({ data: buf }).promise;
    const page = await pdf.getPage(1);
    const viewport = page.getViewport({ scale: 1.5 });
    const canvas = document.createElement("canvas");
    canvas.width = viewport.width;
    canvas.height = viewport.height;
    await page.render({ canvasContext: canvas.getContext("2d"), viewport }).promise;
    return canvas.toDataURL("image/jpeg", 0.85);
  }

  renderPdfBtn?.addEventListener("click", async () => {
    // Two cases: (a) we already have an extracted cover and the user is
    // toggling back from generated — no render needed, just flip state.
    // (b) we don't yet have one and the file is a PDF — render via PDF.js.
    if (extractedCoverDataURL) {
      useGeneratedOverride = false;
      refreshCoverPreview();
      return;
    }
    const file = fileInput?.files?.[0];
    if (!file || !file.name.toLowerCase().endsWith(".pdf")) return;
    setButtonLoading(renderPdfBtn, true, "Rendering…");
    try {
      const dataURL = await renderPdfFirstPageDataURL(file);
      extractedCoverDataURL = dataURL;
      coverRenderedClientSide = true;
      useGeneratedOverride = false;
      refreshCoverPreview();
    } catch (err) {
      setMessage(message, "Could not render the first page client-side: " + (err.message || err), "error");
    } finally {
      setButtonLoading(renderPdfBtn, false);
    }
  });
  // When the user picks their own cover file, hide the preview and drop
  // the salt so the server doesn't persist a generated SVG instead.
  coverFileInput?.addEventListener("change", refreshCoverPreview);
  // Title edits update the preview so it reflects the actual stored title.
  titleInput?.addEventListener("input", refreshCoverPreview);
  authorInput?.addEventListener("input", refreshCoverPreview);

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
        ? "Reflows the PDF text into an EPUB you can resize, theme, and dark-mode."
        : "Wraps the markdown into an EPUB with proper chapters and the full reading-style settings.";
    }
    // Expanded guidance only for PDF — markdown has no real trade-offs.
    // Hidden behind a <details> disclosure so the form stays compact and
    // users only see the longer text when they actively look for it.
    if (convertDetails && convertDetailsBody) {
      if (isPDF) {
        convertDetailsBody.innerHTML =
          '<p><strong>Convert when:</strong> the PDF is mostly plain text (novels, articles, simple reports). You get reflowable text, dark theme, and adjustable font size.</p>'
        + '<p><strong>Keep as PDF when:</strong> the file has columns, diagrams, code blocks, math, or rich layout (textbooks, magazines, design docs). Conversion can reflow these badly — the original layout is preserved if you skip conversion.</p>';
        convertDetails.hidden = false;
      } else {
        convertDetails.hidden = true;
        convertDetails.open = false;
      }
    }
  }

  // Tracks the in-flight inspect request so a fast file swap can cancel the
  // previous one — without this, a slow first response can land after a
  // newer file is selected and clobber its title/author fields.
  let inspectAbort = null;

  // Cover preview state. When inspect returns an extracted cover, we show
  // that as the preview (it's what'll actually be used). When not, we fall
  // back to the salt-generated SVG. The "Use generated instead" toggle lets
  // the user override extraction in favour of the SVG.
  let extractedCoverDataURL = "";

  function clearFileSelection() {
    fileInput.value = "";
    if (dropEmpty) dropEmpty.hidden = false;
    if (dropFilled) dropFilled.hidden = true;
    if (metaFields) metaFields.hidden = true;
    if (titleInput) titleInput.value = "";
    if (authorInput) authorInput.value = "";
    if (descInput) descInput.value = "";
    if (detectedMessage) detectedMessage.textContent = "";
    if (submitBtn) submitBtn.disabled = true;
    if (inspectAbort) {
      inspectAbort.abort();
      inspectAbort = null;
    }
    if (coverPreview) coverPreview.hidden = true;
    if (coverPreviewImg?.dataset.objectUrl) {
      URL.revokeObjectURL(coverPreviewImg.dataset.objectUrl);
      coverPreviewImg.dataset.objectUrl = "";
    }
    if (coverSaltInput) coverSaltInput.value = "";
    if (coverFileInput) coverFileInput.value = "";
    if (coverForceInput) coverForceInput.value = "";
    extractedCoverDataURL = "";
    coverRenderedClientSide = false;
    useGeneratedOverride = false;
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
    if (inspectAbort) inspectAbort.abort();
    const ctrl = new AbortController();
    inspectAbort = ctrl;
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
        signal: ctrl.signal,
      });
      if (!res.ok) throw new Error("inspect failed");
      const data = await res.json();
      if (ctrl.signal.aborted) return;
      if (titleInput && !titleInput.value) titleInput.value = data.title || "";
      if (authorInput && !authorInput.value) authorInput.value = data.author || "";
      if (descInput && !descInput.value) descInput.value = data.description || "";
      if (detectedMessage) {
        const detected = [];
        if (data.title) detected.push("title");
        if (data.author) detected.push("author");
        detectedMessage.textContent = detected.length
          ? `Detected ${detected.join(" and ")} from the file — edit if needed.`
          : "We couldn't detect a title — please add one.";
      }
      // Stash the extracted cover (if any) on the form module's state so the
      // preview can show what'll actually be used — we previously always
      // showed the generated SVG even when extraction would win.
      extractedCoverDataURL = data.extractedCover || "";
      // Reset all per-file state — a new file shouldn't inherit choices
      // made for the previous one.
      coverRenderedClientSide = false;
      useGeneratedOverride = false;
      refreshCoverPreview();
    } catch (err) {
      if (err && err.name === "AbortError") return;
      if (detectedMessage) {
        detectedMessage.textContent = "Could not auto-detect details — please fill them in.";
      }
      if (titleInput && !titleInput.value) {
        // Fallback: derive from filename
        titleInput.value = file.name.replace(/\.[^.]+$/, "").replace(/[_-]+/g, " ");
      }
      refreshCoverPreview();
    } finally {
      if (inspectAbort === ctrl) inspectAbort = null;
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
    // Click anywhere on the dropzone opens the file picker. Bound on the
    // parent (not dropzone-empty, which is pointer-events:none) and not the
    // absolutely-positioned input overlay because some browsers — notably
    // Safari/iOS — treat the native file input's clickable area as the
    // small button widget rather than the full CSS box, so clicks near the
    // edges of the overlay would silently miss.
    dropzone.addEventListener("click", (e) => {
      if (e.target.closest("[data-dropzone-remove]")) return; // Remove handles its own click
      if (e.target === fileInput) return; // native input click already opens picker
      if (dropFilled && !dropFilled.hidden) return; // already have a file, ignore stray clicks
      e.preventDefault();
      fileInput.click();
    });
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

  uploadForm.addEventListener("submit", async (event) => {
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

    // If the user picked "Use book's first page" via PDF.js client-side
    // render AND hasn't switched back to generated, turn the data URL into
    // a Blob and slot it into the form's `cover` field. Server treats it
    // the same as a manually uploaded image. Skipped if the user uploaded
    // their own cover (that wins outright) or if they explicitly chose to
    // use a generated cover (coverForce=1 will tell the server to skip).
    const formData = new FormData(uploadForm);
    const wantClientRendered =
      coverRenderedClientSide &&
      extractedCoverDataURL &&
      !useGeneratedOverride &&
      !coverFileInput?.files?.[0];
    if (wantClientRendered) {
      try {
        const blob = await fetch(extractedCoverDataURL).then((r) => r.blob());
        formData.set("cover", blob, "cover.jpg");
      } catch (err) {
        console.warn("Failed to attach client-rendered cover:", err);
      }
    }

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

    xhr.send(formData);
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

// ---------- PDF continuous-scroll viewer ----------
// Spawned from the PDF reader's settings panel. Renders all pages
// stacked vertically with lazy rendering (IntersectionObserver), pinch
// + button zoom, and a minimal toolbar. Page-note button still works —
// uses whichever page is most-visible in the viewport.
//
// Memory: only pages within ~1 viewport of the visible area are rendered;
// far-away pages have their canvas cleared. A 700-page PDF stays under
// ~50MB of canvas memory regardless of length.
async function openPdfContinuousScroll(pdf, startPage, bookID, isGuest, triggerPageNote, onClose) {
  if (document.querySelector(".pdf-fullscreen")) return;

  const overlay = document.createElement("div");
  overlay.className = "pdf-fullscreen";
  overlay.innerHTML = `
    <header class="pdf-fs-toolbar">
      <button class="pdf-fs-btn" data-pdf-fs-close type="button" aria-label="Exit fullscreen" title="Exit fullscreen (Esc)">×</button>
      <span class="pdf-fs-pageind"><input type="number" data-pdf-fs-page-input min="1" value="${startPage}" /> <span data-pdf-fs-page-total>/ ${pdf.numPages}</span></span>
      <span class="pdf-fs-spacer"></span>
      <button class="pdf-fs-btn" data-pdf-fs-zoom-out type="button" aria-label="Zoom out" title="Zoom out">−</button>
      <span class="pdf-fs-zoom" data-pdf-fs-zoom>100%</span>
      <button class="pdf-fs-btn" data-pdf-fs-zoom-in type="button" aria-label="Zoom in" title="Zoom in">+</button>
    </header>
    <div class="pdf-fs-scroll" data-pdf-fs-scroll>
      <div class="pdf-fs-pages" data-pdf-fs-pages></div>
    </div>
  `;
  document.body.appendChild(overlay);
  document.body.classList.add("pdf-fs-active");

  const dpr = window.devicePixelRatio || 1;
  const pagesEl = overlay.querySelector("[data-pdf-fs-pages]");
  const scrollEl = overlay.querySelector("[data-pdf-fs-scroll]");
  const pageInput = overlay.querySelector("[data-pdf-fs-page-input]");
  const zoomDisp = overlay.querySelector("[data-pdf-fs-zoom]");

  // Compute fit-width base scale from page 1's natural width. Most PDFs
  // have uniform page sizes; mixed-size PDFs still look fine because
  // each placeholder gets its own per-page aspect (set on first render).
  const firstPage = await pdf.getPage(1);
  const firstVp = firstPage.getViewport({ scale: 1 });
  let baseScale = (pagesEl.clientWidth - 32) / firstVp.width;
  if (baseScale <= 0) baseScale = 1;
  let zoom = 1.0;
  let mostVisiblePage = startPage;

  // Build placeholders sized to the first page's aspect ratio. Each will
  // be swapped for a real <canvas> when it scrolls into view.
  const placeholders = [];
  for (let i = 1; i <= pdf.numPages; i++) {
    const ph = document.createElement("div");
    ph.className = "pdf-fs-page";
    ph.dataset.pageNum = String(i);
    placeholders.push(ph);
    pagesEl.appendChild(ph);
  }
  function applyPlaceholderSizes() {
    const s = baseScale * zoom;
    placeholders.forEach((ph) => {
      ph.style.width = `${Math.round(firstVp.width * s)}px`;
      ph.style.height = `${Math.round(firstVp.height * s)}px`;
    });
  }
  applyPlaceholderSizes();

  // Track which pages are currently rendered so we can clear far-away
  // canvases on scroll/zoom.
  const renderedNums = new Set();
  async function renderPage(num) {
    if (renderedNums.has(num)) return;
    const ph = placeholders[num - 1];
    if (!ph) return;
    renderedNums.add(num);
    try {
      const page = await pdf.getPage(num);
      const s = baseScale * zoom;
      const vp = page.getViewport({ scale: s * dpr });
      const canvas = document.createElement("canvas");
      canvas.width = vp.width;
      canvas.height = vp.height;
      canvas.style.width = `${vp.width / dpr}px`;
      canvas.style.height = `${vp.height / dpr}px`;
      const ctx = canvas.getContext("2d");
      await page.render({ canvasContext: ctx, viewport: vp }).promise;
      if (ph.isConnected) {
        ph.innerHTML = "";
        ph.appendChild(canvas);
      }
    } catch (_) {
      renderedNums.delete(num);
    }
  }
  function clearPage(num) {
    const ph = placeholders[num - 1];
    if (ph) ph.innerHTML = "";
    renderedNums.delete(num);
  }
  function evictFarPages() {
    // Keep current page ± 3 rendered, dispose the rest. Plenty for
    // smooth scrolling without eating gigabytes on long PDFs.
    const keep = new Set();
    for (let d = -3; d <= 3; d++) keep.add(mostVisiblePage + d);
    [...renderedNums].forEach((n) => { if (!keep.has(n)) clearPage(n); });
  }

  // IntersectionObserver: render visible, mark most-visible for the
  // page indicator + note button.
  const observer = new IntersectionObserver((entries) => {
    let best = mostVisiblePage;
    let bestArea = 0;
    entries.forEach((entry) => {
      const num = parseInt(entry.target.dataset.pageNum, 10);
      if (entry.isIntersecting) {
        renderPage(num);
        if (entry.intersectionRect.height > bestArea) {
          bestArea = entry.intersectionRect.height;
          best = num;
        }
      }
    });
    if (bestArea > 0 && best !== mostVisiblePage) {
      mostVisiblePage = best;
      if (document.activeElement !== pageInput) {
        pageInput.value = String(best);
      }
      evictFarPages();
    }
  }, { root: scrollEl, rootMargin: "100% 0px" });
  placeholders.forEach((ph) => observer.observe(ph));

  // Re-render on zoom: clear all canvases, resize placeholders, let the
  // observer re-fire to render whatever is now visible.
  function setZoom(z) {
    z = Math.min(Math.max(z, 0.25), 5);
    if (Math.abs(z - zoom) < 0.001) return;
    zoom = z;
    zoomDisp.textContent = `${Math.round(zoom * 100)}%`;
    [...renderedNums].forEach(clearPage);
    applyPlaceholderSizes();
    // Nudge the observer — disconnect/reconnect re-fires entries for
    // currently visible placeholders so they get rendered at new scale.
    observer.disconnect();
    placeholders.forEach((ph) => observer.observe(ph));
  }
  overlay.querySelector("[data-pdf-fs-zoom-in]").addEventListener("click", () => setZoom(zoom * 1.2));
  overlay.querySelector("[data-pdf-fs-zoom-out]").addEventListener("click", () => setZoom(zoom * 0.85));
  // Ctrl/Cmd + scroll = zoom (matches PDF viewer convention).
  scrollEl.addEventListener("wheel", (e) => {
    if (!e.ctrlKey && !e.metaKey) return;
    e.preventDefault();
    setZoom(zoom * (e.deltaY < 0 ? 1.1 : 0.9));
  }, { passive: false });

  // Page input — jump to page on Enter / change.
  function jumpTo(n) {
    n = Math.max(1, Math.min(pdf.numPages, n | 0));
    placeholders[n - 1]?.scrollIntoView({ block: "start", behavior: "smooth" });
  }
  pageInput.addEventListener("change", () => jumpTo(Number(pageInput.value)));
  pageInput.addEventListener("keydown", (e) => {
    if (e.key === "Enter") { e.preventDefault(); jumpTo(Number(pageInput.value)); pageInput.blur(); }
  });

  // Floating note button — same prompt as the paginated FAB, but uses
  // whichever page the user is currently scrolled to.
  if (!isGuest && typeof triggerPageNote === "function") {
    const noteBtn = document.createElement("button");
    noteBtn.type = "button";
    noteBtn.className = "pdf-fs-note-fab";
    noteBtn.setAttribute("aria-label", "Add a note for this page");
    noteBtn.title = "Add a note for this page";
    noteBtn.innerHTML = '<svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 20h9"/><path d="M16.5 3.5a2.121 2.121 0 0 1 3 3L7 19l-4 1 1-4z"/></svg>';
    noteBtn.addEventListener("click", () => triggerPageNote(mostVisiblePage));
    overlay.appendChild(noteBtn);
  }

  function close() {
    observer.disconnect();
    overlay.remove();
    document.body.classList.remove("pdf-fs-active");
    document.removeEventListener("keydown", onKey);
    if (typeof onClose === "function") onClose(mostVisiblePage);
  }
  function onKey(e) {
    if (e.key === "Escape") close();
  }
  document.addEventListener("keydown", onKey);
  overlay.querySelector("[data-pdf-fs-close]").addEventListener("click", close);

  // Scroll to the user's current paginated-reader page.
  requestAnimationFrame(() => placeholders[startPage - 1]?.scrollIntoView({ block: "start" }));
}

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

  // Quietly let guests know that progress / highlights / bookmarks need an
  // account — replaces the always-on banner that used to take a chunk of
  // vertical space at the top of the reader. Auto-dismisses; the per-action
  // guestBlock() toasts above still fire if they try to use those features.
  // Shown on every guest visit (guests have no persistent identity, so we
  // can't reliably track "already seen this"). The tap-anywhere first-run
  // hint below is suppressed for guests to keep the toast stack tidy — the
  // guest hint already implies the controls are reachable.
  if (isGuest) {
    setTimeout(() => {
      toast("Reading as guest — sign in to save progress, highlights, and bookmarks.", "info");
    }, 600);
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
  // Body-level class so CSS can hide floating page controls (the bottom
  // page-jumper pill, side-arrow buttons) while a side panel or modal is
  // open — they sit at higher z-index than the panel and would otherwise
  // overlap its content.
  function syncPanelOpenClass() {
    document.body.classList.toggle("reader-panel-open", panelOrModalOpen());
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
  // Defensive close-button navigation. The bare <a href> alone wasn't
  // firing reliably on iOS Safari for this fixed-position anchor (no
  // single smoking-gun handler — most likely a synthesized-click race
  // when the user taps right after immersive-mode appears the button).
  // We bind both `pointerup` (fires directly from touch, bypasses the
  // synthesized-click pipeline) and `click` (covers desktop / browsers
  // that dispatch click cleanly). location.assign() is the canonical
  // navigation API; setting .href is equivalent but assign() is harder
  // to confuse with property accessors.
  let closeNavigating = false;
  function navigateClose() {
    if (closeNavigating) return;
    closeNavigating = true;
    const href = floatingClose.getAttribute("href") || "/app";
    window.location.assign(href);
  }
  floatingClose?.addEventListener("pointerup", (event) => {
    event.preventDefault();
    event.stopPropagation();
    navigateClose();
  });
  floatingClose?.addEventListener("click", (event) => {
    event.preventDefault();
    event.stopPropagation();
    navigateClose();
  });
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
    syncPanelOpenClass();
    if (panelOrModalOpen()) showChrome({ keepShown: true });
    else scheduleHide();
  });
  document.querySelectorAll("[data-reader-panel]").forEach((panel) => {
    panelObserver.observe(panel, { attributes: true, attributeFilter: ["class"] });
  });
  syncPanelOpenClass();

  // Show chrome briefly on load so users see the controls, then auto-hide.
  // First-time logged-in users get a one-shot toast hint. Guests are skipped
  // to avoid double-stacking with the guest welcome toast above — both are
  // shown on first visit and would crowd the toast stack on phones.
  scheduleHide(3000);
  if (!isGuest && !localStorage.getItem("moco-reader-hint-seen")) {
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
  let applyPdfSpread = () => {};
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

    // Push changes into the EPUB iframe / PDF canvas (each is a no-op for
    // the readers that aren't currently active).
    applyEpubTheme();
    applyEpubSpread();
    applyPdfSpread();
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
    registerPdfSpreadHook:  (fn) => { applyPdfSpread  = fn; },
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
    // Close the Contents panel after the user picks a chapter — without this
    // the panel stays open and covers the page they just navigated to,
    // especially noticeable on phones where the panel is full-width.
    tocLinks.forEach((link) => {
      link.addEventListener("click", () => {
        // Defer so the browser handles the hash navigation first; closing
        // immediately can race with the scroll start in some browsers.
        setTimeout(closeAllPanels, 0);
      });
    });

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
      // Fade out, then remove. Fading first hides the cream-tinted backdrop
      // before the iframe paints, so there's no visual jank as it
      // transitions from "loading…" to first page rendered.
      const removeLoading = () => {
        if (!loadingEl) return;
        loadingEl.classList.add("is-fading");
        setTimeout(() => loadingEl.remove(), 280);
      };

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

      // The loading overlay is `position: absolute` over the stage so
      // epub.js can still append its iframe without colliding with it.
      // We leave it up until the first `rendered` event fires — that's
      // when the iframe has actually painted a page; before then the
      // user would see only the cream stage background.

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
            "min-height": "100%",
            "-webkit-font-smoothing": "antialiased",
          },
          // Reserve ~110px at the bottom so content can't run under the
          // floating reader-page-btn pill (~50px tall + 14px margin + safe
          // area). Without this, the last paragraph of a page sits behind
          // the prev/next buttons on phones.
          "body": { padding: "8px 18px 110px" },
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
      rendition.on("rendered", () => {
        clearTimeout(safety);
        removeLoading();
      });

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
            button.addEventListener("click", () => {
              rendition.display(item.href);
              closeAllPanels();
            });
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
      // Locations are still generated so the chapter-jump dropdown and
      // jump-to-page-number can resolve to a CFI. But the on-screen
      // counter shows a percentage — for PDF-converted EPUBs the location
      // index jumps by 100s when crossing chapter boundaries (the CFI
      // reported by `relocated` rounds to a chapter-start checkpoint),
      // which made next/prev look broken. Percentage advances smoothly.
      let epubLocationsReady = false;
      book.locations.generate(1024).then(() => {
        epubLocationsReady = true;
        if (epubPageInput) epubPageInput.disabled = false;
      }).catch(() => {});
      if (epubPageTotal) epubPageTotal.textContent = "%";
      if (epubPageInput) {
        epubPageInput.min = 0;
        epubPageInput.max = 100;
      }

      rendition.on("relocated", (location) => {
        const locator = location?.start?.cfi || "";
        const progressPercent = location?.start?.percentage ? location.start.percentage * 100 : 0;
        saveProgress(locator, progressPercent);
        if (prev) prev.disabled = !!location?.atStart;
        if (next) next.disabled = !!location?.atEnd;
        const pct = Math.round(progressPercent);
        if (epubPageInput && document.activeElement !== epubPageInput) {
          epubPageInput.value = String(pct);
        }
        setReaderPosition(locator, `${pct}%`);
      });
      jumpToLocator = (locator) => {
        if (!locator) return;
        try { rendition.display(locator); } catch (_) {}
      };

      function jumpToEpubPage() {
        if (!epubLocationsReady) return;
        const total = book.locations.length();
        let pct = parseInt(epubPageInput.value, 10);
        if (isNaN(pct) || pct < 0) pct = 0;
        if (pct > 100) pct = 100;
        epubPageInput.value = String(pct);
        const targetLoc = Math.max(0, Math.min(total - 1, Math.floor((pct / 100) * total)));
        const cfi = book.locations.cfiFromLocation(targetLoc);
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

        // Disable iOS / mobile in-app browser double-tap-to-zoom inside the
        // EPUB iframe. epub.js renders chapter HTML in a separate document,
        // so touch-action on the parent body doesn't reach here.
        const styleNode = doc.createElement("style");
        styleNode.textContent = `html,body{touch-action:manipulation;-webkit-touch-callout:none;}`;
        doc.head?.appendChild(styleNode);

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

      // Spread mode: viewport is wide enough AND the user hasn't forced
      // single-page in settings. Mirrors the EPUB spread rules so settings
      // are consistent across formats. minSpreadWidth=900 matches EPUB.
      const SPREAD_MIN_WIDTH = 900;
      const isSpreadMode = () => {
        const setting = window.__mocoReaderSettings?.settings?.spread || "auto";
        if (setting === "single") return false;
        return stage.clientWidth >= SPREAD_MIN_WIDTH && pdf.numPages >= 2;
      };

      const renderPage = async (num) => {
        if (rendering) return;
        rendering = true;
        try {
          const dpr = window.devicePixelRatio || 1;
          const stageW = Math.max(stage.clientWidth - 24, 200);
          const stageH = Math.max(stage.clientHeight - 40, 200);
          // Portrait stages (mobile, narrow windows) read as too "tall"
          // for a portrait PDF page — fit-page leaves the canvas tiny
          // with vast empty space. We fit by width instead so the page
          // is actually readable; the stage scrolls if the page ends up
          // taller. Landscape stages (typical desktop) keep fit-page so
          // the whole page is visible without scrolling.
          const stageIsPortrait = stageH > stageW;

          if (isSpreadMode()) {
            // Match the physical-book convention: cover alone on page 1,
            // then facing pairs from page 2 onward — (1), (2,3), (4,5), …
            // This gives left/right pages the same parity as the printed
            // book so spreads "feel right". For PDFs without a cover page
            // it still works — page 1 just sits on its own briefly before
            // the user clicks Next.
            let leftNum, rightNum;
            if (num <= 1) {
              leftNum = 1;
              rightNum = 0;
            } else {
              leftNum = num % 2 === 0 ? num : num - 1;
              rightNum = leftNum + 1 <= pdf.numPages ? leftNum + 1 : 0;
            }
            const leftPage = await pdf.getPage(leftNum);
            const rightPage = rightNum > 0 ? await pdf.getPage(rightNum) : null;

            const lBase = leftPage.getViewport({ scale: 1 });
            const rBase = rightPage ? rightPage.getViewport({ scale: 1 }) : null;
            const totalW = lBase.width + (rBase ? rBase.width : 0);
            const maxH = Math.max(lBase.height, rBase ? rBase.height : 0);
            // Spread mode: fit the whole spread to the stage so it
            // never causes scrolling. Brown side-margins are an
            // unavoidable consequence of the stage aspect ratio not
            // matching the spread aspect ratio — we accept that over
            // making the user scroll on desktop.
            const fitScale = Math.min(stageW / totalW, stageH / maxH);

            const lvp = leftPage.getViewport({ scale: fitScale * dpr });
            const rvp = rightPage ? rightPage.getViewport({ scale: fitScale * dpr }) : null;

            canvas.width = lvp.width + (rvp ? rvp.width : 0);
            canvas.height = Math.max(lvp.height, rvp ? rvp.height : 0);
            canvas.style.width = `${canvas.width / dpr}px`;
            canvas.style.height = `${canvas.height / dpr}px`;

            // Clear so a wider previous render doesn't leak through into
            // the now-narrower (or solo last-page) view.
            ctx.clearRect(0, 0, canvas.width, canvas.height);
            await leftPage.render({ canvasContext: ctx, viewport: lvp }).promise;
            if (rightPage && rvp) {
              ctx.save();
              ctx.translate(lvp.width, 0);
              await rightPage.render({ canvasContext: ctx, viewport: rvp }).promise;
              ctx.restore();
            }

            pageNum = leftNum;
            if (pageInput) {
              pageInput.value = String(leftNum);
              pageInput.max = String(pdf.numPages);
            }
            if (pageTotal) {
              pageTotal.textContent = rightNum > 0
                ? `–${rightNum} / ${pdf.numPages}`
                : `/ ${pdf.numPages}`;
            }
            if (prev) prev.disabled = leftNum <= 1;
            // "Next" is disabled when we're already showing the last
            // possible page — either the right slot is the last page, or
            // (when total is even and on the last spread) the left slot is.
            if (next) next.disabled = (rightNum === pdf.numPages) || (rightNum === 0 && leftNum === pdf.numPages);
            setReaderPosition(`page:${leftNum}`, `Page ${leftNum}`);
            await saveProgress(`page:${leftNum}`, (leftNum / pdf.numPages) * 100);
            return;
          }

          // Single-page mode — fit one page to the stage.
          const page = await pdf.getPage(num);
          const base = page.getViewport({ scale: 1 });
          // Mobile (portrait stage): fit to width so the page is large
          // enough to actually read, allow vertical scroll. Desktop
          // (landscape stage): fit page so the whole thing is visible
          // without scrolling, since portrait PDFs already fit landscape
          // monitors comfortably.
          const fitScale = stageIsPortrait
            ? stageW / base.width
            : Math.min(stageW / base.width, stageH / base.height);

          const viewport = page.getViewport({ scale: fitScale * dpr });
          canvas.width = viewport.width;
          canvas.height = viewport.height;
          canvas.style.width = `${viewport.width / dpr}px`;
          canvas.style.height = `${viewport.height / dpr}px`;
          ctx.clearRect(0, 0, canvas.width, canvas.height);
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

      // Re-render when the user toggles Auto/Single in reader settings.
      window.__mocoReaderSettings?.registerPdfSpreadHook(() => {
        renderPage(pageNum);
      });
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

      // First page has rasterized — fade out the loading overlay so the
      // user sees their actual content instead of a blank stage. Mirrors
      // the EPUB flow above.
      const pdfLoadingEl = stage.querySelector("[data-reader-loading]");
      if (pdfLoadingEl) {
        pdfLoadingEl.classList.add("is-fading");
        setTimeout(() => pdfLoadingEl.remove(), 280);
      }

      const goPrev = () => {
        if (pageNum <= 1) return;
        if (isSpreadMode()) {
          // Spread navigation respects the cover-alone convention:
          //   from (4,5) prev = (2,3); from (2,3) prev = (1); from (1) stop.
          if (pageNum <= 3) {
            pageNum = 1;
          } else {
            pageNum = pageNum - 2;
          }
        } else {
          pageNum = Math.max(1, pageNum - 1);
        }
        renderPage(pageNum);
      };
      const goNext = () => {
        if (pageNum >= pdf.numPages) return;
        if (isSpreadMode()) {
          // From page 1 (alone), Next goes to (2,3). From (2,3) → (4,5), …
          if (pageNum === 1) {
            pageNum = Math.min(2, pdf.numPages);
          } else {
            pageNum = Math.min(pdf.numPages, pageNum + 2);
          }
        } else {
          pageNum = Math.min(pdf.numPages, pageNum + 1);
        }
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
          // Resolve a PDF outline destination into a 1-based page number.
          // dest can be a string (named destination — needs lookup) or an
          // array whose first element is a page reference. Returns 0 on
          // failure so the click is silently ignored.
          const destToPage = async (dest) => {
            try {
              let arr = dest;
              if (typeof arr === "string") arr = await pdf.getDestination(arr);
              if (!arr || !arr.length) return 0;
              const pageIndex = await pdf.getPageIndex(arr[0]);
              return (pageIndex || 0) + 1;
            } catch (_) { return 0; }
          };
          const renderItems = (items, depth = 0) => {
            items.forEach((item) => {
              const li = document.createElement("li");
              const button = document.createElement("button");
              button.type = "button";
              button.textContent = item.title || "Untitled";
              if (depth > 0) button.style.paddingLeft = `${14 + depth * 14}px`;
              button.addEventListener("click", async () => {
                const target = await destToPage(item.dest);
                if (target > 0) {
                  pageNum = Math.min(target, pdf.numPages);
                  renderPage(pageNum);
                }
                closeAllPanels();
              });
              li.appendChild(button);
              tocList.appendChild(li);
              if (item.items?.length) renderItems(item.items, depth + 1);
            });
          };
          renderItems(outline);
        }
      } catch (_) {
        tocList.innerHTML = "";
        const li = document.createElement("li");
        li.textContent = "No outline available.";
        tocList.appendChild(li);
      }

      // Settings panel "Enter immersive view" — for PDF only. Closes the
      // panel, then triggers immersive (chrome hides). Tap the page to
      // bring controls back; the FAB stays visible regardless.
      const immersiveBtn = document.querySelector("[data-pdf-immersive-toggle]");
      if (immersiveBtn) {
        immersiveBtn.addEventListener("click", () => {
          closeAllPanels();
          // Defer one frame so the panel close animation runs first.
          requestAnimationFrame(() => {
            readerApp.classList.add("is-immersive");
            syncFloatingClose();
          });
        });
      }

      // Settings panel "Open continuous scroll" — opens a separate
      // fullscreen overlay with all pages stacked + zoom + lazy render.
      // Doesn't replace the paginated reader; closing the overlay
      // returns to it. Doesn't run for non-PDF readers.
      const fullscreenBtn = document.querySelector("[data-pdf-fullscreen-toggle]");
      if (fullscreenBtn) {
        fullscreenBtn.addEventListener("click", async () => {
          closeAllPanels();
          await openPdfContinuousScroll(pdf, pageNum, bookID, isGuest, triggerPdfPageNote, (n) => {
            // Sync paginated reader's pageNum to wherever they were
            // last viewing in continuous mode.
            pageNum = Math.max(1, Math.min(pdf.numPages, n));
            renderPage(pageNum);
          });
        });
      }

      // Report the PDF's total page count back to the server. The server
      // uses it to compute a real reading-time estimate (pages × ~1.2 min)
      // and to display "X pages" on the book detail page. Best-effort —
      // failures are silent (the dashboard estimate from file size is the
      // fallback). isGuest skips since guests can't write.
      if (!isGuest && pdf?.numPages) {
        requestJSON(`/api/v1/books/${bookID}/total-pages`, {
          method: "PUT",
          body: JSON.stringify({ totalPages: pdf.numPages }),
        }).catch(() => { /* ignore — purely optimistic */ });
      }

      // Accepts an optional page number override so the fullscreen
      // continuous-scroll viewer can attach the note to whichever page
      // the user is currently scrolled to (the FAB in paginated mode
      // omits the arg and uses the closure's pageNum).
      const triggerPdfPageNote = async (forPage) => {
        const target = (forPage && forPage > 0) ? forPage : pageNum;
        const note = await openTextPrompt({
          title: `Add a note for page ${target}`,
          body: "Text selection inside PDFs isn't supported in this build. Type a short note instead — it will be saved against this page.",
          placeholder: "What stood out on this page?",
          confirmLabel: "Save note",
        });
        if (!note) return;
        await saveHighlight(`page:${target}`, note);
      };

      // Floating action button — primary entry point for "add a note for
      // this page." Always-visible, fixed bottom-right of the reader so
      // users don't have to dig through the Highlights sidebar to find it.
      // Skipped for guests (they can't save anything).
      // Created with [hidden] so it doesn't render until the first PDF
      // page has actually painted — otherwise the user sees a stray
      // "Add note" pill floating over the loading state for a beat.
      if (!isGuest) {
        const stageContainer = document.querySelector(".ebook-stage-full");
        if (stageContainer && !stageContainer.querySelector(".reader-fab")) {
          const fab = document.createElement("button");
          fab.type = "button";
          fab.className = "reader-fab";
          fab.hidden = true;
          fab.setAttribute("aria-label", "Add a note for this page");
          fab.setAttribute("title", "Add a note for this page");
          fab.innerHTML = '<svg class="reader-fab-icon" width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 20h9"/><path d="M16.5 3.5a2.121 2.121 0 0 1 3 3L7 19l-4 1 1-4z"/></svg><span class="reader-fab-label">Add note</span>';
          fab.addEventListener("click", triggerPdfPageNote);
          stageContainer.appendChild(fab);
          // Reveal on the next frame after a successful initial render —
          // we're already past `await renderPage(pageNum)` here.
          requestAnimationFrame(() => { fab.hidden = false; });
        }
      }

      // Keep the in-panel button too for users who go via the Highlights
      // sidebar — but move it to the TOP of the panel so it's the first
      // thing visible, not buried under the description text.
      const notesCopy = document.querySelector('[data-reader-panel="notes"] .notes-copy');
      if (notesCopy && !notesCopy.querySelector("[data-add-pdf-note]")) {
        const addPdfNoteBtn = document.createElement("button");
        addPdfNoteBtn.type = "button";
        addPdfNoteBtn.className = "button primary";
        addPdfNoteBtn.setAttribute("data-add-pdf-note", "");
        addPdfNoteBtn.textContent = "Add page note";
        addPdfNoteBtn.addEventListener("click", triggerPdfPageNote);
        notesCopy.insertBefore(addPdfNoteBtn, notesCopy.firstChild);
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
      renderBookmarks(data.items || []);
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
    const anonymousOwner = data.get("anonymousOwner") === "1";
    setButtonLoading(submit, true, "Saving…");
    try {
      await requestJSON("/api/v1/auth/me", {
        method: "PUT",
        body: JSON.stringify({ displayName: name, anonymousOwner }),
      });
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

// ---------- Edit book details (owner-only modal on book detail page) ----------
// Wires the inline modal rendered by templates/book.html when .IsOwner is true.
// Three flows the modal handles:
//   1. PATCH title/author      → /api/v1/books/{id}
//   2. PUT cover (file upload) → /api/v1/books/{id}/cover
//   3. POST regenerate cover   → /api/v1/books/{id}/cover/regenerate
// Cover preview is busted with a timestamp query so the new image shows
// immediately without waiting for a hard reload.
// Document-delegated so it works whether the modal is server-rendered into
// the page (visiting /books/{id} directly) or SPA-injected later when the
// user opens a book modal from /app or /discover. Looking up elements at
// event time means there's no stale closure reference and no need to re-run
// init after fragment injection.
(function () {
  function setMessage(modal, text, kind) {
    const el = modal?.querySelector("[data-edit-book-message]");
    if (!el) return;
    el.textContent = text || "";
    el.classList.toggle("is-error", kind === "error");
    el.classList.toggle("is-success", kind === "success");
  }
  function openModal(modal) {
    modal.classList.add("is-open");
    setMessage(modal, "");
    const titleInput = modal.querySelector('input[name="title"]');
    requestAnimationFrame(() => titleInput?.focus());
  }
  function closeEditModal(modal) {
    modal.classList.remove("is-open");
    setMessage(modal, "");
  }
  function currentBookID() {
    // Prefer the URL — works for /books/{id} direct loads AND SPA-pushed
    // book modal URLs. Fall back to data-book-id on the visible card.
    const m = window.location.pathname.match(/\/books\/([A-Za-z0-9_-]+)/);
    if (m) return m[1];
    return document.querySelector("[data-book-detail]")?.closest("[data-book-id]")?.dataset?.bookId || null;
  }
  function bustCoverCache(bookID, modal) {
    const url = `/api/v1/books/${bookID}/cover?t=${Date.now()}`;
    const preview = modal.querySelector("[data-edit-book-cover-preview]");
    if (preview) preview.src = url;
    document.querySelectorAll(`.book-detail-cover img, [data-book-card-cover][data-book-id="${bookID}"] img`).forEach((img) => {
      img.src = url;
    });
  }

  document.addEventListener("click", (event) => {
    if (event.target.closest("[data-edit-book-open]")) {
      event.preventDefault();
      const modal = document.querySelector("[data-edit-book-modal]");
      if (modal) openModal(modal);
      return;
    }
    const closeBtn = event.target.closest("[data-edit-book-close]");
    if (closeBtn) {
      event.preventDefault();
      const modal = closeBtn.closest("[data-edit-book-modal]");
      if (modal) closeEditModal(modal);
      return;
    }
    // Backdrop click (the click landed on the .modal itself, not a child).
    if (event.target.matches?.("[data-edit-book-modal].is-open")) {
      closeEditModal(event.target);
      return;
    }
    const pickBtn = event.target.closest("[data-edit-book-cover-pick]");
    if (pickBtn) {
      event.preventDefault();
      pickBtn.closest("[data-edit-book-modal]")?.querySelector("[data-edit-book-cover-input]")?.click();
      return;
    }
    const regenBtn = event.target.closest("[data-edit-book-cover-regenerate]");
    if (regenBtn) {
      event.preventDefault();
      const modal = regenBtn.closest("[data-edit-book-modal]");
      const bookID = currentBookID();
      if (!modal || !bookID) return;
      setButtonLoading(regenBtn, true, "Re-rolling…");
      requestJSON(`/api/v1/books/${bookID}/cover/regenerate`, { method: "POST", body: "{}" })
        .then(() => { bustCoverCache(bookID, modal); setMessage(modal, "Generated a fresh cover.", "success"); })
        .catch((err) => setMessage(modal, err.message || "Failed to regenerate cover.", "error"))
        .finally(() => setButtonLoading(regenBtn, false));
      return;
    }
  });

  document.addEventListener("keydown", (event) => {
    if (event.key !== "Escape") return;
    const modal = document.querySelector("[data-edit-book-modal].is-open");
    if (modal) closeEditModal(modal);
  });

  document.addEventListener("change", async (event) => {
    const input = event.target.closest("[data-edit-book-cover-input]");
    if (!input) return;
    const modal = input.closest("[data-edit-book-modal]");
    const bookID = currentBookID();
    if (!modal || !bookID) return;
    const file = input.files?.[0];
    if (!file) return;
    if (file.size > 10 * 1024 * 1024) {
      setMessage(modal, "Image is too large (max 10MB).", "error");
      input.value = "";
      return;
    }
    const pickBtn = modal.querySelector("[data-edit-book-cover-pick]");
    setButtonLoading(pickBtn, true, "Uploading…");
    try {
      const fd = new FormData();
      fd.append("cover", file);
      await requestJSON(`/api/v1/books/${bookID}/cover`, { method: "PUT", body: fd });
      bustCoverCache(bookID, modal);
      setMessage(modal, "Cover updated.", "success");
    } catch (err) {
      setMessage(modal, err.message || "Failed to upload cover.", "error");
    } finally {
      setButtonLoading(pickBtn, false);
      input.value = "";
    }
  });

  document.addEventListener("submit", async (event) => {
    const form = event.target.closest("[data-edit-book-form]");
    if (!form) return;
    event.preventDefault();
    const modal = form.closest("[data-edit-book-modal]");
    const bookID = currentBookID();
    if (!modal || !bookID) return;
    const titleInput = form.querySelector('input[name="title"]');
    const authorInput = form.querySelector('input[name="author"]');
    const descriptionInput = form.querySelector('textarea[name="description"]');
    const submitBtn = form.querySelector('button[type="submit"]');
    const title = titleInput.value.trim();
    if (!title) { setMessage(modal, "Title is required.", "error"); return; }
    const description = descriptionInput?.value.trim() || "";
    setButtonLoading(submitBtn, true, "Saving…");
    try {
      const data = await requestJSON(`/api/v1/books/${bookID}`, {
        method: "PATCH",
        body: JSON.stringify({ title, author: authorInput.value.trim(), description }),
      });
      // Reflect the change on the visible card without a full reload.
      const h1 = document.querySelector(".book-detail h1");
      if (h1) h1.textContent = data.title || title;
      const authorEl = document.querySelector(".book-detail-author");
      if (authorEl) {
        const author = data.author || "";
        authorEl.textContent = author;
        authorEl.hidden = !author;
      }
      const meta = document.querySelector(".book-detail-meta");
      let descEl = document.querySelector(".book-detail-description");
      const newDesc = data.description || "";
      if (newDesc) {
        if (!descEl && meta) {
          descEl = document.createElement("p");
          descEl.className = "book-detail-description";
          const actions = meta.querySelector(".book-detail-actions");
          meta.insertBefore(descEl, actions || null);
        }
        if (descEl) descEl.textContent = newDesc;
      } else if (descEl) {
        descEl.remove();
      }
      setMessage(modal, "Saved.", "success");
      setTimeout(() => closeEditModal(modal), 600);
    } catch (err) {
      setMessage(modal, err.message || "Failed to save.", "error");
    } finally {
      setButtonLoading(submitBtn, false);
    }
  });
})();

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

  // Loading placeholder while we fetch — centered spinner so the modal
  // doesn't flash blank text on a wide card before the content arrives.
  card.innerHTML = `
    <div class="modal-loading" role="status" aria-live="polite">
      <span class="reader-spinner" aria-hidden="true"></span>
      <span class="modal-loading-text">Loading book…</span>
    </div>`;
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

// Tracks the path+search currently rendered into <main>. Used by the
// popstate handler to skip pointless re-renders when the URL changed for
// reasons other than SPA navigation (e.g. book modal opens push /books/X,
// closing pops back to the same /app URL that's already on screen).
// Re-rendering would scroll back to top and lose the user's place.
let lastSpaPath = window.location.pathname + window.location.search;

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

    // Sync the .page-shell class — settings/auth use `page-shell-narrow`
    // (760px max), the rest use `page-shell` (1240px). Without this, going
    // from /settings → /app via SPA leaves the narrow class on, squishing
    // the dashboard until a hard refresh.
    const newShell = doc.querySelector(".page-shell");
    const currentShell = document.querySelector(".page-shell");
    if (newShell && currentShell) {
      currentShell.className = newShell.className;
    }

    // Swap the topbar — the landing page renders its own custom topbar
    // (Sign in / Dashboard buttons), while every other page uses the
    // shared primary_topbar (display name + Log out). After login on the
    // landing page, navigating to /app via SPA needs to replace the
    // header too; otherwise the user sees the logged-out chrome over a
    // logged-in dashboard until the next full refresh.
    const newTopbar = doc.querySelector(".topbar");
    const currentTopbar = document.querySelector(".topbar");
    if (newTopbar && currentTopbar) {
      currentTopbar.replaceWith(newTopbar);
    }

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
    lastSpaPath = target;
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
  // Don't SPA-navigate AWAY from the reader: the reader puts a fixed topbar
  // and a `reader-body-paginated` class on <body>, both outside <main>. SPA
  // would swap <main> but leave that chrome behind, so the close buttons
  // would appear to do nothing useful. Force a real navigation instead.
  if (document.body.classList.contains("reader-body")) return;
  event.preventDefault();
  spaNavigate(url.pathname + url.search + url.hash, true);
});

window.addEventListener("popstate", () => {
  if (!isSpaPath(window.location.pathname)) return;
  const current = window.location.pathname + window.location.search;
  // Skip if the visible <main> already matches the URL — happens when the
  // book modal pops history.back() to dismiss /books/X over an /app screen
  // we never actually left. Re-rendering would scroll the user to the top.
  if (current === lastSpaPath) return;
  spaNavigate(current, false);
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
