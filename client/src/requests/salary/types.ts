export type CompensationBasis =
  | "hourly"
  | "daily"
  | "weekly"
  | "monthly"
  | "annual"
  | "piece_rate"
  | "project_based"
  | "contract";

export const compensationBasisOptions: CompensationBasis[] = [
  "hourly",
  "daily",
  "weekly",
  "monthly",
  "annual",
  "piece_rate",
  "project_based",
  "contract",
];

export interface CompensationRecord {
  id: string;
  user_id: string;
  compensation_basis: CompensationBasis;
  rate_amount: string;
  currency: string;
  jurisdiction_id: string;
  effective_start_date: string;
  effective_end_date: string | null;
  note: string;
  created_by_user_id: string;
  created_at: string;
}

export interface CreateCompensationRecordInput {
  compensation_basis: CompensationBasis;
  rate_amount: string;
  currency: string;
  jurisdiction_id: string;
  effective_start_date: string;
  note?: string;
}
