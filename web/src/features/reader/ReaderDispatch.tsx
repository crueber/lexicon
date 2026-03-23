import { type Component, createResource, Show } from "solid-js";
import { useParams, useNavigate, Navigate } from "@solidjs/router";
import { Loader2 } from "lucide-solid";
import { api } from "../../shared/api/client";
import type { BookFile } from "../library/types";

// Fetch the list of files for a book.
async function fetchBookFiles(bookId: string): Promise<BookFile[]> {
  return api<BookFile[]>(`/books/${bookId}/files`);
}

const AUDIO_FORMATS = new Set(["M4B", "M4A", "MP3", "OPUS", "FLAC"]);

// Determine the best file to read from the list.
// Preference order: EPUB > PDF > CBZ > CBR > CB7 > audio > anything else.
function pickBestFile(files: BookFile[]): BookFile | undefined {
  const epub = files.find((f) => f.format === "EPUB");
  if (epub) return epub;
  const pdf = files.find((f) => f.format === "PDF");
  if (pdf) return pdf;
  const cbz = files.find((f) => f.format === "CBZ");
  if (cbz) return cbz;
  const cbr = files.find((f) => f.format === "CBR");
  if (cbr) return cbr;
  const cb7 = files.find((f) => f.format === "CB7");
  if (cb7) return cb7;
  // Audio formats: pick the first track (lowest track number or first by path).
  const audio = files
    .filter((f) => AUDIO_FORMATS.has(f.format.toUpperCase()))
    .sort((a, b) => {
      const tn = (a.trackNumber ?? 9999) - (b.trackNumber ?? 9999);
      if (tn !== 0) return tn;
      return a.filePath.localeCompare(b.filePath);
    })[0];
  if (audio) return audio;
  return files[0];
}

// Map a file format to the reader route segment.
function readerRouteForFormat(format: string): string {
  switch (format.toUpperCase()) {
    case "PDF":
      return "pdf";
    case "CBZ":
    case "CBR":
    case "CB7":
      return "comic";
    case "M4B":
    case "M4A":
    case "MP3":
    case "OPUS":
    case "FLAC":
      return "audio";
    default:
      return "epub";
  }
}

// ReaderDispatch fetches the book's files and redirects to the appropriate reader.
const ReaderDispatch: Component = () => {
  const params = useParams<{ id: string }>();
  const navigate = useNavigate();

  const [files] = createResource(() => params.id, fetchBookFiles);

  // Compute the redirect target once files are loaded.
  const redirectTarget = () => {
    const f = files();
    if (!f) return null;
    const best = pickBestFile(f);
    if (!best) return null;
    const readerType = readerRouteForFormat(best.format);
    return `/books/${params.id}/read/${readerType}?fileId=${best.id}`;
  };

  return (
    <div class="flex min-h-screen items-center justify-center bg-slate-950">
      <Show
        when={!files.loading && !files.error}
        fallback={
          <Show
            when={files.error}
            fallback={
              <div class="flex flex-col items-center gap-3 text-slate-400">
                <Loader2 class="h-8 w-8 animate-spin text-indigo-400" />
                <p class="text-sm">Loading book…</p>
              </div>
            }
          >
            <div class="flex flex-col items-center gap-3 text-center">
              <p class="text-lg font-medium text-red-400">Failed to load book</p>
              <button
                onClick={() => navigate(-1)}
                class="text-sm text-slate-400 hover:text-slate-200 transition-colors"
              >
                ← Go back
              </button>
            </div>
          </Show>
        }
      >
        <Show
          when={redirectTarget()}
          fallback={
            <div class="flex flex-col items-center gap-3 text-center">
              <p class="text-lg font-medium text-slate-300">No readable files found</p>
              <button
                onClick={() => navigate(-1)}
                class="text-sm text-slate-400 hover:text-slate-200 transition-colors"
              >
                ← Go back
              </button>
            </div>
          }
        >
          {(target) => <Navigate href={target()} />}
        </Show>
      </Show>
    </div>
  );
};

export default ReaderDispatch;
