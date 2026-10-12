package otp

import (
	"context"
	"errors"
	"testing"
)

type fakeUpstream struct {
	captchaCalls int
}

func (f *fakeUpstream) VerifyCaptcha(context.Context, PaymentEvent) error {
	f.captchaCalls++
	return nil
}

type fakeNotifier struct {
	calls int
	code  string
}

func (f *fakeNotifier) SendCode(_, code string) error {
	f.calls++
	f.code = code
	return nil
}

type memoryAudit struct{ events []AuditEvent }

func (m *memoryAudit) Record(event AuditEvent) { m.events = append(m.events, event) }

func TestStartAppliesPaymentRiskPolicy(t *testing.T) {
	tests := []struct {
		name       string
		amount     int64
		wantState  string
		wantReview bool
		wantCalls  int
	}{
		{name: "ordinary payment sends code", amount: 4200, wantState: "code_sent", wantCalls: 2},
		{name: "threshold payment enters review", amount: ReviewThreshold, wantState: "manual_review", wantReview: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := &fakeUpstream{}
			audit := &memoryAudit{}
			notifier := &fakeNotifier{}
			login := NewPaymentLogin(upstream, notifier, audit)
			result, err := login.Start(context.Background(), PaymentEvent{
				PaymentID: "pay_1042", Phone: "+14155550123", Amount: tt.amount, CaptchaToken: "captcha_ok", WidgetRecordID: "widget_test",
			})
			if errors.Is(err, ErrManualReview) != tt.wantReview {
				t.Fatalf("review error = %v, want %v", err, tt.wantReview)
			}
			if result.State != tt.wantState {
				t.Fatalf("state = %q, want %q", result.State, tt.wantState)
			}
			if got := upstream.captchaCalls + notifier.calls; got != tt.wantCalls {
				t.Fatalf("upstream calls = %d, want %d", got, tt.wantCalls)
			}
			if len(audit.events) != 1 || audit.events[0].Outcome != tt.wantState {
				t.Fatalf("audit events = %#v", audit.events)
			}
		})
	}
}

func TestStartIsIdempotentByPaymentID(t *testing.T) {
	upstream := &fakeUpstream{}
	notifier := &fakeNotifier{}
	login := NewPaymentLogin(upstream, notifier, &memoryAudit{})
	event := PaymentEvent{PaymentID: "pay_retry", Phone: "+14155550123", Amount: 1200, CaptchaToken: "captcha_ok", WidgetRecordID: "widget_test"}
	for i := 0; i < 2; i++ {
		if _, err := login.Start(context.Background(), event); err != nil {
			t.Fatal(err)
		}
	}
	if upstream.captchaCalls != 1 || notifier.calls != 1 {
		t.Fatalf("calls after retry: captcha=%d notify=%d", upstream.captchaCalls, notifier.calls)
	}
}
