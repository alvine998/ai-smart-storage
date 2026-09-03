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
	"net/http"
	"time"
)

type Service struct {
	token, verifyToken, appSecret, phoneID, graphVersion string
	client                                                *http.Client
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
	url := fmt.Sprintf("https://graph.facebook.com/%s/%s/messages", s.graphVersion, s.phoneID)
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
					ID, From, Type string
					Text           struct {
						Body string `json:"body"`
					} `json:"text"`
				} `json:"messages"`
			} `json:"value"`
		} `json:"changes"`
	} `json:"entry"`
}
