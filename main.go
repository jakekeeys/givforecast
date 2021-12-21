package main

import (
	"ge-charge-optimiser/internal/api"
	"ge-charge-optimiser/internal/projector"
	"ge-charge-optimiser/internal/solcast"
	"github.com/gin-gonic/gin"
)

func main() {
	r := gin.Default()

	sc := solcast.NewClient()
	p := projector.New(sc)
	s := api.NewServer(p)

	r.GET("/project", s.Project)
	r.GET("/config", s.Config)

	err := r.Run(":8080")
	if err != nil {
		panic(err)
	}
}
