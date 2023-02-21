package payment

import (
	"encoding/json"
	"testing"
	"time"

	"kr.dev/diff"
)

const testPaymentMethodJSON = `{
	  "id": "pm_1MdiJIIhUsqiXPckNAfPOQlt",
	  "object": "payment_method",
	  "billing_details": {
	    "address": {
	      "city": "San Francisco",
	      "country": "US",
	      "line1": "1234 Fake Street",
	      "line2": null,
	      "postal_code": "94102",
	      "state": "CA"
	    },
	    "email": "jenny@example.com",
	    "name": null,
	    "phone": "+15555555555"
	  },
	  "card": {
	    "brand": "visa",
	    "checks": {
	      "address_line1_check": null,
	      "address_postal_code_check": null,
	      "cvc_check": "pass"
	    },
	    "country": "US",
	    "exp_month": 8,
	    "exp_year": 2024,
	    "fingerprint": "noAKwq37KQONloUT",
	    "funding": "credit",
	    "generated_from": null,
	    "last4": "4242",
	    "networks": {
	      "available": [
		"visa"
	      ],
	      "preferred": null
	    },
	    "three_d_secure_usage": {
	      "supported": true
	    },
	    "wallet": null
	  },
	  "created": 123456789,
	  "customer": null,
	  "livemode": false,
	  "metadata": {
	    "order_id": "123456789"
	  },
	  "type": "card"
	}`

func TestPayment(t *testing.T) {
	var pm Method
	err := json.Unmarshal([]byte(testPaymentMethodJSON), &pm)
	if err != nil {
		t.Fatal(err)
	}

	if !pm.Created().Equal(time.Unix(123456789, 0)) {
		t.Fatalf("Created = %v; want 123456789", pm.Created().Unix())
	}

	if pm.ProviderID() != "pm_1MdiJIIhUsqiXPckNAfPOQlt" {
		t.Fatalf("ID = %q; want pm_1MdiJIIhUsqiXPckNAfPOQlt", pm.ProviderID())
	}
}

func TestMethodRoundTrip(t *testing.T) {
	var pm Method
	err := json.Unmarshal([]byte(testPaymentMethodJSON), &pm)
	if err != nil {
		t.Fatal(err)
	}

	var ma map[string]any
	if err := json.Unmarshal([]byte(testPaymentMethodJSON), &ma); err != nil {
		t.Fatal(err)
	}

	data, err := json.Marshal(pm)
	if err != nil {
		t.Fatal(err)
	}

	var mb map[string]any
	if err = json.Unmarshal(data, &mb); err != nil {
		t.Fatal(err)
	}

	diff.Test(t, t.Errorf, ma, mb)
}
