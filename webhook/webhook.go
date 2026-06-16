// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"time"
)

// defaultTolerance is the clock skew VerifyTimestamp allows unless overridden.
const defaultTolerance = 5 * time.Minute

// ErrSignatureMismatch is returned when a signature does not match the payload.
var ErrSignatureMismatch = errors.New("webhook: signature mismatch")

// ErrTimestampOutsideTolerance is returned when a timestamped payload is outside
// the verifier's tolerance window.
var ErrTimestampOutsideTolerance = errors.New("webhook: timestamp outside tolerance")

// Sign returns the lowercase hex-encoded HMAC-SHA256 of payload keyed by secret.
func Sign(secret, payload []byte) string {
	return hex.EncodeToString(mac(secret, payload))
}

func mac(secret, payload []byte) []byte {
	h := hmac.New(sha256.New, secret)
	h.Write(payload)
	return h.Sum(nil)
}

// Verifier verifies HMAC-SHA256 webhook signatures. Build one with [NewVerifier];
// it is safe for concurrent use.
type Verifier struct {
	secret    []byte
	tolerance time.Duration
}

// Option configures a [Verifier].
type Option func(*Verifier)

// WithTolerance sets the allowed clock skew for [Verifier.VerifyTimestamp]. The
// default is five minutes; a value <= 0 disables the timestamp check.
func WithTolerance(d time.Duration) Option {
	return func(v *Verifier) { v.tolerance = d }
}

// NewVerifier returns a Verifier keyed by secret.
func NewVerifier(secret []byte, opts ...Option) *Verifier {
	v := &Verifier{secret: secret, tolerance: defaultTolerance}
	for _, opt := range opts {
		opt(v)
	}
	return v
}

// Verify reports whether sigHex is a valid hex-encoded HMAC-SHA256 signature of
// payload, compared in constant time. It returns nil on a match and
// ErrSignatureMismatch otherwise, including when sigHex is not valid hex.
func (v *Verifier) Verify(payload []byte, sigHex string) error {
	provided, err := hex.DecodeString(sigHex)
	if err != nil {
		return ErrSignatureMismatch
	}
	if !hmac.Equal(provided, mac(v.secret, payload)) {
		return ErrSignatureMismatch
	}
	return nil
}

// VerifyTimestamp verifies the common scheme in which the signed payload is the
// Unix timestamp, a ".", and the body. It first checks that timestamp is within
// the configured tolerance of now (unless the tolerance is <= 0), then verifies
// sigHex against "<unix>." + body.
func (v *Verifier) VerifyTimestamp(body []byte, timestamp, now time.Time, sigHex string) error {
	if v.tolerance > 0 {
		diff := now.Sub(timestamp)
		if diff < 0 {
			diff = -diff
		}
		if diff > v.tolerance {
			return ErrTimestampOutsideTolerance
		}
	}
	signed := strconv.FormatInt(timestamp.Unix(), 10) + "." + string(body)
	return v.Verify([]byte(signed), sigHex)
}
