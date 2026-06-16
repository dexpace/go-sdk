// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package webhook_test

import (
	"errors"
	"testing"
	"time"

	"github.com/dexpace/go-sdk/webhook"
)

func TestSignVerifyRoundTrip(t *testing.T) {
	t.Parallel()

	secret := []byte("s3cr3t")
	payload := []byte(`{"event":"ping"}`)
	sig := webhook.Sign(secret, payload)

	v := webhook.NewVerifier(secret)
	if err := v.Verify(payload, sig); err != nil {
		t.Fatalf("Verify of a valid signature: %v", err)
	}

	if err := v.Verify([]byte(`{"event":"pong"}`), sig); !errors.Is(err, webhook.ErrSignatureMismatch) {
		t.Fatalf("tampered payload err = %v, want ErrSignatureMismatch", err)
	}
}

func TestVerifyWrongSecret(t *testing.T) {
	t.Parallel()

	payload := []byte("body")
	sig := webhook.Sign([]byte("right"), payload)
	v := webhook.NewVerifier([]byte("wrong"))
	if err := v.Verify(payload, sig); !errors.Is(err, webhook.ErrSignatureMismatch) {
		t.Fatalf("wrong secret err = %v, want ErrSignatureMismatch", err)
	}
}

func TestVerifyBadHex(t *testing.T) {
	t.Parallel()

	v := webhook.NewVerifier([]byte("s"))
	if err := v.Verify([]byte("body"), "not-hex!!"); !errors.Is(err, webhook.ErrSignatureMismatch) {
		t.Fatalf("bad hex err = %v, want ErrSignatureMismatch (no panic)", err)
	}
}

func TestVerifyTimestampWithinTolerance(t *testing.T) {
	t.Parallel()

	secret := []byte("s3cr3t")
	body := []byte("payload")
	ts := time.Unix(1_700_000_000, 0)
	sig := webhook.Sign(secret, []byte("1700000000."+string(body)))

	v := webhook.NewVerifier(secret)
	if err := v.VerifyTimestamp(body, ts, ts, sig); err != nil {
		t.Fatalf("VerifyTimestamp within tolerance: %v", err)
	}
}

func TestVerifyTimestampOutsideTolerance(t *testing.T) {
	t.Parallel()

	secret := []byte("s3cr3t")
	body := []byte("payload")
	ts := time.Unix(1_700_000_000, 0)
	sig := webhook.Sign(secret, []byte("1700000000."+string(body)))
	v := webhook.NewVerifier(secret)

	if err := v.VerifyTimestamp(body, ts, ts.Add(10*time.Minute), sig); !errors.Is(err, webhook.ErrTimestampOutsideTolerance) {
		t.Fatalf("stale err = %v, want ErrTimestampOutsideTolerance", err)
	}
	if err := v.VerifyTimestamp(body, ts, ts.Add(-10*time.Minute), sig); !errors.Is(err, webhook.ErrTimestampOutsideTolerance) {
		t.Fatalf("future err = %v, want ErrTimestampOutsideTolerance", err)
	}
}

func TestVerifyTimestampZeroToleranceSkipsWindow(t *testing.T) {
	t.Parallel()

	secret := []byte("s3cr3t")
	body := []byte("payload")
	ts := time.Unix(1_700_000_000, 0)
	sig := webhook.Sign(secret, []byte("1700000000."+string(body)))

	v := webhook.NewVerifier(secret, webhook.WithTolerance(0))
	if err := v.VerifyTimestamp(body, ts, ts.Add(48*time.Hour), sig); err != nil {
		t.Fatalf("zero tolerance should skip the window: %v", err)
	}
}

func TestVerifyTimestampSignatureMismatch(t *testing.T) {
	t.Parallel()

	secret := []byte("s3cr3t")
	ts := time.Unix(1_700_000_000, 0)
	sig := webhook.Sign(secret, []byte("1700000000.other"))

	v := webhook.NewVerifier(secret)
	if err := v.VerifyTimestamp([]byte("payload"), ts, ts, sig); !errors.Is(err, webhook.ErrSignatureMismatch) {
		t.Fatalf("err = %v, want ErrSignatureMismatch", err)
	}
}
