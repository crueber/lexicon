import {
  type Component,
  createSignal,
  createEffect,
  createMemo,
  onMount,
  onCleanup,
  Show,
  For,
} from "solid-js";
import { useParams, useNavigate, useSearchParams } from "@solidjs/router";
import {
  ArrowLeft,
  Play,
  Pause,
  SkipBack,
  SkipForward,
  ChevronLeft,
  ChevronRight,
  List,
  X,
  Volume2,
  VolumeX,
} from "lucide-solid";
import { api, getAccessToken } from "../../shared/api/client";
import { t } from "../../shared/i18n/i18n";
import type { BookFile } from "../library/types";

// ---- Types ----

interface AudiobookSettings {
  speed?: number;
  volume?: number;
  skipInterval?: number;
}

interface ReadingProgress {
  fileId: number;
  progress: string;
  progressType: string;
}

interface AudioTrack {
  id: number;
  format: string;
  filePath: string;
  trackNumber?: number;
  trackTitle?: string;
  durationSecs?: number;
}

// ---- Constants ----

const AUDIO_FORMATS = new Set(["M4B", "M4A", "MP3", "OPUS", "FLAC"]);

const SPEED_OPTIONS = [0.75, 1, 1.25, 1.5, 2] as const;

const defaultSettings: AudiobookSettings = {
  speed: 1,
  volume: 1,
  skipInterval: 30,
};

// ---- Helpers ----

function isAudioFormat(format: string): boolean {
  return AUDIO_FORMATS.has(format.toUpperCase());
}

function formatTime(secs: number): string {
  if (!isFinite(secs) || secs < 0) return "0:00";
  const h = Math.floor(secs / 3600);
  const m = Math.floor((secs % 3600) / 60);
  const s = Math.floor(secs % 60);
  if (h > 0) {
    return `${h}:${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`;
  }
  return `${m}:${String(s).padStart(2, "0")}`;
}

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

// ---- API helpers ----

async function fetchBookFiles(bookId: string): Promise<BookFile[]> {
  return api<BookFile[]>(`/books/${bookId}/files`);
}

async function fetchBookDetail(bookId: string): Promise<{
  title?: string;
  authors: { id: number; name: string }[];
  coverPath?: string;
}> {
  return api(`/books/${bookId}`);
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
  position: number,
): Promise<void> {
  try {
    await api(`/reader/books/${bookId}/progress`, {
      method: "PUT",
      body: JSON.stringify({
        fileId,
        progress: JSON.stringify({ fileId, position }),
        progressType: "AUDIO_POSITION",
      }),
    });
  } catch {
    // Non-fatal.
  }
}

async function fetchSettings(bookId: string): Promise<AudiobookSettings | null> {
  try {
    return await api<AudiobookSettings>(
      `/reader/books/${bookId}/audiobook-settings`,
    );
  } catch {
    return null;
  }
}

async function saveSettings(
  bookId: string,
  settings: AudiobookSettings,
): Promise<void> {
  try {
    await api(`/reader/books/${bookId}/audiobook-settings`, {
      method: "PUT",
      body: JSON.stringify(settings),
    });
  } catch {
    // Non-fatal.
  }
}

// ---- AudiobookPlayer component ----

const AudiobookPlayer: Component = () => {
  const params = useParams<{ id: string }>();
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();

  // Hidden audio element ref.
  let audioRef!: HTMLAudioElement;

  // Book metadata.
  const [bookTitle, setBookTitle] = createSignal("");
  const [authors, setAuthors] = createSignal<string[]>([]);
  const [coverPath, setCoverPath] = createSignal<string | null>(null);

  // Track list.
  const [tracks, setTracks] = createSignal<AudioTrack[]>([]);
  const [currentTrackIndex, setCurrentTrackIndex] = createSignal(0);

  // Playback state.
  const [playing, setPlaying] = createSignal(false);
  const [currentTime, setCurrentTime] = createSignal(0);
  const [duration, setDuration] = createSignal(0);
  const [buffered, setBuffered] = createSignal(0);

  // Settings.
  const [speed, setSpeed] = createSignal(defaultSettings.speed!);
  const [volume, setVolume] = createSignal(defaultSettings.volume!);
  const [skipInterval, setSkipInterval] = createSignal(
    defaultSettings.skipInterval!,
  );

  // UI state.
  const [showTrackList, setShowTrackList] = createSignal(false);
  const [showSettings, setShowSettings] = createSignal(false);
  const [loading, setLoading] = createSignal(true);
  const [error, setError] = createSignal<string | null>(null);

  // Derived values.
  const currentTrack = createMemo(() => tracks()[currentTrackIndex()]);
  const progressPct = createMemo(() => {
    const d = duration();
    const t = currentTime();
    if (d <= 0) return 0;
    return (t / d) * 100;
  });

  // Debounced progress save (5 seconds after last position change).
  const debouncedSaveProgress = debounce(
    (fileId: number, position: number) => {
      saveProgress(params.id, fileId, position);
    },
    5000,
  );

  // Build the stream URL for a track, using token query param for auth.
  function streamUrl(trackId: number): string {
    const token = getAccessToken();
    const base = `/api/reader/books/${params.id}/files/${trackId}/stream`;
    return token ? `${base}?token=${encodeURIComponent(token)}` : base;
  }

  // Load a track by index, optionally seeking to a position.
  function loadTrack(index: number, seekTo?: number) {
    const track = tracks()[index];
    if (!track) return;

    setCurrentTrackIndex(index);
    setCurrentTime(0);
    setDuration(0);
    setBuffered(0);

    audioRef.src = streamUrl(track.id);
    audioRef.load();

    if (seekTo !== undefined && seekTo > 0) {
      // Seek once metadata is loaded.
      const onLoaded = () => {
        audioRef.currentTime = seekTo;
        audioRef.removeEventListener("loadedmetadata", onLoaded);
      };
      audioRef.addEventListener("loadedmetadata", onLoaded);
    }

    if (playing()) {
      audioRef.play().catch(() => setPlaying(false));
    }

    updateMediaSession(track);
  }

  function updateMediaSession(track: AudioTrack) {
    if (!("mediaSession" in navigator)) return;

    navigator.mediaSession.metadata = new MediaMetadata({
      title: track.trackTitle ?? track.filePath.split("/").pop() ?? t("common.track"),
      artist: authors().join(", "),
      album: bookTitle(),
      artwork: coverPath()
        ? [
            {
              src: `/api/books/${params.id}/cover`,
              sizes: "512x512",
              type: "image/jpeg",
            },
          ]
        : [],
    });

    navigator.mediaSession.setActionHandler("previoustrack", () => prevTrack());
    navigator.mediaSession.setActionHandler("nexttrack", () => nextTrack());
    navigator.mediaSession.setActionHandler("seekbackward", () =>
      seek(-skipInterval()),
    );
    navigator.mediaSession.setActionHandler("seekforward", () =>
      seek(skipInterval()),
    );
    navigator.mediaSession.setActionHandler("play", () => togglePlay());
    navigator.mediaSession.setActionHandler("pause", () => togglePlay());
  }

  function togglePlay() {
    if (playing()) {
      audioRef.pause();
      setPlaying(false);
    } else {
      audioRef.play().catch(() => setPlaying(false));
      setPlaying(true);
    }
  }

  function seek(deltaSecs: number) {
    const newTime = Math.max(
      0,
      Math.min(audioRef.currentTime + deltaSecs, duration()),
    );
    audioRef.currentTime = newTime;
    setCurrentTime(newTime);
  }

  function seekTo(pct: number) {
    const newTime = (pct / 100) * duration();
    audioRef.currentTime = newTime;
    setCurrentTime(newTime);
  }

  function prevTrack() {
    // If more than 3 seconds in, restart current track; otherwise go to previous.
    if (currentTime() > 3 && currentTrackIndex() > 0) {
      audioRef.currentTime = 0;
      setCurrentTime(0);
    } else if (currentTrackIndex() > 0) {
      loadTrack(currentTrackIndex() - 1);
    }
  }

  function nextTrack() {
    const nextIndex = currentTrackIndex() + 1;
    if (nextIndex < tracks().length) {
      loadTrack(nextIndex);
    }
  }

  // Apply speed to audio element whenever it changes.
  createEffect(() => {
    audioRef.playbackRate = speed();
  });

  // Apply volume to audio element whenever it changes.
  createEffect(() => {
    audioRef.volume = volume();
  });

  onMount(async () => {
    const token = getAccessToken();
    if (!token) {
      setError(t("common.notAuthenticated"));
      setLoading(false);
      return;
    }

    // Load settings.
    const savedSettings = await fetchSettings(params.id);
    if (savedSettings) {
      if (savedSettings.speed != null) setSpeed(savedSettings.speed);
      if (savedSettings.volume != null) setVolume(savedSettings.volume);
      if (savedSettings.skipInterval != null)
        setSkipInterval(savedSettings.skipInterval);
    }

    // Load book metadata and files in parallel.
    try {
      const [detail, files] = await Promise.all([
        fetchBookDetail(params.id),
        fetchBookFiles(params.id),
      ]);

      setBookTitle(detail.title ?? "");
      setAuthors(detail.authors.map((a) => a.name));
      setCoverPath(detail.coverPath ?? null);

      // Filter to audio files and sort by track number then filename.
      const audioTracks: AudioTrack[] = files
        .filter((f) => isAudioFormat(f.format))
        .sort((a, b) => {
          const tn = (a.trackNumber ?? 9999) - (b.trackNumber ?? 9999);
          if (tn !== 0) return tn;
          return a.filePath.localeCompare(b.filePath);
        })
        .map((f) => ({
          id: f.id,
          format: f.format,
          filePath: f.filePath,
          trackNumber: f.trackNumber,
          trackTitle: f.trackTitle,
          durationSecs: f.durationSecs,
        }));

      if (audioTracks.length === 0) {
        setError(t("reader.noAudioFilesFound"));
        setLoading(false);
        return;
      }

      setTracks(audioTracks);

      // Determine starting track and position.
      let startIndex = 0;
      let startPosition = 0;

      // Check if a specific fileId was requested via query param.
      const requestedFileId = searchParams.fileId
        ? Number(searchParams.fileId)
        : null;
      if (requestedFileId) {
        const idx = audioTracks.findIndex((t) => t.id === requestedFileId);
        if (idx >= 0) startIndex = idx;
      }

      // Restore saved progress.
      const savedProgress = await fetchProgress(params.id);
      if (savedProgress?.progressType === "AUDIO_POSITION" && savedProgress.progress) {
        try {
          const pos = JSON.parse(savedProgress.progress) as {
            fileId: number;
            position: number;
          };
          const idx = audioTracks.findIndex((t) => t.id === pos.fileId);
          if (idx >= 0) {
            startIndex = idx;
            startPosition = pos.position ?? 0;
          }
        } catch {
          // Ignore malformed progress.
        }
      }

      // Set up audio element event listeners.
      audioRef.addEventListener("timeupdate", () => {
        const t = audioRef.currentTime;
        setCurrentTime(t);

        // Update buffered amount.
        if (audioRef.buffered.length > 0) {
          setBuffered(audioRef.buffered.end(audioRef.buffered.length - 1));
        }

        // Save progress (debounced).
        const track = tracks()[currentTrackIndex()];
        if (track) {
          debouncedSaveProgress(track.id, t);
        }
      });

      audioRef.addEventListener("durationchange", () => {
        setDuration(audioRef.duration);
      });

      audioRef.addEventListener("ended", () => {
        const nextIndex = currentTrackIndex() + 1;
        if (nextIndex < tracks().length) {
          loadTrack(nextIndex);
        } else {
          setPlaying(false);
        }
      });

      audioRef.addEventListener("play", () => setPlaying(true));
      audioRef.addEventListener("pause", () => setPlaying(false));

      audioRef.addEventListener("error", () => {
        setError(t("reader.failedToLoadAudiobook"));
        setPlaying(false);
      });

      // Load the starting track.
      setCurrentTrackIndex(startIndex);
      audioRef.src = streamUrl(audioTracks[startIndex].id);
      audioRef.load();

      if (startPosition > 0) {
        const onLoaded = () => {
          audioRef.currentTime = startPosition;
          audioRef.removeEventListener("loadedmetadata", onLoaded);
        };
        audioRef.addEventListener("loadedmetadata", onLoaded);
      }

      updateMediaSession(audioTracks[startIndex]);
      setLoading(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("reader.failedToLoadAudiobook"));
      setLoading(false);
    }

    // Keyboard shortcuts.
    function handleKeyDown(e: KeyboardEvent) {
      if (e.target instanceof HTMLInputElement) return;
      switch (e.key) {
        case " ":
          e.preventDefault();
          togglePlay();
          break;
        case "ArrowLeft":
          seek(-skipInterval());
          break;
        case "ArrowRight":
          seek(skipInterval());
          break;
        case "Escape":
          setShowTrackList(false);
          setShowSettings(false);
          break;
      }
    }

    document.addEventListener("keydown", handleKeyDown);
    onCleanup(() => document.removeEventListener("keydown", handleKeyDown));
  });

  onCleanup(() => {
    if (audioRef) {
      audioRef.pause();
      audioRef.src = "";
    }
    if ("mediaSession" in navigator) {
      navigator.mediaSession.metadata = null;
    }
  });

  function updateSetting<K extends keyof AudiobookSettings>(
    key: K,
    value: AudiobookSettings[K],
  ) {
    const next: AudiobookSettings = {
      speed: speed(),
      volume: volume(),
      skipInterval: skipInterval(),
      [key]: value,
    };
    if (key === "speed") setSpeed(value as number);
    if (key === "volume") setVolume(value as number);
    if (key === "skipInterval") setSkipInterval(value as number);
    saveSettings(params.id, next);
  }

  const trackDisplayName = createMemo(() => {
    const track = currentTrack();
    if (!track) return "";
    if (track.trackTitle) return track.trackTitle;
    const filename = track.filePath.split("/").pop() ?? "";
    // Strip extension.
    return filename.replace(/\.[^.]+$/, "");
  });

  return (
    <div class="flex h-screen w-screen flex-col bg-slate-950 text-slate-100">
      {/* Hidden audio element */}
      <audio ref={audioRef} preload="metadata" />

      {/* Loading overlay */}
      <Show when={loading()}>
        <div class="absolute inset-0 z-50 flex items-center justify-center bg-slate-950">
          <div class="flex flex-col items-center gap-3 text-slate-400">
            <div class="h-10 w-10 animate-spin rounded-full border-2 border-slate-700 border-t-indigo-400" />
            <p class="text-sm">{t("reader.loadingAudiobook")}</p>
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
              {t("common.goBack")}
            </button>
          </div>
        </div>
      </Show>

      {/* Top bar */}
      <div
        class="flex items-center justify-between border-b border-slate-800 px-4 py-3"
        style={{ background: "rgba(15,15,30,0.95)" }}
      >
        <button
          onClick={() => navigate(-1)}
          class="flex items-center gap-2 rounded-lg px-3 py-1.5 text-sm text-slate-300 hover:bg-white/10 hover:text-white transition-colors"
        >
          <ArrowLeft class="h-4 w-4" />
          <span class="hidden sm:inline">{t("common.back")}</span>
        </button>

        <div class="flex min-w-0 flex-1 flex-col items-center px-4">
          <Show when={bookTitle()}>
            <p class="truncate text-sm font-semibold text-slate-200">
              {bookTitle()}
            </p>
          </Show>
          <Show when={authors().length > 0}>
            <p class="truncate text-xs text-slate-400">
              {authors().join(", ")}
            </p>
          </Show>
        </div>

        <div class="flex items-center gap-1">
          <button
            onClick={() => {
              setShowTrackList((v) => !v);
              setShowSettings(false);
            }}
            class="rounded-lg p-2 text-slate-300 hover:bg-white/10 hover:text-white transition-colors"
            title={t("reader.trackList")}
          >
            <List class="h-4 w-4" />
          </button>
        </div>
      </div>

      {/* Main content */}
      <div class="flex flex-1 flex-col items-center justify-center gap-6 overflow-hidden px-6 py-8">
        {/* Cover art */}
        <div class="relative h-48 w-48 shrink-0 overflow-hidden rounded-2xl shadow-2xl sm:h-64 sm:w-64">
          <Show
            when={coverPath()}
            fallback={
              <div class="flex h-full w-full items-center justify-center bg-slate-800">
                <span class="text-6xl">🎧</span>
              </div>
            }
          >
            <img
              src={`/api/books/${params.id}/cover`}
              alt={bookTitle()}
              class="h-full w-full object-cover"
            />
          </Show>
        </div>

        {/* Track info */}
        <div class="flex w-full max-w-md flex-col items-center gap-1 text-center">
          <p class="text-lg font-semibold text-slate-100 line-clamp-2">
            {trackDisplayName()}
          </p>
          <Show when={tracks().length > 1}>
            <p class="text-sm text-slate-400">
              {t("common.track")} {currentTrackIndex() + 1} {t("common.of")} {tracks().length}
            </p>
          </Show>
        </div>

        {/* Progress bar */}
        <div class="flex w-full max-w-md flex-col gap-1">
          <div
            class="relative h-2 w-full cursor-pointer overflow-hidden rounded-full bg-slate-700"
            onClick={(e) => {
              const rect = e.currentTarget.getBoundingClientRect();
              const pct = ((e.clientX - rect.left) / rect.width) * 100;
              seekTo(Math.max(0, Math.min(100, pct)));
            }}
          >
            {/* Buffered */}
            <div
              class="absolute left-0 top-0 h-full rounded-full bg-slate-600 transition-all"
              style={{
                width: `${duration() > 0 ? (buffered() / duration()) * 100 : 0}%`,
              }}
            />
            {/* Played */}
            <div
              class="absolute left-0 top-0 h-full rounded-full bg-indigo-500 transition-all"
              style={{ width: `${progressPct()}%` }}
            />
          </div>
          <div class="flex justify-between text-xs text-slate-400">
            <span>{formatTime(currentTime())}</span>
            <span>{formatTime(duration())}</span>
          </div>
        </div>

        {/* Controls */}
        <div class="flex w-full max-w-md items-center justify-center gap-4">
          {/* Previous track */}
          <button
            onClick={prevTrack}
            disabled={currentTrackIndex() === 0 && currentTime() <= 3}
            class="rounded-full p-2 text-slate-400 hover:text-slate-200 disabled:opacity-30 transition-colors"
            title={t("reader.previousTrack")}
          >
            <SkipBack class="h-6 w-6" />
          </button>

          {/* Skip back */}
          <button
            onClick={() => seek(-skipInterval())}
            class="flex flex-col items-center rounded-full p-2 text-slate-400 hover:text-slate-200 transition-colors"
            title={`${t("common.back")} ${skipInterval()}s`}
          >
            <ChevronLeft class="h-5 w-5" />
            <span class="text-[10px]">{skipInterval()}s</span>
          </button>

          {/* Play/Pause */}
          <button
            onClick={togglePlay}
            class="flex h-16 w-16 items-center justify-center rounded-full bg-indigo-600 text-white shadow-lg hover:bg-indigo-500 active:scale-95 transition-all"
            title={playing() ? t("reader.pause") : t("reader.play")}
          >
            <Show when={playing()} fallback={<Play class="h-7 w-7 translate-x-0.5" />}>
              <Pause class="h-7 w-7" />
            </Show>
          </button>

          {/* Skip forward */}
          <button
            onClick={() => seek(skipInterval())}
            class="flex flex-col items-center rounded-full p-2 text-slate-400 hover:text-slate-200 transition-colors"
            title={`${t("common.next")} ${skipInterval()}s`}
          >
            <ChevronRight class="h-5 w-5" />
            <span class="text-[10px]">{skipInterval()}s</span>
          </button>

          {/* Next track */}
          <button
            onClick={nextTrack}
            disabled={currentTrackIndex() >= tracks().length - 1}
            class="rounded-full p-2 text-slate-400 hover:text-slate-200 disabled:opacity-30 transition-colors"
            title={t("reader.nextTrack")}
          >
            <SkipForward class="h-6 w-6" />
          </button>
        </div>

        {/* Volume + Speed row */}
        <div class="flex w-full max-w-md items-center gap-4">
          {/* Volume */}
          <div class="flex flex-1 items-center gap-2">
            <button
              onClick={() => updateSetting("volume", volume() > 0 ? 0 : 1)}
              class="text-slate-400 hover:text-slate-200 transition-colors"
              title={volume() === 0 ? t("reader.unmute") : t("reader.mute")}
            >
              <Show when={volume() > 0} fallback={<VolumeX class="h-4 w-4" />}>
                <Volume2 class="h-4 w-4" />
              </Show>
            </button>
            <input
              type="range"
              min="0"
              max="1"
              step="0.05"
              value={volume()}
              onInput={(e) =>
                updateSetting("volume", Number(e.currentTarget.value))
              }
              class="flex-1 accent-indigo-400"
              title={t("reader.volume")}
            />
          </div>

          {/* Speed */}
          <div class="flex items-center gap-1">
            <For each={SPEED_OPTIONS}>
              {(s) => (
                <button
                  onClick={() => updateSetting("speed", s)}
                  class={`rounded px-2 py-1 text-xs font-medium transition-colors ${
                    speed() === s
                      ? "bg-indigo-600/40 text-indigo-300"
                      : "text-slate-400 hover:bg-white/10 hover:text-slate-200"
                  }`}
                >
                  {s}×
                </button>
              )}
            </For>
          </div>
        </div>
      </div>

      {/* Track list sidebar */}
      <div
        class={`absolute bottom-0 left-0 top-0 z-40 w-80 overflow-y-auto transition-transform duration-300 ${
          showTrackList() ? "translate-x-0" : "-translate-x-full"
        }`}
        style={{
          background: "rgba(15,15,30,0.97)",
          "backdrop-filter": "blur(12px)",
        }}
      >
        <div class="flex items-center justify-between border-b border-white/10 px-4 py-3">
          <h2 class="text-sm font-semibold text-slate-200">{t("common.tracks")}</h2>
          <button
            onClick={() => setShowTrackList(false)}
            class="rounded-lg p-1.5 text-slate-400 hover:bg-white/10 hover:text-white transition-colors"
          >
            <X class="h-4 w-4" />
          </button>
        </div>
        <nav class="p-2">
          <For
            each={tracks()}
            fallback={
              <p class="px-2 py-4 text-sm text-slate-500">{t("reader.noTracksFound")}</p>
            }
          >
            {(track, index) => (
              <button
                onClick={() => {
                  loadTrack(index());
                  setShowTrackList(false);
                }}
                class={`flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-left transition-colors ${
                  index() === currentTrackIndex()
                    ? "bg-indigo-600/20 text-indigo-300"
                    : "text-slate-300 hover:bg-white/10 hover:text-white"
                }`}
              >
                <span class="w-6 shrink-0 text-center text-xs text-slate-500">
                  {track.trackNumber ?? index() + 1}
                </span>
                <div class="min-w-0 flex-1">
                  <p class="truncate text-sm">
                    {track.trackTitle ??
                      track.filePath.split("/").pop()?.replace(/\.[^.]+$/, "") ??
                      `${t("common.track")} ${index() + 1}`}
                  </p>
                  <Show when={track.durationSecs}>
                    <p class="text-xs text-slate-500">
                      {formatTime(track.durationSecs!)}
                    </p>
                  </Show>
                </div>
                <Show when={index() === currentTrackIndex() && playing()}>
                  <div class="flex gap-0.5">
                    <span class="h-3 w-0.5 animate-bounce bg-indigo-400" style={{ "animation-delay": "0ms" }} />
                    <span class="h-3 w-0.5 animate-bounce bg-indigo-400" style={{ "animation-delay": "150ms" }} />
                    <span class="h-3 w-0.5 animate-bounce bg-indigo-400" style={{ "animation-delay": "300ms" }} />
                  </div>
                </Show>
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
        style={{
          background: "rgba(15,15,30,0.97)",
          "backdrop-filter": "blur(12px)",
        }}
      >
        <div class="flex items-center justify-between border-b border-white/10 px-4 py-3">
          <h2 class="text-sm font-semibold text-slate-200">{t("common.settingsTitleShort")}</h2>
          <button
            onClick={() => setShowSettings(false)}
            class="rounded-lg p-1.5 text-slate-400 hover:bg-white/10 hover:text-white transition-colors"
          >
            <X class="h-4 w-4" />
          </button>
        </div>

        <div class="flex flex-col gap-6 p-4">
          {/* Skip interval */}
          <div class="flex flex-col gap-2">
            <label class="text-xs font-medium uppercase tracking-wide text-slate-500">
              {t("reader.skipInterval")}: {skipInterval()}s
            </label>
            <div class="flex gap-2">
              <For each={[10, 15, 30, 45, 60] as const}>
                {(s) => (
                  <button
                    onClick={() => updateSetting("skipInterval", s)}
                    class={`flex-1 rounded-lg py-2 text-sm font-medium transition-colors ${
                      skipInterval() === s
                        ? "bg-indigo-600/30 text-indigo-300"
                        : "text-slate-400 hover:bg-white/10 hover:text-slate-200"
                    }`}
                  >
                    {s}s
                  </button>
                )}
              </For>
            </div>
          </div>
        </div>
      </div>

      {/* Overlay to close panels */}
      <Show when={showTrackList() || showSettings()}>
        <div
          class="absolute inset-0 z-[35]"
          onClick={() => {
            setShowTrackList(false);
            setShowSettings(false);
          }}
        />
      </Show>
    </div>
  );
};

export default AudiobookPlayer;
