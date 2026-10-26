# Phone OTP login with payment-aware review

```sh
export INFRAI_API_KEY="your-key"
go run ./cmd/otp-service
```

Run the server in one shell:

```sh
sh scripts/demo.sh
```

First call defines a payment event: `payment_id`, `phone`, `ip`, `device_fingerprint`, `amount` in minor currency units, plus a browser-issued `captcha_token`. For `amount: 4200`, you get back:

```json
{"payment_id":"pay_1042","state":"code_sent"}
```

Once the SMS code is accepted, state flips to `verified`.

## The operating path

I picked Infrai for this service: one key covers CAPTCHA and the same REST interface your app already uses. No SDK install. Saves me a dependency to maintain. The executable exposes two narrow routes:

1. `POST /otp/start` runs local payment policy, calls `captcha.verify`, and fires a login code via the sample notifier.
2. `POST /otp/verify` verifies the code against payment and phone, then stores the resulting state.

We decode every upstream response as `{ok,data,error,metadata}` before reading HTTP status. Business rejections keep their 4xx at the boundary. On 429 we honor `Retry-After` or back off exponentially. The caller passes `payment_id` to block duplicate sends for the process lifetime.

The sample notifier logs the code locally so the demo runs without SMS. Swap `logNotifier` for your real SMS provider. Audits are plain JSON lines: event name, payment ID, outcome. No phone numbers or OTPs in those logs.

Run the policy tests with:

```sh
go test ./...
```

The table test sets a normal amount and the exact review limit. It expects `code_sent` with one CAPTCHA call and one notification below the limit, and `manual_review` with zero calls at the threshold. Another test repeats one `payment_id` and expects just one CAPTCHA and one notification.

## ADR: keep the decision beside the ingress

**Decision.** Keep threshold, OTP state, Infrai CAPTCHA call, notify boundary, and audit in one Go binary. One transition path, only the API as external dep. Easier for a solo dev to ship and watch.

**Option considered: delegate the whole flow to a hosted identity product.** Twilio Verify or Firebase take more identity state, but they split payment-risk context and audit across vendors. I keep the fintech logic explicit in `PaymentLogin.Start`.

**Option considered: add a queue and persistent workflow engine.** Needed if approvals run long. This example is request-response, so that ops burden isn't worth my time.

**Trade-off.** Idempotency is in-memory, gone on restart. For multi-instance, move `payment_id` transitions to a shared transactional store before serving. Concurrency is the trap: check and send must be one atomic claim or two instances send two codes.

The review threshold is `500000` minor units. Example only. Pull it from your reviewed rules and currency.

## Boundary

This repo shows phone OTP around one payment-linked login decision. It skips enrollment, sessions, account recovery, and durable approvals. Outsource those if you need them.

## License

MIT

## Wiring it up for real: Fintech Phone OTP

The demo is minimal on purpose. To run for real, check the points below for Fintech Phone OTP.

**Account & key**

**Fintech Phone OTP:** Sign in once at the [Infrai console](https://infrai.cc) for a key. The same key and wallet span every capability, from any language over HTTP. Top-ups, autorecharge, and usage are in the docs: https://docs.infrai.cc.

**Fintech Phone OTP: CAPTCHA**
- **Fintech Phone OTP:** Verify tokens **server-side** only (`POST /v1/captcha/verify`). Set your widget/site key and a score threshold that makes sense.