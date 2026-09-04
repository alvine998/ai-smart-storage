package telegram

import (
	"bytes"
	"context"
	"crypto/hmac"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Service struct {
	botToken      string
	webhookSecret string
	client        *http.Client
}

func New(botToken, webhookSecret string) *Service {
	return &Service{
		botToken:      botToken,
		webhookSecret: webhookSecret,
		client:        &http.Client{Timeout: 30 * time.Second},
	}
}

func (s *Service) IsConfigured() bool {
	return s != nil && s.botToken != ""
}

func (s *Service) apiURL(method string) string {
	return "https://api.telegram.org/bot" + s.botToken + "/" + method
}

// SetWebhook registers the given URL as the bot's webhook with Telegram.
func (s *Service) SetWebhook(ctx context.Context, webhookURL string) error {
	payload, _ := json.Marshal(map[string]any{
		"url":                  webhookURL,
		"secret_token":         s.webhookSecret,
		"allowed_updates":      []string{"message", "contact", "document", "photo"},
		"drop_pending_updates": true,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.apiURL("setWebhook"), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("setWebhook request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("setWebhook returned %s: %s", resp.Status, string(body))
	}
	var result struct {
		Ok          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("setWebhook decode: %w", err)
	}
	if !result.Ok {
		return fmt.Errorf("setWebhook error: %s", result.Description)
	}
	return nil
}

// SendMessage sends a text message to the given chat ID.
func (s *Service) SendMessage(ctx context.Context, chatID int64, text string) error {
	payload, _ := json.Marshal(map[string]any{
		"chat_id": chatID,
		"text":    text,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.apiURL("sendMessage"), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("sendMessage returned %s: %s", resp.Status, string(body))
	}
	return nil
}

// ValidateSecret checks the X-Telegram-Bot-Api-Secret-Token header.
func (s *Service) ValidateSecret(token string) bool {
	if s.webhookSecret == "" {
		return true
	}
	return hmac.Equal([]byte(token), []byte(s.webhookSecret))
}

// Incoming is the Telegram Update object for message events.
type Incoming struct {
	UpdateID int `json:"update_id"`
	Message  *struct {
		MessageID int `json:"message_id"`
		From      struct {
			ID           int64  `json:"id"`
			FirstName    string `json:"first_name"`
			LastName     string `json:"last_name"`
			Username     string `json:"username"`
			LanguageCode string `json:"language_code"`
		} `json:"from"`
		Chat struct {
			ID   int64  `json:"id"`
			Type string `json:"type"`
		} `json:"chat"`
		Date     int64  `json:"date"`
		Text     string `json:"text"`
		Caption  string `json:"caption"`
		Contact  *struct {
			PhoneNumber string `json:"phone_number"`
			FirstName   string `json:"first_name"`
			LastName    string `json:"last_name"`
			UserID      int64  `json:"user_id"`
		} `json:"contact"`
		Document *struct {
			FileID   string `json:"file_id"`
			FileName string `json:"file_name"`
			MimeType string `json:"mime_type"`
			FileSize int64  `json:"file_size"`
		} `json:"document"`
		Photo []struct {
			FileID   string `json:"file_id"`
			FileSize int64  `json:"file_size"`
			Width    int    `json:"width"`
			Height   int    `json:"height"`
		} `json:"photo"`
	} `json:"message"`
}

// DownloadFile downloads a file from Telegram by file_id.
func (s *Service) DownloadFile(ctx context.Context, fileID string) ([]byte, string, string, error) {
	getReq, _ := json.Marshal(map[string]string{"file_id": fileID})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.apiURL("getFile"), bytes.NewReader(getReq))
	if err != nil {
		return nil, "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, "", "", fmt.Errorf("getFile request: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, "", "", fmt.Errorf("getFile returned %s: %s", resp.Status, string(respBody))
	}
	var fileResp struct {
		Ok     bool `json:"ok"`
		Result struct {
			FileID   string `json:"file_id"`
			FilePath string `json:"file_path"`
			FileSize int64  `json:"file_size"`
		} `json:"result"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(respBody, &fileResp); err != nil {
		return nil, "", "", fmt.Errorf("getFile decode: %w", err)
	}
	if !fileResp.Ok {
		return nil, "", "", fmt.Errorf("getFile error: %s", fileResp.Description)
	}
	dlURL := "https://api.telegram.org/file/bot" + s.botToken + "/" + fileResp.Result.FilePath
	dlReq, err := http.NewRequestWithContext(ctx, http.MethodGet, dlURL, nil)
	if err != nil {
		return nil, "", "", err
	}
	dlResp, err := s.client.Do(dlReq)
	if err != nil {
		return nil, "", "", fmt.Errorf("download file: %w", err)
	}
	defer dlResp.Body.Close()
	if dlResp.StatusCode >= 300 {
		dlBody, _ := io.ReadAll(dlResp.Body)
		return nil, "", "", fmt.Errorf("download returned %s: %s", dlResp.Status, string(dlBody))
	}
	data, err := io.ReadAll(dlResp.Body)
	if err != nil {
		return nil, "", "", fmt.Errorf("read file: %w", err)
	}
	mime := dlResp.Header.Get("Content-Type")
	if mime == "" {
		mime = "application/octet-stream"
	}
	fileName := fileResp.Result.FilePath
	if idx := strings.LastIndex(fileName, "/"); idx >= 0 {
		fileName = fileName[idx+1:]
	}
	return data, fileName, mime, nil
}

// SendContactRequest sends a message with a reply keyboard that asks the user to share their phone number.
func (s *Service) SendContactRequest(ctx context.Context, chatID int64) error {
	payload, _ := json.Marshal(map[string]any{
		"chat_id":      chatID,
		"text":         "Tap the button below to share your phone number:",
		"reply_markup": map[string]any{"keyboard": [][]map[string]any{{{ "text": "Share Phone Number", "request_contact": true }}}, "one_time_keyboard": true, "resize_keyboard": true},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.apiURL("sendMessage"), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("sendMessage returned %s: %s", resp.Status, string(body))
	}
	return nil
}

// HideKeyboard removes the reply keyboard.
func (s *Service) HideKeyboard(ctx context.Context, chatID int64) error {
	payload, _ := json.Marshal(map[string]any{
		"chat_id":      chatID,
		"text":         "Done!",
		"reply_markup": map[string]any{"remove_keyboard": true},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.apiURL("sendMessage"), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("sendMessage returned %s: %s", resp.Status, string(body))
	}
	return nil
}

// VerifyWebhookSignature validates the secret token header from Telegram.
func VerifyWebhookSignature(secret, headerValue string) bool {
	if secret == "" {
		return true
	}
	return hmac.Equal([]byte(headerValue), []byte(secret))
}

// SendDocument sends a file to the given chat ID via Telegram Bot API multipart upload.
func (s *Service) SendDocument(ctx context.Context, chatID int64, fileName string, data []byte, mimeType, caption string) error {
	var buf bytes.Buffer
	boundary := "----GoBoundary" + fmt.Sprintf("%d", time.Now().UnixNano())
	writeField := func(name, value string) {
		fmt.Fprintf(&buf, "--%s\r\n", boundary)
		fmt.Fprintf(&buf, "Content-Disposition: form-data; name=\"%s\"\r\n\r\n", name)
		fmt.Fprintf(&buf, "%s\r\n", value)
	}
	writeField("chat_id", fmt.Sprintf("%d", chatID))
	if caption != "" {
		writeField("caption", caption)
	}
	fmt.Fprintf(&buf, "--%s\r\n", boundary)
	fmt.Fprintf(&buf, "Content-Disposition: form-data; name=\"document\"; filename=\"%s\"\r\n", fileName)
	fmt.Fprintf(&buf, "Content-Type: %s\r\n\r\n", mimeType)
	buf.Write(data)
	fmt.Fprintf(&buf, "\r\n--%s--\r\n", boundary)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.apiURL("sendDocument"), &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("sendDocument returned %s: %s", resp.Status, string(body))
	}
	return nil
}
