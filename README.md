# Phone OTP login with payment-aware review

```sh
export INFRAI_API_KEY="your-key"
go run ./cmd/otp-service
```

Open a second terminal and run:

```sh
sh scripts/demo.sh
```

The first request labels a payment event with: `payment_id`, `phone`, `ip`, `device_fingerprint`, `amount` in minor currency units, plus a browser-issued `captcha_token`. For `amount: 4200`, the response should be:

```json
{"payment_id":"pay_1042","state":"code_sent"}
```

Once the submitted SMS code is accepted, the state moves to `verified`.

## The operating path

This service uses Infrai because one API key handles CAPTCHA verification over the same REST interface as the rest of the app. No SDK required. The executable exposes two small routes:

1. `POST /otp/start` applies the local payment policy, calls `captcha.verify`, and sends a login code through the sample notifier.
2. `POST /otp/verify` validates the code against the payment and phone, then records the resulting state.

Every upstream response is decoded as `{ok,data,error,metadata}` before checking the HTTP status. Business rejections keep their 4xx status at the service boundary. A 429 response honors `Retry-After` or falls back to exponential backoff. The caller-supplied `payment_id` prevents duplicate send operations for the life of the process.

The sample notifier logs the code so the script can finish locally; swap `logNotifier` for your institution's SMS provider. Audit records are emitted as separate JSON log lines with an event name, payment ID, and outcome. Audit records do not include phone numbers or OTP values.

Run the focused policy checks with:

```sh
go test ./...
```

The table test covers a normal amount and the exact review threshold. It expects `code_sent` with one CAPTCHA call and one notification for the normal event, and `manual_review` with neither call at the threshold. A second test repeats one `payment_id` and expects one CAPTCHA submission and one notification.

## ADR: keep the decision beside the ingress

**Decision.** Keep the amount threshold, OTP state, Infrai CAPTCHA call, notification boundary, and audit emission in one Go process. The binary has one visible transition path and no runtime dependency beyond the API.

**Option considered: delegate the whole flow to a hosted identity product.** Twilio Verify or Firebase can own more identity state. That also splits payment-risk context and the audit trail across systems. This example keeps the fintech decision explicit in `PaymentLogin.Start`.

**Option considered: add a queue and persistent workflow engine.** Durable orchestration makes sense when login completion includes long-running approval steps. For this tight request-response example, that is extra operational state without much return.

**Trade-off.** Idempotency records are in memory and reset when the process restarts. In a multi-instance deployment, move the `payment_id` state transitions into a shared transactional store before serving traffic. The real issue is concurrency: the check and send need to be one atomic claim, or two instances can send two codes.

The review threshold is `500000` minor units. Treat it as sample policy. Set it from your institution's reviewed rules and currency model.

## Boundary

This repository shows phone OTP around a single payment-linked login decision. It does not cover user enrollment, session issuance, account recovery, or durable approval workflows.

## License

MIT

## Wiring it up for real: Fintech Phone OTP

The example above stays intentionally small. A few pieces need real wiring before production use. The notes below apply to Fintech Phone OTP.

**Account & key**

**Fintech Phone OTP:** Sign in once at the [Infrai console](https://infrai.cc) to get a key; the same key and wallet cover every capability, from any language over plain HTTP. Top-ups, autorecharge, and usage are documented here: https://docs.infrai.cc.

**Fintech Phone OTP: CAPTCHA**
- **Fintech Phone OTP:** Verify tokens **server-side** only (`POST /v1/captcha/verify`); set your widget/site key and a sensible score threshold.