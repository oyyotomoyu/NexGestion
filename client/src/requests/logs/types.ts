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
}

export interface LogResponse {
  logs: LogRecord[];
  next_cursor: string;
}
