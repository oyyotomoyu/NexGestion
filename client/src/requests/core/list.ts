export interface ListQuery {
  page?: number;
  page_size?: number;
  keyword?: string;
  sort?: string;
  order?: "asc" | "desc";
  [filter: string]: string | number | boolean | undefined;
}

export interface Pagination {
  page: number;
  page_size: number;
  total: number;
  total_pages: number;
}

export interface SortState {
  field: string;
  order: "asc" | "desc";
}

export interface ListResponse<TItem, TKey extends string> {
  pagination: Pagination;
  sort: SortState;
  keyword: string;
  [key: string]: TItem[] | Pagination | SortState | string;
}

export function buildListPath(path: string, query: ListQuery = {}) {
  const parameters = new URLSearchParams();
  Object.entries(query).forEach(([key, value]) => {
    if (value === undefined || value === "" || value === null) return;
    parameters.set(key, String(value));
  });
  const queryString = parameters.toString();
  return `${path}${queryString ? `?${queryString}` : ""}`;
}

export function listItems<TItem, TKey extends string>(
  response: ListResponse<TItem, TKey>,
  key: TKey,
) {
  return response[key] as TItem[];
}
