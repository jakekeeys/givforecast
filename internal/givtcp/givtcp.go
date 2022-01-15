package givtcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

type Client struct {
	c       *http.Client
	baseURL string
}

func NewClient() *Client {
	return &Client{
		c:       http.DefaultClient,
		baseURL: "http://givtcp:80", // todo make config
	}
}

func (c *Client) SetChargeTarget(target int) error {
	type SetChargeTargetRequest struct {
		ChargeToPercent int `json:"chargeToPercent"`
	}

	req := SetChargeTargetRequest{ChargeToPercent: target}
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}

	type SetChargeTargetResponse struct {
		Result string `json:"result"`
	}

	resp, err := c.c.Post(fmt.Sprintf("%s/setChargeTarget", c.baseURL), "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}

	var setChargeTargetResponse SetChargeTargetResponse
	err = json.NewDecoder(resp.Body).Decode(&setChargeTargetResponse)
	if err != nil {
		return err
	}

	if setChargeTargetResponse.Result != "Setting Charge Target was a success" {
		return errors.New("error setting charge target via givtcp")
	}

	return nil
}
