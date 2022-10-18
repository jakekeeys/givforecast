package givenergy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
)

const (
	geCloudV1BaseURL                  = "https://api.givenergy.cloud/v1"
	ACUpperChargeLimitSettingID       = 77
	ACUpperChargeLimitEnableSettingID = 17
)

type Client struct {
	c       *http.Client
	serials []string
	apiKey  string
}

func NewClient(serials []string, apiKey string) *Client {
	jar, err := cookiejar.New(nil)
	if err != nil {
		panic(err)
	}
	client := http.Client{
		Jar: jar,
	}

	return &Client{
		c:       &client,
		serials: serials,
		apiKey:  apiKey,
	}
}

func (c *Client) SetChargeUpperLimit(limit int) error {
	for _, serial := range c.serials {
		if limit == 100 {
			err := c.sendModifySettingRequest(serial, ACUpperChargeLimitEnableSettingID, false)
			if err != nil {
				return err
			}
		} else {
			err := c.sendModifySettingRequest(serial, ACUpperChargeLimitEnableSettingID, true)
			if err != nil {
				return err
			}
		}

		err := c.sendModifySettingRequest(serial, ACUpperChargeLimitSettingID, limit)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *Client) sendModifySettingRequest(serial string, id int, value interface{}) error {
	type ModifySettingRequest struct {
		Value interface{} `json:"value"`
	}

	type ModifySettingResponse struct {
		Data struct {
			Value   interface{} `json:"value"`
			Success bool        `json:"success"`
			Message string      `json:"message"`
		} `json:"data"`
	}

	msr := ModifySettingRequest{Value: value}

	b, err := json.Marshal(msr)
	if err != nil {
		return err
	}
	br := bytes.NewReader(b)

	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/inverter/%s/settings/%d/write", geCloudV1BaseURL, serial, id), br)
	if err != nil {
		return err
	}

	resp, err := c.doRequest(req)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("unexpected response code %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	resp.Body.Close()

	msResp := ModifySettingResponse{}
	err = json.Unmarshal(body, &msResp)
	if err != nil {
		return err
	}

	if !msResp.Data.Success {
		return fmt.Errorf("error writing setting %d for %s, value %s, message %s", id, serial, value, msResp.Data.Message)
	}

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
