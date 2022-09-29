package forecaster

import (
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/jakekeeys/givforecast/internal/givenergy"
	"github.com/jakekeeys/givforecast/internal/solcast"
)

type Config struct {
	StorageCapacityKwh float64
	InverterEfficiency float64
	ACChargeStart      time.Time
	ACChargeEnd        time.Time
	BatteryReserve     float64
	MaxChargeKw        float64
	MaxDischargeKw     float64
	AvgConsumptionKw   float64
}

func WithConfig(c *Config) Option {
	return func(p *Forecaster) {
		p.config = c
	}
}

type Option func(p *Forecaster)

type Forecaster struct {
	sc     *solcast.Client
	gec    *givenergy.Client
	config *Config
	m      sync.RWMutex
}

func New(sc *solcast.Client, gec *givenergy.Client, opts ...Option) *Forecaster {
	// Eco 7 times in the UK do not shift with BST
	acChargeStart := time.Date(1, 1, 1, 0, 35, 0, 0, time.UTC)
	acChargeEnd := time.Date(1, 1, 1, 7, 25, 0, 0, time.UTC)

	projector := &Forecaster{
		sc:  sc,
		gec: gec,
		config: &Config{
			StorageCapacityKwh: 16.38,         // todo consume from ge cloud (inverter/getInverterInfo)
			InverterEfficiency: 0.965,         // todo consume from ge cloud
			ACChargeStart:      acChargeStart, // todo consume from ge cloud (BatteryData/All)
			ACChargeEnd:        acChargeEnd,   // todo consume from ge cloud (BatteryData/All)
			BatteryReserve:     4.0,           // todo consume from ge cloud (BatteryData/All)
			MaxChargeKw:        3.0,           // todo consume from ge cloud
			MaxDischargeKw:     3.0,           // todo consume from ge cloud
		},
	}

	ackws := os.Getenv("AVG_CONS_KWH") // todo do this properly using the opts
	if ackws != "" {
		ackw, err := strconv.ParseFloat(ackws, 10)
		if err != nil {
			println(fmt.Errorf("err parsing AVG_CONS_KWH: %w", err).Error())
		} else {
			projector.config.AvgConsumptionKw = ackw
		}
	}

	sckwhs := os.Getenv("STORAGE_CAPACITY_KWH") // todo do this properly using the opts
	if sckwhs != "" {
		sckwh, err := strconv.ParseFloat(sckwhs, 10)
		if err != nil {
			println(fmt.Errorf("err parsing STORAGE_CAPACITY_KWH: %w", err).Error())
		} else {
			projector.config.StorageCapacityKwh = sckwh
		}
	}

	mckws := os.Getenv("MAX_CHARGE_KW") // todo do this properly using the opts
	if mckws != "" {
		mckw, err := strconv.ParseFloat(mckws, 10)
		if err != nil {
			println(fmt.Errorf("err parsing MAX_CHARGE_KW: %w", err).Error())
		} else {
			projector.config.MaxChargeKw = mckw
		}
	}

	mdhws := os.Getenv("MAX_DISCHARGE_KW") // todo do this properly using the opts
	if mdhws != "" {
		mdhw, err := strconv.ParseFloat(mdhws, 10)
		if err != nil {
			println(fmt.Errorf("err parsing MAX_DISCHARGE_KW: %w", err).Error())
		} else {
			projector.config.MaxDischargeKw = mdhw
		}
	}

	for _, opt := range opts {
		opt(projector)
	}

	return projector
}

type ForecastDay struct {
	Date                    time.Time
	ProductionKwh           float64
	ConsumptionKwh          float64
	ChargeKwh               float64
	DischargeKwh            float64
	RecommendedChargeTarget float64
	Forecasts               []*Forecast
}

type Forecast struct {
	PeriodEnd      time.Time
	ProductionKwh  float64
	ConsumptionKwh float64
	ChargeKwh      float64
	DischargeKwh   float64
	ProductionW    float64
	ConsumptionW   float64
	ChargeW        float64
	DischargeW     float64
	SOC            float64
}

func (f *Forecaster) GetConfig() *Config {
	return f.config
}

func (f *Forecaster) SetConfig(c Config) {
	f.m.Lock()
	defer f.m.Unlock()

	f.config = &c
	return
}

func (f *Forecaster) ForecastNow(t time.Time) (*Forecast, error) {
	chargingPeriodStart := time.Date(t.Year(), t.Month(), t.Day(), f.config.ACChargeStart.Hour(), f.config.ACChargeStart.Minute(), 0, 0, time.UTC)
	chargingPeriodEnd := time.Date(t.Year(), t.Month(), t.Day(), f.config.ACChargeEnd.Hour(), f.config.ACChargeEnd.Minute(), 0, 0, time.UTC)

	// if it's after midnight,
	// and it's before the charging period
	// we actually want yesterday's forecast
	// this again assumes the charging period occurs during the early hours of the morning
	ft := t
	if t.Hour() >= 0 && t.Before(chargingPeriodStart) {
		ft = ft.AddDate(0, 0, -1)
	}

	fc, err := f.Forecast(ft)
	if err != nil {
		return nil, err
	}

	// if we're within the charging period
	if t.After(chargingPeriodStart) && t.Before(chargingPeriodEnd) {
		return &Forecast{
			PeriodEnd:      chargingPeriodEnd,
			ProductionKwh:  0,
			ConsumptionKwh: 0,
			ChargeKwh:      0,
			DischargeKwh:   0,
			ProductionW:    0,
			ConsumptionW:   0,
			ChargeW:        0,
			DischargeW:     0,
			SOC:            fc.RecommendedChargeTarget,
		}, nil
	}

	for _, forecast := range fc.Forecasts {
		if forecast.PeriodEnd.Before(t) {
			continue
		}

		return &Forecast{
			PeriodEnd:      forecast.PeriodEnd.Local(),
			ProductionKwh:  forecast.ProductionKwh,
			ConsumptionKwh: forecast.ConsumptionKwh,
			ChargeKwh:      forecast.ChargeKwh,
			DischargeKwh:   forecast.DischargeKwh,
			ProductionW:    forecast.ProductionW,
			ConsumptionW:   forecast.ConsumptionW,
			ChargeW:        forecast.ChargeW,
			DischargeW:     forecast.DischargeW,
			SOC:            forecast.SOC,
		}, nil
	}

	return nil, errors.New("unable find matching forecast for current period")
}

func (f *Forecaster) Forecast(t time.Time) (*ForecastDay, error) {
	forecast, err := f.sc.GetForecast()
	if err != nil {
		return nil, err
	}

	var consumptionAverages map[time.Time]float64
	if f.config.AvgConsumptionKw == 0 {
		consumptionAverages, err = f.gec.GetConsumptionAverages()
		if err != nil {
			return nil, err
		}
	}

	t = time.Date(t.Local().Year(), t.Local().Month(), t.Local().Day(), 0, 0, 0, 0, time.Local)
	dischargingPeriodStart := time.Date(t.Year(), t.Month(), t.Day(), f.config.ACChargeEnd.Hour(), 30, 0, 0, time.UTC) // minute hardcoded to avoid initial 5m period caused by charge window offsets
	// This will only work if the charging period starts after midnight as we're assuming this and setting the date to tomorrow
	dischargingPeriodEnd := time.Date(t.Year(), t.Month(), t.Day()+1, f.config.ACChargeStart.Hour(), 30, 0, 0, time.UTC) // minute hardcoded to avoid initial 5m period caused by charge window offsets
	var dayProductionKwh, dayConsumptionKwh, dayMaxSOC, dayDischargeKwh, dayChargeKwh float64
	var forecasts []*Forecast
	for _, forecast := range forecast.Forecasts {
		if forecast.PeriodEnd.After(dischargingPeriodEnd) {
			continue
		}

		if forecast.PeriodEnd.Before(dischargingPeriodStart) || forecast.PeriodEnd.Equal(dischargingPeriodStart) {
			continue
		}

		consumptionKwh := 0.0
		if f.config.AvgConsumptionKw != 0 {
			consumptionKwh = f.config.AvgConsumptionKw * 0.5
		} else if forecast.PeriodEnd.Minute() == 0 { // todo replace time.roundUpTo()
			consumptionKwh = (consumptionAverages[time.Date(1, 1, 1, forecast.PeriodEnd.Hour()-1, 30, 0, 0, time.Local)] / 1000) * 0.5
		} else {
			consumptionKwh = (consumptionAverages[time.Date(1, 1, 1, forecast.PeriodEnd.Hour(), 0, 0, 0, time.Local)] / 1000) * 0.5
		}

		dayConsumptionKwh = dayConsumptionKwh + consumptionKwh

		productionKwh := forecast.PvEstimate * 0.5
		dayProductionKwh = dayProductionKwh + productionKwh

		netKwh := consumptionKwh - productionKwh
		var chargeKw, dischargeKw float64
		if netKwh > 0 {
			dischargeKw = math.Min(netKwh*((1-f.config.InverterEfficiency)+1), f.config.MaxDischargeKw*0.5)
			dayDischargeKwh = dayDischargeKwh + dischargeKw
		} else {
			chargeKw = math.Min(math.Abs(netKwh)*f.config.InverterEfficiency, f.config.MaxChargeKw*0.5)
			dayChargeKwh = dayChargeKwh + chargeKw
		}

		storageSOC := (((f.config.StorageCapacityKwh - dayDischargeKwh) + dayChargeKwh) / f.config.StorageCapacityKwh) * 100
		if storageSOC > dayMaxSOC {
			dayMaxSOC = storageSOC
		}

		forecasts = append(forecasts, &Forecast{
			PeriodEnd:      forecast.PeriodEnd.Local(),
			ProductionKwh:  dayProductionKwh,
			ConsumptionKwh: dayConsumptionKwh,
			ChargeKwh:      dayChargeKwh,
			DischargeKwh:   dayDischargeKwh,
			ProductionW:    productionKwh * 2 * 1000,
			ConsumptionW:   consumptionKwh * 2 * 1000,
			ChargeW:        chargeKw * 2 * 1000,
			DischargeW:     dischargeKw * 2 * 1000,
			SOC:            storageSOC,
		})
	}

	recommendedChargeTarget := 100.0
	if dayMaxSOC > 100 && dayMaxSOC < 200 {
		recommendedChargeTarget = math.Abs((dayMaxSOC - 100) - 100)
	} else {
		recommendedChargeTarget = f.config.BatteryReserve
	}

	for i, projection := range forecasts {
		projection.SOC = projection.SOC - 100 + recommendedChargeTarget
		// if the battery is empty unwind discharge and soc changes
		// this will slightly underestimate the dischargeKwh as it would have emptied mid-period
		// todo we could set dayDischargeKwh to the battery capacity and figure out the W from that
		if projection.SOC <= f.config.BatteryReserve {
			projection.SOC = f.config.BatteryReserve
			dayDischargeKwh = forecasts[i-1].DischargeKwh
			projection.DischargeKwh = forecasts[i-1].DischargeKwh
			projection.DischargeW = 0
		}
		if projection.SOC >= 100 {
			projection.SOC = 100
			dayChargeKwh = forecasts[i-1].ChargeKwh
			projection.ChargeKwh = forecasts[i-1].ChargeKwh
			projection.ChargeW = 0
		}
	}

	return &ForecastDay{
		Date:                    t,
		ProductionKwh:           dayProductionKwh,
		ConsumptionKwh:          dayConsumptionKwh,
		ChargeKwh:               dayChargeKwh,
		DischargeKwh:            dayDischargeKwh,
		RecommendedChargeTarget: recommendedChargeTarget,
		Forecasts:               forecasts,
	}, nil
}
