export type LogStatus = "info" | "warning" | "error";

export interface LogRecord {
  timestamp: string;
  status: LogStatus;
  ip: string;
  user_id: string;
  content: string;
}

export interface LogQuery {
  start?: Date | string;
  end?: Date | string;
  status?: LogStatus | LogStatus[];
  limit?: number;
  cursor?: string;
  page?: number;
  page_size?: number;
  keyword?: string;
  sort?: "timestamp" | "status";
  order?: "asc" | "desc";
}

export interface LogResponse {
  logs: LogRecord[];
  next_cursor: string;
  pagination?: {
    page: number;
    page_size: number;
    total: number;
    total_pages: number;
  };
  sort?: {
    field: string;
    order: "asc" | "desc";
  };
  keyword?: string;
}
