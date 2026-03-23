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
