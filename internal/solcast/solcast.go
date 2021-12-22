package solcast

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"time"
)

type Client struct {
	m          sync.RWMutex
	apiKey     string
	baseURL    string
	resourceID string
	data       *ForecastData
	c          *http.Client
}

func NewClient(apiKey, resourceID string) *Client {
	return &Client{
		m:          sync.RWMutex{},
		apiKey:     apiKey,
		baseURL:    "https://api.solcast.com.au/rooftop_sites",
		resourceID: resourceID,
		c:          http.DefaultClient,
	}
}

type ForecastData struct {
	Forecasts []Forecast `json:"forecasts"`
}

type Forecast struct {
	PvEstimate   float64   `json:"pv_estimate"`
	PvEstimate10 float64   `json:"pv_estimate10"`
	PvEstimate90 float64   `json:"pv_estimate90"`
	PeriodEnd    time.Time `json:"period_end"`
	Period       string    `json:"period"`
}

func (c *Client) SetForecast(fcd ForecastData) error {
	c.m.Lock()
	defer c.m.Unlock()

	c.data = &fcd
	return nil
}

func (c *Client) UpdateForecast() error {
	c.m.Lock()
	defer c.m.Unlock()

	get, err := c.c.Get(fmt.Sprintf("%s/%s/forecasts?format=json", c.baseURL, c.resourceID))
	if err != nil {
		return err
	}

	body, err := ioutil.ReadAll(get.Body)
	if err != nil {
		return err
	}

	var forecastResponse *ForecastData
	err = json.NewDecoder(bytes.NewReader(body)).Decode(forecastResponse)
	if err != nil {
		return err
	}

	c.data = forecastResponse
	return nil
}

func (c *Client) GetForecast() (*ForecastData, error) {
	if c.data == nil {
		err := c.UpdateForecast()
		if err != nil {
			return nil, err
		}
	}

	c.m.RLock()
	defer c.m.RUnlock()

	data := *c.data
	return &data, nil
}
