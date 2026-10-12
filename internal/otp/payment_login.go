package otp

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
)

const ReviewThreshold = int64(500000)

var ErrManualReview = errors.New("payment context requires manual review")

type PaymentEvent struct {
	PaymentID         string `json:"payment_id"`
	Phone             string `json:"phone"`
	IP                string `json:"ip"`
	DeviceFingerprint string `json:"device_fingerprint"`
	Amount            int64  `json:"amount"`
	CaptchaToken      string `json:"captcha_token"`
	WidgetRecordID    string `json:"widget_record_id"`
}

type StartResult struct {
	PaymentID string `json:"payment_id"`
	State     string `json:"state"`
}

type VerifyInput struct {
	PaymentID string `json:"payment_id"`
	Phone     string `json:"phone"`
	Code      string `json:"code"`
}

type Upstream interface {
	VerifyCaptcha(context.Context, PaymentEvent) error
}

type Notifier interface {
	SendCode(phone, code string) error
}

type AuditEvent struct {
	Event     string `json:"event"`
	PaymentID string `json:"payment_id"`
	Outcome   string `json:"outcome"`
}

type AuditSink interface {
	Record(AuditEvent)
}

type PaymentLogin struct {
	upstream Upstream
	notifier Notifier
	audit    AuditSink
	mu       sync.Mutex
	records  map[string]loginRecord
}

type loginRecord struct {
	state string
	phone string
	code  string
}

func NewPaymentLogin(upstream Upstream, notifier Notifier, audit AuditSink) *PaymentLogin {
	return &PaymentLogin{upstream: upstream, notifier: notifier, audit: audit, records: make(map[string]loginRecord)}
}

func (p *PaymentLogin) Start(ctx context.Context, event PaymentEvent) (StartResult, error) {
	if event.PaymentID == "" || event.Phone == "" || event.CaptchaToken == "" || event.WidgetRecordID == "" || event.Amount < 0 {
		return StartResult{}, errors.New("payment_id, phone, captcha_token, widget_record_id, and a non-negative amount are required")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if record, ok := p.records[event.PaymentID]; ok {
		return StartResult{PaymentID: event.PaymentID, State: record.state}, nil
	}
	if event.Amount >= ReviewThreshold {
		p.records[event.PaymentID] = loginRecord{state: "manual_review", phone: event.Phone}
		p.audit.Record(AuditEvent{Event: "otp_start", PaymentID: event.PaymentID, Outcome: "manual_review"})
		return StartResult{PaymentID: event.PaymentID, State: "manual_review"}, ErrManualReview
	}
	if err := p.upstream.VerifyCaptcha(ctx, event); err != nil {
		return StartResult{}, fmt.Errorf("verify login captcha: %w", err)
	}
	code, err := verificationCode()
	if err != nil {
		return StartResult{}, fmt.Errorf("generate login code: %w", err)
	}
	if err := p.notifier.SendCode(event.Phone, code); err != nil {
		return StartResult{}, fmt.Errorf("send login code: %w", err)
	}
	p.records[event.PaymentID] = loginRecord{state: "code_sent", phone: event.Phone, code: code}
	p.audit.Record(AuditEvent{Event: "otp_start", PaymentID: event.PaymentID, Outcome: "code_sent"})
	return StartResult{PaymentID: event.PaymentID, State: "code_sent"}, nil
}

func (p *PaymentLogin) Verify(ctx context.Context, input VerifyInput) (StartResult, error) {
	if input.PaymentID == "" || input.Phone == "" || input.Code == "" {
		return StartResult{}, errors.New("payment_id, phone, and code are required")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	record, ok := p.records[input.PaymentID]
	if !ok || record.state != "code_sent" {
		return StartResult{}, errors.New("code has not been requested for this payment")
	}
	if record.phone != input.Phone || record.code != input.Code {
		return StartResult{}, errors.New("code does not match this payment and phone")
	}
	p.records[input.PaymentID] = loginRecord{state: "verified", phone: record.phone}
	p.audit.Record(AuditEvent{Event: "otp_verify", PaymentID: input.PaymentID, Outcome: "verified"})
	return StartResult{PaymentID: input.PaymentID, State: "verified"}, nil
}

func verificationCode() (string, error) {
	var random [4]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	value := binary.BigEndian.Uint32(random[:])
	return fmt.Sprintf("%06d", value%1000000), nil
}
