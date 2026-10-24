---
title: LDAP directory sign-in
description: Authenticate existing Alga users against LDAP or Active Directory.
---

# LDAP directory sign-in

Alga can authenticate existing users with an LDAP or Active Directory password. Alga keeps its own users, roles, sessions, and account lockout state. LDAP does not create users or grant roles.

## Before you start

1. Create the Alga users who will sign in. Their Alga email must exactly match the directory email attribute on first sign-in.
2. Create a directory service account that can search users and read the login, stable ID, and email attributes. Give it only the needed read permissions.
3. Make the directory available through LDAPS (`ldaps://host:636`) or StartTLS (`ldap://host:389` with **Use StartTLS** enabled). Alga verifies the server certificate and hostname using the host operating system's trust store. Install your organization's CA in that trust store if needed.

## Configure the provider

Open **Settings → SSO Providers → LDAP directories** and add a provider. Enter the directory URL, search base DN, service bind DN and password, and attribute names. Leave the provider disabled until the values are ready, then enable it.

Common attributes:

| Directory        | Login            | Stable ID    | Email  |
| ---------------- | ---------------- | ------------ | ------ |
| OpenLDAP         | `uid`            | `entryUUID`  | `mail` |
| Active Directory | `sAMAccountName` | `objectGUID` | `mail` |

Choose an ID that stays unchanged when a user is renamed or moved. Alga treats its raw directory value as an opaque identity. The directory URL, search base, and stable ID attribute cannot be changed after creation; create a new provider when switching identity authorities. Changing the service bind password does not change user links.

The service bind password is encrypted in Alga's database and cannot be read back through the API. During an update, leave the password field blank to keep the current password. The Alga server must have its normal encryption keyring configured.

## Sign in

On Alga's login page, select the LDAP provider and enter the directory login and password. Alga searches for exactly one entry, binds with that entry's DN and the submitted password, and links its stable ID to an existing Alga user with the same email on first sign-in. Later sign-ins use the stable ID link even if the directory email changes. A locked Alga account cannot sign in.

Alga never stores the submitted user password. Failed LDAP authentication, missing local users, ambiguous search results, directory outages, and TLS failures return a generic credential error to the browser; administrators can investigate directory connectivity and certificate health separately. Local sign-in remains available.

## Operational notes

- Limit who can administer providers through the `oidc:manage` permission. A directory administrator who can change users' email attributes can influence first-use account linking; allow only trusted directory administrators to edit that attribute.
- Rotate the service bind password from the provider editor. Replacing the directory certificate requires the issuing CA to remain trusted by the Alga server.
- Disable a provider to stop new LDAP sign-ins. Deleting it also deletes its directory identity links; existing Alga users and sessions are managed separately.
