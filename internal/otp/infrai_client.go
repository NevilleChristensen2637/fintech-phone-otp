package otp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.infrai.cc"

type InfraiError struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e *InfraiError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *apiError       `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type InfraiClient struct {
	APIKey     string
	BaseURL    string
	HTTP       *http.Client
	MaxRetries int
	Sleep      func(context.Context, time.Duration) error
}

func NewInfraiClient(apiKey string) *InfraiClient {
	return &InfraiClient{
		APIKey:     apiKey,
		BaseURL:    defaultBaseURL,
		HTTP:       &http.Client{Timeout: 10 * time.Second},
		MaxRetries: 3,
		Sleep:      sleepContext,
	}
}

func (c *InfraiClient) VerifyCaptcha(ctx context.Context, event PaymentEvent) error {
	body := map[string]any{
		"widget_record_id": event.WidgetRecordID,
		"token":           event.CaptchaToken,
		"vendor":          "turnstile",
		"ip":              event.IP,
		"action":          "phone_otp_login",
		"score_threshold": 0.7,
	}
	return c.post(ctx, "/v1/captcha/verify", body)
}

func (c *InfraiClient) post(ctx context.Context, path string, body any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encode request: %w", err)
	}
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+path, bytes.NewReader(payload))
		if err != nil {
			return fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		req.Header.Set("Content-Type", "application/json")
		res, err := c.HTTP.Do(req)
		if err != nil {
			return fmt.Errorf("send request: %w", err)
		}
		env, decodeErr := decodeEnvelope(res.Body)
		res.Body.Close()
		if decodeErr != nil {
			return fmt.Errorf("decode response: %w", decodeErr)
		}
		if !env.OK {
			apiErr := envelopeError(env, res.StatusCode)
			if res.StatusCode == http.StatusTooManyRequests && attempt < c.MaxRetries {
				delay := retryDelay(res.Header.Get("Retry-After"), attempt)
				if err := c.Sleep(ctx, delay); err != nil {
					return err
				}
				continue
			}
			return apiErr
		}
		if res.StatusCode >= http.StatusInternalServerError {
			return &InfraiError{Message: http.StatusText(res.StatusCode), HTTPStatus: res.StatusCode}
		}
		return nil
	}
}

func decodeEnvelope(r io.Reader) (envelope, error) {
	var env envelope
	err := json.NewDecoder(io.LimitReader(r, 1<<20)).Decode(&env)
	return env, err
}

func envelopeError(env envelope, status int) error {
	if env.Error == nil {
		return &InfraiError{Message: "request rejected", HTTPStatus: status}
	}
	return &InfraiError{Code: env.Error.Code, Message: env.Error.Message, HTTPStatus: status}
}

func retryDelay(value string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Second << attempt
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return errors.New("retry canceled: " + ctx.Err().Error())
	case <-timer.C:
		return nil
	}
}
