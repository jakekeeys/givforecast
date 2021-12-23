package givenergy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/cookiejar"
	"sync"
	"time"
)

const (
	reqDateFormat         = "2006-01-02"
	measurementTimeFormat = "2006-01-02 15:04:05"
)

type Client struct {
	c                   *http.Client
	baseURL             string
	username            string
	password            string
	serial              string
	consumptionAverages *map[time.Time]float64
	m                   sync.RWMutex
}

func NewClient(username, password, serial string) *Client {
	jar, err := cookiejar.New(nil)
	if err != nil {
		panic(err)
	}
	client := http.Client{
		Jar: jar,
	}

	return &Client{
		c:        &client,
		baseURL:  "https://www.givenergy.cloud/GivManage/api",
		username: username,
		password: password,
		serial:   serial,
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

	post, err := c.c.Post(fmt.Sprintf("%s/login?account=%s&password=%s", c.baseURL, c.username, c.password), "application/json", nil)
	if err != nil {
		return err
	}

	var loginResponse LoginResponse
	err = json.NewDecoder(post.Body).Decode(&loginResponse)
	if err != nil {
		return err
	}

	if !loginResponse.Success {
		return errors.New("error authenticating")
	}

	return nil
}

type GivEnergyResponse struct {
	Success bool `json:"success"`
	MsgCode int  `json:"msgCode"`
}

func (c *Client) doRequest(r *http.Request, retry bool) (*http.Response, error) {
	resp, err := c.c.Do(r)
	if err != nil {
		return nil, err
	}

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

	return resp, nil
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
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/invChart/dayLine?serialNum=%s&attr=loadpower&dateText=%s", c.baseURL, c.serial, date.Format(reqDateFormat)), nil)
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

func (c *Client) UpdateConsumptionAverages() error {
	consumptionAverages := make(map[time.Time]float64)

	now := time.Now().Truncate(time.Hour * 24)
	for i := -1; i > -8; i-- {
		measurements, err := c.InverterChartDayLineLoad(now.AddDate(0, 0, i))
		if err != nil {
			return err
		}

		for _, measurement := range measurements.Data {
			mt, err := time.Parse(measurementTimeFormat, measurement.Time)
			if err != nil {
				return err
			}
			period := time.Date(0, 0, 0, mt.Hour(), 0, 0, 0, time.Local)

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
