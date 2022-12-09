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
	StorageCapacityKwh  float64
	InverterEfficiency  float64
	ACChargeStart       time.Time
	ACChargeEnd         time.Time
	BatteryLowerReserve float64
	MaxChargeKw         float64
	MaxDischargeKw      float64
	AvgConsumptionKw    float64
	BatteryUpperReserve float64
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
			StorageCapacityKwh:  7.38,          // todo consume from ge cloud (inverter/getInverterInfo)
			InverterEfficiency:  0.965,         // todo consume from ge cloud
			ACChargeStart:       acChargeStart, // todo consume from ge cloud (BatteryData/All)
			ACChargeEnd:         acChargeEnd,   // todo consume from ge cloud (BatteryData/All)
			BatteryLowerReserve: 4.0,           // todo consume from ge cloud (BatteryData/All)
			MaxChargeKw:         3.0,           // todo consume from ge cloud
			MaxDischargeKw:      3.0,           // todo consume from ge cloud
			BatteryUpperReserve: 100.0,
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

	burs := os.Getenv("BATTERY_UPPER_RESERVE") // todo do this properly using the opts
	if burs != "" {
		bur, err := strconv.ParseFloat(burs, 10)
		if err != nil {
			println(fmt.Errorf("err parsing BATTERY_UPPER_RESERVE: %w", err).Error())
		} else {
			projector.config.BatteryUpperReserve = bur
		}
	}

	blrs := os.Getenv("BATTERY_LOWER_RESERVE") // todo do this properly using the opts
	if blrs != "" {
		blr, err := strconv.ParseFloat(blrs, 10)
		if err != nil {
			println(fmt.Errorf("err parsing BATTERY_LOWER_RESERVE: %w", err).Error())
		} else {
			projector.config.BatteryLowerReserve = blr
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

type Simulation struct {
	Date                               time.Time
	ProductionKwh                      float64
	ConsumptionKwh                     float64
	ChargeKwh                          float64
	DischargeKwh                       float64
	Forecasts                          []*Forecast
	DayStorageMaxKwh                   float64
	ConsumptionBeforeSelfSufficientKwh float64
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
	storageReserveKwh := (f.config.BatteryLowerReserve / 100) * f.config.StorageCapacityKwh
	simulation, err := f.simulate(t, storageReserveKwh)
	if err != nil {
		return nil, err
	}

	recommendedChargeKwh := (f.config.StorageCapacityKwh - simulation.DayStorageMaxKwh) + simulation.ConsumptionBeforeSelfSufficientKwh + storageReserveKwh
	if f.config.BatteryUpperReserve != 100 {
		capacityReserveKwh := f.config.StorageCapacityKwh - ((f.config.BatteryUpperReserve / 100) * f.config.StorageCapacityKwh)
		recommendedChargeKwh = recommendedChargeKwh - capacityReserveKwh
	}

	if recommendedChargeKwh < storageReserveKwh {
		recommendedChargeKwh = storageReserveKwh
	}

	if recommendedChargeKwh > f.config.StorageCapacityKwh {
		recommendedChargeKwh = f.config.StorageCapacityKwh
	}

	simulation, err = f.simulate(t, recommendedChargeKwh)
	if err != nil {
		return nil, err
	}

	recommendedChargeTarget := (recommendedChargeKwh / f.config.StorageCapacityKwh) * 100
	return &ForecastDay{
		Date:                    t,
		ProductionKwh:           simulation.ProductionKwh,
		ConsumptionKwh:          simulation.ConsumptionKwh,
		ChargeKwh:               simulation.ChargeKwh,
		DischargeKwh:            simulation.DischargeKwh,
		RecommendedChargeTarget: recommendedChargeTarget,
		Forecasts:               simulation.Forecasts,
	}, nil
}

func (f *Forecaster) simulate(t time.Time, storageDayStartKwh float64) (*Simulation, error) {
	forecast, err := f.sc.GetForecast()
	if err != nil {
		return nil, err
	}

	var consumptionAverages map[time.Time]float64
	if f.config.AvgConsumptionKw == 0 {
		//consumptionAverages, err = f.gec.GetConsumptionAverages()
		//if err != nil {
		//	return nil, err
		//}
	}

	t = time.Date(t.Local().Year(), t.Local().Month(), t.Local().Day(), 0, 0, 0, 0, time.Local)
	dischargingPeriodStart := time.Date(t.Year(), t.Month(), t.Day(), f.config.ACChargeEnd.Hour(), 30, 0, 0, time.UTC) // minute hardcoded to avoid initial 5m period caused by charge window offsets
	// This will only work if the charging period starts after midnight as we're assuming this and setting the date to tomorrow
	dischargingPeriodEnd := time.Date(t.Year(), t.Month(), t.Day()+1, f.config.ACChargeStart.Hour(), 30, 0, 0, time.UTC) // minute hardcoded to avoid initial 5m period caused by charge window offsets
	storageReserveKwh := (f.config.BatteryLowerReserve / 100) * f.config.StorageCapacityKwh

	var dayProductionKwh, dayConsumptionKwh, dayDischargeKwh, dayChargeKwh, dayStorageKwh, dayStorageMaxKwh, consumptionBeforeSelfSufficientKwh float64
	var selfSufficient bool
	dayStorageKwh = storageDayStartKwh
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

		netKwh := productionKwh - consumptionKwh
		var chargeKwh, dischargeKwh float64
		if netKwh < 0 {
			dischargeKwh = math.Min(math.Abs(netKwh)*((1-f.config.InverterEfficiency)+1), f.config.MaxDischargeKw*0.5)
			dayDischargeKwh = dayDischargeKwh + dischargeKwh

			if !selfSufficient {
				consumptionBeforeSelfSufficientKwh = consumptionBeforeSelfSufficientKwh + dischargeKwh
			}
		} else {
			chargeKwh = math.Min(math.Abs(netKwh)*f.config.InverterEfficiency, f.config.MaxChargeKw*0.5)
			dayChargeKwh = dayChargeKwh + chargeKwh

			selfSufficient = true
		}

		dayStorageKwh = dayStorageKwh + (chargeKwh - dischargeKwh)
		if dayStorageKwh < storageReserveKwh { // Handle battery empty
			dayStorageKwh = storageReserveKwh
			dayDischargeKwh = dayDischargeKwh - dischargeKwh
			dischargeKwh = 0
		}
		if dayStorageKwh > f.config.StorageCapacityKwh { // Handle battery full
			dayStorageKwh = f.config.StorageCapacityKwh
			dayChargeKwh = dayChargeKwh - chargeKwh
			chargeKwh = 0
		}

		storageSOC := (dayStorageKwh / f.config.StorageCapacityKwh) * 100
		if dayStorageKwh > dayStorageMaxKwh {
			dayStorageMaxKwh = dayStorageKwh
		}

		forecasts = append(forecasts, &Forecast{
			PeriodEnd:      forecast.PeriodEnd.Local(),
			ProductionKwh:  dayProductionKwh,
			ConsumptionKwh: dayConsumptionKwh,
			ChargeKwh:      dayChargeKwh,
			DischargeKwh:   dayDischargeKwh,
			ProductionW:    productionKwh * 2 * 1000,
			ConsumptionW:   consumptionKwh * 2 * 1000,
			ChargeW:        chargeKwh * 2 * 1000,
			DischargeW:     dischargeKwh * 2 * 1000,
			SOC:            storageSOC,
		})
	}

	return &Simulation{
		ProductionKwh:                      dayProductionKwh,
		ConsumptionKwh:                     dayConsumptionKwh,
		ChargeKwh:                          dayChargeKwh,
		DischargeKwh:                       dayDischargeKwh,
		Forecasts:                          forecasts,
		DayStorageMaxKwh:                   dayStorageMaxKwh,
		ConsumptionBeforeSelfSufficientKwh: consumptionBeforeSelfSufficientKwh,
	}, nil
}
