package whatsapp

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"time"
)

type Service struct {
	token, verifyToken, appSecret, phoneID, graphVersion string
	client                                               *http.Client
}

func New(token, verifyToken, appSecret, phoneID, graphVersion string) *Service {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	return &Service{token: token, verifyToken: verifyToken, appSecret: appSecret, phoneID: phoneID, graphVersion: graphVersion, client: client}
}

// Ping verifies connectivity to the WhatsApp Cloud API by fetching the
// phone number metadata. Suitable for startup diagnostics / health probes.
func (s *Service) Ping(ctx context.Context) error {
	if s == nil {
		return fmt.Errorf("whatsapp service not initialized")
	}
	if s.token == "" {
		return fmt.Errorf("WHATSAPP_ACCESS_TOKEN not set")
	}
	if s.phoneID == "" {
		return fmt.Errorf("WHATSAPP_PHONE_NUMBER_ID not set")
	}
	url := fmt.Sprintf("https://graph.facebook.com/%s/%s?fields=id,display_phone_number", s.graphVersion, s.phoneID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("whatsapp API returned %s: %s", resp.Status, string(body))
	}
	return nil
}

// IsConfigured returns true when mandatory credentials are present.
func (s *Service) IsConfigured() bool {
	return s != nil && s.token != "" && s.phoneID != ""
}

func (s *Service) Verify(mode, challenge, verifyToken string) (string, error) {
	if mode != "subscribe" || verifyToken != s.verifyToken {
		return "", fmt.Errorf("webhook verification failed")
	}
	return challenge, nil
}

func (s *Service) ValidSignature(body []byte, signature string) bool {
	const prefix = "sha256="
	if len(signature) <= len(prefix) || signature[:len(prefix)] != prefix {
		return false
	}
	expected := make([]byte, sha256.Size)
	mac := hmac.New(sha256.New, []byte(s.appSecret))
	mac.Write(body)
	if _, err := hex.Decode(expected, []byte(signature[len(prefix):])); err != nil {
		return false
	}
	return hmac.Equal(expected, mac.Sum(nil))
}

func (s *Service) SendText(ctx context.Context, recipient, text string) error {
	payload, _ := json.Marshal(map[string]any{"messaging_product": "whatsapp", "to": recipient, "type": "text", "text": map[string]string{"body": text}})
	return s.sendJSON(ctx, "messages", payload)
}

func (s *Service) DownloadMedia(ctx context.Context, mediaID string) ([]byte, string, error) {
	url := fmt.Sprintf("https://graph.facebook.com/%s/%s", s.graphVersion, mediaID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("media metadata request: %w", err)
	}
	body, readErr := io.ReadAll(resp.Body)
	resp.Body.Close()
	if readErr != nil {
		return nil, "", readErr
	}
	if resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("media metadata returned %s: %s", resp.Status, string(body))
	}
	var metadata struct {
		URL      string `json:"url"`
		MimeType string `json:"mime_type"`
	}
	if err := json.Unmarshal(body, &metadata); err != nil {
		return nil, "", fmt.Errorf("media metadata decode: %w", err)
	}
	if metadata.URL == "" {
		return nil, "", fmt.Errorf("media metadata missing download URL")
	}
	downloadReq, err := http.NewRequestWithContext(ctx, http.MethodGet, metadata.URL, nil)
	if err != nil {
		return nil, "", err
	}
	downloadReq.Header.Set("Authorization", "Bearer "+s.token)
	downloadResp, err := s.client.Do(downloadReq)
	if err != nil {
		return nil, "", fmt.Errorf("media download request: %w", err)
	}
	defer downloadResp.Body.Close()
	if downloadResp.StatusCode >= 300 {
		downloadBody, _ := io.ReadAll(downloadResp.Body)
		return nil, "", fmt.Errorf("media download returned %s: %s", downloadResp.Status, string(downloadBody))
	}
	data, err := io.ReadAll(downloadResp.Body)
	if err != nil {
		return nil, "", err
	}
	mimeType := metadata.MimeType
	if mimeType == "" {
		mimeType = downloadResp.Header.Get("Content-Type")
	}
	return data, mimeType, nil
}

func (s *Service) SendMedia(ctx context.Context, recipient, fileName, mimeType string, data []byte, image bool) error {
	mediaID, err := s.uploadMedia(ctx, fileName, mimeType, data)
	if err != nil {
		return err
	}
	messageType := "document"
	mediaKey := "document"
	if image {
		messageType = "image"
		mediaKey = "image"
	}
	payload, _ := json.Marshal(map[string]any{
		"messaging_product": "whatsapp",
		"to":                recipient,
		"type":              messageType,
		mediaKey:            map[string]string{"id": mediaID},
	})
	return s.sendJSON(ctx, "messages", payload)
}

func (s *Service) uploadMedia(ctx context.Context, fileName, mimeType string, data []byte) (string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("messaging_product", "whatsapp")
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, fileName))
	header.Set("Content-Type", mimeType)
	part, err := writer.CreatePart(header)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(data); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}
	url := fmt.Sprintf("https://graph.facebook.com/%s/%s/media", s.graphVersion, s.phoneID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	responseBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("media upload returned %s: %s", resp.Status, string(responseBody))
	}
	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return "", err
	}
	if result.ID == "" {
		return "", fmt.Errorf("media upload returned no media ID")
	}
	return result.ID, nil
}

func (s *Service) sendJSON(ctx context.Context, endpoint string, payload []byte) error {
	url := fmt.Sprintf("https://graph.facebook.com/%s/%s/%s", s.graphVersion, s.phoneID, endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("whatsapp API returned %s: %s", resp.Status, string(body))
	}
	return nil
}

type Incoming struct {
	Entry []struct {
		Changes []struct {
			Value struct {
				Messages []struct {
					ID, From, Type, Timestamp string
					Text                      struct {
						Body string `json:"body"`
					} `json:"text"`
					Document *struct {
						ID       string `json:"id"`
						Filename string `json:"filename"`
						MimeType string `json:"mime_type"`
						SHA256   string `json:"sha256"`
						Caption  string `json:"caption"`
					} `json:"document"`
					Image *struct {
						ID       string `json:"id"`
						MimeType string `json:"mime_type"`
						SHA256   string `json:"sha256"`
						Caption  string `json:"caption"`
					} `json:"image"`
				} `json:"messages"`
			} `json:"value"`
		} `json:"changes"`
	} `json:"entry"`
}
