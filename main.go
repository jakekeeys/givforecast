package main

import (
	"fmt"
	"os"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/gin-gonic/gin"
	"github.com/jakekeeys/givforecast/internal/api"
	"github.com/jakekeeys/givforecast/internal/forecaster"
	"github.com/jakekeeys/givforecast/internal/givenergy"
	"github.com/jakekeeys/givforecast/internal/givtcp"
	"github.com/jakekeeys/givforecast/internal/solcast"
)

func main() {
	r := gin.Default()

	sc := solcast.NewClient(os.Getenv("SOLCAST_API_KEY"), os.Getenv("SOLCAST_RESOURCE_ID"))
	gec := givenergy.NewClient(os.Getenv("GIVENERGY_USERNAME"), os.Getenv("GIVENERGY_PASSWORD"), os.Getenv("GIVENERGY_SERIAL"), os.Getenv("GIVENERGY_API_KEY"), os.Getenv("CONSUMPTION_AVERAGE_DAYS"))
	f := forecaster.New(sc, gec)
	gtcpc := givtcp.NewClient()
	s := api.NewServer(f, sc, gtcpc, gec)

	r.GET("/forecast", s.ForecastHandler)
	r.GET("/forecast/now", s.ForecastNowHandler)
	r.GET("/forecast/config", s.ConfigHandler)
	r.PUT("/forecast/config", s.SetConfigHandler)

	r.POST("/givtcp/chargetarget", s.UpdateChargeTargetHandler)
	r.PUT("/givtcp/chargetarget", s.SetChargeTargetHandler)

	r.POST("/soclast/forecast", s.UpdateForecastDataHandler)
	r.PUT("/solcast/forecast", s.SetForecastDataHandler)
	r.GET("/solcast/forecast", s.GetForecastDataHandler)
	r.POST("/solcast/actuals", s.SubmitSolarActualsHandler)

	r.POST("/givenergy/consumptionaverages", s.UpdateConsumptionAveragesHandler)
	r.GET("/givenergy/consumptionaverages", s.GetConsumptionAveragesHandler)
	r.PUT("/givenergy/consumptionaverages", s.SetConsumptionAveragesHandler)
	r.GET("/givenergy/batterydata", s.GetBatteryDataHandler)

	// todo post actual measurements production measurement from ge to solcast

	c := cron.New(cron.WithLocation(time.Local))

	uc := os.Getenv("UPDATE_TARGET_CRON")
	if uc != "" {
		_, err := c.AddFunc(uc, func() {
			err := s.UpdateChargeTarget()
			if err != nil {
				println(fmt.Errorf("err updating charge target: %w", err).Error())
			}
		})
		if err != nil {
			panic(fmt.Errorf("err scheduling UpdateChargeTarget: %w", err))
		}
	}

	ss := os.Getenv("SUBMIT_SOLAR_CRON")
	if ss != "" {
		_, err := c.AddFunc(ss, func() {
			err := s.SubmitSolarActuals()
			if err != nil {
				println(fmt.Errorf("err submitting solar measurements: %w", err).Error())
			}
		})
		if err != nil {
			panic(fmt.Errorf("err scheduling UpdateChargeTarget: %w", err))
		}
	}

	c.Start()

	err := r.Run(":8080")
	if err != nil {
		panic(err)
	}
}
