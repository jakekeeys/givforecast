package main

import (
	"github.com/gin-gonic/gin"
	"givforecast/internal/api"
	"givforecast/internal/forecaster"
	"givforecast/internal/givenergy"
	"givforecast/internal/givtcp"
	"givforecast/internal/solcast"
	"os"
)

func main() {
	r := gin.Default()

	sc := solcast.NewClient(os.Getenv("SOLCAST_API_KEY"), os.Getenv("SOLCAST_RESOURCE_ID"))
	gec := givenergy.NewClient(os.Getenv("GIVENERGY_USERNAME"), os.Getenv("GIVENERGY_PASSWORD"), os.Getenv("GIVENERGY_SERIAL"))
	p := forecaster.New(sc, gec)
	gtcpc := givtcp.NewClient()
	s := api.NewServer(p, sc, gtcpc)

	r.GET("/forecast", s.Forecast)
	r.GET("/forecast/now", s.ForecastNow)
	r.GET("/forecast/config", s.Config)
	r.POST("/givtcp/chargetarget/update", s.UpdateChargeTarget)
	r.POST("/soclast/forecast/update", s.UpdateForecastData)
	r.POST("/solcast/forecast/set", s.SetForecastData)

	err := r.Run(":8080")
	if err != nil {
		panic(err)
	}
}
