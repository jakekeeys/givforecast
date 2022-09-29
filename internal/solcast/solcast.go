package solcast

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path"
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
	cacheDir   string
}

const dataCacheFile = "solcastData.gob"

func NewClient(apiKey, resourceID, cacheDir string) *Client {
	return &Client{
		apiKey:     apiKey,
		baseURL:    "https://api.solcast.com.au/rooftop_sites",
		resourceID: resourceID,
		c:          http.DefaultClient,
		cacheDir:   cacheDir,
	}
}

type EstimatedActualData struct {
	Forecasts []Forecast `json:"estimated_actuals"`
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

func (c *Client) writeDataCache(data *ForecastData) error {
	dataCacheFilePath := path.Join(c.cacheDir, dataCacheFile)
	f, err := os.Create(dataCacheFilePath)
	if err != nil {
		return fmt.Errorf("error creating data cache file: %w", err)
	}
	defer f.Close()

	err = gob.NewEncoder(f).Encode(data)
	if err != nil {
		return fmt.Errorf("error encoding data cache file: %w", err)
	}

	return nil
}

func (c *Client) readDataCache() (*ForecastData, error) {
	dataCacheFilePath := path.Join(c.cacheDir, dataCacheFile)
	f, err := os.Open(dataCacheFilePath)
	if err != nil {
		return nil, fmt.Errorf("error opening data cache file: %w", err)
	}
	defer f.Close()

	data := &ForecastData{}
	err = gob.NewDecoder(f).Decode(data)
	if err != nil {
		return nil, fmt.Errorf("error decoding data cache file: %w", err)
	}

	return data, nil
}

func (c *Client) SetForecast(fcd ForecastData) error {
	c.m.Lock()
	defer c.m.Unlock()

	sort.Slice(fcd.Forecasts, func(i, j int) bool {
		return fcd.Forecasts[i].PeriodEnd.Before(fcd.Forecasts[j].PeriodEnd)
	})

	c.data = &fcd
	if c.cacheDir != "" {
		err := c.writeDataCache(&fcd)
		if err != nil {
			println(fmt.Errorf("error updating data cache: %w", err))
		}
	}
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

	var actualsResponse EstimatedActualData
	err = json.NewDecoder(get.Body).Decode(&actualsResponse)
	if err != nil {
		return err
	}

	fcd.Forecasts = append(fcd.Forecasts, actualsResponse.Forecasts...)

	sort.Slice(fcd.Forecasts, func(i, j int) bool {
		return fcd.Forecasts[i].PeriodEnd.Before(fcd.Forecasts[j].PeriodEnd)
	})

	c.data = &fcd
	if c.cacheDir != "" {
		err := c.writeDataCache(&fcd)
		if err != nil {
			println(fmt.Errorf("error updating data cache: %w", err))
		}
	}
	return nil
}

func (c *Client) GetForecast() (*ForecastData, error) {
	if c.data == nil {
		// probably don't want to let this auto fetch as we're limited to 10 requests a day
		//err := c.UpdateForecast()
		//if err != nil {
		//	return nil, err
		//}

		if c.cacheDir != "" {
			fcd, err := c.readDataCache()
			if err != nil {
				return nil, fmt.Errorf("no solcast forecast data available: %w", err)
			}

			c.m.Lock()
			c.data = fcd
			c.m.Unlock()
		}
	}

	c.m.RLock()
	defer c.m.RUnlock()

	data := *c.data
	return &data, nil
}

type Measurement struct {
	PeriodEnd  time.Time `json:"period_end"`
	Period     string    `json:"period"`
	TotalPower float64   `json:"total_power"`
}

type SubmitMeasurementsRequest struct {
	Measurements []Measurement `json:"measurements"`
}

func (c *Client) SubmitMeasurements(request *SubmitMeasurementsRequest) error {
	bodyBytes, err := json.Marshal(request)
	if err != nil {
		return err
	}

	resp, err := c.c.Post(fmt.Sprintf("%s/%s/measurements?api_key=%s", c.baseURL, c.resourceID, c.apiKey), "application/json", bytes.NewBuffer(bodyBytes))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		respBody, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("error submitting solar measurements: %d, %s", resp.StatusCode, string(respBody))
	}

	return nil
}
