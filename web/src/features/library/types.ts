export interface Library {
  id: number;
  name: string;
  icon?: string;
  iconColor?: string;
  organizationMode: "BOOK_PER_FILE" | "BOOK_PER_FOLDER";
  paths: { id: number; path: string }[];
  createdAt: string;
}

export interface Book {
  id: number;
  libraryId: number;
  bookType: "EBOOK" | "AUDIOBOOK" | "COMIC";
  title?: string;
  authors: string[];
  coverPath?: string;
  addedDate?: string;
}

export interface BooksResponse {
  books: Book[];
  total: number;
  page: number;
  size: number;
}

export interface BookMetadata {
  title?: string;
  subtitle?: string;
  description?: string;
  publisher?: string;
  publishDate?: string;
  pageCount?: number;
  language?: string;
  isbn10?: string;
  isbn13?: string;
  coverPath?: string;
  googleBooksId?: string;
  amazonId?: string;
  goodreadsId?: string;
  hardcoverId?: string;
}

export interface Author {
  id: number;
  name: string;
}

export interface Series {
  id: number;
  name: string;
  seriesNumber?: number;
}

export interface Category {
  id: number;
  name: string;
}

export interface Tag {
  id: number;
  name: string;
}

export interface BookFile {
  id: number;
  format: string;
  fileSize?: number;
  filePath: string;
  trackNumber?: number;
  trackTitle?: string;
  durationSecs?: number;
}

export interface BookDetail {
  id: number;
  libraryId: number;
  bookType: "EBOOK" | "AUDIOBOOK" | "COMIC";
  folderPath?: string;
  addedDate?: string;
  title?: string;
  coverPath?: string;
  metadata?: BookMetadata;
  authors: Author[];
  series: Series[];
  categories: Category[];
  tags: Tag[];
  files: BookFile[];
}

export interface Shelf {
  id: number;
  userId: number;
  name: string;
  description?: string;
  icon?: string;
  iconColor?: string;
  isPublic: boolean;
  bookCount?: number;
  createdAt: string;
  updatedAt: string;
}

export interface ShelfBook {
  id: number;
  libraryId: number;
  bookType: string;
  title?: string;
  coverPath?: string;
  addedAt: string;
  sortOrder: number;
}
