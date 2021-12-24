package forecaster

import (
	"errors"
	"math"
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
}

func New(sc *solcast.Client, gec *givenergy.Client, opts ...Option) *Forecaster {
	acChargeStart := time.Date(1, 1, 1, 0, 30, 0, 0, time.Local) // todo make config
	acChargeEnd := time.Date(1, 1, 1, 7, 30, 0, 0, time.Local)   // todo make config

	projector := &Forecaster{
		sc:  sc,
		gec: gec,
		config: &Config{
			StorageCapacityKwh: 16.38, // todo consume from ge cloud
			InverterEfficiency: 0.965,
			ACChargeStart:      acChargeStart,
			ACChargeEnd:        acChargeEnd,
			BatteryReserve:     4.0,
		},
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

func (f *Forecaster) ForecastNow(t time.Time) (*Forecast, error) {
	chargingPeriodStart := time.Date(t.Year(), t.Month(), t.Day(), f.config.ACChargeStart.Hour(), f.config.ACChargeStart.Minute(), 0, 0, time.Local)
	chargingPeriodEnd := time.Date(t.Year(), t.Month(), t.Day(), f.config.ACChargeEnd.Hour(), f.config.ACChargeEnd.Minute(), 0, 0, time.Local)

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

	consumptionAverages, err := f.gec.GetConsumptionAverages()
	if err != nil {
		return nil, err
	}

	t = t.Truncate(time.Hour * 24)
	dischargingPeriodStart := time.Date(t.Year(), t.Month(), t.Day(), f.config.ACChargeEnd.Hour(), f.config.ACChargeEnd.Minute(), 0, 0, time.Local)
	// This will only work if the charging period starts after midnight as we're assuming this and setting the date to tomorrow
	dischargingPeriodEnd := time.Date(t.Year(), t.Month(), t.Day()+1, f.config.ACChargeStart.Hour(), f.config.ACChargeStart.Minute(), 0, 0, time.Local)
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
		if forecast.PeriodEnd.Minute() == 0 {
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
			dischargeKw = netKwh * ((1 - f.config.InverterEfficiency) + 1)
			dayDischargeKwh = dayDischargeKwh + dischargeKw
		} else {
			chargeKw = math.Abs(netKwh) * f.config.InverterEfficiency
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
	if dayMaxSOC > 100 {
		recommendedChargeTarget = math.Abs((dayMaxSOC - 100) - 100)
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
