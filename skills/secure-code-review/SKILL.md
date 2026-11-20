---
name: secure-code-review
description: Professional skills and methodology for secure code review
version: 1.0.0
---

# Secure Code Review

## Overview

Secure code review is an important method for identifying security vulnerabilities in code. This skill provides methods, tools, and best practices for secure code review.

## Review Scope

### 1. Input Validation

**Checklist items:**
- User input validation
- Parameter validation
- Data filtering
- Bounds checking

### 2. Output Encoding

**Checklist items:**
- XSS protection
- Output encoding
- Content Security Policy
- Response header settings

### 3. Authentication and Authorization

**Checklist items:**
- Authentication mechanism
- Session management
- Access control
- Password handling

### 4. Cryptography and Keys

**Checklist items:**
- Data encryption
- Key management
- Hashing algorithms
- Random number generation

## Review Method

### 1. Static Analysis

**Using SAST tools:**
```bash
# SonarQube
sonar-scanner

# Checkmarx
# Use the web interface

# Fortify
sourceanalyzer -b project build.sh
sourceanalyzer -b project -scan

# Semgrep
semgrep --config=auto .
```

### 2. Manual Review

**Review checklist:**
- [ ] Input validation
- [ ] Output encoding
- [ ] SQL injection
- [ ] XSS vulnerabilities
- [ ] Authentication and authorization
- [ ] Cryptography usage
- [ ] Error handling
- [ ] Logging

### 3. Code Pattern Recognition

**Dangerous functions:**
```python
# Python dangerous functions
eval()
exec()
pickle.loads()
os.system()
subprocess.call()
```

```java
// Java dangerous functions
Runtime.exec()
ProcessBuilder()
Class.forName()
```

```php
// PHP dangerous functions
eval()
exec()
system()
passthru()
```

## Common Vulnerability Patterns

### SQL Injection

**Dangerous code:**
```java
String query = "SELECT * FROM users WHERE id = " + userId;
Statement stmt = connection.createStatement();
ResultSet rs = stmt.executeQuery(query);
```

**Secure code:**
```java
String query = "SELECT * FROM users WHERE id = ?";
PreparedStatement stmt = connection.prepareStatement(query);
stmt.setInt(1, userId);
ResultSet rs = stmt.executeQuery();
```

### XSS Vulnerability

**Dangerous code:**
```javascript
document.innerHTML = userInput;
element.innerHTML = "<div>" + userInput + "</div>";
```

**Secure code:**
```javascript
element.textContent = userInput;
element.setAttribute("data-value", userInput);
// Or use an encoding library
element.innerHTML = escapeHtml(userInput);
```

### Command Injection

**Dangerous code:**
```python
import os
os.system("ping " + user_input)
```

**Secure code:**
```python
import subprocess
subprocess.run(["ping", "-c", "1", validated_input])
```

### Path Traversal

**Dangerous code:**
```java
String filePath = "/uploads/" + fileName;
File file = new File(filePath);
```

**Secure code:**
```java
String basePath = "/uploads/";
String fileName = Paths.get(fileName).getFileName().toString();
String filePath = basePath + fileName;
File file = new File(filePath);
if (!file.getCanonicalPath().startsWith(basePath)) {
    throw new SecurityException("Invalid path");
}
```

### Hardcoded Secrets

**Dangerous code:**
```java
String apiKey = "1234567890abcdef";
String password = "admin123";
```

**Secure code:**
```java
String apiKey = System.getenv("API_KEY");
String password = keyStore.getPassword("db_password");
```

## Tool Usage

### SonarQube

```bash
# Start SonarQube
docker run -d -p 9000:9000 sonarqube

# Run a scan
sonar-scanner \
  -Dsonar.projectKey=myproject \
  -Dsonar.sources=. \
  -Dsonar.host.url=http://localhost:9000
```

### Semgrep

```bash
# Install
pip install semgrep

# Run a scan
semgrep --config=auto .

# Use a ruleset
semgrep --config=p/security-audit .
```

### CodeQL

```bash
# Create a database
codeql database create database --language=java --source-root=.

# Run a query
codeql database analyze database security-and-quality.qls --format=sarif-latest
```

## Review Checklist

### Input Validation
- [ ] All user input is validated
- [ ] Allowlist validation is used
- [ ] Data type and range are validated
- [ ] Special characters are handled

### Output Encoding
- [ ] HTML output encoding
- [ ] URL encoding
- [ ] JavaScript encoding
- [ ] SQL parameterization

### Authentication and Authorization
- [ ] Strong password policy
- [ ] Secure session management
- [ ] Permission validation
- [ ] Multi-factor authentication

### Cryptography
- [ ] Strong encryption algorithms are used
- [ ] Keys are stored securely
- [ ] Encryption in transit
- [ ] Encryption at rest

### Error Handling
- [ ] No sensitive information is leaked
- [ ] Unified error responses
- [ ] Error logging
- [ ] Exception handling

## Best Practices

### 1. Secure Coding Standards

- Follow the OWASP Top 10
- Use secure coding guidelines
- Code review process
- Security training

### 2. Automation Tools

- Integrate SAST tools
- CI/CD security checks
- Automated scanning
- Result analysis

### 3. Code Review Process

- Peer review
- Security expert review
- Regular review
- Record issues

## Notes

- Combine tooling with manual review
- Pay attention to business logic vulnerabilities
- Update tool rules regularly
- Build a secure coding culture
