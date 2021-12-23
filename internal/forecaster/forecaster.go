package forecaster

import (
	"errors"
	"ge-charge-optimiser/internal/solcast"
	"math"
	"time"
)

type Config struct {
	BaseConsumptionKwh float64
	StorageCapacityKwh float64
	GridPeakStartH     int
	GridPeakStartM     int
	InverterEfficiency float64
}

func WithConfig(c *Config) Option {
	return func(p *Forecaster) {
		p.config = c
	}
}

type Option func(p *Forecaster)

type Forecaster struct {
	sc     *solcast.Client
	config *Config
}

func New(sc *solcast.Client, opts ...Option) *Forecaster {
	projector := &Forecaster{
		sc: sc,
		config: &Config{
			BaseConsumptionKwh: 1.1,
			StorageCapacityKwh: 16.38,
			GridPeakStartH:     7,
			GridPeakStartM:     30,
			InverterEfficiency: 0.965,
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

func (f *Forecaster) ForecastNow() (*Forecast, error) {
	fc, err := f.Forecast(time.Now().Local())
	if err != nil {
		return nil, err
	}

	now := time.Now().Local()
	peakStartToday := time.Date(now.Year(), now.Month(), now.Day(), f.config.GridPeakStartH, f.config.GridPeakStartH, 0, 0, time.Local)
	if now.Before(peakStartToday) {
		return &Forecast{
			PeriodEnd:      peakStartToday,
			ProductionKwh:  0,
			ConsumptionKwh: 0,
			ChargeKwh:      0,
			DischargeKwh:   0,
			SOC:            fc.RecommendedChargeTarget,
		}, nil
	}

	for _, forecast := range fc.Forecasts {
		if forecast.PeriodEnd.Before(now) {
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

	return nil, errors.New("unable find matching forecast")
}

func (f *Forecaster) Forecast(t time.Time) (*ForecastDay, error) {
	forecast, err := f.sc.GetForecast()
	if err != nil {
		return nil, err
	}

	t = t.Truncate(time.Hour * 24)
	var dayProductionKwh, dayConsumptionKwh, dayMaxSOC, dayDischargeKwh, dayChargeKwh float64
	var forecasts []*Forecast
	for _, forecast := range forecast.Forecasts {
		if forecast.PeriodEnd.After(t.AddDate(0, 0, 1)) || forecast.PeriodEnd.Before(t) {
			continue
		}

		if forecast.PeriodEnd.Hour() <= f.config.GridPeakStartH {
			continue
		}

		if forecast.PeriodEnd.Hour() == f.config.GridPeakStartH && forecast.PeriodEnd.Minute() == f.config.GridPeakStartM {
			continue
		}

		consumptionKwh := f.config.BaseConsumptionKwh * 0.5
		dayConsumptionKwh = dayConsumptionKwh + consumptionKwh

		productionKwh := forecast.PvEstimate * 0.5
		dayProductionKwh = dayProductionKwh + productionKwh

		netKwh := consumptionKwh - productionKwh
		var chargeKw, dischargeKw float64
		if netKwh > 0 {
			dischargeKw = netKwh * ((1 - f.config.InverterEfficiency) + 1)
			dayDischargeKwh = dayDischargeKwh + dischargeKw
		} else {
			chargeKw = (math.Abs(netKwh) * f.config.InverterEfficiency)
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

		//println(fmt.Sprintf("time: %s, consumption: %.2f, production %.2f, soc: %.2f, net: %.2f, discharged: %.2f, charged: %.2f", forecast.PeriodEnd.Local(), dayConsumptionKwh, dayProductionKwh, storageSOC, netKwh, dayDischargeKwh, dayChargeKwh))
	}

	recommendedChargeTarget := 100.0
	if dayMaxSOC > 100 {
		recommendedChargeTarget = math.Abs((dayMaxSOC - 100) - 100)
	}

	for _, projection := range forecasts {
		projection.SOC = projection.SOC - 100 + recommendedChargeTarget
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
