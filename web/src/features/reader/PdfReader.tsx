import {
  type Component,
  createSignal,
  createEffect,
  createResource,
  onMount,
  onCleanup,
  Show,
  For,
} from "solid-js";
import { useParams, useNavigate, useSearchParams } from "@solidjs/router";
import {
  ArrowLeft,
  ChevronLeft,
  ChevronRight,
  Settings,
  List,
  X,
  ZoomIn,
  ZoomOut,
  FileText,
  Highlighter,
  MessageSquare,
  Trash2,
} from "lucide-solid";
import { api, getAccessToken } from "../../shared/api/client";
import { t } from "../../shared/i18n/i18n";

// ---- Types ----

interface PdfReaderSettings {
  zoom?: number;
  spreadMode?: "single" | "odd" | "even";
  scrollMode?: "vertical" | "horizontal" | "wrapped";
}

interface ReadingProgress {
  fileId: number;
  progress: string;
  progressType: string;
}

interface OutlineItem {
  title: string;
  dest: any;
  items?: OutlineItem[];
}

interface Annotation {
  id: number;
  bookId: number;
  bookFileId: number | null;
  type: string;
  cfi: string | null;
  pageNumber: number | null;
  text: string | null;
  note: string | null;
  color: string;
  createdAt: string;
}

// ---- Annotation colors ----

const annotationColors = ["yellow", "green", "blue", "pink", "purple"] as const;
type AnnotationColor = (typeof annotationColors)[number];

const colorDotClasses: Record<AnnotationColor, string> = {
  yellow: "bg-yellow-400",
  green: "bg-green-400",
  blue: "bg-blue-400",
  pink: "bg-pink-400",
  purple: "bg-purple-400",
};

// ---- Default settings ----

const defaultSettings: PdfReaderSettings = {
  zoom: 1.0,
  spreadMode: "single",
  scrollMode: "vertical",
};

// ---- Zoom presets ----

const zoomPresets = [0.75, 1.0, 1.25, 1.5, 2.0];
const zoomLabels: Record<number, string> = {
  0.75: "75%",
  1.0: "100%",
  1.25: "125%",
  1.5: "150%",
  2.0: "200%",
};

// ---- API helpers ----

async function fetchProgress(bookId: string): Promise<ReadingProgress | null> {
  try {
    return await api<ReadingProgress>(`/reader/books/${bookId}/progress`);
  } catch {
    return null;
  }
}

async function saveProgress(
  bookId: string,
  fileId: number,
  page: number,
): Promise<void> {
  try {
    await api(`/reader/books/${bookId}/progress`, {
      method: "PUT",
      body: JSON.stringify({
        fileId,
        progress: `page:${page}`,
        progressType: "PAGE",
      }),
    });
  } catch {
    // Non-fatal: progress save failure should not interrupt reading.
  }
}

async function fetchSettings(
  bookId: string,
): Promise<PdfReaderSettings | null> {
  try {
    return await api<PdfReaderSettings>(`/reader/books/${bookId}/settings`);
  } catch {
    return null;
  }
}

async function saveSettings(
  bookId: string,
  settings: PdfReaderSettings,
): Promise<void> {
  try {
    await api(`/reader/books/${bookId}/settings`, {
      method: "PUT",
      body: JSON.stringify(settings),
    });
  } catch {
    // Non-fatal.
  }
}

async function fetchAnnotations(bookId: string): Promise<Annotation[]> {
  try {
    return await api<Annotation[]>(`/reader/books/${bookId}/annotations`);
  } catch {
    return [];
  }
}

async function createPdfAnnotation(
  bookId: string,
  fileId: number,
  pageNumber: number,
  note: string,
  color: AnnotationColor,
): Promise<Annotation | null> {
  try {
    return await api<Annotation>(`/reader/books/${bookId}/annotations`, {
      method: "POST",
      body: JSON.stringify({
        bookFileId: fileId,
        type: "NOTE",
        pageNumber,
        text: "",
        note,
        color,
      }),
    });
  } catch {
    return null;
  }
}

async function deleteAnnotationApi(
  bookId: string,
  annotationId: number,
): Promise<void> {
  try {
    await api(`/reader/books/${bookId}/annotations/${annotationId}`, {
      method: "DELETE",
    });
  } catch {
    // Non-fatal.
  }
}

// ---- Debounce helper ----

function debounce<T extends (...args: Parameters<T>) => void>(
  fn: T,
  ms: number,
): (...args: Parameters<T>) => void {
  let timer: ReturnType<typeof setTimeout> | undefined;
  return (...args: Parameters<T>) => {
    clearTimeout(timer);
    timer = setTimeout(() => fn(...args), ms);
  };
}

// ---- PdfReader component ----

const PdfReader: Component = () => {
  const params = useParams<{ id: string }>();
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();

  let canvasRef!: HTMLCanvasElement;
  let containerRef!: HTMLDivElement;

  const [showUI, setShowUI] = createSignal(true);
  const [showSettings, setShowSettings] = createSignal(false);
  const [showSidebar, setShowSidebar] = createSignal(false);
  const [sidebarTab, setSidebarTab] = createSignal<"toc" | "thumbnails" | "annotations">("toc");
  const [loading, setLoading] = createSignal(true);
  const [error, setError] = createSignal<string | null>(null);
  const [rendering, setRendering] = createSignal(false);

  const [currentPage, setCurrentPage] = createSignal(1);
  const [totalPages, setTotalPages] = createSignal(0);
  const [zoom, setZoom] = createSignal(1.0);
  const [pageInput, setPageInput] = createSignal("1");
  const [outline, setOutline] = createSignal<OutlineItem[]>([]);
  const [thumbnails, setThumbnails] = createSignal<
    { page: number; dataUrl: string }[]
  >([]);

  const [settings, setSettings] = createSignal<PdfReaderSettings>(defaultSettings);

  // Annotation state.
  const [annotations, setAnnotations] = createSignal<Annotation[]>([]);
  const [showAddNoteForm, setShowAddNoteForm] = createSignal(false);
  const [newNoteText, setNewNoteText] = createSignal("");
  const [newNoteColor, setNewNoteColor] = createSignal<AnnotationColor>("yellow");

  const pdfState: { pdf: any; renderTask: any } = {
    pdf: null,
    renderTask: null,
  };

  let hideTimer: ReturnType<typeof setTimeout> | undefined;

  function resetHideTimer() {
    setShowUI(true);
    clearTimeout(hideTimer);
    hideTimer = setTimeout(() => {
      if (!showSettings() && !showSidebar() && !showAddNoteForm()) {
        setShowUI(false);
      }
    }, 3000);
  }

  const debouncedSaveProgress = debounce(
    (fileId: number, page: number) => {
      saveProgress(params.id, fileId, page);
    },
    2000,
  );

  async function renderPage(pageNum: number, scale: number) {
    if (!pdfState.pdf || !canvasRef) return;
    if (pdfState.renderTask) {
      pdfState.renderTask.cancel();
      pdfState.renderTask = null;
    }
    setRendering(true);
    try {
      const page = await pdfState.pdf.getPage(pageNum);
      const viewport = page.getViewport({ scale });
      canvasRef.width = viewport.width;
      canvasRef.height = viewport.height;
      const ctx = canvasRef.getContext("2d");
      if (!ctx) return;
      const renderContext = { canvasContext: ctx, viewport };
      pdfState.renderTask = page.render(renderContext);
      await pdfState.renderTask.promise;
      pdfState.renderTask = null;
    } catch (err: any) {
      if (err?.name !== "RenderingCancelledException") {
        console.error("PDF render error:", err);
      }
    } finally {
      setRendering(false);
    }
  }

  async function generateThumbnail(
    pageNum: number,
  ): Promise<{ page: number; dataUrl: string } | null> {
    if (!pdfState.pdf) return null;
    try {
      const page = await pdfState.pdf.getPage(pageNum);
      const viewport = page.getViewport({ scale: 0.2 });
      const canvas = document.createElement("canvas");
      canvas.width = viewport.width;
      canvas.height = viewport.height;
      const ctx = canvas.getContext("2d");
      if (!ctx) return null;
      const task = page.render({ canvasContext: ctx, viewport });
      await task.promise;
      return { page: pageNum, dataUrl: canvas.toDataURL("image/jpeg", 0.7) };
    } catch {
      return null;
    }
  }

  async function loadThumbnails() {
    if (!pdfState.pdf || thumbnails().length > 0) return;
    const total = pdfState.pdf.numPages as number;
    const results: { page: number; dataUrl: string }[] = [];
    for (let i = 1; i <= total; i++) {
      const thumb = await generateThumbnail(i);
      if (thumb) results.push(thumb);
      if (i % 5 === 0 || i === total) {
        setThumbnails([...results]);
      }
    }
  }

  async function navigateToOutlineItem(dest: any) {
    if (!pdfState.pdf) return;
    try {
      let pageIndex: number;
      if (typeof dest === "string") {
        pageIndex = await pdfState.pdf.getPageIndex(
          await pdfState.pdf.getDestination(dest),
        );
      } else if (Array.isArray(dest)) {
        pageIndex = await pdfState.pdf.getPageIndex(dest[0]);
      } else {
        return;
      }
      const pageNum = pageIndex + 1;
      setCurrentPage(pageNum);
      setPageInput(String(pageNum));
      setShowSidebar(false);
    } catch {
      // Ignore navigation errors.
    }
  }

  function goToPrevPage() {
    const page = currentPage();
    if (page > 1) {
      const next = page - 1;
      setCurrentPage(next);
      setPageInput(String(next));
    }
    resetHideTimer();
  }

  function goToNextPage() {
    const page = currentPage();
    const total = totalPages();
    if (page < total) {
      const next = page + 1;
      setCurrentPage(next);
      setPageInput(String(next));
    }
    resetHideTimer();
  }

  function zoomIn() {
    const current = zoom();
    const next = zoomPresets.find((z) => z > current);
    if (next !== undefined) {
      setZoom(next);
      updateSetting("zoom", next);
    }
  }

  function zoomOut() {
    const current = zoom();
    const prev = [...zoomPresets].reverse().find((z) => z < current);
    if (prev !== undefined) {
      setZoom(prev);
      updateSetting("zoom", prev);
    }
  }

  function updateSetting<K extends keyof PdfReaderSettings>(
    key: K,
    value: PdfReaderSettings[K],
  ) {
    const next = { ...settings(), [key]: value };
    setSettings(next);
    saveSettings(params.id, next);
  }

  function goToPage(page: number) {
    if (page >= 1 && page <= totalPages()) {
      setCurrentPage(page);
      setPageInput(String(page));
      setShowSidebar(false);
    }
  }

  async function handleAddNote() {
    const fileId = searchParams.fileId;
    if (!fileId) return;
    const text = newNoteText().trim();
    if (!text) return;
    const annotation = await createPdfAnnotation(
      params.id,
      Number(fileId),
      currentPage(),
      text,
      newNoteColor(),
    );
    if (annotation) {
      setAnnotations((prev) => [annotation, ...prev]);
      setNewNoteText("");
      setShowAddNoteForm(false);
    }
  }

  async function handleDeleteAnnotation(annotation: Annotation) {
    await deleteAnnotationApi(params.id, annotation.id);
    setAnnotations((prev) => prev.filter((a) => a.id !== annotation.id));
  }

  function annotationsForPage(page: number): Annotation[] {
    return annotations().filter((a) => a.pageNumber === page);
  }

  function pageHasAnnotations(page: number): boolean {
    return annotations().some((a) => a.pageNumber === page);
  }

  function annotationColorClass(color: string): string {
    const classes: Record<string, string> = {
      yellow: "border-yellow-400/40 bg-yellow-400/10",
      green: "border-green-400/40 bg-green-400/10",
      blue: "border-blue-400/40 bg-blue-400/10",
      pink: "border-pink-400/40 bg-pink-400/10",
      purple: "border-purple-400/40 bg-purple-400/10",
    };
    return classes[color] ?? classes.yellow;
  }

  function handlePageInputChange(e: Event) {
    const val = (e.currentTarget as HTMLInputElement).value;
    setPageInput(val);
  }

  function handlePageInputCommit(e: Event) {
    e.preventDefault();
    const num = parseInt(pageInput(), 10);
    if (!isNaN(num) && num >= 1 && num <= totalPages()) {
      setCurrentPage(num);
    } else {
      setPageInput(String(currentPage()));
    }
  }

  onMount(async () => {
    const fileId = searchParams.fileId;
    if (!fileId) {
      setError(t("reader.noFileSpecified"));
      setLoading(false);
      return;
    }

    const token = getAccessToken();
    if (!token) {
      setError(t("reader.notAuthenticated"));
      setLoading(false);
      return;
    }

    const savedSettings = await fetchSettings(params.id);
    const mergedSettings: PdfReaderSettings = {
      ...defaultSettings,
      ...(savedSettings ?? {}),
    };
    setSettings(mergedSettings);
    setZoom(mergedSettings.zoom ?? 1.0);

    // Load existing annotations.
    const existingAnnotations = await fetchAnnotations(params.id);
    setAnnotations(existingAnnotations);

    const pdfjsLib = await import("pdfjs-dist");
    pdfjsLib.GlobalWorkerOptions.workerSrc = new URL(
      "pdfjs-dist/build/pdf.worker.min.mjs",
      import.meta.url,
    ).toString();

    const streamUrl = `/api/reader/books/${params.id}/files/${fileId}/stream`;

    try {
      const loadingTask = pdfjsLib.getDocument({
        url: streamUrl,
        httpHeaders: { Authorization: `Bearer ${token}` },
        withCredentials: false,
      });

      pdfState.pdf = await loadingTask.promise;
      const total = pdfState.pdf.numPages as number;
      setTotalPages(total);

      try {
        const outlineData = await pdfState.pdf.getOutline();
        if (outlineData) {
          setOutline(outlineData as OutlineItem[]);
        }
      } catch {
        // Outline is optional.
      }

      const savedProgress = await fetchProgress(params.id);
      let startPage = 1;
      if (savedProgress?.progress?.startsWith("page:")) {
        const parsed = parseInt(savedProgress.progress.slice(5), 10);
        if (!isNaN(parsed) && parsed >= 1 && parsed <= total) {
          startPage = parsed;
        }
      }
      setCurrentPage(startPage);
      setPageInput(String(startPage));

      setLoading(false);

      function handleKeyDown(e: KeyboardEvent) {
        if (
          e.target instanceof HTMLInputElement ||
          e.target instanceof HTMLTextAreaElement
        ) {
          return;
        }
        if (e.key === "ArrowLeft" || e.key === "ArrowUp") {
          goToPrevPage();
        } else if (e.key === "ArrowRight" || e.key === "ArrowDown") {
          goToNextPage();
        } else if (e.key === "+" || e.key === "=") {
          zoomIn();
        } else if (e.key === "-") {
          zoomOut();
        } else if (e.key === "Escape") {
          setShowSettings(false);
          setShowSidebar(false);
          setShowAddNoteForm(false);
        }
      }

      document.addEventListener("keydown", handleKeyDown);

      onCleanup(() => {
        document.removeEventListener("keydown", handleKeyDown);
      });
    } catch (err) {
      setError(
        err instanceof Error ? err.message : t("reader.failedToLoadPDF"),
      );
      setLoading(false);
    }
  });

  onCleanup(() => {
    clearTimeout(hideTimer);
    if (pdfState.renderTask) {
      pdfState.renderTask.cancel();
    }
    if (pdfState.pdf) {
      pdfState.pdf.destroy();
    }
  });

  createEffect(() => {
    const page = currentPage();
    const scale = zoom();
    const fileId = searchParams.fileId;
    if (!pdfState.pdf || loading()) return;
    renderPage(page, scale);
    if (fileId) {
      debouncedSaveProgress(Number(fileId), page);
    }
  });

  createEffect(() => {
    if (showSidebar() && sidebarTab() === "thumbnails") {
      loadThumbnails();
    }
  });

  const zoomPercent = () => `${Math.round(zoom() * 100)}%`;

  return (
    <div
      class="relative flex h-screen w-screen flex-col overflow-hidden bg-slate-950"
      onMouseMove={resetHideTimer}
      onClick={resetHideTimer}
    >
      <Show when={loading()}>
        <div class="absolute inset-0 z-50 flex items-center justify-center bg-slate-950">
          <div class="flex flex-col items-center gap-3 text-slate-400">
            <FileText class="h-10 w-10 animate-pulse text-indigo-400" />
            <p class="text-sm">{t("reader.loadingPDF")}</p>
          </div>
        </div>
      </Show>

      <Show when={error()}>
        <div class="absolute inset-0 z-50 flex items-center justify-center bg-slate-950">
          <div class="flex flex-col items-center gap-4 text-center">
            <p class="text-lg font-medium text-red-400">{error()}</p>
            <button
              onClick={() => navigate(-1)}
              class="text-sm text-slate-400 hover:text-slate-200 transition-colors"
            >
              {t("common.goBack")}
            </button>
          </div>
        </div>
      </Show>

      <div
        class={`absolute left-0 right-0 top-0 z-30 flex items-center justify-between px-4 py-3 transition-all duration-300 ${
          showUI() ? "opacity-100 translate-y-0" : "opacity-0 -translate-y-full"
        }`}
        style={{ background: "rgba(0,0,0,0.7)", "backdrop-filter": "blur(8px)" }}
      >
        <button
          onClick={() => navigate(-1)}
          class="flex items-center gap-2 rounded-lg px-3 py-1.5 text-sm text-slate-300 hover:bg-white/10 hover:text-white transition-colors"
        >
          <ArrowLeft class="h-4 w-4" />
          <span class="hidden sm:inline">{t("reader.back")}</span>
        </button>

        <div class="flex min-w-0 flex-1 flex-col items-center px-4">
          <Show when={totalPages() > 0}>
            <p class="text-sm font-medium text-slate-200">
              {t("reader.pages")} {currentPage()} {t("common.of")} {totalPages()}
            </p>
          </Show>
        </div>

        <div class="flex items-center gap-1">
          <button
            onClick={zoomOut}
            class="rounded-lg p-2 text-slate-300 hover:bg-white/10 hover:text-white transition-colors"
            title={t("reader.zoomOut")}
          >
            <ZoomOut class="h-4 w-4" />
          </button>
          <span class="min-w-[3rem] text-center text-xs text-slate-400">
            {zoomPercent()}
          </span>
          <button
            onClick={zoomIn}
            class="rounded-lg p-2 text-slate-300 hover:bg-white/10 hover:text-white transition-colors"
            title={t("reader.zoomIn")}
          >
            <ZoomIn class="h-4 w-4" />
          </button>
          <button
            onClick={() => {
              setShowSidebar((v) => !v);
              setShowSettings(false);
              setSidebarTab("annotations");
            }}
            class="rounded-lg p-2 text-slate-300 hover:bg-white/10 hover:text-white transition-colors"
            title={t("reader.annotations")}
          >
            <Highlighter class="h-4 w-4" />
          </button>
          <button
            onClick={() => {
              setShowSidebar((v) => !v);
              setShowSettings(false);
              setSidebarTab("toc");
            }}
            class="rounded-lg p-2 text-slate-300 hover:bg-white/10 hover:text-white transition-colors"
            title={t("reader.tableOfContents")}
          >
            <List class="h-4 w-4" />
          </button>
          <button
            onClick={() => {
              setShowSettings((v) => !v);
              setShowSidebar(false);
            }}
            class="rounded-lg p-2 text-slate-300 hover:bg-white/10 hover:text-white transition-colors"
            title={t("reader.readerSettingsTitle")}
          >
            <Settings class="h-4 w-4" />
          </button>
        </div>
      </div>

      <div
        ref={containerRef}
        class="relative flex flex-1 items-center justify-center overflow-auto"
        style={{ background: "#525659" }}
      >
        <canvas
          ref={canvasRef}
          class="shadow-2xl"
          style={{
            display: loading() ? "none" : "block",
            opacity: rendering() ? "0.7" : "1",
            transition: "opacity 0.15s ease",
          }}
        />

        {/* Current page annotations overlay */}
        <Show when={annotationsForPage(currentPage()).length > 0 && !loading()}>
          <div
            class="absolute right-4 top-4 z-20 flex max-w-xs flex-col gap-2 rounded-lg border border-white/10 p-3 shadow-xl"
            style={{ background: "rgba(15,15,30,0.92)", "backdrop-filter": "blur(12px)" }}
            onClick={(e) => e.stopPropagation()}
          >
            <div class="flex items-center gap-2">
              <MessageSquare class="h-3.5 w-3.5 text-slate-400" />
              <span class="text-xs font-medium text-slate-300">
                {t("reader.pageAnnotations")}
              </span>
            </div>
            <For each={annotationsForPage(currentPage())}>
              {(annotation) => (
                <div
                  class={`rounded border px-2 py-1.5 text-xs ${annotationColorClass(annotation.color)}`}
                >
                  <p class="text-slate-200">{annotation.note}</p>
                </div>
              )}
            </For>
          </div>
        </Show>
      </div>

      <div
        class={`absolute bottom-0 left-0 right-0 z-30 flex items-center justify-between px-4 py-3 transition-all duration-300 ${
          showUI() ? "opacity-100 translate-y-0" : "opacity-0 translate-y-full"
        }`}
        style={{ background: "rgba(0,0,0,0.7)", "backdrop-filter": "blur(8px)" }}
      >
        <button
          onClick={goToPrevPage}
          disabled={currentPage() <= 1}
          class="flex items-center gap-2 rounded-lg px-4 py-2 text-sm text-slate-300 hover:bg-white/10 hover:text-white transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
        >
          <ChevronLeft class="h-4 w-4" />
          <span class="hidden sm:inline">{t("reader.previous")}</span>
        </button>

        <form onSubmit={handlePageInputCommit} class="flex items-center gap-2">
          <input
            type="number"
            min="1"
            max={totalPages()}
            value={pageInput()}
            onInput={handlePageInputChange}
            onBlur={handlePageInputCommit}
            class="w-16 rounded-lg bg-white/10 px-2 py-1 text-center text-sm text-slate-200 focus:outline-none focus:ring-1 focus:ring-indigo-400"
          />
          <Show when={totalPages() > 0}>
            <span class="text-xs text-slate-500">/ {totalPages()}</span>
          </Show>
        </form>

        <button
          onClick={goToNextPage}
          disabled={currentPage() >= totalPages()}
          class="flex items-center gap-2 rounded-lg px-4 py-2 text-sm text-slate-300 hover:bg-white/10 hover:text-white transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
        >
          <span class="hidden sm:inline">{t("reader.next")}</span>
          <ChevronRight class="h-4 w-4" />
        </button>
      </div>

      <div
        class={`absolute bottom-0 left-0 top-0 z-40 flex w-72 flex-col overflow-hidden transition-transform duration-300 ${
          showSidebar() ? "translate-x-0" : "-translate-x-full"
        }`}
        style={{
          background: "rgba(15,15,30,0.95)",
          "backdrop-filter": "blur(12px)",
        }}
      >
        <div class="flex items-center justify-between border-b border-white/10 px-4 py-3">
          <div class="flex gap-2">
            <button
              onClick={() => setSidebarTab("toc")}
              class={`rounded-lg px-3 py-1 text-xs font-medium transition-colors ${
                sidebarTab() === "toc"
                  ? "bg-indigo-600/30 text-indigo-300"
                  : "text-slate-400 hover:bg-white/10 hover:text-slate-200"
              }`}
            >
              {t("reader.contents")}
            </button>
            <button
              onClick={() => setSidebarTab("thumbnails")}
              class={`rounded-lg px-3 py-1 text-xs font-medium transition-colors ${
                sidebarTab() === "thumbnails"
                  ? "bg-indigo-600/30 text-indigo-300"
                  : "text-slate-400 hover:bg-white/10 hover:text-slate-200"
              }`}
            >
              {t("reader.pages")}
            </button>
            <button
              onClick={() => setSidebarTab("annotations")}
              class={`rounded-lg px-3 py-1 text-xs font-medium transition-colors ${
                sidebarTab() === "annotations"
                  ? "bg-indigo-600/30 text-indigo-300"
                  : "text-slate-400 hover:bg-white/10 hover:text-slate-200"
              }`}
            >
              {t("reader.annotations")}
            </button>
          </div>
          <button
            onClick={() => setShowSidebar(false)}
            class="rounded-lg p-1.5 text-slate-400 hover:bg-white/10 hover:text-white transition-colors"
          >
            <X class="h-4 w-4" />
          </button>
        </div>

        <Show when={sidebarTab() === "toc"}>
          <nav class="flex-1 overflow-y-auto p-2">
            <Show
              when={outline().length > 0}
              fallback={
                <p class="px-2 py-4 text-sm text-slate-500">
                  {t("reader.noTableOfContents")}
                </p>
              }
            >
              <For each={outline()}>
                {(item) => (
                  <OutlineItemButton
                    item={item}
                    onNavigate={navigateToOutlineItem}
                  />
                )}
              </For>
            </Show>
          </nav>
        </Show>

        <Show when={sidebarTab() === "thumbnails"}>
          <div class="flex-1 overflow-y-auto p-2">
            <Show
              when={thumbnails().length > 0}
              fallback={
                <div class="flex items-center justify-center py-8">
                  <p class="text-sm text-slate-500">{t("reader.loadingThumbnails")}</p>
                </div>
              }
            >
              <div class="grid grid-cols-2 gap-2">
                <For each={thumbnails()}>
                  {(thumb) => (
                    <button
                      onClick={() => {
                        setCurrentPage(thumb.page);
                        setPageInput(String(thumb.page));
                        setShowSidebar(false);
                      }}
                      class={`relative flex flex-col items-center gap-1 rounded-lg p-1 transition-colors ${
                        currentPage() === thumb.page
                          ? "bg-indigo-600/30 ring-1 ring-indigo-400"
                          : "hover:bg-white/10"
                      }`}
                    >
                      <img
                        src={thumb.dataUrl}
                        alt={`${t("reader.pages")} ${thumb.page}`}
                        class="w-full rounded border border-white/10"
                      />
                      <span class="text-xs text-slate-400">{thumb.page}</span>
                      <Show when={pageHasAnnotations(thumb.page)}>
                        <div class="absolute right-2 top-2 flex gap-0.5">
                          <For each={annotationsForPage(thumb.page)}>
                            {(a) => (
                              <span class={`h-2 w-2 rounded-full ${colorDotClasses[(a.color as AnnotationColor) ?? "yellow"]}`} />
                            )}
                          </For>
                        </div>
                      </Show>
                    </button>
                  )}
                </For>
              </div>
            </Show>
          </div>
        </Show>

        <Show when={sidebarTab() === "annotations"}>
          <div class="flex-1 overflow-y-auto p-3">
            <Show
              when={annotations().length > 0}
              fallback={
                <div class="flex flex-col items-center justify-center py-8 text-center">
                  <MessageSquare class="mb-3 h-10 w-10 text-slate-600" />
                  <p class="text-sm text-slate-500">{t("reader.noAnnotationsYet")}</p>
                  <p class="mt-1 text-xs text-slate-500">
                    {t("reader.addNoteFromSettings")}
                  </p>
                </div>
              }
            >
              <div class="flex flex-col gap-2">
                <For each={annotations()}>
                  {(annotation) => (
                    <div
                      class={`rounded-lg border p-3 ${annotationColorClass(annotation.color)}`}
                    >
                      <div class="flex items-start justify-between gap-2">
                        <button
                          class="flex-1 min-w-0 text-left"
                          onClick={() => annotation.pageNumber && goToPage(annotation.pageNumber)}
                        >
                          <p class="text-xs font-medium text-slate-300">
                            {t("reader.page")} {annotation.pageNumber}
                          </p>
                          <Show when={annotation.note}>
                            <p class="mt-1 text-xs leading-relaxed text-slate-200">
                              {annotation.note}
                            </p>
                          </Show>
                        </button>
                        <button
                          onClick={() => handleDeleteAnnotation(annotation)}
                          class="shrink-0 rounded p-1 text-slate-500 hover:bg-white/10 hover:text-red-400 transition-colors"
                          title={t("common.delete")}
                        >
                          <Trash2 class="h-3.5 w-3.5" />
                        </button>
                      </div>
                    </div>
                  )}
                </For>
              </div>
            </Show>
          </div>
        </Show>
      </div>

      <div
        class={`absolute bottom-0 right-0 top-0 z-40 w-72 overflow-y-auto transition-transform duration-300 ${
          showSettings() ? "translate-x-0" : "translate-x-full"
        }`}
        style={{
          background: "rgba(15,15,30,0.95)",
          "backdrop-filter": "blur(12px)",
        }}
      >
        <div class="flex items-center justify-between border-b border-white/10 px-4 py-3">
          <h2 class="text-sm font-semibold text-slate-200">{t("reader.readerSettings")}</h2>
          <button
            onClick={() => setShowSettings(false)}
            class="rounded-lg p-1.5 text-slate-400 hover:bg-white/10 hover:text-white transition-colors"
          >
            <X class="h-4 w-4" />
          </button>
        </div>

        <div class="flex flex-col gap-6 p-4">
          <div class="flex flex-col gap-2">
            <label class="text-xs font-medium uppercase tracking-wide text-slate-500">
              {t("reader.zoom")}
            </label>
            <div class="flex flex-col gap-1">
              <For each={zoomPresets}>
                {(preset) => (
                  <button
                    onClick={() => {
                      setZoom(preset);
                      updateSetting("zoom", preset);
                    }}
                    class={`rounded-lg px-3 py-2 text-left text-sm transition-colors ${
                      zoom() === preset
                        ? "bg-indigo-600/30 text-indigo-300"
                        : "text-slate-400 hover:bg-white/10 hover:text-slate-200"
                    }`}
                  >
                    {zoomLabels[preset] ?? `${Math.round(preset * 100)}%`}
                  </button>
                )}
              </For>
            </div>
          </div>

          <div class="flex flex-col gap-2">
            <label class="text-xs font-medium uppercase tracking-wide text-slate-500">
              {t("reader.pageLayout")}
            </label>
            <div class="flex flex-col gap-1">
              <For
                each={
                  [
                    { value: "single", label: t("reader.singlePage") },
                    { value: "odd", label: t("reader.twoPagesOdd") },
                    { value: "even", label: t("reader.twoPagesEven") },
                  ] as const
                }
              >
                {(opt) => (
                  <button
                    onClick={() => updateSetting("spreadMode", opt.value)}
                    class={`rounded-lg px-3 py-2 text-left text-sm transition-colors ${
                      settings().spreadMode === opt.value
                        ? "bg-indigo-600/30 text-indigo-300"
                        : "text-slate-400 hover:bg-white/10 hover:text-slate-200"
                    }`}
                  >
                    {opt.label}
                  </button>
                )}
              </For>
            </div>
          </div>

          <div class="flex flex-col gap-2">
            <label class="text-xs font-medium uppercase tracking-wide text-slate-500">
              {t("reader.scrollMode")}
            </label>
            <div class="flex flex-col gap-1">
              <For
                each={
                  [
                    { value: "vertical", label: t("reader.vertical") },
                    { value: "horizontal", label: t("reader.horizontal") },
                    { value: "wrapped", label: t("reader.wrapped") },
                  ] as const
                }
              >
                {(opt) => (
                  <button
                    onClick={() => updateSetting("scrollMode", opt.value)}
                    class={`rounded-lg px-3 py-2 text-left text-sm transition-colors ${
                      settings().scrollMode === opt.value
                        ? "bg-indigo-600/30 text-indigo-300"
                        : "text-slate-400 hover:bg-white/10 hover:text-slate-200"
                    }`}
                  >
                    {opt.label}
                  </button>
                )}
              </For>
            </div>
          </div>

          {/* Add Note */}
          <div class="flex flex-col gap-2 border-t border-white/10 pt-4">
            <label class="text-xs font-medium uppercase tracking-wide text-slate-500">
              {t("reader.addNote")}
            </label>
            <Show
              when={showAddNoteForm()}
              fallback={
                <button
                  onClick={() => setShowAddNoteForm(true)}
                  class="flex items-center gap-2 rounded-lg px-3 py-2 text-left text-sm text-slate-400 hover:bg-white/10 hover:text-slate-200 transition-colors"
                >
                  <MessageSquare class="h-4 w-4" />
                  {t("reader.addNoteToPage")} {currentPage()}
                </button>
              }
            >
              <div class="flex flex-col gap-2">
                <textarea
                  value={newNoteText()}
                  onInput={(e) => setNewNoteText(e.currentTarget.value)}
                  placeholder={t("reader.notePlaceholder")}
                  class="w-full rounded-lg border border-slate-700 bg-slate-800 px-3 py-2 text-sm text-slate-200 placeholder-slate-500 focus:border-indigo-500 focus:outline-none"
                  rows={3}
                />
                <div class="flex items-center gap-2">
                  <For each={annotationColors}>
                    {(color) => (
                      <button
                        onClick={() => setNewNoteColor(color)}
                        class={`h-5 w-5 rounded-full transition-all ${colorDotClasses[color]} ${
                          newNoteColor() === color
                            ? "ring-2 ring-white ring-offset-1 ring-offset-slate-900"
                            : "opacity-60 hover:opacity-100"
                        }`}
                        title={color}
                      />
                    )}
                  </For>
                </div>
                <div class="flex gap-2">
                  <button
                    onClick={handleAddNote}
                    class="rounded-lg bg-indigo-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-indigo-500 transition-colors"
                  >
                    {t("common.save")}
                  </button>
                  <button
                    onClick={() => {
                      setShowAddNoteForm(false);
                      setNewNoteText("");
                    }}
                    class="rounded-lg px-3 py-1.5 text-xs font-medium text-slate-400 hover:bg-white/10 hover:text-slate-200 transition-colors"
                  >
                    {t("common.cancel")}
                  </button>
                </div>
              </div>
            </Show>
          </div>
        </div>
      </div>

      <Show when={showSidebar() || showSettings() || showAddNoteForm()}>
        <div
          class="absolute inset-0 z-[35]"
          onClick={() => {
            setShowSidebar(false);
            setShowSettings(false);
          }}
        />
      </Show>
    </div>
  );
};

const OutlineItemButton: Component<{
  item: OutlineItem;
  onNavigate: (dest: any) => void;
  depth?: number;
}> = (props) => {
  const depth = () => props.depth ?? 0;

  return (
    <>
      <button
        onClick={() => props.onNavigate(props.item.dest)}
        class="w-full rounded-lg px-3 py-2 text-left text-sm text-slate-300 hover:bg-white/10 hover:text-white transition-colors"
        style={{ "padding-left": `${12 + depth() * 16}px` }}
      >
        {props.item.title}
      </button>
      <Show when={props.item.items && props.item.items.length > 0}>
        <For each={props.item.items}>
          {(child) => (
            <OutlineItemButton
              item={child}
              onNavigate={props.onNavigate}
              depth={depth() + 1}
            />
          )}
        </For>
      </Show>
    </>
  );
};

export default PdfReader;
