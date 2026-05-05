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

const authForm = document.querySelector("[data-auth-form]");
if (authForm) {
  authForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    const message = document.querySelector("[data-form-message]");
    const formData = new FormData(authForm);
    try {
      message.textContent = "Working...";
      await requestJSON(`/api/v1/auth/${authForm.dataset.mode}`, {
        method: "POST",
        body: JSON.stringify({
          email: String(formData.get("email") || "").trim(),
          password: String(formData.get("password") || ""),
        }),
      });
      window.location.href = "/app";
    } catch (error) {
      message.textContent = error.message;
    }
  });
}

const logoutButton = document.querySelector("[data-logout-button]");
if (logoutButton) {
  logoutButton.addEventListener("click", async () => {
    await requestJSON("/api/v1/auth/logout", { method: "POST", body: "{}" });
    window.location.href = "/";
  });
}

const uploadForm = document.querySelector("[data-upload-form]");
if (uploadForm) {
  uploadForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    const message = document.querySelector("[data-upload-message]");
    try {
      message.textContent = "Uploading...";
      await requestJSON("/api/v1/books/upload", {
        method: "POST",
        body: new FormData(uploadForm),
      });
      window.location.reload();
    } catch (error) {
      message.textContent = error.message;
    }
  });
}

document.querySelectorAll("[data-delete-book]").forEach((button) => {
  button.addEventListener("click", async () => {
    const bookID = button.getAttribute("data-delete-book");
    if (!bookID || !window.confirm("Remove this book?")) return;
    await requestJSON(`/api/v1/books/${bookID}`, { method: "DELETE", body: "{}" });
    window.location.reload();
  });
});

document.querySelectorAll("[data-toggle-visibility]").forEach((button) => {
  button.addEventListener("click", async () => {
    const bookID = button.getAttribute("data-toggle-visibility");
    const visibility = button.getAttribute("data-next-visibility");
    if (!bookID || !visibility) return;
    await requestJSON(`/api/v1/books/${bookID}/visibility`, {
      method: "PUT",
      body: JSON.stringify({ visibility }),
    });
    window.location.reload();
  });
});

function attachHighlightDelete(button) {
  if (!button) return;
  button.addEventListener("click", async () => {
    const id = button.getAttribute("data-delete-highlight");
    await requestJSON(`/api/v1/highlights/${id}`, { method: "DELETE", body: "{}" });
    button.closest(".note-card")?.remove();
  });
}

document.querySelectorAll("[data-delete-highlight]").forEach(attachHighlightDelete);

function saveReaderProgressFactory(bookID, progressStatus) {
  return async (locator, progressPercent) => {
    try {
      await requestJSON(`/api/v1/books/${bookID}/progress`, {
        method: "PUT",
        body: JSON.stringify({ locator, progressPercent }),
      });
      if (progressStatus) progressStatus.textContent = `${Math.round(progressPercent)}% read`;
    } catch (_) {}
  };
}

const readerRoot = document.querySelector("[data-reader]");
if (readerRoot) {
  const bookID = readerRoot.getAttribute("data-book-id");
  const readerKind = readerRoot.getAttribute("data-reader-kind");
  const fileURL = readerRoot.getAttribute("data-file-url");
  const progressStatus = document.querySelector("[data-progress-status]");
  const highlightMessage = document.querySelector("[data-highlight-message]");
  const highlightsList = document.querySelector("[data-highlights-list]");
  const saveProgress = saveReaderProgressFactory(bookID, progressStatus);

  document.querySelectorAll("[data-reader-toggle]").forEach((button) => {
    button.addEventListener("click", () => {
      const target = button.getAttribute("data-reader-toggle");
      document.querySelectorAll("[data-reader-panel]").forEach((panel) => {
        const matches = panel.getAttribute("data-reader-panel") === target;
        panel.classList.toggle("is-open", matches ? !panel.classList.contains("is-open") : false);
      });
    });
  });

  const renderHighlightCard = (item) => {
    const card = document.createElement("article");
    card.className = "note-card";
    card.innerHTML = `<span class="note-tag amber">${item.color || "amber"}</span><p>${item.selectedText}</p><div class="book-panel-actions"><a class="text-link" href="#${item.locator}">Jump to passage</a><button class="text-link destructive" data-delete-highlight="${item.id}" type="button">Delete</button></div>`;
    attachHighlightDelete(card.querySelector("[data-delete-highlight]"));
    return card;
  };

  const saveHighlight = async (locator, text) => {
    if (!text || !highlightMessage) return;
    try {
      highlightMessage.textContent = "Saving...";
      const payload = await requestJSON(`/api/v1/books/${bookID}/highlights`, {
        method: "POST",
        body: JSON.stringify({ locator, selectedText: text, color: "amber" }),
      });
      const onlyCard = highlightsList.querySelector(".note-card");
      if (onlyCard && onlyCard.textContent.includes("No highlights")) highlightsList.innerHTML = "";
      highlightsList.prepend(renderHighlightCard(payload.highlight));
      highlightMessage.textContent = "Saved.";
    } catch (error) {
      highlightMessage.textContent = error.message;
    }
  };

  if (readerKind === "md") {
    const readerContent = document.getElementById("reader-content");
    requestJSON(`/api/v1/books/${bookID}/progress`).then((data) => {
      if (data.progress?.locator) {
        const target = document.getElementById(data.progress.locator);
        if (target) target.scrollIntoView({ block: "start" });
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
        const progressPercent = scrollHeight > 0 ? Math.max(0, Math.min(100, window.scrollY / scrollHeight * 100)) : 0;
        await saveProgress(active?.id || "start", progressPercent);
      }, 300);
    }, { passive: true });

    const highlightButton = document.querySelector("[data-save-highlight]");
    if (highlightButton) {
      highlightButton.addEventListener("click", async () => {
        const selection = window.getSelection();
        const text = selection ? selection.toString().trim() : "";
        if (!text) {
          if (highlightMessage) highlightMessage.textContent = "Select some text in the reader first.";
          return;
        }
        let container = selection.anchorNode && selection.anchorNode.nodeType === Node.TEXT_NODE ? selection.anchorNode.parentElement : selection.anchorNode;
        if (!(container instanceof Element)) container = readerContent;
        const section = container.closest("h1,h2,h3,[id]");
        const locator = section?.id || "start";
        await saveHighlight(locator, text);
        selection.removeAllRanges();
      });
    }
  }

  if (readerKind === "epub" && window.ePub) {
    const area = document.getElementById("reader-content");
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
        const a = document.createElement("a");
        a.href = "#";
        a.textContent = item.label;
        a.addEventListener("click", (event) => {
          event.preventDefault();
          rendition.display(item.href);
        });
        li.appendChild(a);
        tocList.appendChild(li);
      });
    }).catch(() => {
      tocList.innerHTML = "<li>No table of contents</li>";
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

    const highlightButton = document.querySelector("[data-save-highlight]");
    if (highlightButton) {
      highlightButton.addEventListener("click", async () => {
        const selection = rendition.getContents()[0]?.window?.getSelection?.();
        const text = selection ? selection.toString().trim() : "";
        const locator = rendition.currentLocation()?.start?.cfi || "start";
        if (!text) {
          if (highlightMessage) highlightMessage.textContent = "Select some EPUB text first.";
          return;
        }
        await saveHighlight(locator, text);
        selection.removeAllRanges();
      });
    }
    void area;
  }

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
      const pdf = await pdfjsLib.getDocument(fileURL).promise;
      let pageNum = 1;

      const renderPage = async (num) => {
        const page = await pdf.getPage(num);
        const viewport = page.getViewport({ scale: 1.25 });
        canvas.width = viewport.width;
        canvas.height = viewport.height;
        await page.render({ canvasContext: ctx, viewport }).promise;
        pageLabel.textContent = `Page ${num} / ${pdf.numPages}`;
        await saveProgress(`page:${num}`, (num / pdf.numPages) * 100);
      };

      requestJSON(`/api/v1/books/${bookID}/progress`).then((data) => {
        const locator = data.progress?.locator || "";
        if (locator.startsWith("page:")) {
          const parsed = Number(locator.split(":")[1]);
          if (parsed > 0) pageNum = parsed;
        }
        renderPage(pageNum);
      }).catch(() => renderPage(pageNum));

      prev?.addEventListener("click", () => {
        if (pageNum <= 1) return;
        pageNum -= 1;
        renderPage(pageNum);
      });
      next?.addEventListener("click", () => {
        if (pageNum >= pdf.numPages) return;
        pageNum += 1;
        renderPage(pageNum);
      });

      pdf.getOutline().then((outline) => {
        if (!outline || outline.length === 0) {
          tocList.innerHTML = "<li>No outline available</li>";
          return;
        }
        tocList.innerHTML = "";
        outline.forEach((item) => {
          const li = document.createElement("li");
          li.textContent = item.title || "Untitled";
          tocList.appendChild(li);
        });
      }).catch(() => {
        tocList.innerHTML = "<li>No outline available</li>";
      });

      const highlightButton = document.querySelector("[data-save-highlight]");
      if (highlightButton) {
        highlightButton.addEventListener("click", async () => {
          const text = window.prompt("PDF text extraction/highlight selection is limited in this lightweight build. Paste a short note for this page:");
          if (!text) return;
          await saveHighlight(`page:${pageNum}`, text.trim());
        });
      }
    };
    initPdf();
  }
}
