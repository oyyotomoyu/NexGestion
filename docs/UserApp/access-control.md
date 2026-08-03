# Access Control

## Overview

Access Control is managed from Settings. It includes users, roles, permissions, and groups.

Available screens and actions depend on the signed-in user's permissions.

## Roles

Role management is available under **Settings -> Access Control -> Roles**.

The role list shows:

- Role name
- Description
- System/protected status
- Assigned permission count
- Available actions

Authorized users can create, edit, and delete custom roles.

Only the protected initial administrator can change permission grants. Other users with role access may see assigned permissions as read-only.

System roles are displayed as protected and cannot be edited or deleted.

## Users

Custom roles are assigned from **Settings -> Users -> User Detail**. This keeps role assignment user-centered and easier to manage when the organization has many users.

## Groups

Group management is available under **Settings -> Access Control -> Groups**.

The group list shows:

- Name
- Type
- Parent group
- Status
- Active member count
- Available actions

The detail screen supports group metadata editing and membership management according to the user's permissions.

Groups describe organization membership. They do not automatically grant request permissions.

## Permissions

Permissions decide which screens and actions a user can access.

Permission grants are additive: if any assigned role grants a permission, the user receives that permission.
