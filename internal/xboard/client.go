package xboard

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"xboard_link_3x-ui/internal/config"
)

type Client struct {
	baseURL  string
	token    string
	nodeID   int64
	nodeType string
	http     *http.Client
}

type User struct {
	ID          int64  `json:"id"`
	UUID        string `json:"uuid"`
	SpeedLimit  int64  `json:"speed_limit"`
	DeviceLimit int    `json:"device_limit"`
}

type usersResponse struct {
	Users []User `json:"users"`
}

func NewClient(cfg config.XboardConfig) (*Client, error) {
	return &Client{
		baseURL:  cfg.BaseURL,
		token:    cfg.Token,
		nodeID:   cfg.NodeID,
		nodeType: cfg.NodeType,
		http: &http.Client{
			Timeout: cfg.Timeout(),
		},
	}, nil
}

func (c *Client) Users(ctx context.Context, etag string) ([]User, string, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint("/api/v1/server/UniProxy/user"), nil)
	if err != nil {
		return nil, "", false, err
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return nil, etag, true, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, "", false, fmt.Errorf("xboard users status %d: %s", resp.StatusCode, string(body))
	}

	var data usersResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, "", false, err
	}
	return data.Users, resp.Header.Get("ETag"), false, nil
}

func (c *Client) PushTraffic(ctx context.Context, traffic map[int64][2]uint64) error {
	if len(traffic) == 0 {
		return nil
	}

	payload := make(map[string][2]uint64, len(traffic))
	for uid, value := range traffic {
		payload[strconv.FormatInt(uid, 10)] = value
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint("/api/v1/server/UniProxy/push"), bytes.NewReader(body))
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
		return fmt.Errorf("xboard push status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func (c *Client) endpoint(path string) string {
	u, _ := url.Parse(c.baseURL + path)
	q := u.Query()
	q.Set("token", c.token)
	q.Set("node_id", strconv.FormatInt(c.nodeID, 10))
	q.Set("node_type", c.nodeType)
	u.RawQuery = q.Encode()
	return u.String()
}
