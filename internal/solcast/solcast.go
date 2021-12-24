package solcast

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
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

	sort.Slice(fcd.Forecasts, func(i, j int) bool {
		return fcd.Forecasts[i].PeriodEnd.Before(fcd.Forecasts[j].PeriodEnd)
	})

	c.data = &fcd
	return nil
}

func (c *Client) UpdateForecast() error {
	c.m.Lock()
	defer c.m.Unlock()
	fcd := ForecastData{Forecasts: []Forecast{}}

	get, err := c.c.Get(fmt.Sprintf("%s/%s/forecasts?format=json&api_key=%s", c.baseURL, c.resourceID, c.apiKey))
	if err != nil {
		return err
	}

	var forecastResponse ForecastData
	err = json.NewDecoder(get.Body).Decode(&forecastResponse)
	if err != nil {
		return err
	}
	fcd.Forecasts = append(fcd.Forecasts, forecastResponse.Forecasts...)

	get, err = c.c.Get(fmt.Sprintf("%s/%s/estimated_actuals?format=json&api_key=%s", c.baseURL, c.resourceID, c.apiKey))
	if err != nil {
		return err
	}

	var actualsResponse ForecastData
	err = json.NewDecoder(get.Body).Decode(&actualsResponse)
	if err != nil {
		return err
	}

	fcd.Forecasts = append(fcd.Forecasts, actualsResponse.Forecasts...)

	sort.Slice(fcd.Forecasts, func(i, j int) bool {
		return fcd.Forecasts[i].PeriodEnd.Before(fcd.Forecasts[j].PeriodEnd)
	})

	c.data = &fcd
	return nil
}

func (c *Client) GetForecast() (*ForecastData, error) {
	if c.data == nil {
		// probably don't want to let this auto fetch as we're limited to 10 requests a day
		//err := c.UpdateForecast()
		//if err != nil {
		//	return nil, err
		//}

		return nil, errors.New("no solcast forecast data available")
	}

	c.m.RLock()
	defer c.m.RUnlock()

	data := *c.data
	return &data, nil
}
