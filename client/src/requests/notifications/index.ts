import { request } from "@/requests/core/client";
import { buildListPath, listItems, type ListQuery, type ListResponse } from "@/requests/core/list";
import type {
  CreateNotificationInput,
  Notification,
  NotificationAudience,
  NotificationType,
  UpdateNotificationInput,
} from "@/requests/notifications/types";

const mockTypes: NotificationType[] = [
  {
    id: "notification-type-info",
    code: "info",
    name: "Info",
    description: "General information or routine notice",
    severity: 10,
    required_permission_key: "notifications.type.info",
    is_active: true,
  },
  {
    id: "notification-type-success",
    code: "success",
    name: "Success",
    description: "Positive completion or confirmation notice",
    severity: 20,
    required_permission_key: "notifications.type.success",
    is_active: true,
  },
  {
    id: "notification-type-warning",
    code: "warning",
    name: "Warning",
    description: "Warning notice that needs attention",
    severity: 60,
    required_permission_key: "notifications.type.warning",
    is_active: true,
  },
  {
    id: "notification-type-important",
    code: "important",
    name: "Important",
    description: "Important business notice",
    severity: 80,
    required_permission_key: "notifications.type.important",
    is_active: true,
  },
  {
    id: "notification-type-urgent",
    code: "urgent",
    name: "Urgent",
    description: "Urgent notice requiring fast action",
    severity: 100,
    required_permission_key: "notifications.type.urgent",
    is_active: true,
  },
];

let mockNotifications: Notification[] = [];

function nowISO() {
  return new Date().toISOString();
}

function addDuration(showTime: CreateNotificationInput["show_time"]) {
  const date = new Date();
  switch (showTime) {
    case "hour":
      date.setHours(date.getHours() + 1);
      break;
    case "day":
      date.setDate(date.getDate() + 1);
      break;
    case "week":
      date.setDate(date.getDate() + 7);
      break;
    case "month":
      date.setMonth(date.getMonth() + 1);
      break;
    case "year":
      date.setFullYear(date.getFullYear() + 1);
      break;
    case "forever":
      return null;
  }
  return date.toISOString();
}

function mockAudience(input: CreateNotificationInput["audiences"][number]): NotificationAudience {
  return {
    id: crypto.randomUUID(),
    scope: input.scope,
    target_group_id: input.target_group_id || null,
    target_role_id: input.target_role_id || null,
    target_user_id: input.target_user_id || null,
  };
}

function retainUntil(showUntil: string | null) {
  if (!showUntil) return null;
  const date = new Date(showUntil);
  date.setMonth(date.getMonth() + 3);
  return date.toISOString();
}

export async function listNotificationTypes(query: ListQuery = {}) {
  if (import.meta.env.DEV) return [...mockTypes];
  const response = await request<ListResponse<NotificationType, "notification_types">>(
    buildListPath("/api/notifications/types", { sort: "severity", order: "asc", page_size: 100, ...query }),
  );
  return listItems(response, "notification_types");
}

export async function listNotifications(query: ListQuery = {}) {
  if (import.meta.env.DEV) {
    const now = Date.now();
    return mockNotifications.filter(
      (item) =>
        (item.status === "active" || item.status === "edited") &&
        Date.parse(item.show_from) <= now &&
        (!item.show_until || Date.parse(item.show_until) > now),
    );
  }
  const response = await request<ListResponse<Notification, "notifications">>(
    buildListPath("/api/notifications", { sort: "updated_at", order: "desc", page_size: 100, ...query }),
  );
  return listItems(response, "notifications");
}

export async function listAdminNotifications(query: ListQuery = {}) {
  if (import.meta.env.DEV) return [...mockNotifications];
  const response = await request<ListResponse<Notification, "notifications">>(
    buildListPath("/api/notifications/admin", { sort: "updated_at", order: "desc", page_size: 100, ...query }),
  );
  return listItems(response, "notifications");
}

export async function createNotification(input: CreateNotificationInput) {
  if (import.meta.env.DEV) {
    const type = mockTypes.find((item) => item.code === input.type);
    if (!type) throw new Error(`Mock notification type ${input.type} was not found`);
    const showUntil = addDuration(input.show_time);
    const now = nowISO();
    const notification: Notification = {
      id: crypto.randomUUID(),
      sender_user_id: "0",
      title: input.title.trim(),
      message: input.message.trim(),
      type,
      status: "active",
      show_from: now,
      show_until: showUntil,
      retain_until: retainUntil(showUntil),
      duration_code: input.show_time,
      audiences: input.audiences.map(mockAudience),
      created_at: now,
      updated_at: now,
      edited_at: null,
      hidden_at: null,
      expired_at: null,
    };
    mockNotifications = [notification, ...mockNotifications];
    return { ...notification };
  }
  return request<Notification>("/api/notifications", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export async function updateNotification(id: string, input: UpdateNotificationInput) {
  if (import.meta.env.DEV) {
    const index = mockNotifications.findIndex((item) => item.id === id);
    if (index < 0) throw new Error(`Mock notification ${id} was not found`);
    const current = mockNotifications[index];
    const nextType = input.type
      ? mockTypes.find((item) => item.code === input.type) ?? current.type
      : current.type;
    const showUntil = input.show_time ? addDuration(input.show_time) : current.show_until;
    const now = nowISO();
    const updated: Notification = {
      ...current,
      title: input.title?.trim() || current.title,
      message: input.message?.trim() || current.message,
      type: nextType,
      status: "edited",
      show_until: showUntil,
      retain_until: retainUntil(showUntil),
      duration_code: input.show_time || current.duration_code,
      audiences: input.audiences?.map(mockAudience) || current.audiences,
      updated_at: now,
      edited_at: now,
    };
    mockNotifications = mockNotifications.map((item) => (item.id === id ? updated : item));
    return { ...updated };
  }
  return request<Notification>(`/api/notifications/${encodeURIComponent(id)}`, {
    method: "PATCH",
    body: JSON.stringify(input),
  });
}

export function hideNotification(id: string) {
  if (import.meta.env.DEV) {
    const now = nowISO();
    mockNotifications = mockNotifications.map((item) =>
      item.id === id
        ? { ...item, status: "hidden", hidden_at: now, updated_at: now }
        : item,
    );
    return Promise.resolve();
  }
  return request<void>(`/api/notifications/${encodeURIComponent(id)}/hide`, {
    method: "POST",
  });
}

export function notificationCSVURL(month: string) {
  return `/api/notifications/exports/${encodeURIComponent(month)}/csv`;
}
