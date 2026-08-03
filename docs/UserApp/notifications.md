# Notifications

## Overview

Notifications let authorized users send messages to people in the organization. A notification can be addressed to the whole organization, selected groups, the sender's own group, specific roles, or specific users.

Users see notifications when the message target matches their role, group membership, user account, or the whole organization.

## Permissions In The App

Only administrators can manage notification settings and notification types.

Sending options depend on the sender's permissions:

- Whole organization: send to everyone.
- Selected groups: send to any chosen group, even if the sender is not in that group.
- Own group: send only to groups where the sender is a member.
- Type permission: choose only notification types the sender is allowed to use.

When a user does not have permission for a target or type, the app should hide or disable that option.

## Notification Types

The common notification types are:

| Type | Use For |
| --- | --- |
| Info | General information or routine notices |
| Success | Positive confirmation or completion messages |
| Warning | Messages that need attention |
| Important | Business-critical notices |
| Urgent | Messages that need fast action |

## Sending A Notification

The send form requires:

- Title
- Message
- Type
- Show time
- Audience

Show time options:

| Show Time | Meaning |
| --- | --- |
| An hour | Show for one hour |
| A day | Show for one day |
| A week | Show for one week |
| A month | Show for one month |
| A year | Show for one year |
| Forever | Keep showing until hidden or changed |

After sending, the notification is visible to users in its audience during the show time.

## Editing And Hiding

The sender can hide a notification they sent. Hidden notifications are no longer shown to users, but remain stored for records.

If a sent notification is edited, the app should show that the notification was edited. This helps users know that an older message has changed and should be checked again.

## Expired Notifications

When the show time ends, the notification becomes expired and is no longer shown in the active notification list. Expired notifications remain available to administrators for record keeping until the retention period ends.

## Admin Export

Administrators can export notification records by month as CSV, or use an admin API when available.

Administrators can also open the notification message list to review all notification records, including active, edited, hidden, and expired messages.
