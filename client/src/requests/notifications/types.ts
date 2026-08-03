export type NotificationStatus = "draft" | "active" | "edited" | "hidden" | "expired";

export type NotificationDuration = "hour" | "day" | "week" | "month" | "year" | "forever";

export type NotificationAudienceScope =
  | "organization"
  | "group"
  | "own_group"
  | "role"
  | "user";

export interface NotificationType {
  id: string;
  code: string;
  name: string;
  description: string | null;
  severity: number;
  required_permission_key: string | null;
  is_active: boolean;
}

export interface NotificationAudience {
  id: string;
  scope: NotificationAudienceScope;
  target_group_id: string | null;
  target_role_id: string | null;
  target_user_id: string | null;
}

export interface Notification {
  id: string;
  sender_user_id: string;
  title: string;
  message: string;
  type: NotificationType;
  status: NotificationStatus;
  show_from: string;
  show_until: string | null;
  retain_until: string | null;
  duration_code: NotificationDuration;
  audiences: NotificationAudience[];
  created_at: string;
  updated_at: string;
  edited_at: string | null;
  hidden_at: string | null;
  expired_at: string | null;
}

export interface NotificationAudienceInput {
  scope: NotificationAudienceScope;
  target_group_id?: string;
  target_role_id?: string;
  target_user_id?: string;
}

export interface CreateNotificationInput {
  title: string;
  message: string;
  type: string;
  show_time: NotificationDuration;
  audiences: NotificationAudienceInput[];
}

export interface UpdateNotificationInput {
  title?: string;
  message?: string;
  type?: string;
  show_time?: NotificationDuration;
  audiences?: NotificationAudienceInput[];
}
