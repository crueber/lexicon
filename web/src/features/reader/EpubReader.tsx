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
  List,
  X,
  Sun,
  Moon,
  BookOpen,
} from "lucide-solid";
import { api, getAccessToken } from "../../shared/api/client";

// ---- Types ----

interface EpubReaderSettings {
  fontFamily?: "serif" | "sans-serif" | "monospace";
  fontSize?: number;
  lineHeight?: number;
  margins?: "small" | "medium" | "large";
  theme?: "light" | "sepia" | "dark";
  flow?: "paginated" | "scrolled";
}

interface ReadingProgress {
  fileId: number;
  progress: string;
  progressType: string;
}

interface TocItem {
  id: string;
  href: string;
  label: string;
  subitems?: TocItem[];
}

// ---- Default settings ----

const defaultSettings: EpubReaderSettings = {
  fontFamily: "serif",
  fontSize: 18,
  lineHeight: 1.6,
  margins: "medium",
  theme: "dark",
  flow: "paginated",
};

// ---- Theme styles ----

const themeStyles: Record<
  NonNullable<EpubReaderSettings["theme"]>,
  { bg: string; fg: string; containerBg: string }
> = {
  light: {
    bg: "#ffffff",
    fg: "#1a1a1a",
    containerBg: "bg-white",
  },
  sepia: {
    bg: "#f4ecd8",
    fg: "#3b2f1e",
    containerBg: "bg-amber-50",
  },
  dark: {
    bg: "#1a1a2e",
    fg: "#e0e0e0",
    containerBg: "bg-slate-900",
  },
};

// ---- Margin sizes ----

const marginSizes: Record<NonNullable<EpubReaderSettings["margins"]>, string> =
  {
    small: "2%",
    medium: "8%",
    large: "16%",
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
  progress: string,
  progressType: string,
): Promise<void> {
  try {
    await api(`/reader/books/${bookId}/progress`, {
      method: "PUT",
      body: JSON.stringify({ fileId, progress, progressType }),
    });
  } catch {
    // Non-fatal: progress save failure should not interrupt reading.
  }
}

async function fetchSettings(
  bookId: string,
): Promise<EpubReaderSettings | null> {
  try {
    return await api<EpubReaderSettings>(`/reader/books/${bookId}/settings`);
  } catch {
    return null;
  }
}

async function saveSettings(
  bookId: string,
  settings: EpubReaderSettings,
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

// ---- EpubReader component ----

const EpubReader: Component = () => {
  const params = useParams<{ id: string }>();
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();

  // DOM ref for epubjs to render into.
  let containerRef!: HTMLDivElement;

  // UI state.
  const [showUI, setShowUI] = createSignal(true);
  const [showSettings, setShowSettings] = createSignal(false);
  const [showToc, setShowToc] = createSignal(false);
  const [bookTitle, setBookTitle] = createSignal("");
  const [chapterTitle, setChapterTitle] = createSignal("");
  const [progressPct, setProgressPct] = createSignal(0);
  const [toc, setToc] = createSignal<TocItem[]>([]);
  const [loading, setLoading] = createSignal(true);
  const [error, setError] = createSignal<string | null>(null);

  // Reader settings.
  const [settings, setSettings] = createSignal<EpubReaderSettings>(defaultSettings);

  // epubjs instances — stored in a plain object to avoid reactivity wrapping.
  const epub: { book: any; rendition: any } = { book: null, rendition: null };

  // Auto-hide UI after 3 seconds of inactivity.
  let hideTimer: ReturnType<typeof setTimeout> | undefined;

  function resetHideTimer() {
    setShowUI(true);
    clearTimeout(hideTimer);
    hideTimer = setTimeout(() => {
      if (!showSettings() && !showToc()) {
        setShowUI(false);
      }
    }, 3000);
  }

  // Debounced progress save (2 seconds after last location change).
  const debouncedSaveProgress = debounce(
    (fileId: number, cfi: string) => {
      saveProgress(params.id, fileId, cfi, "CFI");
    },
    2000,
  );

  // Apply settings to the epubjs rendition.
  function applySettings(s: EpubReaderSettings) {
    if (!epub.rendition) return;

    const theme = themeStyles[s.theme ?? "dark"];
    const margin = marginSizes[s.margins ?? "medium"];
    const fontFamily =
      s.fontFamily === "serif"
        ? "Georgia, 'Times New Roman', serif"
        : s.fontFamily === "monospace"
          ? "'Courier New', Courier, monospace"
          : "system-ui, -apple-system, sans-serif";

    epub.rendition.themes.default({
      body: {
        background: theme.bg,
        color: theme.fg,
        "font-family": fontFamily,
        "font-size": `${s.fontSize ?? 18}px`,
        "line-height": String(s.lineHeight ?? 1.6),
        "padding-left": margin,
        "padding-right": margin,
      },
      a: { color: "inherit" },
    });

    epub.rendition.themes.select("default");
  }

  onMount(async () => {
    const fileId = searchParams.fileId;
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
    const mergedSettings: EpubReaderSettings = {
      ...defaultSettings,
      ...(savedSettings ?? {}),
    };
    setSettings(mergedSettings);

    // Dynamically import epubjs to avoid SSR issues.
    const { default: ePub } = await import("epubjs");

    const streamUrl = `/api/reader/books/${params.id}/files/${fileId}/stream`;

    try {
      epub.book = ePub(streamUrl, {
        requestHeaders: { Authorization: `Bearer ${token}` },
      });

      epub.rendition = epub.book.renderTo(containerRef, {
        width: "100%",
        height: "100%",
        flow: mergedSettings.flow ?? "paginated",
        spread: "none",
      });

      // Apply initial theme/settings.
      applySettings(mergedSettings);

      // Load book metadata and TOC after the book is ready.
      epub.book.ready.then(() => {
        const meta = epub.book.package?.metadata;
        if (meta?.title) {
          setBookTitle(meta.title);
        }
      });

      // Load TOC — navigation is a promise in epubjs.
      epub.book.loaded.navigation.then((nav: any) => {
        if (nav?.toc) {
          setToc(nav.toc as TocItem[]);
        }
      });

      // Track location changes for progress and chapter title.
      epub.rendition.on("relocated", (location: any) => {
        const cfi = location?.start?.cfi;
        if (cfi) {
          debouncedSaveProgress(Number(fileId), cfi);
        }

        // Update progress percentage.
        const pct = epub.book.locations?.percentageFromCfi?.(cfi);
        if (typeof pct === "number") {
          setProgressPct(Math.round(pct * 100));
        }

        // Update chapter title from TOC.
        const href = location?.start?.href;
        if (href) {
          const chapter = epub.book.navigation?.get?.(href);
          if (chapter?.label) {
            setChapterTitle(chapter.label.trim());
          }
        }
      });

      // Restore saved progress.
      const savedProgress = await fetchProgress(params.id);
      if (savedProgress?.progress) {
        await epub.rendition.display(savedProgress.progress);
      } else {
        await epub.rendition.display();
      }

      setLoading(false);
    } catch (err) {
      setError(
        err instanceof Error ? err.message : "Failed to load EPUB",
      );
      setLoading(false);
    }

    // Keyboard navigation.
    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === "ArrowLeft") {
        epub.rendition?.prev();
        resetHideTimer();
      } else if (e.key === "ArrowRight") {
        epub.rendition?.next();
        resetHideTimer();
      } else if (e.key === "Escape") {
        setShowSettings(false);
        setShowToc(false);
      }
    }

    document.addEventListener("keydown", handleKeyDown);

    onCleanup(() => {
      document.removeEventListener("keydown", handleKeyDown);
    });
  });

  onCleanup(() => {
    clearTimeout(hideTimer);
    if (epub.rendition) {
      epub.rendition.destroy();
    }
    if (epub.book) {
      epub.book.destroy();
    }
  });

  // Apply settings whenever they change.
  createEffect(() => {
    applySettings(settings());
  });

  function prevPage() {
    epub.rendition?.prev();
    resetHideTimer();
  }

  function nextPage() {
    epub.rendition?.next();
    resetHideTimer();
  }

  function navigateToTocItem(href: string) {
    epub.rendition?.display(href);
    setShowToc(false);
  }

  function updateSetting<K extends keyof EpubReaderSettings>(
    key: K,
    value: EpubReaderSettings[K],
  ) {
    const next = { ...settings(), [key]: value };
    setSettings(next);
    saveSettings(params.id, next);

    // If flow changes, we need to re-render.
    if (key === "flow" && epub.rendition) {
      epub.rendition.flow(value as string);
    }
  }

  const currentTheme = () => themeStyles[settings().theme ?? "dark"];

  return (
    <div
      class="relative flex h-screen w-screen flex-col overflow-hidden"
      style={{ background: currentTheme().bg }}
      onMouseMove={resetHideTimer}
      onClick={resetHideTimer}
    >
      {/* Loading overlay */}
      <Show when={loading()}>
        <div class="absolute inset-0 z-50 flex items-center justify-center bg-slate-950">
          <div class="flex flex-col items-center gap-3 text-slate-400">
            <BookOpen class="h-10 w-10 animate-pulse text-indigo-400" />
            <p class="text-sm">Loading book…</p>
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
          showUI() ? "opacity-100 translate-y-0" : "opacity-0 -translate-y-full"
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
          <Show when={bookTitle()}>
            <p class="truncate text-sm font-medium text-slate-200">
              {bookTitle()}
            </p>
          </Show>
          <Show when={chapterTitle()}>
            <p class="truncate text-xs text-slate-400">{chapterTitle()}</p>
          </Show>
        </div>

        <div class="flex items-center gap-1">
          <Show when={progressPct() > 0}>
            <span class="mr-2 text-xs text-slate-400">{progressPct()}%</span>
          </Show>
          <button
            onClick={() => {
              setShowToc((v) => !v);
              setShowSettings(false);
            }}
            class="rounded-lg p-2 text-slate-300 hover:bg-white/10 hover:text-white transition-colors"
            title="Table of contents"
          >
            <List class="h-4 w-4" />
          </button>
          <button
            onClick={() => {
              setShowSettings((v) => !v);
              setShowToc(false);
            }}
            class="rounded-lg p-2 text-slate-300 hover:bg-white/10 hover:text-white transition-colors"
            title="Reader settings"
          >
            <Settings class="h-4 w-4" />
          </button>
        </div>
      </div>

      {/* EPUB render area */}
      <div ref={containerRef} class="flex-1" />

      {/* Bottom toolbar */}
      <div
        class={`absolute bottom-0 left-0 right-0 z-30 flex items-center justify-between px-4 py-3 transition-all duration-300 ${
          showUI() ? "opacity-100 translate-y-0" : "opacity-0 translate-y-full"
        }`}
        style={{ background: "rgba(0,0,0,0.7)", "backdrop-filter": "blur(8px)" }}
      >
        <button
          onClick={prevPage}
          class="flex items-center gap-2 rounded-lg px-4 py-2 text-sm text-slate-300 hover:bg-white/10 hover:text-white transition-colors"
        >
          <ChevronLeft class="h-4 w-4" />
          <span class="hidden sm:inline">Previous</span>
        </button>

        <div class="flex items-center gap-2">
          <Show when={progressPct() > 0}>
            <div class="h-1 w-32 overflow-hidden rounded-full bg-white/20">
              <div
                class="h-full rounded-full bg-indigo-400 transition-all"
                style={{ width: `${progressPct()}%` }}
              />
            </div>
          </Show>
        </div>

        <button
          onClick={nextPage}
          class="flex items-center gap-2 rounded-lg px-4 py-2 text-sm text-slate-300 hover:bg-white/10 hover:text-white transition-colors"
        >
          <span class="hidden sm:inline">Next</span>
          <ChevronRight class="h-4 w-4" />
        </button>
      </div>

      {/* TOC sidebar */}
      <div
        class={`absolute bottom-0 left-0 top-0 z-40 w-72 overflow-y-auto transition-transform duration-300 ${
          showToc() ? "translate-x-0" : "-translate-x-full"
        }`}
        style={{ background: "rgba(15,15,30,0.95)", "backdrop-filter": "blur(12px)" }}
      >
        <div class="flex items-center justify-between border-b border-white/10 px-4 py-3">
          <h2 class="text-sm font-semibold text-slate-200">Contents</h2>
          <button
            onClick={() => setShowToc(false)}
            class="rounded-lg p-1.5 text-slate-400 hover:bg-white/10 hover:text-white transition-colors"
          >
            <X class="h-4 w-4" />
          </button>
        </div>
        <nav class="p-2">
          <For each={toc()} fallback={<p class="px-2 py-4 text-sm text-slate-500">No chapters found</p>}>
            {(item) => (
              <button
                onClick={() => navigateToTocItem(item.href)}
                class="w-full rounded-lg px-3 py-2 text-left text-sm text-slate-300 hover:bg-white/10 hover:text-white transition-colors"
              >
                {item.label}
              </button>
            )}
          </For>
        </nav>
      </div>

      {/* Settings panel */}
      <div
        class={`absolute bottom-0 right-0 top-0 z-40 w-72 overflow-y-auto transition-transform duration-300 ${
          showSettings() ? "translate-x-0" : "translate-x-full"
        }`}
        style={{ background: "rgba(15,15,30,0.95)", "backdrop-filter": "blur(12px)" }}
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
          {/* Theme */}
          <div class="flex flex-col gap-2">
            <label class="text-xs font-medium uppercase tracking-wide text-slate-500">
              Theme
            </label>
            <div class="flex gap-2">
              <ThemeButton
                label="Light"
                icon={<Sun class="h-4 w-4" />}
                active={settings().theme === "light"}
                onClick={() => updateSetting("theme", "light")}
              />
              <ThemeButton
                label="Sepia"
                icon={<BookOpen class="h-4 w-4" />}
                active={settings().theme === "sepia"}
                onClick={() => updateSetting("theme", "sepia")}
              />
              <ThemeButton
                label="Dark"
                icon={<Moon class="h-4 w-4" />}
                active={settings().theme === "dark"}
                onClick={() => updateSetting("theme", "dark")}
              />
            </div>
          </div>

          {/* Font family */}
          <div class="flex flex-col gap-2">
            <label class="text-xs font-medium uppercase tracking-wide text-slate-500">
              Font
            </label>
            <div class="flex flex-col gap-1">
              <For
                each={
                  [
                    { value: "serif", label: "Serif" },
                    { value: "sans-serif", label: "Sans-serif" },
                    { value: "monospace", label: "Monospace" },
                  ] as const
                }
              >
                {(opt) => (
                  <button
                    onClick={() => updateSetting("fontFamily", opt.value)}
                    class={`rounded-lg px-3 py-2 text-left text-sm transition-colors ${
                      settings().fontFamily === opt.value
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

          {/* Font size */}
          <div class="flex flex-col gap-2">
            <label class="text-xs font-medium uppercase tracking-wide text-slate-500">
              Font Size: {settings().fontSize}px
            </label>
            <input
              type="range"
              min="12"
              max="28"
              step="1"
              value={settings().fontSize ?? 18}
              onInput={(e) =>
                updateSetting("fontSize", Number(e.currentTarget.value))
              }
              class="w-full accent-indigo-400"
            />
          </div>

          {/* Line height */}
          <div class="flex flex-col gap-2">
            <label class="text-xs font-medium uppercase tracking-wide text-slate-500">
              Line Height: {settings().lineHeight?.toFixed(1)}
            </label>
            <input
              type="range"
              min="1.2"
              max="2.0"
              step="0.1"
              value={settings().lineHeight ?? 1.6}
              onInput={(e) =>
                updateSetting("lineHeight", Number(e.currentTarget.value))
              }
              class="w-full accent-indigo-400"
            />
          </div>

          {/* Margins */}
          <div class="flex flex-col gap-2">
            <label class="text-xs font-medium uppercase tracking-wide text-slate-500">
              Margins
            </label>
            <div class="flex gap-2">
              <For
                each={
                  [
                    { value: "small", label: "S" },
                    { value: "medium", label: "M" },
                    { value: "large", label: "L" },
                  ] as const
                }
              >
                {(opt) => (
                  <button
                    onClick={() => updateSetting("margins", opt.value)}
                    class={`flex-1 rounded-lg py-2 text-sm font-medium transition-colors ${
                      settings().margins === opt.value
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

          {/* Flow */}
          <div class="flex flex-col gap-2">
            <label class="text-xs font-medium uppercase tracking-wide text-slate-500">
              Layout
            </label>
            <div class="flex gap-2">
              <button
                onClick={() => updateSetting("flow", "paginated")}
                class={`flex-1 rounded-lg py-2 text-sm font-medium transition-colors ${
                  settings().flow === "paginated"
                    ? "bg-indigo-600/30 text-indigo-300"
                    : "text-slate-400 hover:bg-white/10 hover:text-slate-200"
                }`}
              >
                Paginated
              </button>
              <button
                onClick={() => updateSetting("flow", "scrolled")}
                class={`flex-1 rounded-lg py-2 text-sm font-medium transition-colors ${
                  settings().flow === "scrolled"
                    ? "bg-indigo-600/30 text-indigo-300"
                    : "text-slate-400 hover:bg-white/10 hover:text-slate-200"
                }`}
              >
                Scrolled
              </button>
            </div>
          </div>
        </div>
      </div>

      {/* Overlay to close panels when clicking outside */}
      <Show when={showToc() || showSettings()}>
        <div
          class="absolute inset-0 z-[35]"
          onClick={() => {
            setShowToc(false);
            setShowSettings(false);
          }}
        />
      </Show>
    </div>
  );
};

// ---- ThemeButton sub-component ----

const ThemeButton: Component<{
  label: string;
  icon: any;
  active: boolean;
  onClick: () => void;
}> = (props) => (
  <button
    onClick={props.onClick}
    class={`flex flex-1 flex-col items-center gap-1 rounded-lg py-2 text-xs font-medium transition-colors ${
      props.active
        ? "bg-indigo-600/30 text-indigo-300"
        : "text-slate-400 hover:bg-white/10 hover:text-slate-200"
    }`}
  >
    {props.icon}
    {props.label}
  </button>
);

export default EpubReader;
