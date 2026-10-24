---
title: AWS IAM Identity Center SAML SSO
description: Configure production SAML 2.0 sign-in to Alga through AWS IAM Identity Center.
---

# AWS IAM Identity Center SAML SSO

Alga can use AWS IAM Identity Center as a SAML 2.0 identity provider. The integration uses service-provider-initiated sign-in, creates Alga sessions after validating signed assertions, and links users to existing Alga accounts. It does not provision new Alga users.

## Requirements

- Configure `ALGA_PUBLIC_URL` as Alga's public HTTPS origin. The browser must be able to reach the SAML endpoints at that origin.
- Keep Valkey available to every Alga instance. SAML request state and assertion replay protection use Valkey and fail closed if it is unavailable.
- Use an account with the `oidc:manage` permission to configure providers.
- Provision each user in Alga before their first SAML sign-in. The email attribute from IAM Identity Center must exactly match the Alga account email.

## Configure AWS IAM Identity Center

1. In Alga, open **Settings → SSO Providers → SAML providers** and create a disabled provider draft. A draft has no IdP metadata yet; it shows the Alga ACS URL, SAML audience, application start URL, and SP metadata URL.
2. In the AWS console, open **IAM Identity Center → Applications → Customer managed → Add application**. Select **I have an application I want to set up**, then choose **SAML 2.0**.
3. Download the IAM Identity Center SAML metadata file. Keep it available for the Alga provider form.
4. In **Application metadata**, select **Manually type your metadata values**. Copy the draft's ACS URL into **Application ACS URL** and its entity ID into **Application SAML audience**.
5. Set the **Application start URL** to the draft's start URL. The AWS access portal uses this URL to begin Alga's SP-initiated flow.
6. Map the SAML subject to a stable, non-transient identifier. Map a single email value to the attribute name configured in Alga (default: `email`). Assign the intended AWS users or groups to the application.
7. Return to Alga, edit the draft, paste the IAM Identity Center metadata XML, and save. Alga validates the issuer, HTTPS sign-in endpoint, and current signing certificate. Enable the provider after confirming the AWS values and mappings.
8. Test with an assigned user whose exact email already belongs to an Alga account.

AWS's [custom SAML application setup guide](https://docs.aws.amazon.com/singlesignon/latest/userguide/customermanagedapps-set-up-your-own-app-saml2.html) describes downloading IAM Identity Center metadata and entering the ACS and audience values. The [application start URL guide](https://docs.aws.amazon.com/singlesignon/latest/userguide/appproperties.html) describes using a start URL for SP-initiated applications.

## What Alga validates

Alga uses the HTTP-POST request binding when the IdP metadata advertises it, as AWS IAM Identity Center's application-start flow expects, and falls back to HTTP-Redirect when that is the only advertised request binding. The POST form uses a per-response CSP nonce. Alga accepts the HTTP-POST SAML response binding and requires a signed response or signed assertion using a SHA-2 signature and a signing certificate in the configured metadata. It checks the issuer, destination, request ID, audience, subject confirmation recipient, assertion timestamps, and replay status. Only one plaintext assertion is accepted. IdP-initiated sign-in and artifact binding are not supported.

AWS IAM Identity Center does not validate signatures on incoming SAML authentication requests. Alga therefore sends SP-initiated requests without a request signature and requires AWS to sign the response or assertion. See AWS's [SAML access overview](https://docs.aws.amazon.com/singlesignon/latest/userguide/customermanagedapps-saml2-oauth2.html).

## User matching and access

On first sign-in, Alga matches the configured email attribute to an existing account and stores the stable SAML subject as that user's identity for this provider. Later sign-ins use the subject link. Account lockouts are honored. Roles and permissions remain managed in Alga; IAM Identity Center assignment controls who can reach the SAML application, but does not set Alga roles.

The configured IdP and its administrators are trusted to provide accurate email attributes. Only enable account linking after restricting AWS application assignments to the intended users or groups. A missing Alga account, malformed email, invalid signature, or other validation failure returns the same generic sign-in error.

To rotate AWS signing certificates, download the refreshed IAM Identity Center metadata and replace the provider metadata in Alga. The IdP issuer is fixed after initial setup so existing subject links cannot silently move to a different identity source; create a new provider if the issuer changes. Keep the provider disabled if a rotation must not overlap with active sign-ins.
