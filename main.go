package main

import (
	"ge-charge-optimiser/internal/api"
	"ge-charge-optimiser/internal/forecaster"
	"ge-charge-optimiser/internal/givtcp"
	"ge-charge-optimiser/internal/solcast"
	"github.com/gin-gonic/gin"
	"os"
)

func main() {
	r := gin.Default()

	sc := solcast.NewClient(os.Getenv("SOLCAST_API_KEY"), os.Getenv("SOLCAST_RESOURCE_ID"))
	p := forecaster.New(sc)
	gtcpc := givtcp.NewClient()
	s := api.NewServer(p, sc, gtcpc)

	r.GET("/forecast", s.Forecast)
	r.GET("/forecast/now", s.ForecastNow)
	r.GET("/forecast/config", s.Config)
	r.POST("/givtcp/chargetarget/update", s.UpdateChargeTarget)
	r.GET("/soclast/forecast/update", s.UpdateForecastData)
	r.POST("/solcast/forecast/set", s.SetForecastData)

	err := r.Run(":8080")
	if err != nil {
		panic(err)
	}
}
