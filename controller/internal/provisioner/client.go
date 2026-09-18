package provisioner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	StateProvisioning = "PROVISIONING"
	StateReady        = "READY"
	StateFailed       = "FAILED"
)

var (
	ErrUnavailable = errors.New("provisioner: unavailable, safe to retry")
	ErrNotFound    = errors.New("provisioner: not found")
	ErrAmbiguous   = errors.New("provisioner: create outcome unknown, do not retry")
)

type Database struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Engine   string `json:"engine"`
	SizeGB   int    `json:"sizeGB"`
	State    string `json:"state"`
	Endpoint string `json:"endpoint,omitempty"`
}

type Client struct {
	baseURL string
	http    *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) do(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("provisioner: build %s request: %w", method, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.http.Do(req)
}

type createRequest struct {
	Name   string `json:"name"`
	Engine string `json:"engine"`
	SizeGB int    `json:"sizeGB"`
}

func (c *Client) Create(ctx context.Context, name, engine string, sizeGB int) (Database, error) {
	body, err := json.Marshal(createRequest{Name: name, Engine: engine, SizeGB: sizeGB})
	if err != nil {
		return Database{}, fmt.Errorf("provisioner: encode create request: %w", err)
	}

	resp, err := c.do(ctx, http.MethodPost, "/databases", bytes.NewReader(body))
	if err != nil {
		return Database{}, fmt.Errorf("%w: %v", ErrAmbiguous, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusServiceUnavailable {
		return Database{}, ErrUnavailable
	}
	if resp.StatusCode != http.StatusCreated {
		return Database{}, fmt.Errorf("%w: create returned %s", ErrAmbiguous, resp.Status)
	}

	var db Database
	if err := json.NewDecoder(resp.Body).Decode(&db); err != nil {
		return Database{}, fmt.Errorf("%w: decode create response: %v", ErrAmbiguous, err)
	}
	return db, nil
}

func (c *Client) Get(ctx context.Context, id string) (Database, error) {
	resp, err := c.do(ctx, http.MethodGet, "/databases/"+id, nil)
	if err != nil {
		return Database{}, fmt.Errorf("provisioner: get: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var db Database
		if err := json.NewDecoder(resp.Body).Decode(&db); err != nil {
			return Database{}, fmt.Errorf("provisioner: decode get response: %w", err)
		}
		return db, nil
	case http.StatusNotFound:
		return Database{}, ErrNotFound
	default:
		return Database{}, fmt.Errorf("provisioner: get returned %s", resp.Status)
	}
}

func (c *Client) Delete(ctx context.Context, id string) error {
	resp, err := c.do(ctx, http.MethodDelete, "/databases/"+id, nil)
	if err != nil {
		return fmt.Errorf("provisioner: delete: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	switch resp.StatusCode {
	case http.StatusNoContent, http.StatusNotFound:
		return nil
	default:
		return fmt.Errorf("provisioner: delete returned %s", resp.Status)
	}
}
