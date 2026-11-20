---
name: mobile-app-security-testing
description: Professional skills and methodology for mobile application security testing
version: 1.0.0
---

# Mobile Application Security Testing

## Overview

Mobile application security testing is a key step in ensuring the security of mobile applications. This skill provides methods, tools and best practices for mobile application security testing, covering the Android and iOS platforms.

## Testing Scope

### 1. Application Security

**Items to check:**
- Code obfuscation
- Anti-decompilation protection
- Anti-debugging protection
- Certificate pinning

### 2. Data Security

**Items to check:**
- Data encryption
- Key management
- Sensitive data storage
- Data transmission

### 3. Authentication and Authorization

**Items to check:**
- Authentication mechanism
- Token management
- Biometrics
- Session management

### 4. Communication Security

**Items to check:**
- TLS/SSL configuration
- Certificate validation
- API security
- Man-in-the-middle protection

## Android Security Testing

### Static Analysis

**Using APKTool:**
```bash
# Decompile the APK
apktool d app.apk

# View AndroidManifest.xml
cat app/AndroidManifest.xml

# View the Smali code
find app/smali -name "*.smali"
```

**Using Jadx:**
```bash
# Decompile the APK
jadx -d output app.apk

# View the Java source
find output -name "*.java"
```

**Using MobSF:**
```bash
# Start MobSF
docker run -it -p 8000:8000 opensecurity/mobsf

# Upload the APK for analysis
# Visit http://localhost:8000
```

### Dynamic Analysis

**Using Frida:**
```javascript
// Hook a function
Java.perform(function() {
    var MainActivity = Java.use("com.example.MainActivity");
    MainActivity.onCreate.implementation = function(savedInstanceState) {
        console.log("[*] onCreate called");
        this.onCreate(savedInstanceState);
    };
});
```

**Using Objection:**
```bash
# Start Objection
objection -g com.example.app explore

# Hook a function
android hooking watch class_method com.example.MainActivity.onCreate
```

**Using Burp Suite:**
```bash
# Configure the proxy
# Point the Android proxy setting at Burp Suite
# Install the Burp certificate
```

### Common Vulnerabilities

**Hardcoded keys:**
```java
// Insecure code
String apiKey = "1234567890abcdef";
String password = "admin123";
```

**Insecure storage:**
```java
// Storing sensitive data in SharedPreferences
SharedPreferences prefs = getSharedPreferences("data", MODE_WORLD_READABLE);
prefs.edit().putString("password", password).apply();
```

**Certificate validation bypass:**
```java
// Does not validate the certificate
TrustManager[] trustAllCerts = new TrustManager[] {
    new X509TrustManager() {
        public X509Certificate[] getAcceptedIssuers() { return null; }
        public void checkClientTrusted(X509Certificate[] certs, String authType) { }
        public void checkServerTrusted(X509Certificate[] certs, String authType) { }
    }
};
```

## iOS Security Testing

### Static Analysis

**Using class-dump:**
```bash
# Export header files
class-dump app.ipa

# View the headers
find app -name "*.h"
```

**Using Hopper:**
```bash
# Disassemble with Hopper
# Open the app binary
# Analyze the assembly
```

**Using otool:**
```bash
# View Mach-O info
otool -L app

# View strings
strings app | grep -i "password\|key\|secret"
```

### Dynamic Analysis

**Using Frida:**
```javascript
// Hook an Objective-C method
var className = ObjC.classes.ViewController;
var method = className['- login:password:'];
Interceptor.attach(method.implementation, {
    onEnter: function(args) {
        console.log("[*] Login called");
        console.log("Username: " + ObjC.Object(args[2]).toString());
        console.log("Password: " + ObjC.Object(args[3]).toString());
    }
});
```

**Using Cycript:**
```bash
# Attach to the process
cycript -p app

# Run a command
[UIApplication sharedApplication]
```

### Common Vulnerabilities

**Hardcoded keys:**
```objective-c
// Insecure code
NSString *apiKey = @"1234567890abcdef";
NSString *password = @"admin123";
```

**Insecure storage:**
```objective-c
// Improper Keychain storage
NSUserDefaults *defaults = [NSUserDefaults standardUserDefaults];
[defaults setObject:password forKey:@"password"];
```

**Certificate validation bypass:**
```objective-c
// Does not validate the certificate
- (void)connection:(NSURLConnection *)connection 
didReceiveAuthenticationChallenge:(NSURLAuthenticationChallenge *)challenge {
    [challenge.sender useCredential:[NSURLCredential credentialForTrust:challenge.protectionSpace.serverTrust] 
          forAuthenticationChallenge:challenge];
}
```

## Tool Usage

### MobSF

```bash
# Start MobSF
docker run -it -p 8000:8000 opensecurity/mobsf

# Upload an app for analysis
# Supports Android and iOS
```

### Frida

```bash
# Install Frida
pip install frida-tools

# Run a script
frida -U -f com.example.app -l script.js
```

### Objection

```bash
# Install Objection
pip install objection

# Start Objection
objection -g com.example.app explore
```

### Burp Suite

**Configure the proxy:**
1. Configure the Burp Suite listener
2. Set the proxy on the mobile device
3. Install the Burp certificate
4. Intercept and analyze traffic

## Testing Checklist

### Application Security
- [ ] Check code obfuscation
- [ ] Anti-decompilation protection
- [ ] Anti-debugging protection
- [ ] Certificate pinning

### Data Security
- [ ] Check data encryption
- [ ] Key management
- [ ] Sensitive data storage
- [ ] Data transmission security

### Authentication and Authorization
- [ ] Test the authentication mechanism
- [ ] Token management
- [ ] Session management
- [ ] Biometrics

### Communication Security
- [ ] TLS/SSL configuration
- [ ] Certificate validation
- [ ] API security testing
- [ ] Man-in-the-middle protection

## Common Security Issues

### 1. Hardcoded Keys

**Problem:**
- API key hardcoded
- Password hardcoded
- Encryption key hardcoded

**Fix:**
- Use a key management service
- Use environment variables
- Use secure storage

### 2. Insecure Storage

**Problem:**
- Sensitive data stored in plaintext
- Insecure storage method used
- Data not encrypted

**Fix:**
- Use encrypted storage
- Use Keychain / Keystore
- Encrypt data

### 3. Certificate Validation Bypass

**Problem:**
- SSL certificate not validated
- Self-signed certificates accepted
- Certificate pinning not implemented

**Fix:**
- Implement certificate pinning
- Validate the certificate chain
- Use the system certificate store

### 4. Debug Information Disclosure

**Problem:**
- Logs contain sensitive information
- Error messages leak information
- Debug mode not disabled

**Fix:**
- Remove debug code
- Restrict log output
- Disable debugging in production

## Best Practices

### 1. Code Security

- Implement code obfuscation
- Disable debugging features
- Implement anti-debugging protection
- Use certificate pinning

### 2. Data Security

- Encrypt sensitive data
- Use secure storage
- Implement key management
- Restrict data access

### 3. Communication Security

- Use TLS/SSL
- Implement certificate pinning
- Validate the server certificate
- Use secure APIs

### 4. Authentication Security

- Implement strong authentication
- Secure token management
- Implement session management
- Use biometrics

## Notes

- Only test in authorized environments
- Comply with laws and regulations
- Be mindful of differences between platforms
- Protect user privacy
