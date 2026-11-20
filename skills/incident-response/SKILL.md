---
name: incident-response
description: Professional skills and methodology for security incident response
version: 1.0.0
---

# Security Incident Response

## Overview

Security incident response is the key process for handling security incidents. This skill provides methods, tools and best practices for security incident response.

## Response Process

### 1. Preparation Phase

**Preparation:**
- Establish a response team
- Develop a response plan
- Prepare tools and resources
- Establish communication channels

### 2. Identification Phase

**Identify the incident:**
- Monitoring alerts
- Anomaly detection
- Log analysis
- User reports

### 3. Containment Phase

**Containment measures:**
- Isolate affected systems
- Disable accounts
- Block network connections
- Back up evidence

### 4. Eradication Phase

**Eradicate the threat:**
- Remove malware
- Patch vulnerabilities
- Reset credentials
- Clean up backdoors

### 5. Recovery Phase

**Restore systems:**
- Restore from backup
- Verify system integrity
- Monitor the system
- Restore services gradually

### 6. Lessons-Learned Phase

**Capture lessons learned:**
- Incident report
- Lessons learned
- Improvement measures
- Update processes

## Tool Usage

### Log Analysis

**Using Splunk:**
```bash
# Search logs
index=security event_type="failed_login"

# Statistical analysis
index=security | stats count by src_ip

# Time-series analysis
index=security | timechart count by event_type
```

**Using ELK:**
```bash
# Elasticsearch query
GET /logs/_search
{
  "query": {
    "match": {
      "event_type": "malware"
    }
  }
}
```

### Forensics Tools

**Using Volatility:**
```bash
# Analyze a memory image
volatility -f memory.dump imageinfo

# List processes
volatility -f memory.dump --profile=Win7SP1x64 pslist

# Dump process memory
volatility -f memory.dump --profile=Win7SP1x64 memdump -p 1234 -D output/
```

**Using Autopsy:**
```bash
# Start Autopsy
# Create a case
# Add evidence
# Analyze the data
```

### Network Analysis

**Using Wireshark:**
```bash
# Capture traffic
wireshark -i eth0

# Analyze a PCAP file
wireshark -r capture.pcap

# Filter traffic
# Display filter: ip.addr == 192.168.1.100
# Capture filter: host 192.168.1.100
```

**Using tcpdump:**
```bash
# Capture traffic
tcpdump -i eth0 -w capture.pcap

# Analyze traffic
tcpdump -r capture.pcap -A
```

## Incident Types

### Malware

**Response steps:**
1. Isolate affected systems
2. Collect samples
3. Analyze the malware
4. Eradicate the threat
5. Patch vulnerabilities

**Tools:**
- VirusTotal
- Cuckoo Sandbox
- YARA rules

### Data Breach

**Response steps:**
1. Determine the scope of the breach
2. Contain the breach
3. Assess the impact
4. Notify the relevant parties
5. Patch vulnerabilities

**Items to check:**
- Volume of data leaked
- Affected users
- Breach channel
- Data sensitivity

### Denial of Service

**Response steps:**
1. Confirm the type of attack
2. Enable protective measures
3. Filter malicious traffic
4. Monitor system status
5. Restore normal service

**Protective measures:**
- DDoS protection service
- Traffic scrubbing
- Rate limiting
- CDN protection

### Unauthorized Access

**Response steps:**
1. Disable affected accounts
2. Reset credentials
3. Review access logs
4. Assess data access
5. Patch vulnerabilities

**Items to check:**
- Access time
- Content accessed
- Source of access
- Data modifications

## Response Checklist

### Preparation Phase
- [ ] Establish a response team
- [ ] Develop a response plan
- [ ] Prepare tools
- [ ] Establish communication channels

### Identification Phase
- [ ] Confirm the incident
- [ ] Gather information
- [ ] Assess the impact
- [ ] Record the timeline

### Containment Phase
- [ ] Isolate systems
- [ ] Disable accounts
- [ ] Block connections
- [ ] Back up evidence

### Eradication Phase
- [ ] Remove the threat
- [ ] Patch vulnerabilities
- [ ] Reset credentials
- [ ] Verify eradication

### Recovery Phase
- [ ] Restore systems
- [ ] Verify integrity
- [ ] Monitor the system
- [ ] Restore services

### Lessons-Learned Phase
- [ ] Write the report
- [ ] Capture lessons learned
- [ ] Improvement measures
- [ ] Update processes

## Best Practices

### 1. Preparation

- Establish a response team
- Develop a response plan
- Run regular drills
- Prepare tools

### 2. Response

- Respond quickly
- Handle it systematically
- Record every action
- Protect evidence

### 3. Communication

- Internal communication
- External notification
- Status updates
- Post-incident report

### 4. Improvement

- Incident analysis
- Process improvement
- Tool updates
- Training improvement

## Notes

- Respond quickly
- Protect evidence
- Record actions
- Comply with laws and regulations
