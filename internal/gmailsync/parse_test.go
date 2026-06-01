package gmailsync

import "testing"

func TestParseCardEmail(t *testing.T) {
	cases := []struct {
		name     string
		subject  string
		body     string
		wantOK   bool
		amount   float64
		merchant string
	}{
		{
			name:    "HDFC spend",
			subject: "Alert: Update on your HDFC Bank Credit Card",
			body:    "Dear Customer, Rs.1,234.50 was spent on your HDFC Bank Credit Card xx1234 at AMAZON on 01-06-2026. Available limit ...",
			wantOK:  true, amount: 1234.50, merchant: "AMAZON",
		},
		{
			name:    "ICICI spend INR",
			subject: "Transaction alert on ICICI Bank Credit Card",
			body:    "INR 2,500.00 spent on ICICI Bank Card XX9012 at FLIPKART on 02-Jun-26. ...",
			wantOK:  true, amount: 2500.00, merchant: "FLIPKART",
		},
		{
			name:    "ICICI real email (Info/UPI payee, time not mistaken for merchant)",
			subject: "Transaction alert for your ICICI Bank Credit Card",
			body: "Dear Customer,\n\nYour ICICI Bank Credit Card XX4003 has been used for a transaction of INR 80.00 on Jun 01, 2026 at 09:00:24. Info: UPI-651816620443-Mr Aasi.\n\n" +
				"The Available Credit Limit on your card is INR 10,29,889.00 and Total Credit Limit is INR 10,30,000.00. The above limits are a total of the limits of all the Credit Cards issued to the primary card holder, including any supplementary cards.",
			wantOK: true, amount: 80.00, merchant: "Mr Aasi",
		},
		{
			name:    "ICICI real subject line",
			subject: "Transaction alert for your ICICI Bank Credit Card",
			body:    "Dear Customer, Your ICICI Bank Credit Card XX9012 has been used for a transaction of INR 3,499.00 on 04-Jun-26 at MYNTRA. The available limit ...",
			wantOK:  true, amount: 3499.00, merchant: "MYNTRA",
		},
		{
			name:    "Axis Rs no dot",
			subject: "Spent on Axis Bank Credit Card",
			body:    "Thank you for using your Axis Bank Credit Card for Rs 999.00 at SWIGGY on 03-06-2026.",
			wantOK:  true, amount: 999.00, merchant: "SWIGGY",
		},
		{
			name:    "rupee symbol",
			subject: "Card used",
			body:    "₹450 was charged at UBER on your card.",
			wantOK:  true, amount: 450, merchant: "UBER",
		},
		{name: "OTP rejected", subject: "OTP", body: "Your OTP is 123456 for Rs.5000 transaction", wantOK: false},
		{name: "payment received rejected", subject: "Payment received", body: "Payment of Rs.5,000.00 received on your Credit Card. Thank you.", wantOK: false},
		{name: "statement rejected", subject: "Your statement is ready", body: "Total amount due Rs.10,000.00 spent this cycle", wantOK: false},
		{name: "no amount", subject: "Hello", body: "You spent some money at the store", wantOK: false},
		{name: "credited rejected", subject: "Refund", body: "Rs.300 credited to your card as refund", wantOK: false},
	}
	for _, c := range cases {
		got, ok := ParseCardEmail(c.subject, c.body)
		if ok != c.wantOK {
			t.Errorf("%s: ok=%v, want %v (parsed %+v)", c.name, ok, c.wantOK, got)
			continue
		}
		if !ok {
			continue
		}
		if got.Amount != c.amount {
			t.Errorf("%s: amount=%v, want %v", c.name, got.Amount, c.amount)
		}
		if got.Merchant != c.merchant {
			t.Errorf("%s: merchant=%q, want %q", c.name, got.Merchant, c.merchant)
		}
	}
}

func TestStripHTML(t *testing.T) {
	in := `<html><body><p>Rs.<b>1,234.00</b> spent at <a href="x">AMAZON</a></p><style>.x{}</style></body></html>`
	got := stripHTML(in)
	if got != "Rs. 1,234.00 spent at AMAZON" {
		t.Errorf("stripHTML = %q", got)
	}
	if p, ok := ParseCardEmail("alert", got); !ok || p.Amount != 1234 || p.Merchant != "AMAZON" {
		t.Errorf("parse after strip failed: %+v ok=%v", p, ok)
	}
}
