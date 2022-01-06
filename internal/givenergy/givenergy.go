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

type BatteryDataResponse struct {
	Success       bool `json:"success"`
	Lost          bool `json:"lost"`
	MaxVoltModule int  `json:"maxVoltModule"`
	MinVoltModule int  `json:"minVoltModule"`
	Modules       []struct {
		CellVoltage8         int    `json:"cellVoltage8"`
		CellVoltage9         int    `json:"cellVoltage9"`
		CellVoltage4         int    `json:"cellVoltage4"`
		ModuleVoltage        int    `json:"moduleVoltage"`
		CellVoltage5         int    `json:"cellVoltage5"`
		DesignCapText        string `json:"designCapText"`
		CellVoltage6         int    `json:"cellVoltage6"`
		CellVoltage7         int    `json:"cellVoltage7"`
		ModuleSoc            int    `json:"moduleSoc"`
		CellVoltage10        int    `json:"cellVoltage10"`
		CellVoltage1         int    `json:"cellVoltage1"`
		CellVoltage2         int    `json:"cellVoltage2"`
		CellVoltage3         int    `json:"cellVoltage3"`
		CellVoltage14        int    `json:"cellVoltage14"`
		HasBmsCellModuleInfo bool   `json:"hasBmsCellModuleInfo"`
		CellVoltage13        int    `json:"cellVoltage13"`
		CellVoltage12        int    `json:"cellVoltage12"`
		CellTempreture2Text  string `json:"cellTempreture2Text"`
		CellVoltage11        int    `json:"cellVoltage11"`
		MinVoltItem          int    `json:"minVoltItem"`
		MaxVoltDiffValue     int    `json:"maxVoltDiffValue"`
		CellTempreture3Text  string `json:"cellTempreture3Text"`
		Module               int    `json:"module"`
		Charging             int    `json:"charging"`
		MaxVoltItem          int    `json:"maxVoltItem"`
		CellTempreture4Text  string `json:"cellTempreture4Text"`
		CellVoltage16        int    `json:"cellVoltage16"`
		CellVoltage15        int    `json:"cellVoltage15"`
		Discharging          int    `json:"discharging"`
		ModuleTempretureText string `json:"moduleTempretureText"`
		FullCapText          string `json:"fullCapText"`
		Time                 string `json:"time"`
		CellTempreture1Text  string `json:"cellTempreture1Text"`
	} `json:"modules"`
}

func (c *Client) GetBatteryData() (*BatteryDataResponse, error) {
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/pcs/batCell/getBatCellData?serialNum=%s", geCloudBaseURL, c.serial), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.doRequest(req, false)
	if err != nil {
		return nil, err
	}

	var batteryDataResponse BatteryDataResponse
	err = json.NewDecoder(resp.Body).Decode(&batteryDataResponse)
	if err != nil {
		return nil, err
	}

	return &batteryDataResponse, nil
}

type AllBatteryDataResponse struct {
	BatteryStatus          string `json:"batteryStatus"`
	Mode                   string `json:"mode"`
	BatteryPercent         string `json:"batteryPercent"`
	SelfConsumptionMode    string `json:"selfConsumptionMode"`
	ShallowCharge          string `json:"shallowCharge"`
	DischargeFlag          string `json:"dischargeFlag"`
	DischargeScheduleStart string `json:"dischargeScheduleStart"`
	DischargeScheduleEnd   string `json:"dischargeScheduleEnd"`
	DischargeDownTo        string `json:"dischargeDownTo"`
	ChargeFlag             string `json:"chargeFlag"`
	ChargeScheduleStart    string `json:"chargeScheduleStart"`
	ChargeScheduleEnd      string `json:"chargeScheduleEnd"`
	ChargeUpTo             string `json:"chargeUpTo"`
}

func (c *Client) GetAllBatteryData() (*AllBatteryDataResponse, error) {
	request, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/batteryData/all", geBatteryBaseURL), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.doRequest(request, false)
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("error setting charge target via ge battery api")
	}

	var allBatteryData AllBatteryDataResponse
	err = json.NewDecoder(resp.Body).Decode(&allBatteryData)
	if err != nil {
		return nil, err
	}

	// normalise times
	switch len(allBatteryData.ChargeScheduleStart) {
	case 2:
		allBatteryData.ChargeScheduleStart = "00" + allBatteryData.ChargeScheduleStart
	case 3:
		allBatteryData.ChargeScheduleStart = "0" + allBatteryData.ChargeScheduleStart
	}

	switch len(allBatteryData.ChargeScheduleEnd) {
	case 2:
		allBatteryData.ChargeScheduleEnd = "00" + allBatteryData.ChargeScheduleEnd
	case 3:
		allBatteryData.ChargeScheduleEnd = "0" + allBatteryData.ChargeScheduleEnd
	}

	return &allBatteryData, nil
}

type PlantChartDayResponse struct {
	IsOnPlant bool   `json:"isOnPlant"`
	XAxis     string `json:"xAxis"`
	Data      []struct {
		BatPowerActual float64 `json:"batPowerActual"`
		LoadPower      float64 `json:"loadPower"`
		Year           float64 `json:"year"`
		PacImport      float64 `json:"pacImport"`
		BatPower       float64 `json:"batPower"`
		Minute         int     `json:"minute"`
		Second         int     `json:"second"`
		Pac            float64 `json:"pac"`
		Month          int     `json:"month"`
		Hour           int     `json:"hour"`
		Ppv            float64 `json:"ppv"`
		Time           string  `json:"time"`
		Day            int     `json:"day"`
		PacExport      float64 `json:"pacExport"`
	} `json:"data"`
	Success bool `json:"success"`
}

func (c *Client) PlantChartDay(date time.Time) (*PlantChartDayResponse, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/plantChart/day/%s", geBatteryBaseURL, date.Format(reqDateFormat)), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.doRequest(req, false)
	if err != nil {
		return nil, err
	}

	var plantCharDayResponse PlantChartDayResponse
	err = json.NewDecoder(resp.Body).Decode(&plantCharDayResponse)
	if err != nil {
		return nil, err
	}

	return &plantCharDayResponse, nil
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

func (c *Client) InverterChartDayLineLoad(date time.Time, attribute string) (*InverterChartDayLineResponse, error) {
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/invChart/dayLine?serialNum=%s&attr=%s&dateText=%s", geCloudBaseURL, c.serial, attribute, date.Format(reqDateFormat)), nil)
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
	for i := -1; i > -8; i-- { // todo make amount of days config
		measurements, err := c.InverterChartDayLineLoad(now.AddDate(0, 0, i), "loadpower")
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
				consumptionAverages[period] = (v + measurement.Value) / 2
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
