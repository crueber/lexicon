import {
  type Component,
  createSignal,
  createEffect,
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
  X,
  BookOpen,
} from "lucide-solid";
import { api, getAccessToken } from "../../shared/api/client";

// ---- Types ----

interface ComicPageInfo {
  index: number;
  filename: string;
}

interface ComicReaderSettings {
  displayMode?: "single" | "double";
  fitMode?: "width" | "height" | "original";
  readingDirection?: "ltr" | "rtl";
}

interface ReadingProgress {
  fileId: number;
  progress: string;
  progressType: string;
}

// ---- Default settings ----

const defaultSettings: ComicReaderSettings = {
  displayMode: "single",
  fitMode: "width",
  readingDirection: "ltr",
};

// ---- API helpers ----

async function fetchPages(
  bookId: string,
  fileId: string,
): Promise<ComicPageInfo[]> {
  return api<ComicPageInfo[]>(`/reader/books/${bookId}/files/${fileId}/pages`);
}

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
): Promise<ComicReaderSettings | null> {
  try {
    return await api<ComicReaderSettings>(`/reader/books/${bookId}/settings`);
  } catch {
    return null;
  }
}

async function saveSettings(
  bookId: string,
  settings: ComicReaderSettings,
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

// ---- Page image URL helper ----

// Fetches a comic page image with auth and returns an object URL.
// The caller is responsible for revoking the URL when done.
async function fetchPageObjectURL(
  bookId: string,
  fileId: string,
  pageIndex: number,
  token: string,
): Promise<string> {
  const response = await fetch(
    `/api/reader/books/${bookId}/files/${fileId}/pages/${pageIndex}`,
    {
      headers: { Authorization: `Bearer ${token}` },
    },
  );
  if (!response.ok) {
    throw new Error(`Failed to load page ${pageIndex}: ${response.status}`);
  }
  const blob = await response.blob();
  return URL.createObjectURL(blob);
}

// ---- ComicReader component ----

const ComicReader: Component = () => {
  const params = useParams<{ id: string }>();
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();

  // UI state.
  const [showUI, setShowUI] = createSignal(true);
  const [showSettings, setShowSettings] = createSignal(false);
  const [loading, setLoading] = createSignal(true);
  const [error, setError] = createSignal<string | null>(null);

  // Comic state.
  const [pages, setPages] = createSignal<ComicPageInfo[]>([]);
  const [currentPage, setCurrentPage] = createSignal(0);
  const [pageInput, setPageInput] = createSignal("1");

  // Page image object URLs — keyed by page index.
  // Stored outside reactive state to avoid reactivity wrapping.
  const pageURLs: Map<number, string> = new Map();
  const [currentPageURL, setCurrentPageURL] = createSignal<string | null>(null);
  const [nextPageURL, setNextPageURL] = createSignal<string | null>(null);

  // Settings.
  const [settings, setSettings] =
    createSignal<ComicReaderSettings>(defaultSettings);

  // Auto-hide UI after 3 seconds of inactivity.
  let hideTimer: ReturnType<typeof setTimeout> | undefined;

  function resetHideTimer() {
    setShowUI(true);
    clearTimeout(hideTimer);
    hideTimer = setTimeout(() => {
      if (!showSettings()) {
        setShowUI(false);
      }
    }, 3000);
  }

  // Debounced progress save (2 seconds after last page change).
  const debouncedSaveProgress = debounce(
    (fileId: number, page: number) => {
      saveProgress(params.id, fileId, page);
    },
    2000,
  );

  // Load a page image, caching the object URL.
  async function loadPageURL(
    pageIndex: number,
    token: string,
  ): Promise<string | null> {
    if (pageURLs.has(pageIndex)) {
      return pageURLs.get(pageIndex)!;
    }
    const fileId = Array.isArray(searchParams.fileId)
      ? searchParams.fileId[0]
      : searchParams.fileId;
    if (!fileId) return null;
    try {
      const url = await fetchPageObjectURL(params.id, fileId, pageIndex, token);
      pageURLs.set(pageIndex, url);
      return url;
    } catch {
      return null;
    }
  }

  // Navigate to a specific page.
  async function goToPage(pageIndex: number) {
    const total = pages().length;
    if (total === 0) return;
    const clamped = Math.max(0, Math.min(pageIndex, total - 1));

    const token = getAccessToken();
    if (!token) return;

    // Load current page.
    const url = await loadPageURL(clamped, token);
    setCurrentPage(clamped);
    setPageInput(String(clamped + 1));
    setCurrentPageURL(url);

    // Preload next page in background.
    const nextIndex = clamped + 1;
    if (nextIndex < total) {
      loadPageURL(nextIndex, token).then((nextUrl) => {
        setNextPageURL(nextUrl);
      });
    } else {
      setNextPageURL(null);
    }

    // Also preload the page after next.
    const nextNextIndex = clamped + 2;
    if (nextNextIndex < total) {
      loadPageURL(nextNextIndex, token);
    }

    // Save progress.
    const rawFileId = searchParams.fileId;
    const fileId = Array.isArray(rawFileId) ? rawFileId[0] : rawFileId;
    if (fileId) {
      debouncedSaveProgress(Number(fileId), clamped);
    }
  }

  function goToPrevPage() {
    const page = currentPage();
    const dir = settings().readingDirection ?? "ltr";
    if (dir === "rtl") {
      if (page < pages().length - 1) goToPage(page + 1);
    } else {
      if (page > 0) goToPage(page - 1);
    }
    resetHideTimer();
  }

  function goToNextPage() {
    const page = currentPage();
    const dir = settings().readingDirection ?? "ltr";
    if (dir === "rtl") {
      if (page > 0) goToPage(page - 1);
    } else {
      if (page < pages().length - 1) goToPage(page + 1);
    }
    resetHideTimer();
  }

  function updateSetting<K extends keyof ComicReaderSettings>(
    key: K,
    value: ComicReaderSettings[K],
  ) {
    const next = { ...settings(), [key]: value };
    setSettings(next);
    saveSettings(params.id, next);
  }

  function handlePageInputChange(e: Event) {
    const val = (e.currentTarget as HTMLInputElement).value;
    setPageInput(val);
  }

  function handlePageInputCommit(e: Event) {
    e.preventDefault();
    const num = parseInt(pageInput(), 10);
    const total = pages().length;
    if (!isNaN(num) && num >= 1 && num <= total) {
      goToPage(num - 1);
    } else {
      setPageInput(String(currentPage() + 1));
    }
  }

  // Image fit style based on settings.
  const imageFitStyle = () => {
    const fit = settings().fitMode ?? "width";
    switch (fit) {
      case "width":
        return { "max-width": "100%", "max-height": "none", height: "auto" };
      case "height":
        return { "max-height": "100vh", "max-width": "none", width: "auto" };
      case "original":
        return { "max-width": "none", "max-height": "none" };
      default:
        return { "max-width": "100%", height: "auto" };
    }
  };

  onMount(async () => {
    const rawFileId = searchParams.fileId;
    const fileId = Array.isArray(rawFileId) ? rawFileId[0] : rawFileId;
    if (!fileId) {
      setError("No file specified");
      setLoading(false);
      return;
    }

    const token = getAccessToken();
    if (!token) {
      setError("Not authenticated");
      setLoading(false);
      return;
    }

    // Load settings first.
    const savedSettings = await fetchSettings(params.id);
    const mergedSettings: ComicReaderSettings = {
      ...defaultSettings,
      ...(savedSettings ?? {}),
    };
    setSettings(mergedSettings);

    // Fetch page list.
    let pageList: ComicPageInfo[];
    try {
      pageList = await fetchPages(params.id, fileId);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load comic");
      setLoading(false);
      return;
    }

    if (pageList.length === 0) {
      setError("No pages found in this comic");
      setLoading(false);
      return;
    }

    setPages(pageList);

    // Restore saved progress.
    const savedProgress = await fetchProgress(params.id);
    let startPage = 0;
    if (savedProgress?.progress?.startsWith("page:")) {
      const parsed = parseInt(savedProgress.progress.slice(5), 10);
      if (!isNaN(parsed) && parsed >= 0 && parsed < pageList.length) {
        startPage = parsed;
      }
    }

    setLoading(false);

    // Navigate to the start page (loads the image).
    await goToPage(startPage);

    // Keyboard navigation.
    function handleKeyDown(e: KeyboardEvent) {
      if (
        e.target instanceof HTMLInputElement ||
        e.target instanceof HTMLTextAreaElement
      ) {
        return;
      }
      if (e.key === "ArrowLeft") {
        goToPrevPage();
      } else if (e.key === "ArrowRight") {
        goToNextPage();
      } else if (e.key === "Escape") {
        setShowSettings(false);
      }
    }

    document.addEventListener("keydown", handleKeyDown);

    onCleanup(() => {
      document.removeEventListener("keydown", handleKeyDown);
    });
  });

  onCleanup(() => {
    clearTimeout(hideTimer);
    // Revoke all cached object URLs to free memory.
    for (const url of pageURLs.values()) {
      URL.revokeObjectURL(url);
    }
    pageURLs.clear();
  });

  // Double-page mode: show current + next page side by side.
  const isDoublePage = () => settings().displayMode === "double";
  const totalPages = () => pages().length;

  return (
    <div
      class="relative flex h-screen w-screen flex-col overflow-hidden bg-black"
      onMouseMove={resetHideTimer}
      onClick={resetHideTimer}
    >
      {/* Loading overlay */}
      <Show when={loading()}>
        <div class="absolute inset-0 z-50 flex items-center justify-center bg-slate-950">
          <div class="flex flex-col items-center gap-3 text-slate-400">
            <BookOpen class="h-10 w-10 animate-pulse text-indigo-400" />
            <p class="text-sm">Loading comic…</p>
          </div>
        </div>
      </Show>

      {/* Error overlay */}
      <Show when={error()}>
        <div class="absolute inset-0 z-50 flex items-center justify-center bg-slate-950">
          <div class="flex flex-col items-center gap-4 text-center">
            <p class="text-lg font-medium text-red-400">{error()}</p>
            <button
              onClick={() => navigate(-1)}
              class="text-sm text-slate-400 hover:text-slate-200 transition-colors"
            >
              ← Go back
            </button>
          </div>
        </div>
      </Show>

      {/* Top bar */}
      <div
        class={`absolute left-0 right-0 top-0 z-30 flex items-center justify-between px-4 py-3 transition-all duration-300 ${
          showUI()
            ? "opacity-100 translate-y-0"
            : "opacity-0 -translate-y-full"
        }`}
        style={{ background: "rgba(0,0,0,0.7)", "backdrop-filter": "blur(8px)" }}
      >
        <button
          onClick={() => navigate(-1)}
          class="flex items-center gap-2 rounded-lg px-3 py-1.5 text-sm text-slate-300 hover:bg-white/10 hover:text-white transition-colors"
        >
          <ArrowLeft class="h-4 w-4" />
          <span class="hidden sm:inline">Back</span>
        </button>

        <div class="flex min-w-0 flex-1 flex-col items-center px-4">
          <Show when={totalPages() > 0}>
            <p class="text-sm font-medium text-slate-200">
              Page {currentPage() + 1} of {totalPages()}
            </p>
          </Show>
        </div>

        <div class="flex items-center gap-1">
          <button
            onClick={() => setShowSettings((v) => !v)}
            class="rounded-lg p-2 text-slate-300 hover:bg-white/10 hover:text-white transition-colors"
            title="Reader settings"
          >
            <Settings class="h-4 w-4" />
          </button>
        </div>
      </div>

      {/* Reading area */}
      <div class="flex flex-1 items-center justify-center overflow-auto">
        <Show when={!loading() && currentPageURL()}>
          <div
            class={`flex items-center justify-center gap-0 ${
              isDoublePage() ? "flex-row" : ""
            }`}
          >
            {/* Current page */}
            <img
              src={currentPageURL()!}
              alt={`Page ${currentPage() + 1}`}
              style={imageFitStyle()}
              class="select-none"
              draggable={false}
            />
            {/* Second page in double-page mode */}
            <Show when={isDoublePage() && nextPageURL()}>
              <img
                src={nextPageURL()!}
                alt={`Page ${currentPage() + 2}`}
                style={imageFitStyle()}
                class="select-none"
                draggable={false}
              />
            </Show>
          </div>
        </Show>
      </div>

      {/* Click zones for page navigation */}
      <Show when={!loading() && !showSettings()}>
        {/* Left zone — previous page */}
        <div
          class="absolute bottom-16 left-0 top-12 z-20 w-1/4 cursor-pointer"
          onClick={(e) => {
            e.stopPropagation();
            goToPrevPage();
          }}
        />
        {/* Right zone — next page */}
        <div
          class="absolute bottom-16 right-0 top-12 z-20 w-1/4 cursor-pointer"
          onClick={(e) => {
            e.stopPropagation();
            goToNextPage();
          }}
        />
      </Show>

      {/* Bottom toolbar */}
      <div
        class={`absolute bottom-0 left-0 right-0 z-30 flex items-center justify-between px-4 py-3 transition-all duration-300 ${
          showUI()
            ? "opacity-100 translate-y-0"
            : "opacity-0 translate-y-full"
        }`}
        style={{ background: "rgba(0,0,0,0.7)", "backdrop-filter": "blur(8px)" }}
      >
        <button
          onClick={goToPrevPage}
          disabled={
            settings().readingDirection === "rtl"
              ? currentPage() >= totalPages() - 1
              : currentPage() <= 0
          }
          class="flex items-center gap-2 rounded-lg px-4 py-2 text-sm text-slate-300 hover:bg-white/10 hover:text-white transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
        >
          <ChevronLeft class="h-4 w-4" />
          <span class="hidden sm:inline">Previous</span>
        </button>

        {/* Page number input */}
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
          disabled={
            settings().readingDirection === "rtl"
              ? currentPage() <= 0
              : currentPage() >= totalPages() - 1
          }
          class="flex items-center gap-2 rounded-lg px-4 py-2 text-sm text-slate-300 hover:bg-white/10 hover:text-white transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
        >
          <span class="hidden sm:inline">Next</span>
          <ChevronRight class="h-4 w-4" />
        </button>
      </div>

      {/* Settings panel */}
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
          <h2 class="text-sm font-semibold text-slate-200">Reader Settings</h2>
          <button
            onClick={() => setShowSettings(false)}
            class="rounded-lg p-1.5 text-slate-400 hover:bg-white/10 hover:text-white transition-colors"
          >
            <X class="h-4 w-4" />
          </button>
        </div>

        <div class="flex flex-col gap-6 p-4">
          {/* Display mode */}
          <div class="flex flex-col gap-2">
            <label class="text-xs font-medium uppercase tracking-wide text-slate-500">
              Display Mode
            </label>
            <div class="flex gap-2">
              <button
                onClick={() => updateSetting("displayMode", "single")}
                class={`flex-1 rounded-lg py-2 text-sm font-medium transition-colors ${
                  settings().displayMode === "single"
                    ? "bg-indigo-600/30 text-indigo-300"
                    : "text-slate-400 hover:bg-white/10 hover:text-slate-200"
                }`}
              >
                Single
              </button>
              <button
                onClick={() => updateSetting("displayMode", "double")}
                class={`flex-1 rounded-lg py-2 text-sm font-medium transition-colors ${
                  settings().displayMode === "double"
                    ? "bg-indigo-600/30 text-indigo-300"
                    : "text-slate-400 hover:bg-white/10 hover:text-slate-200"
                }`}
              >
                Double
              </button>
            </div>
          </div>

          {/* Fit mode */}
          <div class="flex flex-col gap-2">
            <label class="text-xs font-medium uppercase tracking-wide text-slate-500">
              Fit Mode
            </label>
            <div class="flex flex-col gap-1">
              <For
                each={
                  [
                    { value: "width", label: "Fit Width" },
                    { value: "height", label: "Fit Height" },
                    { value: "original", label: "Original Size" },
                  ] as const
                }
              >
                {(opt) => (
                  <button
                    onClick={() => updateSetting("fitMode", opt.value)}
                    class={`rounded-lg px-3 py-2 text-left text-sm transition-colors ${
                      settings().fitMode === opt.value
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

          {/* Reading direction */}
          <div class="flex flex-col gap-2">
            <label class="text-xs font-medium uppercase tracking-wide text-slate-500">
              Reading Direction
            </label>
            <div class="flex gap-2">
              <button
                onClick={() => updateSetting("readingDirection", "ltr")}
                class={`flex-1 rounded-lg py-2 text-sm font-medium transition-colors ${
                  settings().readingDirection === "ltr"
                    ? "bg-indigo-600/30 text-indigo-300"
                    : "text-slate-400 hover:bg-white/10 hover:text-slate-200"
                }`}
              >
                LTR
              </button>
              <button
                onClick={() => updateSetting("readingDirection", "rtl")}
                class={`flex-1 rounded-lg py-2 text-sm font-medium transition-colors ${
                  settings().readingDirection === "rtl"
                    ? "bg-indigo-600/30 text-indigo-300"
                    : "text-slate-400 hover:bg-white/10 hover:text-slate-200"
                }`}
              >
                RTL
              </button>
            </div>
          </div>
        </div>
      </div>

      {/* Overlay to close settings when clicking outside */}
      <Show when={showSettings()}>
        <div
          class="absolute inset-0 z-[35]"
          onClick={() => setShowSettings(false)}
        />
      </Show>
    </div>
  );
};

export default ComicReader;
