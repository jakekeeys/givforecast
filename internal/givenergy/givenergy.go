package givenergy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"time"
)

const (
	geCloudV1BaseURL                  = "https://api.givenergy.cloud/v1"
	ACUpperChargeLimitSettingID       = 77
	ACUpperChargeLimitEnableSettingID = 17
	EMSChargeSlot1SOCLimit            = 395
)

type Client struct {
	c       *http.Client
	serials []string
	apiKey  string
	ems     bool
}

func NewClient(serials []string, apiKey string, ems bool) *Client {
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
		ems:     ems,
	}
}

type SystemData struct {
	Data struct {
		Time   time.Time `json:"time"`
		Status string    `json:"status"`
		Solar  struct {
			Power  int `json:"power"`
			Arrays []struct {
				Array   int     `json:"array"`
				Voltage float64 `json:"voltage"`
				Current float64 `json:"current"`
				Power   int     `json:"power"`
			} `json:"arrays"`
		} `json:"solar"`
		Grid struct {
			Voltage   float64 `json:"voltage"`
			Current   float64 `json:"current"`
			Power     int     `json:"power"`
			Frequency float64 `json:"frequency"`
		} `json:"grid"`
		Battery struct {
			Percent     int `json:"percent"`
			Power       int `json:"power"`
			Temperature int `json:"temperature"`
		} `json:"battery"`
		Inverter struct {
			Temperature     float64 `json:"temperature"`
			Power           int     `json:"power"`
			OutputVoltage   float64 `json:"output_voltage"`
			OutputFrequency float64 `json:"output_frequency"`
			EpsPower        int     `json:"eps_power"`
		} `json:"inverter"`
		Consumption int `json:"consumption"`
	} `json:"data"`
}

func (c *Client) GetLatestSystemData() (map[string]SystemData, error) {
	sds := make(map[string]SystemData)
	for _, serial := range c.serials {
		req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/inverter/%s/meter-data/latest", geCloudV1BaseURL, serial), nil)
		if err != nil {
			return nil, err
		}

		resp, err := c.doRequest(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("unexpected response code %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		resp.Body.Close()

		sd := SystemData{}
		err = json.Unmarshal(body, &sd)
		if err != nil {
			return nil, err
		}

		sds[serial] = sd
	}

	return nil, nil
}

func (c *Client) SetChargeUpperLimit(limit int) error {
	for _, serial := range c.serials {
		if c.ems {
			err := c.sendModifySettingRequest(serial, EMSChargeSlot1SOCLimit, limit)
			if err != nil {
				return err
			}
		} else {
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
