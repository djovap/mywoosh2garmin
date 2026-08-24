package garmin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const apiUserAgent = "GCM-iOS-5.19.1.2"

// Client manages authentication and uploads to Garmin Connect.
type Client struct {
	OAuth1   *OAuth1Token
	OAuth2   *OAuth2Token
	Domain   string
	TokenDir string // directory where tokens are cached
}

// NewClient creates a Client that caches tokens in tokenDir.
func NewClient(tokenDir string) *Client {
	return &Client{
		Domain:   "garmin.com",
		TokenDir: tokenDir,
	}
}

// Resume tries to load cached tokens and refresh if needed.
// Returns nil if a valid session was restored, error otherwise.
func (c *Client) Resume() error {
	if c.TokenDir == "" {
		return fmt.Errorf("no token directory configured")
	}

	oauth1, oauth2, err := LoadTokens(c.TokenDir)
	if err != nil {
		return fmt.Errorf("no cached session: %w", err)
	}

	c.OAuth1 = oauth1
	c.OAuth2 = oauth2
	if oauth1.Domain != "" {
		c.Domain = oauth1.Domain
	}

	// Token still valid
	if !oauth2.Expired() {
		return nil
	}

	// OAuth2 expired — try to refresh using OAuth1 (lasts ~1 year)
	fmt.Println("  session expired, refreshing...")
	if err := c.refreshOAuth2(); err != nil {
		return fmt.Errorf("refresh failed: %w", err)
	}
	return nil
}

// Login performs a fresh SSO login with the given credentials.
func (c *Client) Login(email, password string) error {
	oauth1, oauth2, err := Login(email, password, c.Domain)
	if err != nil {
		return err
	}

	c.OAuth1 = oauth1
	c.OAuth2 = oauth2

	// Cache tokens for next time
	if c.TokenDir != "" {
		if err := SaveTokens(c.TokenDir, oauth1, oauth2); err != nil {
			fmt.Printf("  warning: could not cache tokens: %v\n", err)
		}
	}
	return nil
}

// UnitIDForDevice finds the FIT unit ID of a registered Garmin device. Garmin
// Connect's device-service product ID is not always the FIT profile product ID,
// so productName is used as an exact model-name fallback. FIT calls unitId
// serial_number.
func (c *Client) UnitIDForDevice(productID uint16, productName string) (uint32, error) {
	if c.OAuth2 == nil {
		return 0, fmt.Errorf("not authenticated")
	}

	url := fmt.Sprintf("https://connectapi.%s/device-service/deviceregistration/devices", c.Domain)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, fmt.Errorf("create device request: %w", err)
	}
	req.Header.Set("Authorization", c.OAuth2.Bearer())
	req.Header.Set("User-Agent", apiUserAgent)
	req.Header.Set("DI-Backend", fmt.Sprintf("connectapi.%s", c.Domain))
	req.Header.Set("NK", "NT")

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return 0, fmt.Errorf("request registered devices: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("read registered devices: %w", err)
	}
	if resp.StatusCode >= 400 {
		return 0, fmt.Errorf("fetch registered devices failed (HTTP %d): %s", resp.StatusCode, string(body[:min(300, len(body))]))
	}

	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var devices any
	if err := decoder.Decode(&devices); err != nil {
		return 0, fmt.Errorf("parse registered devices: %w", err)
	}
	unitID, ok := findUnitIDForDevice(devices, productID, productName)
	if !ok {
		return 0, fmt.Errorf("no registered Garmin device found with FIT product ID %d or model %q", productID, productName)
	}
	return unitID, nil
}

// findUnitIDForDevice searches Garmin's undocumented, versioned device-list
// response for an object containing a FIT unit ID and either the matching
// product ID or exact model display name.
func findUnitIDForDevice(value any, productID uint16, productName string) (uint32, bool) {
	switch value := value.(type) {
	case []any:
		for _, item := range value {
			if unitID, ok := findUnitIDForDevice(item, productID, productName); ok {
				return unitID, true
			}
		}
	case map[string]any:
		var product, unitID uint64
		var name string
		var hasProduct, hasUnitID bool
		for key, fieldValue := range value {
			switch normalizeDeviceField(key) {
			case "productid", "garminproduct":
				product, hasProduct = jsonUint(fieldValue)
			case "product", "productdisplayname", "productname", "modelname":
				if id, ok := jsonUint(fieldValue); ok {
					product, hasProduct = id, true
				} else if text, ok := fieldValue.(string); ok {
					name = text
				}
			case "unitid":
				unitID, hasUnitID = jsonUint(fieldValue)
			}
		}
		matchesProduct := hasProduct && product == uint64(productID)
		matchesName := normalizeDeviceName(name) != "" && normalizeDeviceName(name) == normalizeDeviceName(productName)
		if hasUnitID && (matchesProduct || matchesName) && unitID > 0 && unitID <= uint64(^uint32(0)) {
			return uint32(unitID), true
		}
		for _, child := range value {
			if unitID, ok := findUnitIDForDevice(child, productID, productName); ok {
				return unitID, true
			}
		}
	}
	return 0, false
}

func normalizeDeviceField(field string) string {
	return strings.NewReplacer("_", "", "-", "", " ", "").Replace(strings.ToLower(field))
}

func normalizeDeviceName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.TrimPrefix(name, "garmin ")
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return -1
	}, name)
}

func jsonUint(value any) (uint64, bool) {
	switch value := value.(type) {
	case json.Number:
		number, err := strconv.ParseUint(value.String(), 10, 64)
		return number, err == nil
	case string:
		number, err := strconv.ParseUint(value, 10, 64)
		return number, err == nil
	case float64:
		if value < 0 || value != float64(uint64(value)) {
			return 0, false
		}
		return uint64(value), true
	default:
		return 0, false
	}
}

// UploadFIT uploads a FIT file to Garmin Connect.
// Automatically refreshes the OAuth2 token if expired.
func (c *Client) UploadFIT(filePath string) error {
	if c.OAuth2 == nil {
		return fmt.Errorf("not authenticated")
	}

	// Auto-refresh if expired
	if c.OAuth2.Expired() {
		if err := c.refreshOAuth2(); err != nil {
			return fmt.Errorf("token refresh: %w", err)
		}
	}

	// First attempt
	status, body, err := c.doUpload(filePath)
	if err != nil {
		return err
	}

	// Retry once on 401 (token might be stale despite not being expired)
	if status == 401 {
		fmt.Println("  token rejected, refreshing...")
		if err := c.refreshOAuth2(); err != nil {
			return fmt.Errorf("token refresh: %w", err)
		}
		status, body, err = c.doUpload(filePath)
		if err != nil {
			return err
		}
	}

	return parseUploadResult(status, body)
}

// refreshOAuth2 exchanges the OAuth1 token for a fresh OAuth2 token.
func (c *Client) refreshOAuth2() error {
	oauth2, err := ExchangeForOAuth2(c.OAuth1)
	if err != nil {
		return err
	}
	c.OAuth2 = oauth2

	// Update cache (only OAuth2, keep OAuth1 as-is)
	if c.TokenDir != "" {
		_ = SaveTokens(c.TokenDir, nil, oauth2)
	}
	return nil
}

// doUpload performs the actual multipart upload and returns status + body.
func (c *Client) doUpload(filePath string) (int, []byte, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return 0, nil, err
	}
	defer f.Close()

	// Build multipart request
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return 0, nil, err
	}
	if _, err := io.Copy(part, f); err != nil {
		return 0, nil, err
	}
	writer.Close()

	uploadURL := fmt.Sprintf("https://connectapi.%s/upload-service/upload", c.Domain)
	req, err := http.NewRequest("POST", uploadURL, &buf)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", c.OAuth2.Bearer())
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("User-Agent", apiUserAgent)
	req.Header.Set("DI-Backend", fmt.Sprintf("connectapi.%s", c.Domain))
	req.Header.Set("NK", "NT")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("upload request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, err
	}

	return resp.StatusCode, body, nil
}

// parseUploadResult checks the upload response for errors.
func parseUploadResult(status int, body []byte) error {
	if status == 409 {
		return fmt.Errorf("duplicate activity (already uploaded to Garmin)")
	}
	if status >= 400 {
		return fmt.Errorf("upload failed (HTTP %d): %s", status,
			string(body[:min(300, len(body))]))
	}

	// Check for failures in the detailed result
	var result struct {
		DetailedImportResult struct {
			Failures  []interface{} `json:"failures"`
			Successes []interface{} `json:"successes"`
		} `json:"detailedImportResult"`
	}
	if err := json.Unmarshal(body, &result); err == nil {
		if len(result.DetailedImportResult.Failures) > 0 {
			return fmt.Errorf("upload reported failures: %v",
				result.DetailedImportResult.Failures)
		}
	}

	return nil
}
