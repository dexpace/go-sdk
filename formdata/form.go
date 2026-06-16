// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package formdata

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"

	"github.com/dexpace/go-sdk/header"
)

// Form builds a multipart/form-data request body. The zero value is not usable;
// create one with [New]. A Form is not safe for concurrent use.
type Form struct {
	buf   bytes.Buffer
	w     *multipart.Writer
	err   error
	built bool
}

// New returns an empty Form.
func New() *Form {
	f := &Form{}
	f.w = multipart.NewWriter(&f.buf)
	return f
}

// Field adds a text field. It returns f for chaining; the first error
// encountered is reported by [Form.Build].
func (f *Form) Field(name, value string) *Form {
	if f.err != nil {
		return f
	}
	if f.built {
		f.err = errors.New("formdata: Field called after Build")
		return f
	}
	f.err = f.w.WriteField(name, value)
	return f
}

// File adds a file part named field, with the given filename, read from r. It
// returns f for chaining.
func (f *Form) File(field, filename string, r io.Reader) *Form {
	if f.err != nil {
		return f
	}
	if f.built {
		f.err = errors.New("formdata: File called after Build")
		return f
	}
	pw, err := f.w.CreateFormFile(field, filename)
	if err != nil {
		f.err = err
		return f
	}
	if _, err := io.Copy(pw, r); err != nil {
		f.err = err
	}
	return f
}

// FileBytes adds a file part from an in-memory byte slice.
func (f *Form) FileBytes(field, filename string, data []byte) *Form {
	return f.File(field, filename, bytes.NewReader(data))
}

// ContentType returns the multipart Content-Type, including the boundary. It is
// stable for the lifetime of the Form (the boundary is fixed at New).
func (f *Form) ContentType() string {
	return f.w.FormDataContentType()
}

// Build finalizes the form and returns a replayable body. It returns the first
// error encountered while adding parts. After a successful Build no more parts
// may be added, and a second Build returns an error.
func (f *Form) Build() (*bytes.Reader, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.built {
		return nil, errors.New("formdata: already built")
	}
	if err := f.w.Close(); err != nil {
		return nil, err
	}
	f.built = true
	return bytes.NewReader(f.buf.Bytes()), nil
}

// NewRequest builds the body and returns an *http.Request with the multipart
// Content-Type header set. The request body is replayable, so the retry policy
// can re-send it.
func (f *Form) NewRequest(ctx context.Context, method, url string) (*http.Request, error) {
	body, err := f.Build()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set(header.ContentType, f.ContentType())
	return req, nil
}
