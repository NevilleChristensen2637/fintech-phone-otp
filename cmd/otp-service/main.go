package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"

	"github.com/example/fintech-phone-otp/internal/otp"
)

type jsonAudit struct{ logger *log.Logger }

func (a jsonAudit) Record(event otp.AuditEvent) {
	payload, _ := json.Marshal(event)
	a.logger.Printf("audit=%s", payload)
}

type logNotifier struct{ logger *log.Logger }

func (n logNotifier) SendCode(phone, code string) error {
	n.logger.Printf("otp_notification phone=%s code=%s", phone, code)
	return nil
}

type server struct{ login *otp.PaymentLogin }

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	client := otp.NewInfraiClient(key)
	s := server{login: otp.NewPaymentLogin(client, logNotifier{logger: log.Default()}, jsonAudit{logger: log.Default()})}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /otp/start", s.start)
	mux.HandleFunc("POST /otp/verify", s.verify)
	log.Println("otp service listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

func (s server) start(w http.ResponseWriter, r *http.Request) {
	var event otp.PaymentEvent
	if err := decodeJSON(r, &event); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	result, err := s.login.Start(r.Context(), event)
	if err != nil {
		if errors.Is(err, otp.ErrManualReview) {
			writeJSON(w, http.StatusAccepted, result)
			return
		}
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}

func (s server) verify(w http.ResponseWriter, r *http.Request) {
	var input otp.VerifyInput
	if err := decodeJSON(r, &input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	result, err := s.login.Verify(r.Context(), input)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func decodeJSON(r *http.Request, target any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func writeServiceError(w http.ResponseWriter, err error) {
	var apiErr *otp.InfraiError
	if errors.As(err, &apiErr) && apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500 {
		writeJSON(w, apiErr.HTTPStatus, map[string]string{"error": apiErr.Code, "message": apiErr.Message})
		return
	}
	writeJSON(w, http.StatusBadGateway, map[string]string{"error": "upstream_request_failed"})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
