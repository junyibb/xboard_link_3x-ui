package xui

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"

	"xboard_link_3x-ui/internal/config"
)

type Client struct {
	baseURL       string
	apiToken      string
	username      string
	password      string
	twoFactorCode string
	http          *http.Client
	loggedIn      bool
	mode          apiMode
}

type apiMode int

const (
	modeAuto apiMode = iota
	modeClients
	modeInbounds
)

type APIResponse struct {
	Success bool            `json:"success"`
	Msg     string          `json:"msg"`
	Obj     json.RawMessage `json:"obj"`
}

type ManagedClient struct {
	ID         int64       `json:"id,omitempty"`
	Email      string      `json:"email"`
	SubID      string      `json:"subId,omitempty"`
	UUID       string      `json:"uuid,omitempty"`
	Password   string      `json:"password,omitempty"`
	Protocol   string      `json:"protocol,omitempty"`
	TotalGB    int64       `json:"totalGB"`
	ExpiryTime int64       `json:"expiryTime"`
	Enable     bool        `json:"enable"`
	TgID       int64       `json:"tgId"`
	LimitIP    int         `json:"limitIp"`
	Comment    string      `json:"comment,omitempty"`
	InboundIDs []int       `json:"inboundIds,omitempty"`
	Traffic    TrafficInfo `json:"traffic,omitempty"`
	Reverse    any         `json:"reverse,omitempty"`
}

type TrafficInfo struct {
	Email      string `json:"email,omitempty"`
	Up         uint64 `json:"up"`
	Down       uint64 `json:"down"`
	Total      int64  `json:"total,omitempty"`
	ExpiryTime int64  `json:"expiryTime,omitempty"`
	Enable     bool   `json:"enable,omitempty"`
}

type statusError struct {
	status int
	body   string
}

func (e statusError) Error() string {
	return fmt.Sprintf("status %d: %s", e.status, e.body)
}

func NewClient(cfg config.XUIConfig) (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if cfg.InsecureTLS {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}

	return &Client{
		baseURL:       cfg.BaseURL,
		apiToken:      cfg.APIToken,
		username:      cfg.Username,
		password:      cfg.Password,
		twoFactorCode: cfg.TwoFactorCode,
		http: &http.Client{
			Timeout:   cfg.Timeout(),
			Jar:       jar,
			Transport: transport,
		},
	}, nil
}

func (c *Client) ListClients(ctx context.Context) ([]ManagedClient, error) {
	if c.mode == modeInbounds {
		return c.listClientsFromInbounds(ctx)
	}

	var clients []ManagedClient
	if err := c.do(ctx, http.MethodGet, "/panel/api/clients/list", nil, &clients); err != nil {
		if isStatus(err, http.StatusNotFound) && c.mode == modeAuto {
			c.mode = modeInbounds
			return c.listClientsFromInbounds(ctx)
		}
		return nil, err
	}
	c.mode = modeClients
	return clients, nil
}

func (c *Client) AddClient(ctx context.Context, client ManagedClient, inboundIDs []int) error {
	if c.mode == modeInbounds {
		return c.addClientToInbounds(ctx, client, inboundIDs)
	}

	body := map[string]any{
		"client":     clientPayload(client),
		"inboundIds": inboundIDs,
	}
	if err := c.do(ctx, http.MethodPost, "/panel/api/clients/add", body, nil); err != nil {
		if isStatus(err, http.StatusNotFound) && c.mode == modeAuto {
			c.mode = modeInbounds
			return c.addClientToInbounds(ctx, client, inboundIDs)
		}
		return err
	}
	c.mode = modeClients
	return nil
}

func (c *Client) UpdateClient(ctx context.Context, oldEmail string, client ManagedClient) error {
	if c.mode == modeInbounds {
		return c.updateClientInInbounds(ctx, oldEmail, client)
	}

	if err := c.do(ctx, http.MethodPost, "/panel/api/clients/update/"+url.PathEscape(oldEmail), clientPayload(client), nil); err != nil {
		if isStatus(err, http.StatusNotFound) && c.mode == modeAuto {
			c.mode = modeInbounds
			return c.updateClientInInbounds(ctx, oldEmail, client)
		}
		return err
	}
	c.mode = modeClients
	return nil
}

func (c *Client) DeleteClient(ctx context.Context, email string, keepTraffic bool) error {
	if c.mode == modeInbounds {
		return c.deleteClientFromInbounds(ctx, email)
	}

	keep := 0
	if keepTraffic {
		keep = 1
	}
	path := "/panel/api/clients/del/" + url.PathEscape(email) + "?keepTraffic=" + strconv.Itoa(keep)
	if err := c.do(ctx, http.MethodPost, path, nil, nil); err != nil {
		if isStatus(err, http.StatusNotFound) && c.mode == modeAuto {
			c.mode = modeInbounds
			return c.deleteClientFromInbounds(ctx, email)
		}
		return err
	}
	c.mode = modeClients
	return nil
}

func (c *Client) AttachClient(ctx context.Context, email string, inboundIDs []int) error {
	if c.mode == modeInbounds {
		client, err := c.getLegacyClient(ctx, email)
		if err != nil {
			return err
		}
		return c.addClientToInbounds(ctx, client, inboundIDs)
	}

	body := map[string]any{"inboundIds": inboundIDs}
	if err := c.do(ctx, http.MethodPost, "/panel/api/clients/"+url.PathEscape(email)+"/attach", body, nil); err != nil {
		if isStatus(err, http.StatusNotFound) && c.mode == modeAuto {
			c.mode = modeInbounds
			client, getErr := c.getLegacyClient(ctx, email)
			if getErr != nil {
				return getErr
			}
			return c.addClientToInbounds(ctx, client, inboundIDs)
		}
		return err
	}
	c.mode = modeClients
	return nil
}

func (c *Client) DetachClient(ctx context.Context, email string, inboundIDs []int) error {
	if c.mode == modeInbounds {
		return c.deleteClientFromSpecificInbounds(ctx, email, inboundIDs)
	}

	body := map[string]any{"inboundIds": inboundIDs}
	if err := c.do(ctx, http.MethodPost, "/panel/api/clients/"+url.PathEscape(email)+"/detach", body, nil); err != nil {
		if isStatus(err, http.StatusNotFound) && c.mode == modeAuto {
			c.mode = modeInbounds
			return c.deleteClientFromSpecificInbounds(ctx, email, inboundIDs)
		}
		return err
	}
	c.mode = modeClients
	return nil
}

func (c *Client) Traffic(ctx context.Context, email string) (TrafficInfo, error) {
	if c.mode == modeInbounds {
		return c.legacyTraffic(ctx, email)
	}

	var traffic TrafficInfo
	if err := c.do(ctx, http.MethodGet, "/panel/api/clients/traffic/"+url.PathEscape(email), nil, &traffic); err != nil {
		if isStatus(err, http.StatusNotFound) && c.mode == modeAuto {
			c.mode = modeInbounds
			return c.legacyTraffic(ctx, email)
		}
		return TrafficInfo{}, err
	}
	c.mode = modeClients
	return traffic, nil
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	if err := c.ensureAuth(ctx); err != nil {
		return err
	}

	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiToken)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("3x-ui %s %s %w", method, path, statusError{status: resp.StatusCode, body: string(respBody)})
	}

	var apiResp APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return err
	}
	if !apiResp.Success {
		return fmt.Errorf("3x-ui %s %s failed: %s", method, path, apiResp.Msg)
	}
	if out != nil && len(apiResp.Obj) > 0 && string(apiResp.Obj) != "null" {
		if err := json.Unmarshal(apiResp.Obj, out); err != nil {
			return err
		}
	}
	return nil
}

type legacyInbound struct {
	ID          int64              `json:"id"`
	Protocol    string             `json:"protocol"`
	Settings    json.RawMessage    `json:"settings"`
	ClientStats []legacyClientStat `json:"clientStats"`
}

type legacySettings struct {
	Clients []legacyClient `json:"clients"`
}

type legacyClient struct {
	ID         string `json:"id,omitempty"`
	Flow       string `json:"flow,omitempty"`
	Email      string `json:"email"`
	LimitIP    int    `json:"limitIp"`
	TotalGB    int64  `json:"totalGB"`
	ExpiryTime int64  `json:"expiryTime"`
	Enable     bool   `json:"enable"`
	TgID       any    `json:"tgId,omitempty"`
	SubID      string `json:"subId,omitempty"`
	Comment    string `json:"comment,omitempty"`
	Reset      int    `json:"reset"`
	Password   string `json:"password,omitempty"`
	Security   string `json:"security,omitempty"`
}

type legacyClientStat struct {
	Email      string `json:"email"`
	Up         uint64 `json:"up"`
	Down       uint64 `json:"down"`
	Total      int64  `json:"total"`
	ExpiryTime int64  `json:"expiryTime"`
	Enable     bool   `json:"enable"`
}

func (c *Client) listClientsFromInbounds(ctx context.Context) ([]ManagedClient, error) {
	inbounds, err := c.listInbounds(ctx)
	if err != nil {
		return nil, err
	}

	byEmail := make(map[string]ManagedClient)
	for _, inbound := range inbounds {
		settings, err := parseLegacySettings(inbound.Settings)
		if err != nil {
			return nil, fmt.Errorf("parse inbound %d settings: %w", inbound.ID, err)
		}

		statsByEmail := make(map[string]legacyClientStat, len(inbound.ClientStats))
		for _, stat := range inbound.ClientStats {
			statsByEmail[strings.ToLower(stat.Email)] = stat
		}

		for _, lc := range settings.Clients {
			if lc.Email == "" {
				continue
			}
			mc := byEmail[lc.Email]
			if mc.Email == "" {
				mc = managedFromLegacy(lc, inbound.Protocol)
			}
			mc.InboundIDs = appendIfMissing(mc.InboundIDs, int(inbound.ID))
			if stat, ok := statsByEmail[strings.ToLower(lc.Email)]; ok {
				mc.Traffic = TrafficInfo{
					Email:      stat.Email,
					Up:         stat.Up,
					Down:       stat.Down,
					Total:      stat.Total,
					ExpiryTime: stat.ExpiryTime,
					Enable:     stat.Enable,
				}
			}
			byEmail[lc.Email] = mc
		}
	}

	clients := make([]ManagedClient, 0, len(byEmail))
	for _, client := range byEmail {
		clients = append(clients, client)
	}
	return clients, nil
}

func (c *Client) listInbounds(ctx context.Context) ([]legacyInbound, error) {
	var inbounds []legacyInbound
	if err := c.do(ctx, http.MethodGet, "/panel/api/inbounds/list", nil, &inbounds); err != nil {
		return nil, err
	}
	return inbounds, nil
}

func (c *Client) addClientToInbounds(ctx context.Context, client ManagedClient, inboundIDs []int) error {
	for _, inboundID := range inboundIDs {
		if err := c.do(ctx, http.MethodPost, "/panel/api/inbounds/addClient", legacyInboundBody(inboundID, client), nil); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) updateClientInInbounds(ctx context.Context, oldEmail string, client ManagedClient) error {
	clients, err := c.listClientsFromInbounds(ctx)
	if err != nil {
		return err
	}
	found := false
	for _, existing := range clients {
		if existing.Email != oldEmail {
			continue
		}
		found = true
		for _, inboundID := range existing.InboundIDs {
			if err := c.do(ctx, http.MethodPost, "/panel/api/inbounds/updateClient/"+url.PathEscape(clientIDForProtocol(existing)), legacyInboundBody(inboundID, client), nil); err != nil {
				return err
			}
		}
	}
	if !found {
		return c.addClientToInbounds(ctx, client, client.InboundIDs)
	}
	return nil
}

func (c *Client) deleteClientFromInbounds(ctx context.Context, email string) error {
	clients, err := c.listClientsFromInbounds(ctx)
	if err != nil {
		return err
	}
	for _, client := range clients {
		if client.Email != email {
			continue
		}
		for _, inboundID := range client.InboundIDs {
			path := fmt.Sprintf("/panel/api/inbounds/%d/delClient/%s", inboundID, url.PathEscape(clientIDForProtocol(client)))
			if err := c.do(ctx, http.MethodPost, path, nil, nil); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *Client) deleteClientFromSpecificInbounds(ctx context.Context, email string, inboundIDs []int) error {
	client, err := c.getLegacyClient(ctx, email)
	if err != nil {
		return err
	}
	for _, inboundID := range inboundIDs {
		path := fmt.Sprintf("/panel/api/inbounds/%d/delClient/%s", inboundID, url.PathEscape(clientIDForProtocol(client)))
		if err := c.do(ctx, http.MethodPost, path, nil, nil); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) getLegacyClient(ctx context.Context, email string) (ManagedClient, error) {
	clients, err := c.listClientsFromInbounds(ctx)
	if err != nil {
		return ManagedClient{}, err
	}
	for _, client := range clients {
		if client.Email == email {
			return client, nil
		}
	}
	return ManagedClient{}, fmt.Errorf("legacy client %s not found", email)
}

func (c *Client) legacyTraffic(ctx context.Context, email string) (TrafficInfo, error) {
	var traffic TrafficInfo
	if err := c.do(ctx, http.MethodGet, "/panel/api/inbounds/getClientTraffics/"+url.PathEscape(email), nil, &traffic); err != nil {
		return TrafficInfo{}, err
	}
	return traffic, nil
}

func parseLegacySettings(raw json.RawMessage) (legacySettings, error) {
	var settings legacySettings
	if len(raw) == 0 || string(raw) == "null" {
		return settings, nil
	}
	if raw[0] == '"' {
		var encoded string
		if err := json.Unmarshal(raw, &encoded); err != nil {
			return settings, err
		}
		if encoded == "" {
			return settings, nil
		}
		return settings, json.Unmarshal([]byte(encoded), &settings)
	}
	return settings, json.Unmarshal(raw, &settings)
}

func managedFromLegacy(client legacyClient, protocol string) ManagedClient {
	return ManagedClient{
		Email:      client.Email,
		SubID:      client.SubID,
		UUID:       client.ID,
		Password:   client.Password,
		Protocol:   protocol,
		TotalGB:    client.TotalGB,
		ExpiryTime: client.ExpiryTime,
		Enable:     client.Enable,
		LimitIP:    client.LimitIP,
		Comment:    client.Comment,
	}
}

func legacyInboundBody(inboundID int, client ManagedClient) map[string]any {
	return map[string]any{
		"id": inboundID,
		"settings": mustJSON(map[string]any{"clients": []map[string]any{
			legacyClientPayload(client),
		}}),
	}
}

func legacyClientPayload(client ManagedClient) map[string]any {
	uuid := client.UUID
	if uuid == "" {
		uuid = client.Password
	}
	if uuid == "" {
		uuid = client.SubID
	}
	payload := map[string]any{
		"id":         uuid,
		"flow":       "",
		"email":      client.Email,
		"limitIp":    client.LimitIP,
		"totalGB":    client.TotalGB,
		"expiryTime": client.ExpiryTime,
		"enable":     client.Enable,
		"tgId":       "",
		"subId":      client.SubID,
		"comment":    client.Comment,
		"reset":      0,
		"password":   client.Password,
	}
	if payload["password"] == "" {
		payload["password"] = uuid
	}
	return payload
}

func clientIDForProtocol(client ManagedClient) string {
	switch strings.ToLower(client.Protocol) {
	case "trojan":
		if client.Password != "" {
			return client.Password
		}
	case "shadowsocks":
		return client.Email
	}
	if client.UUID != "" {
		return client.UUID
	}
	if client.Password != "" {
		return client.Password
	}
	return client.Email
}

func appendIfMissing(values []int, value int) []int {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func mustJSON(value any) string {
	body, _ := json.Marshal(value)
	return string(body)
}

func isStatus(err error, status int) bool {
	var statusErr statusError
	return errors.As(err, &statusErr) && statusErr.status == status
}

func (c *Client) ensureAuth(ctx context.Context) error {
	if c.apiToken != "" || c.loggedIn {
		return nil
	}

	body := map[string]string{
		"username":      c.username,
		"password":      c.password,
		"twoFactorCode": c.twoFactorCode,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/login", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("3x-ui login status %d: %s", resp.StatusCode, string(respBody))
	}

	var apiResp APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return err
	}
	if !apiResp.Success {
		return fmt.Errorf("3x-ui login failed: %s", apiResp.Msg)
	}
	c.loggedIn = true
	return nil
}

func clientPayload(client ManagedClient) map[string]any {
	payload := map[string]any{
		"email":      client.Email,
		"uuid":       client.UUID,
		"id":         client.UUID,
		"password":   client.Password,
		"totalGB":    client.TotalGB,
		"expiryTime": client.ExpiryTime,
		"enable":     client.Enable,
		"tgId":       client.TgID,
		"limitIp":    client.LimitIP,
	}
	if client.SubID != "" {
		payload["subId"] = client.SubID
	}
	if client.Comment != "" {
		payload["comment"] = client.Comment
	}
	return payload
}
