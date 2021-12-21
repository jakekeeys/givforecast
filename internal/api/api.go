package api

import (
	"ge-charge-optimiser/internal/projector"
	"github.com/gin-gonic/gin"
	"net/http"
	"time"
)

const dateFormat = "2006-01-02"

type Server struct {
	p *projector.Projector
}

func NewServer(p *projector.Projector) *Server {
	return &Server{p: p}
}

func (s *Server) GetChargeTarget(c *gin.Context) {
	ds := c.Query("date")
	if ds == "" {
		ds = time.Now().Format(dateFormat)
	}

	d, err := time.Parse(dateFormat, ds)
	if err != nil {
		_ = c.Error(err)
		return
	}

	e, err := s.p.EstimateChargeTargetForDate(d)
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusOK, e)
	return
}
