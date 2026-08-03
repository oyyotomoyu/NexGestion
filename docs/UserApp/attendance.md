# Attendance

## Overview

The Attendance page lets users manage their own work attendance for the current day.

The authenticated navigation shows Attendance to users who have an attendance permission.

## Main Page

The Attendance page should show:

- Current attendance status
- Current local date and time
- Today's work sessions
- Total worked time
- One primary action button

When the user is not working, the primary action is **Sign in**.

When the user is working, the primary action is **Sign out**.

After sign-out, the status returns to not working and the user can sign in again for another work session during the same day.

## Reports

Users with report permission can download the official completed monthly attendance CSV from the Attendance reports area.

## Behavior

The page should wait for server confirmation before changing attendance state.

All labels, statuses, dates, errors, and accessibility text should support English, Traditional Chinese, and Japanese.

On mobile:

- The action button spans the available width.
- Today's sessions stack vertically.
- Timestamps and durations stay readable without horizontal page scrolling.
