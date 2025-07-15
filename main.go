package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/gin-gonic/gin"
	"github.com/jakekeeys/givforecast/internal/api"
	"github.com/jakekeeys/givforecast/internal/assist"
	"github.com/jakekeeys/givforecast/internal/forecaster"
	"github.com/jakekeeys/givforecast/internal/givenergy"
	"github.com/jakekeeys/givforecast/internal/givtcp"
	"github.com/jakekeeys/givforecast/internal/solcast"
	"github.com/jakekeeys/givforecast/internal/supervisor"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)

	r := gin.Default()

	tzl := os.Getenv("TZ_LOCATION")
	if tzl != "" {
		loc, err := time.LoadLocation(tzl)
		if err != nil {
			panic(err)
		}
		time.Local = loc
	}

	hac := assist.New(context.Background(), http.DefaultClient, os.Getenv("HA_TOKEN"), os.Getenv("HA_URL"))
	supervisor, err := supervisor.New(supervisor.Config{
		PollInterval: 60 * time.Second,
	}, context.Background(), hac)
	if err != nil {
		panic(err)
	}
	supervisor.Start()

	sc := solcast.NewClient(os.Getenv("SOLCAST_API_KEY"), os.Getenv("SOLCAST_RESOURCE_ID"), os.Getenv("CACHE_DIR"))
	gec := givenergy.NewClient(strings.Split(os.Getenv("GIVENERGY_SERIALS"), ","), os.Getenv("GIVENERGY_API_KEY"), os.Getenv("GIVENERGY_EMS") == "true")
	f := forecaster.New(sc, gec)
	gtcpc := givtcp.NewClient()
	s := api.NewServer(f, sc, gtcpc, gec)

	r.GET("/", s.RootHandler)

	r.GET("/forecast", s.ForecastHandler)
	r.GET("/forecast/now", s.ForecastNowHandler)
	r.GET("/forecast/config", s.ConfigHandler)
	r.PUT("/forecast/config", s.SetConfigHandler)
	r.PUT("/forecast/config/consumptionaverage", s.SetConsumptionAverage)
	r.PUT("/forecast/config/batteryupper", s.SetBatteryUpper)
	r.PUT("/forecast/config/batterylower", s.SetBatteryLower)
	r.PUT("/forecast/config/automatictargets", s.SetAutomaticTargets)

	r.POST("/givtcp/chargetarget", s.UpdateChargeTargetHandler)
	r.PUT("/givtcp/chargetarget", s.SetChargeTargetHandler)

	r.POST("/soclast/forecast", s.UpdateForecastDataHandler)
	r.PUT("/solcast/forecast", s.SetForecastDataHandler)
	r.GET("/solcast/forecast", s.GetForecastDataHandler)
	//r.POST("/solcast/actuals", s.SubmitSolarActualsHandler)

	//r.POST("/givenergy/consumptionaverages", s.UpdateConsumptionAveragesHandler)
	//r.GET("/givenergy/consumptionaverages", s.GetConsumptionAveragesHandler)
	//r.PUT("/givenergy/consumptionaverages", s.SetConsumptionAveragesHandler)
	//r.GET("/givenergy/batterydata", s.GetBatteryDataHandler)

	// todo post actual measurements production measurement from ge to solcast

	c := cron.New(cron.WithLocation(time.UTC))

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

	//ss := os.Getenv("SUBMIT_SOLAR_CRON")
	//if ss != "" {
	//	_, err := c.AddFunc(ss, func() {
	//		err := s.SubmitSolarActuals()
	//		if err != nil {
	//			println(fmt.Errorf("err submitting solar measurements: %w", err).Error())
	//		}
	//	})
	//	if err != nil {
	//		panic(fmt.Errorf("err scheduling UpdateChargeTarget: %w", err))
	//	}
	//}

	c.Start()

	err = r.Run(":8080")
	if err != nil {
		panic(err)
	}
}
