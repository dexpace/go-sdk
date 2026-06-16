// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package formdata_test

import (
	"context"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/dexpace/go-sdk/formdata"
)

func partsOf(t *testing.T, contentType string, body io.Reader) (fields map[string]string, files map[string][2]string) {
	t.Helper()
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatalf("ParseMediaType(%q): %v", contentType, err)
	}
	mr := multipart.NewReader(body, params["boundary"])
	fields = map[string]string{}
	files = map[string][2]string{}
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("NextPart: %v", err)
		}
		data, _ := io.ReadAll(p)
		if p.FileName() != "" {
			files[p.FormName()] = [2]string{p.FileName(), string(data)}
		} else {
			fields[p.FormName()] = string(data)
		}
	}
	return fields, files
}

func TestFormBuildRoundTrip(t *testing.T) {
	t.Parallel()

	form := formdata.New().
		Field("name", "alice").
		FileBytes("file", "a.txt", []byte("hello"))

	body, err := form.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	fields, files := partsOf(t, form.ContentType(), body)
	if fields["name"] != "alice" {
		t.Fatalf("field name = %q, want alice", fields["name"])
	}
	if files["file"] != [2]string{"a.txt", "hello"} {
		t.Fatalf("file = %v, want {a.txt hello}", files["file"])
	}
}

func TestFormNewRequest(t *testing.T) {
	t.Parallel()

	form := formdata.New().Field("k", "v")
	req, err := form.NewRequest(context.Background(), http.MethodPost, "https://api.example.test/upload")
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	ct := req.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "multipart/form-data; boundary=") {
		t.Fatalf("Content-Type = %q, want multipart/form-data; boundary=...", ct)
	}

	if req.GetBody == nil {
		t.Fatal("GetBody is nil; body is not replayable")
	}
	b1, _ := io.ReadAll(req.Body)
	rc, err := req.GetBody()
	if err != nil {
		t.Fatalf("GetBody: %v", err)
	}
	b2, _ := io.ReadAll(rc)
	if string(b1) != string(b2) || len(b1) == 0 {
		t.Fatal("replayed body does not match the original")
	}
}

type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) { return 0, r.err }

func TestFormFileReaderError(t *testing.T) {
	t.Parallel()

	boom := errors.New("read failed")
	form := formdata.New().File("file", "x", errReader{err: boom})
	if _, err := form.Build(); !errors.Is(err, boom) {
		t.Fatalf("Build err = %v, want boom", err)
	}
}

func TestFormBuildTwiceErrors(t *testing.T) {
	t.Parallel()

	form := formdata.New().Field("k", "v")
	if _, err := form.Build(); err != nil {
		t.Fatalf("first Build: %v", err)
	}
	if _, err := form.Build(); err == nil {
		t.Fatal("second Build should error (already built)")
	}
}

func TestFormFieldAfterBuildErrors(t *testing.T) {
	t.Parallel()

	form := formdata.New().Field("k", "v")
	if _, err := form.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	form.Field("late", "x")
	if _, err := form.Build(); err == nil {
		t.Fatal("Build after a post-build Field should error")
	}
}

func TestFormEmpty(t *testing.T) {
	t.Parallel()

	form := formdata.New()
	body, err := form.Build()
	if err != nil {
		t.Fatalf("empty Build: %v", err)
	}
	fields, files := partsOf(t, form.ContentType(), body)
	if len(fields) != 0 || len(files) != 0 {
		t.Fatalf("empty form has parts: fields=%v files=%v", fields, files)
	}
}
