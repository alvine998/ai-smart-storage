package whatsapp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type mediaRoundTripper struct {
	requests []struct {
		method string
		path   string
		body   string
	}
}

func (r *mediaRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	var body []byte
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body)
	}
	r.requests = append(r.requests, struct {
		method string
		path   string
		body   string
	}{req.Method, req.URL.Path, string(body)})
	response := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Request: req}
	switch {
	case req.URL.Path == "/v22.0/media-1":
		response.Body = io.NopCloser(strings.NewReader(`{"url":"https://media.example/file","mime_type":"image/jpeg"}`))
	case req.URL.Host == "media.example":
		response.Header.Set("Content-Type", "image/jpeg")
		response.Body = io.NopCloser(strings.NewReader("image-data"))
	case req.URL.Path == "/v22.0/phone-id/media":
		response.Body = io.NopCloser(strings.NewReader(`{"id":"uploaded-1"}`))
	case req.URL.Path == "/v22.0/phone-id/messages":
		response.Body = io.NopCloser(strings.NewReader(`{"messages":[{"id":"sent-1"}]}`))
	default:
		response.StatusCode = http.StatusNotFound
		response.Body = io.NopCloser(strings.NewReader("not found"))
	}
	return response, nil
}

func TestDownloadMedia(t *testing.T) {
	transport := &mediaRoundTripper{}
	service := New("token", "verify", "secret", "phone-id", "v22.0")
	service.client = &http.Client{Transport: transport}

	data, mimeType, err := service.DownloadMedia(context.Background(), "media-1")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "image-data" || mimeType != "image/jpeg" {
		t.Fatalf("DownloadMedia() = %q, %q", data, mimeType)
	}
	if len(transport.requests) != 2 || transport.requests[0].method != http.MethodGet || transport.requests[1].path != "/file" {
		t.Fatalf("requests = %+v, want metadata and media requests", transport.requests)
	}
}

func TestSendMediaUploadsAndSendsImage(t *testing.T) {
	transport := &mediaRoundTripper{}
	service := New("token", "verify", "secret", "phone-id", "v22.0")
	service.client = &http.Client{Transport: transport}

	if err := service.SendMedia(context.Background(), "15551234567", "photo.jpg", "image/jpeg", []byte("data"), true); err != nil {
		t.Fatal(err)
	}
	if len(transport.requests) != 2 {
		t.Fatalf("requests = %d, want upload and send", len(transport.requests))
	}
	var payload struct {
		Type  string `json:"type"`
		Image struct {
			ID string `json:"id"`
		} `json:"image"`
	}
	if err := json.Unmarshal([]byte(transport.requests[1].body), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Type != "image" || payload.Image.ID != "uploaded-1" {
		t.Fatalf("message payload = %+v", payload)
	}
	if !strings.Contains(transport.requests[0].body, "data") {
		t.Fatalf("upload body does not contain file data: %q", transport.requests[0].body)
	}
}
