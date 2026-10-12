#!/bin/sh
set -eu

curl --fail-with-body -X POST http://localhost:8080/otp/start \
  -H 'Content-Type: application/json' \
  -d '{"payment_id":"pay_1042","phone":"+14155550123","ip":"203.0.113.8","device_fingerprint":"device_a7","amount":4200,"captcha_token":"replace-with-browser-token"}'

printf '\nEnter the received code: '
read -r otp_code

curl --fail-with-body -X POST http://localhost:8080/otp/verify \
  -H 'Content-Type: application/json' \
  -d "{\"payment_id\":\"pay_1042\",\"phone\":\"+14155550123\",\"code\":\"$otp_code\"}"
