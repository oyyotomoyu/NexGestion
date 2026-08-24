import { request } from "@/requests/core/client";
import { buildListPath, listItems, type ListQuery, type ListResponse } from "@/requests/core/list";
import type { CompensationRecord, CreateCompensationRecordInput } from "@/requests/salary/types";

const mockRecordsByUser: Record<string, CompensationRecord[]> = {};

function nowISO() {
  return new Date().toISOString();
}

function currentOf(records: CompensationRecord[]) {
  return records.find((record) => record.effective_end_date === null) ?? null;
}

function sortedHistory(records: CompensationRecord[]) {
  return [...records].sort((a, b) => (a.effective_start_date < b.effective_start_date ? 1 : -1));
}

function createMockRecord(key: string, userId: string, input: CreateCompensationRecordInput): CompensationRecord {
  const records = mockRecordsByUser[key] ?? [];
  const active = currentOf(records);
  if (active) {
    const closeDate = new Date(`${input.effective_start_date}T00:00:00Z`);
    closeDate.setUTCDate(closeDate.getUTCDate() - 1);
    active.effective_end_date = closeDate.toISOString().slice(0, 10);
  }
  const created: CompensationRecord = {
    id: crypto.randomUUID(),
    user_id: userId,
    compensation_basis: input.compensation_basis,
    rate_amount: input.rate_amount,
    currency: input.currency.toUpperCase(),
    jurisdiction_id: input.jurisdiction_id,
    effective_start_date: input.effective_start_date,
    effective_end_date: null,
    note: input.note ?? "",
    created_by_user_id: "mock-user",
    created_at: nowISO(),
  };
  mockRecordsByUser[key] = [...records, created];
  return created;
}

export async function getMyCurrentCompensationRecord() {
  if (import.meta.env.DEV) return currentOf(mockRecordsByUser["me"] ?? []);
  return request<CompensationRecord>("/api/salary/me/compensation-records/current");
}

export async function listMyCompensationRecords(query: ListQuery = {}) {
  if (import.meta.env.DEV) return sortedHistory(mockRecordsByUser["me"] ?? []);
  const response = await request<ListResponse<CompensationRecord, "compensation_records">>(
    buildListPath("/api/salary/me/compensation-records", {
      sort: "effective_start_date",
      order: "desc",
      page_size: 100,
      ...query,
    }),
  );
  return listItems(response, "compensation_records");
}

export async function getEmployeeCurrentCompensationRecord(userId: string) {
  if (import.meta.env.DEV) return currentOf(mockRecordsByUser[userId] ?? []);
  return request<CompensationRecord>(
    `/api/salary/employees/${encodeURIComponent(userId)}/compensation-records/current`,
  );
}

export async function listEmployeeCompensationRecords(userId: string, query: ListQuery = {}) {
  if (import.meta.env.DEV) return sortedHistory(mockRecordsByUser[userId] ?? []);
  const response = await request<ListResponse<CompensationRecord, "compensation_records">>(
    buildListPath(`/api/salary/employees/${encodeURIComponent(userId)}/compensation-records`, {
      sort: "effective_start_date",
      order: "desc",
      page_size: 100,
      ...query,
    }),
  );
  return listItems(response, "compensation_records");
}

export async function createCompensationRecord(userId: string, input: CreateCompensationRecordInput) {
  if (import.meta.env.DEV) return createMockRecord(userId, userId, input);
  return request<CompensationRecord>(
    `/api/salary/employees/${encodeURIComponent(userId)}/compensation-records`,
    { method: "POST", body: JSON.stringify(input) },
  );
}
