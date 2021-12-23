package main

import (
	"os"

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
	gec := givenergy.NewClient(os.Getenv("GIVENERGY_USERNAME"), os.Getenv("GIVENERGY_PASSWORD"), os.Getenv("GIVENERGY_SERIAL"))
	p := forecaster.New(sc, gec)
	gtcpc := givtcp.NewClient()
	s := api.NewServer(p, sc, gtcpc, gec)

	r.GET("/forecast", s.Forecast)
	r.GET("/forecast/now", s.ForecastNow)
	r.GET("/forecast/config", s.Config)
	r.POST("/givtcp/chargetarget/update", s.UpdateChargeTarget)
	r.POST("/soclast/forecast/update", s.UpdateForecastData)
	r.POST("/solcast/forecast/set", s.SetForecastData)
	r.POST("/givenergy/consumptionaverages/update", s.UpdateConsumptionAverages)
	r.GET("/givenergy/consumptionaverages", s.GetConsumptionAverages)

	err := r.Run(":8080")
	if err != nil {
		panic(err)
	}
}
