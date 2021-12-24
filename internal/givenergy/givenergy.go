package givenergy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/cookiejar"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	reqDateFormat         = "2006-01-02"
	measurementTimeFormat = "2006-01-02 15:04:05"
	geCloudBaseURL        = "https://www.givenergy.cloud/GivManage/api"
	geBatteryBaseURL      = "https://api.givenergy.cloud"
)

type Client struct {
	c                   *http.Client
	username            string
	password            string
	serial              string
	apiKey              string
	consumptionAverages *map[time.Time]float64
	m                   sync.RWMutex
}

func NewClient(username, password, serial, apiKey string) *Client {
	jar, err := cookiejar.New(nil)
	if err != nil {
		panic(err)
	}
	client := http.Client{
		Jar: jar,
	}

	return &Client{
		c:        &client,
		username: username,
		password: password,
		serial:   serial,
		apiKey:   apiKey,
	}
}

func (c *Client) Login() error {
	type LoginResponse struct {
		Role      string `json:"role"`
		Success   bool   `json:"success"`
		Inverters []struct {
			SerialNum string `json:"serialNum"`
		} `json:"inverters"`
	}

	post, err := c.c.Post(fmt.Sprintf("%s/login?account=%s&password=%s", geCloudBaseURL, c.username, c.password), "application/json", nil)
	if err != nil {
		return err
	}

	var loginResponse LoginResponse
	err = json.NewDecoder(post.Body).Decode(&loginResponse)
	if err != nil {
		return err
	}

	if !loginResponse.Success {
		return errors.New("error authenticating to ge cloud")
	}

	return nil
}

type GivEnergyResponse struct {
	Success bool `json:"success"`
	MsgCode int  `json:"msgCode"`
}

func (c *Client) doRequest(r *http.Request, retry bool) (*http.Response, error) {
	if strings.HasPrefix(r.URL.String(), geBatteryBaseURL) {
		r.Header.Add("Authorization", c.apiKey)
		r.Header.Add("Content-Type", "application/json")
	}

	resp, err := c.c.Do(r)
	if err != nil {
		return nil, err
	}

	if strings.HasPrefix(r.URL.String(), geCloudBaseURL) {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		resp.Body = ioutil.NopCloser(bytes.NewBuffer(body))

		var givEnergyResponse GivEnergyResponse
		err = json.NewDecoder(bytes.NewBuffer(body)).Decode(&givEnergyResponse)
		if err != nil {
			return nil, err
		}

		if givEnergyResponse.MsgCode == 102 && !retry {
			err := c.Login()
			if err != nil {
				return nil, err
			}
			return c.doRequest(r, true)
		}
	}

	return resp, nil
}

func (c *Client) SetChargeTarget(target int) error {
	type SetChargeTargetRequest struct {
		Value string `json:"value"`
	}

	bodyBytes, err := json.Marshal(&SetChargeTargetRequest{Value: strconv.Itoa(target)})
	if err != nil {
		return err
	}

	request, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/registers/chargeUpTo", geBatteryBaseURL), bytes.NewBuffer(bodyBytes))
	if err != nil {
		return err
	}

	resp, err := c.doRequest(request, false)
	if resp.StatusCode != http.StatusOK {
		return errors.New("error setting charge target via ge battery api")
	}

	return nil
}

type InverterChartDayLineResponse struct {
	AvgValue     float64 `json:"avgValue"`
	MinValueText string  `json:"minValueText"`
	YAxis        string  `json:"yAxis"`
	XAxis        string  `json:"xAxis"`
	Data         []struct {
		Month  int     `json:"month"`
		Hour   int     `json:"hour"`
		Year   int     `json:"year"`
		Time   string  `json:"time"`
		Day    int     `json:"day"`
		Value  float64 `json:"value"`
		Minute int     `json:"minute"`
		Second int     `json:"second"`
	} `json:"data"`
	Success      bool   `json:"success"`
	AvgValueText string `json:"avgValueText"`
	MaxValueText string `json:"maxValueText"`
}

func (c *Client) InverterChartDayLineLoad(date time.Time) (*InverterChartDayLineResponse, error) {
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/invChart/dayLine?serialNum=%s&attr=loadpower&dateText=%s", geCloudBaseURL, c.serial, date.Format(reqDateFormat)), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.doRequest(req, false)
	if err != nil {
		return nil, err
	}

	var inverterChartDayLineResponse InverterChartDayLineResponse
	err = json.NewDecoder(resp.Body).Decode(&inverterChartDayLineResponse)
	if err != nil {
		return nil, err
	}

	return &inverterChartDayLineResponse, nil
}

func (c *Client) GetConsumptionAverages() (map[time.Time]float64, error) {
	if c.consumptionAverages == nil {
		err := c.UpdateConsumptionAverages()
		if err != nil {
			return nil, err
		}
	}

	c.m.RLock()
	defer c.m.RUnlock()

	consumptionAverages := *c.consumptionAverages
	return consumptionAverages, nil
}

func (c *Client) SetConsumptionAverages(consumptionAverages map[time.Time]float64) {
	// normalise time keys
	nca := map[time.Time]float64{}
	for k, v := range consumptionAverages {
		nca[time.Date(1, 1, 1, k.Hour(), k.Minute(), 0, 0, time.Local)] = v
	}

	c.m.Lock()
	defer c.m.Unlock()

	c.consumptionAverages = &nca
}

func (c *Client) UpdateConsumptionAverages() error {
	consumptionAverages := make(map[time.Time]float64)

	now := time.Now().Truncate(time.Hour * 24)
	for i := -1; i > -8; i-- {
		measurements, err := c.InverterChartDayLineLoad(now.AddDate(0, 0, i))
		if err != nil {
			return err
		}

		for _, measurement := range measurements.Data {
			if measurement.Value <= 0 {
				continue
			}

			mt, err := time.Parse(measurementTimeFormat, measurement.Time)
			if err != nil {
				return err
			}

			var period = time.Time{}
			if mt.Minute() < 30 {
				period = time.Date(1, 1, 1, mt.Hour(), 0, 0, 0, time.Local)
			} else {
				period = time.Date(1, 1, 1, mt.Hour(), 30, 0, 0, time.Local)
			}

			if v, ok := consumptionAverages[period]; ok {
				v = (v + measurement.Value) / 2
			} else {
				consumptionAverages[period] = measurement.Value
			}
		}
	}

	c.m.Lock()
	defer c.m.Unlock()

	c.consumptionAverages = &consumptionAverages
	return nil
}
