package whatsapp

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Service struct {
	token, appSecret, phoneID, graphVersion string
	client                                  *http.Client
}

func New(token, appSecret, phoneID, graphVersion string) *Service {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	return &Service{token: token, appSecret: appSecret, phoneID: phoneID, graphVersion: graphVersion, client: client}
}

func (s *Service) Verify(mode, challenge, verifyToken string) (string, error) {
	if mode != "subscribe" || verifyToken != s.token {
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
		return fmt.Errorf("whatsapp API returned %s", resp.Status)
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
