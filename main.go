package main

import (
	"fmt"
	"os"
	"time"

	"github.com/jasonlvhit/gocron"

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
	gec := givenergy.NewClient(os.Getenv("GIVENERGY_USERNAME"), os.Getenv("GIVENERGY_PASSWORD"), os.Getenv("GIVENERGY_SERIAL"), os.Getenv("GIVENERGY_API_KEY"))
	f := forecaster.New(sc, gec)
	gtcpc := givtcp.NewClient()
	s := api.NewServer(f, sc, gtcpc, gec)

	r.GET("/forecast", s.Forecast)
	r.GET("/forecast/now", s.ForecastNow)
	r.GET("/forecast/config", s.Config)
	r.POST("/givtcp/chargetarget/update", s.UpdateChargeTarget)
	r.POST("/soclast/forecast/update", s.UpdateForecastData)
	r.POST("/solcast/forecast/set", s.SetForecastData)
	r.POST("/givenergy/consumptionaverages/update", s.UpdateConsumptionAverages)
	r.GET("/givenergy/consumptionaverages", s.GetConsumptionAverages)

	gocron.Every(1).Day().At("00:15").Do(func() {
		println("updating solar forecasts")
		err := sc.UpdateForecast()
		if err != nil {
			println(err)
			return
		}

		println("updating consumption averages")
		err = gec.UpdateConsumptionAverages()
		if err != nil {
			println(err)
			return
		}

		d := time.Now().Local().Truncate(time.Hour * 24)
		println(fmt.Sprintf("forecasting date %s", d.String()))
		forecast, err := f.Forecast(time.Now().Truncate(time.Hour * 24))
		if err != nil {
			println(err)
			return
		}

		t := int(forecast.RecommendedChargeTarget)
		println(fmt.Sprintf("setting charge target to %d", t))
		// todo make this an interface supported by either givtcp or gecloud
		err = gtcpc.SetChargeTarget(t)
		if err != nil {
			println(err)
			return
		}
	})
	gocron.Start()

	err := r.Run(":8080")
	if err != nil {
		panic(err)
	}
}
