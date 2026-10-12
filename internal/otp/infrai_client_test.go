package otp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVerifyCaptchaIncludesWidgetRecordID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if body["widget_record_id"] != "widget_test" || body["token"] != "captcha_test" {
			t.Errorf("captcha request = %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"data":{}}`))
	}))
	defer server.Close()
	client := NewInfraiClient("test-key")
	client.BaseURL = server.URL
	if err := client.VerifyCaptcha(context.Background(), PaymentEvent{WidgetRecordID: "widget_test", CaptchaToken: "captcha_test"}); err != nil {
		t.Fatal(err)
	}
}
