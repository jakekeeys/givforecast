package assist

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	ctx    context.Context
	c      *http.Client
	apiKey string
	apiURL string
}

func New(ctx context.Context, c *http.Client, apiKey string, apiURL string) *Client {
	return &Client{
		ctx:    ctx,
		c:      c,
		apiKey: apiKey,
		apiURL: apiURL,
	}
}

type State struct {
	EntityID     string          `json:"entity_id"`
	State        string          `json:"state"`
	Attributes   json.RawMessage `json:"attributes"`
	LastChanged  time.Time       `json:"last_changed"`
	LastReported time.Time       `json:"last_reported"`
	LastUpdated  time.Time       `json:"last_updated"`
	Context      json.RawMessage `json:"context"`
}

func (c *Client) GetState(entityID string) (*State, error) {
	var state State
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/states/%s", c.apiURL, entityID), nil)
	if err != nil {
		return &state, err
	}

	resp, err := c.doRequest(req)
	if err != nil {
		return &state, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &state, err
	}

	err = json.Unmarshal(body, &state)
	if err != nil {
		return &state, err
	}

	return &state, nil
}

func (c *Client) SetState(state *State) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/states/%s", c.apiURL, state.EntityID), bytes.NewBuffer(data))
	if err != nil {
		return err
	}

	resp, err := c.doRequest(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

func (c *Client) SetNumberValue(entityID string, value string) error {
	type setNumberValue struct {
		EntityID string `json:"entity_id"`
		Value    string `json:"value"`
	}

	data, err := json.Marshal(setNumberValue{
		EntityID: entityID,
		Value:    value,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/services/number/set_value", c.apiURL), bytes.NewBuffer(data))
	if err != nil {
		return err
	}

	resp, err := c.doRequest(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

func (c *Client) SetSelectOption(entityID string, option string) error {
	type setSelectOption struct {
		EntityID string `json:"entity_id"`
		Option   string `json:"option"`
	}

	data, err := json.Marshal(setSelectOption{
		EntityID: entityID,
		Option:   option,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/services/select/select_option", c.apiURL), bytes.NewBuffer(data))
	if err != nil {
		return err
	}

	resp, err := c.doRequest(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

func (c *Client) doRequest(r *http.Request) (*http.Response, error) {
	r.Header.Add("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))
	r.Header.Add("Content-Type", "application/json")
	r.Header.Add("Accept", "application/json")

	resp, err := c.c.Do(r)
	if err != nil {
		return nil, err
	}

	return resp, nil
}
