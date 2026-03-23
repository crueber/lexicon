import type { Book } from "../library/types";

export type DashboardRowType =
  | "CONTINUE_READING"
  | "RECENTLY_ADDED"
  | "RANDOM_PICKS";

export interface DashboardRowConfig {
  type: DashboardRowType;
  enabled: boolean;
  title: string;
}

export interface DashboardRow {
  type: DashboardRowType;
  title: string;
  books: Book[];
}

export interface DashboardStats {
  totalBooks: number;
  totalLibraries: number;
  booksReadThisMonth: number;
  totalReadingTime: number;
}

export interface DashboardResponse {
  rows: DashboardRow[];
  stats: DashboardStats;
}

export interface DashboardSettingsResponse {
  rows: DashboardRowConfig[];
}
